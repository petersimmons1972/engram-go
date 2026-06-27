package llmclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultOllaHost    = "https://olla.petersimmons.com"
	ollaModelsPath     = "/olla/models"
	ollaCompletePath   = "/v1/chat/completions"
	defaultOllaTimeout = 60 * time.Second
)

// skipFamilies lists Olla model families whose thinking-mode token leakage
// breaks structured JSON output (benchmark 2026-04-24).
// See: ~/projects/instinct/consolidator/instinct/haiku_client.py:30.
var skipFamilies = map[string]struct{}{
	"qwen3":    {},
	"qwen3moe": {},
}

// ollaClient implements LLMClient using Olla's OpenAI-compatible endpoint.
// Model selection is dynamic: the client queries /olla/models on the first
// Complete call, picks the first text-generation-capable available model not
// in the skip list, and caches the result for the lifetime of the client.
// The cache avoids an HTTP round-trip per call for callers that invoke Complete
// many times (e.g. audit, which calls Complete once per pattern).
//
// Cache invalidation: if the cached model becomes unavailable (ErrBackendUnavailable
// from the completion endpoint), the cache is cleared so the next call re-resolves.
//
// Unavailability semantics: if the model discovery endpoint is unreachable,
// returns no suitable model, or the completion fails, Complete returns
// ("", error wrapping ErrBackendUnavailable). Callers decide whether to
// skip-and-continue (consolidator: errors.Is(err, ErrBackendUnavailable) →
// skip this batch) or surface as failure (audit: any error → exit non-zero).
// The previous ("", nil) contract conflated unavailability with success and
// prevented backend-agnostic callers from making correct decisions.
type ollaClient struct {
	host    string
	timeout time.Duration
	client  *http.Client

	// mu guards modelID and modelResolved.  A mutex (rather than sync.Once) is
	// used so resetModel can safely clear the cache while another goroutine may
	// concurrently be running resolvedModel — assigning to a sync.Once struct
	// field is a data race.
	mu            sync.Mutex
	modelID       string
	modelResolved bool
}

// NewOllaClient constructs an Olla LLMClient from cfg.
// cfg.Endpoint is the base host (e.g. "https://olla.petersimmons.com").
// Defaults to defaultOllaHost when empty.
// cfg.APIKey and cfg.Model are ignored; Olla resolves the model dynamically.
func NewOllaClient(cfg Config) (LLMClient, error) {
	host := cfg.Endpoint
	if host == "" {
		host = defaultOllaHost
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultOllaTimeout
	}
	return &ollaClient{host: host, timeout: timeout, client: &http.Client{Timeout: timeout}}, nil
}

// pickModel queries /olla/models and returns the first model id suitable for
// text generation.  Returns "" if none is found or if the endpoint is
// unreachable.
func (c *ollaClient) pickModel(ctx context.Context) string {
	url := c.host + ollaModelsPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		slog.Warn("llm/olla: build model-discovery request", "err", err)
		return ""
	}

	resp, err := c.client.Do(req)
	if err != nil {
		slog.Warn("llm/olla: cannot reach Olla", "host", c.host, "err", err)
		return ""
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("llm/olla: model discovery non-200", "status", resp.StatusCode)
		return ""
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Warn("llm/olla: read model-discovery body", "err", err)
		return ""
	}

	var payload struct {
		Data []struct {
			ID   string `json:"id"`
			Olla struct {
				Family       string   `json:"family"`
				Capabilities []string `json:"capabilities"`
				Availability []struct {
					State string `json:"state"`
				} `json:"availability"`
			} `json:"olla"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		slog.Warn("llm/olla: parse model-discovery response", "err", err)
		return ""
	}

	for _, model := range payload.Data {
		olla := model.Olla

		// Must support text generation.
		hasTextGen := false
		for _, cap := range olla.Capabilities {
			if cap == "text-generation" {
				hasTextGen = true
				break
			}
		}
		if !hasTextGen {
			continue
		}

		// At least one availability slot must be "available".
		available := false
		for _, a := range olla.Availability {
			if a.State == "available" {
				available = true
				break
			}
		}
		if !available {
			continue
		}

		// Skip families in the deny-list — thinking-mode token leakage breaks
		// JSON output (benchmark 2026-04-24).  The skipFamilies map is the
		// single source of truth; do not add parallel prefix checks here.
		if _, skip := skipFamilies[olla.Family]; skip {
			continue
		}

		return model.ID
	}

	slog.Warn("llm/olla: no suitable model found", "host", c.host)
	return ""
}

// resolvedModel returns the cached model ID, calling pickModel on the first
// invocation.  If the cached model is empty (discovery failed), it returns "".
// Call resetModel to clear the cache so the next call retries discovery.
func (c *ollaClient) resolvedModel(ctx context.Context) string {
	c.mu.Lock()
	if !c.modelResolved {
		c.modelID = c.pickModel(ctx)
		c.modelResolved = true
	}
	m := c.modelID
	c.mu.Unlock()
	return m
}

// resetModel clears the model cache so the next Complete call re-discovers.
// Called when the completion endpoint returns ErrBackendUnavailable, indicating
// the previously selected model may have become unavailable.
func (c *ollaClient) resetModel() {
	c.mu.Lock()
	c.modelResolved = false
	c.modelID = ""
	c.mu.Unlock()
}

// Complete sends systemPrompt + userPrompt to Olla using the OpenAI-compatible
// chat completions API.  If no suitable model is available it returns
// ("", ErrBackendUnavailable) so callers can distinguish infrastructure
// absence from an empty model response.  HTTP or JSON parse failures return a
// non-sentinel error — these are protocol bugs that the caller should surface,
// not treat as a missing backend.
//
// The model is resolved once per client lifetime via pickModel and cached.
// Subsequent calls reuse the cached model without an HTTP round-trip.
// The cache is cleared on ErrBackendUnavailable so a flapping host recovers.
func (c *ollaClient) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	// Per-call context with timeout so a slow Olla does not block forever.
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	model := c.resolvedModel(callCtx)
	if model == "" {
		// No usable model — wrap sentinel so caller can distinguish "backend
		// unavailable" from "model returned empty string".
		return "", fmt.Errorf("llm/olla: no usable model: %w", ErrBackendUnavailable)
	}

	reqBody, err := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]any{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 0,
	})
	if err != nil {
		return "", fmt.Errorf("llm/olla: marshal completion request: %w", err)
	}

	url := c.host + ollaCompletePath
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("llm/olla: build completion request: %w", err)
	}
	req.Header.Set("content-type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		slog.Warn("llm/olla: completion HTTP error", "err", err)
		c.resetModel() // cached model may be gone; re-resolve next call
		return "", fmt.Errorf("llm/olla: completion transport: %w", ErrBackendUnavailable)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("llm/olla: completion non-200", "status", resp.StatusCode)
		c.resetModel() // model may have become unavailable
		return "", fmt.Errorf("llm/olla: completion status %d: %w", resp.StatusCode, ErrBackendUnavailable)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Warn("llm/olla: read completion body", "err", err)
		return "", fmt.Errorf("llm/olla: read body: %w", ErrBackendUnavailable)
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		slog.Warn("llm/olla: parse completion response", "err", err)
		return "", fmt.Errorf("llm/olla: parse response: %w", err)
	}
	if len(apiResp.Choices) == 0 {
		slog.Warn("llm/olla: empty choices array")
		return "", fmt.Errorf("llm/olla: empty choices: %w", ErrBackendUnavailable)
	}

	text := apiResp.Choices[0].Message.Content
	// Strip accidental markdown fences — matches Python haiku_client.py:116-117.
	if strings.HasPrefix(text, "```") {
		text = strings.SplitN(text, "\n", 2)[1]
		if idx := strings.LastIndex(text, "```"); idx >= 0 {
			text = text[:idx]
		}
	}
	text = strings.TrimSpace(text)

	return text, nil
}
