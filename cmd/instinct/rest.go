// Package main — REST client for sessionless Engram endpoints.
//
// High-volume bulk writes (ingest, store) and reads (recall) use POST
// /quick-store and POST /quick-recall directly over plain HTTP.  No MCP SSE
// session is required, so there is no sessionId-staleness risk on large
// episodes.  This matches the Python consolidator pattern in
// instinct/engram_client.py.
//
// Low-volume, per-episode operations (episodeStart, episodeEnd, correct) still
// use the existing sseEngram path because no sessionless REST equivalent exists
// for those tools.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	restCallTimeout  = 30 * time.Second
	restMaxAttempts  = 8
	restMaxBackoffEx = 4 // cap exponent: 1<<4 = 16s
)

// restEngram calls engram's sessionless REST endpoints (/quick-store,
// /quick-recall) directly over plain HTTP — no MCP SSE session, no
// sessionId-staleness risk.
type restEngram struct {
	baseURL string // e.g., "http://127.0.0.1:8788" (no trailing slash)
	token   string
	http    *http.Client
}

// newRestEngram constructs a restEngram pointed at baseURL with Bearer auth.
// baseURL may include a trailing slash or a /sse suffix — both are stripped.
func newRestEngram(baseURL, token string) *restEngram {
	base := strings.TrimRight(baseURL, "/")
	base = strings.TrimSuffix(base, "/sse")
	return &restEngram{
		baseURL: base,
		token:   token,
		http:    &http.Client{Timeout: restCallTimeout},
	}
}

// quickStore posts content to POST /quick-store and returns the assigned memory
// ID.  Retries up to restMaxAttempts on network errors and 429/5xx responses
// with exponential backoff, matching the longmemeval.RestClient pattern.
func (r *restEngram) quickStore(ctx context.Context, project, content string, tags []string, importance int) (string, error) {
	body := map[string]any{
		"content":    content,
		"project":    project,
		"tags":       tags,
		"importance": importance,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal quickStore body: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < restMaxAttempts; attempt++ {
		if attempt > 0 {
			exp := attempt - 1
			if exp > restMaxBackoffEx {
				exp = restMaxBackoffEx
			}
			backoff := time.Duration(1<<exp) * time.Second
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/quick-store", bytes.NewReader(data))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		if r.token != "" {
			req.Header.Set("Authorization", "Bearer "+r.token)
		}

		resp, err := r.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		var result struct {
			OK    bool   `json:"ok"`
			ID    string `json:"id"`
			Error string `json:"error"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&result)
		_ = resp.Body.Close()

		if decodeErr != nil {
			lastErr = fmt.Errorf("quick-store decode: %w", decodeErr)
			continue
		}
		if resp.StatusCode == 429 {
			lastErr = fmt.Errorf("quick-store rate limited (status 429)")
			continue
		}
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("quick-store server error (status %d): %s", resp.StatusCode, result.Error)
			continue
		}
		if !result.OK || result.ID == "" {
			return "", fmt.Errorf("quick-store failed: %s (status %d)", result.Error, resp.StatusCode)
		}
		return result.ID, nil
	}
	return "", lastErr
}

// quickRecall posts a query to POST /quick-recall and returns matching memories.
func (r *restEngram) quickRecall(ctx context.Context, project, query string, tags []string, limit int) ([]map[string]any, error) {
	body := map[string]any{
		"query":   query,
		"project": project,
		"limit":   limit,
	}
	if len(tags) > 0 {
		body["tags"] = tags
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal quickRecall body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/quick-recall", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if r.token != "" {
		req.Header.Set("Authorization", "Bearer "+r.token)
	}

	resp, err := r.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("quick-recall status %d", resp.StatusCode)
	}

	var result struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("quick-recall decode: %w", err)
	}
	return result.Results, nil
}

// ── hybridEngram ─────────────────────────────────────────────────────────────

// hybridEngram routes bulk writes and reads to REST endpoints, keeping
// per-episode lifecycle operations on the MCP SSE path.
//
//	REST (sessionless, high-volume):
//	  ingest  → POST /quick-store
//	  store   → POST /quick-store
//	  recall  → POST /quick-recall
//
//	MCP SSE (low-volume, no REST equivalent):
//	  episodeStart → memory_episode_start
//	  episodeEnd   → memory_episode_end
//	  correct      → memory_correct
type hybridEngram struct {
	sse  *sseEngram
	rest *restEngram
}

// newHybridEngram creates a hybridEngram.  sseURL is the engram-go SSE URL
// (e.g., "http://127.0.0.1:8788" or "http://…/sse").  restURL is the base
// URL for REST calls; when empty it is derived from sseURL by stripping /sse.
func newHybridEngram(sseURL, token, restURL string) (*hybridEngram, error) {
	sse, err := newSSEEngram(sseURL, token)
	if err != nil {
		return nil, err
	}
	if restURL == "" {
		restURL = sseURL
	}
	return &hybridEngram{
		sse:  sse,
		rest: newRestEngram(restURL, token),
	}, nil
}

// connect initialises the SSE session (required for episodeStart/End and correct).
func (h *hybridEngram) connect(ctx context.Context) error {
	return h.sse.connect(ctx)
}

// close closes the SSE client.
func (h *hybridEngram) close() error {
	return h.sse.close()
}

// episodeStart delegates to the SSE path.
func (h *hybridEngram) episodeStart(ctx context.Context, sessionID, projectID string) (string, error) {
	return h.sse.episodeStart(ctx, sessionID, projectID)
}

// episodeEnd delegates to the SSE path.
func (h *hybridEngram) episodeEnd(ctx context.Context, episodeID string) error {
	return h.sse.episodeEnd(ctx, episodeID)
}

// ingest stores a raw tool event via REST POST /quick-store.
// importance=2 (0.2×10) mirrors the previous MCP importance:0.2 value.
func (h *hybridEngram) ingest(ctx context.Context, ev Event, projectID, sessionID string) error {
	raw, _ := json.Marshal(ev)
	_, err := h.rest.quickStore(ctx, projectID, string(raw),
		[]string{"instinct-raw", "session-" + sessionID}, 2)
	return err
}

// store persists a detected pattern via REST POST /quick-store.
// confidence [0,1] → importance [0,10] integer, matching the Python pattern.
func (h *hybridEngram) store(ctx context.Context, p Pattern, confidence float64, projectID string) (string, error) {
	content := fmt.Sprintf("%s | PROVENANCE: observed 1 time, first seen %s",
		p.Description, time.Now().UTC().Format("2006-01-02"))
	importance := int(confidence * 10)
	return h.rest.quickStore(ctx, projectID, content,
		[]string{"instinct", p.Type, p.Domain, p.TagSignature}, importance)
}

// recall looks up an existing pattern by tag signature via REST POST /quick-recall.
func (h *hybridEngram) recall(ctx context.Context, tagSignature, projectID string) (*recallResult, error) {
	results, err := h.rest.quickRecall(ctx, projectID,
		"instinct pattern "+tagSignature, []string{tagSignature}, 10)
	if err != nil {
		return nil, err
	}
	for _, m := range results {
		tags, _ := m["tags"].([]any)
		for _, t := range tags {
			if s, ok := t.(string); ok && s == tagSignature {
				id, _ := m["id"].(string)
				var conf float64
				if pc, ok := m["pattern_confidence"].(float64); ok {
					conf = pc
				} else {
					// Legacy fallback: importance stored before E2.
					conf, _ = m["importance"].(float64)
				}
				return &recallResult{id: id, confidence: conf}, nil
			}
		}
	}
	return nil, nil
}

// correct updates pattern confidence via the SSE path (no REST equivalent).
func (h *hybridEngram) correct(ctx context.Context, memoryID string, confidence float64) error {
	return h.sse.correct(ctx, memoryID, confidence)
}
