package longmemeval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/atom"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
)

// Client wraps the MCP Streamable HTTP client with retry logic for eval use.
type Client struct {
	mcp       *client.Client
	retries   int
	serverURL string
	apiKey    string
	streaming bool
}

func baseServerURL(serverURL string) string {
	u := strings.TrimRight(serverURL, "/")
	u = strings.TrimSuffix(u, "/sse")
	u = strings.TrimSuffix(u, "/mcp")
	return u
}

func connectMCP(ctx context.Context, serverURL, apiKey string, streaming bool) (*client.Client, bool, error) {
	baseURL := baseServerURL(serverURL)
	headers := map[string]string{}
	if apiKey != "" {
		headers["Authorization"] = "Bearer " + apiKey
	}

	var c *client.Client
	var err error
	if streaming {
		c, err = client.NewStreamableHttpClient(baseURL+"/mcp", transport.WithHTTPHeaders(headers))
	} else {
		c, err = client.NewSSEMCPClient(baseURL+"/sse", transport.WithHeaders(headers))
	}
	if err != nil {
		return nil, false, err
	}
	if err := c.Start(ctx); err != nil {
		_ = c.Close()
		return nil, false, err
	}

	_, err = c.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "longmemeval", Version: "1.0.0"},
		},
	})
	if err != nil {
		_ = c.Close()
		return nil, false, fmt.Errorf("initialize MCP: %w", err)
	}
	return c, streaming, nil
}

func toolErrorMsg(result *mcp.CallToolResult, toolName string) error {
	if len(result.Content) > 0 {
		if tc, ok := result.Content[0].(mcp.TextContent); ok {
			return fmt.Errorf("%s tool error: %s", toolName, tc.Text)
		}
	}
	return fmt.Errorf("%s tool error", toolName)
}

// Connect creates an authenticated MCP client connected to serverURL.
// It prefers Streamable HTTP (/mcp) and falls back to legacy SSE (/sse)
// when the server responds with legacy-SSE initialize responses.
func Connect(ctx context.Context, serverURL, apiKey string) (*Client, error) {
	c, streaming, err := connectMCP(ctx, serverURL, apiKey, true)
	if err != nil {
		if !errors.Is(err, transport.ErrLegacySSEServer) {
			return nil, err
		}
		c, streaming, err = connectMCP(ctx, serverURL, apiKey, false)
		if err != nil {
			return nil, err
		}
	}
	return &Client{
		mcp:       c,
		retries:   1,
		serverURL: serverURL,
		apiKey:    apiKey,
		streaming: streaming,
	}, nil
}

// Connect re-establishes the MCP connection using the stored server URL and
// API key. Called by Recall before each retry attempt because a failed connection
// may need to be re-initialized — reconnect is required to recover from transient errors.
func (c *Client) Connect(ctx context.Context) error {
	streaming := c.streaming
	newMCP, _, err := connectMCP(ctx, c.serverURL, c.apiKey, streaming)
	if err != nil && errors.Is(err, transport.ErrLegacySSEServer) && streaming {
		streaming = false
		newMCP, _, err = connectMCP(ctx, c.serverURL, c.apiKey, false)
	}
	if err != nil {
		return fmt.Errorf("initialize MCP: %w", err)
	}
	// Close the old connection before replacing it to avoid leaking goroutines.
	_ = c.mcp.Close()
	c.mcp = newMCP
	c.streaming = streaming
	return nil
}

// Store stores one session as a memory and returns the memory ID.
func (c *Client) Store(ctx context.Context, project, content string, tags []string) (string, error) {
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		id, err := c.store(ctx, project, content, tags)
		if err == nil {
			return id, nil
		}
		lastErr = err
		if attempt < c.retries {
			select {
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
	}
	return "", lastErr
}

func (c *Client) store(ctx context.Context, project, content string, tags []string) (string, error) {
	result, err := c.mcp.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "memory_store",
			Arguments: map[string]any{
				"content": content,
				"project": project,
				"tags":    tags,
			},
		},
	})
	if err != nil {
		return "", err
	}
	if result.IsError {
		return "", toolErrorMsg(result, "memory_store")
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("memory_store returned no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		return "", fmt.Errorf("unexpected content type from memory_store")
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(tc.Text), &resp); err != nil {
		return "", fmt.Errorf("parse store response: %w", err)
	}
	return resp.ID, nil
}

// BatchItem is one entry for StoreBatch.
type BatchItem struct {
	Content string
	Tags    []string
}

// StoreBatch stores up to 100 sessions in a single MCP call and returns their IDs
// in the same order as items. Uses memory_store_batch to reduce HTTP round-trips.
func (c *Client) StoreBatch(ctx context.Context, project string, items []BatchItem) ([]string, error) {
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		ids, err := c.storeBatch(ctx, project, items)
		if err == nil {
			return ids, nil
		}
		lastErr = err
		if attempt < c.retries {
			select {
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	return nil, lastErr
}

func (c *Client) storeBatch(ctx context.Context, project string, items []BatchItem) ([]string, error) {
	memories := make([]any, len(items))
	for i, item := range items {
		memories[i] = map[string]any{
			"content": item.Content,
			"tags":    item.Tags,
		}
	}
	result, err := c.mcp.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "memory_store_batch",
			Arguments: map[string]any{
				"project":  project,
				"memories": memories,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return nil, toolErrorMsg(result, "memory_store_batch")
	}
	if len(result.Content) == 0 {
		return nil, fmt.Errorf("memory_store_batch returned no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		return nil, fmt.Errorf("unexpected content type from memory_store_batch: %T", result.Content[0])
	}
	var resp struct {
		IDs    []string `json:"ids"`
		Count  int      `json:"count"`
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal([]byte(tc.Text), &resp); err != nil {
		return nil, fmt.Errorf("parse store_batch response: %w", err)
	}
	if len(resp.Errors) > 0 {
		// Server validated items and rejected them; surface up to 3 messages.
		sample := resp.Errors
		if len(sample) > 3 {
			sample = sample[:3]
		}
		return nil, fmt.Errorf("memory_store_batch rejected %d items: %s", len(resp.Errors), strings.Join(sample, "; "))
	}
	if len(resp.IDs) != len(items) {
		return nil, fmt.Errorf("memory_store_batch returned %d ids for %d items", len(resp.IDs), len(items))
	}
	return resp.IDs, nil
}

// Recall calls memory_recall and returns ranked memory IDs.
// The server returns {"results":[{"memory":{"id":"..."},"score":...},...]} as JSON.
//
// On failure, Recall reconnects before retrying (issue #861): the mcp-go SSE
// server silently drops responses under a client-teardown race, leaving the
// connection in a state where every subsequent call fails identically. A bare
// retry on the same connection will not recover; reconnect is required.
func (c *Client) Recall(ctx context.Context, project, query string, topK int) ([]string, error) {
	return c.RecallWithDateRange(ctx, project, query, topK, nil, nil)
}

func (c *Client) RecallWithDateRange(ctx context.Context, project, query string, topK int, since, before *time.Time) ([]string, error) {
	hits, err := c.RecallScoredWithDateRange(ctx, project, query, topK, since, before)
	if err != nil {
		return nil, err
	}
	return IDsFromScoredRecall(hits), nil
}

func (c *Client) RecallWithDateRangeResult(ctx context.Context, project, query string, topK int, since, before *time.Time) (RecallResult, error) {
	return c.recallResultWithParams(ctx, recallParams{
		project: project, query: query, topK: topK, since: since, before: before,
	})
}

func (c *Client) RecallScored(ctx context.Context, project, query string, topK int) ([]ScoredMemoryID, error) {
	return c.RecallScoredWithDateRange(ctx, project, query, topK, nil, nil)
}

func (c *Client) RecallScoredWithDateRange(ctx context.Context, project, query string, topK int, since, before *time.Time) ([]ScoredMemoryID, error) {
	return c.recallScoredWithParams(ctx, recallParams{
		project: project, query: query, topK: topK, since: since, before: before,
	})
}

// ScoredMemoryID is one recall hit with its server-provided score.
type ScoredMemoryID struct {
	ID    string
	Score float64
}

// IDsFromScoredRecall strips scores while preserving ranked order.
func IDsFromScoredRecall(hits []ScoredMemoryID) []string {
	ids := make([]string, 0, len(hits))
	for _, hit := range hits {
		if hit.ID != "" {
			ids = append(ids, hit.ID)
		}
	}
	return ids
}

// MergeScoredRecall unions recall hits by memory ID, keeps the maximum score
// per ID, and re-ranks the merged slice by score descending.
func MergeScoredRecall(primary, secondary []ScoredMemoryID) []ScoredMemoryID {
	type mergedHit struct {
		hit   ScoredMemoryID
		order int
	}
	merged := make(map[string]mergedHit, len(primary)+len(secondary))
	order := 0
	for _, batch := range [][]ScoredMemoryID{primary, secondary} {
		for _, hit := range batch {
			if hit.ID == "" {
				continue
			}
			current, ok := merged[hit.ID]
			if !ok {
				merged[hit.ID] = mergedHit{hit: hit, order: order}
				order++
				continue
			}
			if hit.Score > current.hit.Score {
				current.hit.Score = hit.Score
				merged[hit.ID] = current
			}
		}
	}
	out := make([]mergedHit, 0, len(merged))
	for _, hit := range merged {
		out = append(out, hit)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].hit.Score == out[j].hit.Score {
			return out[i].order < out[j].order
		}
		return out[i].hit.Score > out[j].hit.Score
	})
	flattened := make([]ScoredMemoryID, 0, len(out))
	for _, hit := range out {
		flattened = append(flattened, hit.hit)
	}
	return flattened
}

// RecallResult is the structured memory_recall response used by LongMemEval
// when auxiliary metadata such as atom preambles must survive transport.
type RecallResult struct {
	IDs          []string
	AtomPreamble string
	// Hits carries the scored recall results for callers that need score-based
	// merging (e.g. H15 dual preference recall). IDsFromScoredRecall(Hits) == IDs.
	Hits []ScoredMemoryID
	// MemoryMap maps memory ID to session ID extracted from handle-mode tags.
	// Populated when handle-mode recall returns tags with a "session:<id>" entry.
	MemoryMap map[string]string
	// Results carries the full SearchResult objects when recall was issued in
	// "full" mode. Populated by RecallResultsWith* wrappers for callers that
	// need access to memory tags (e.g. session dominance diagnostics).
	Results []types.SearchResult
}

// RecallWithTemporalWindow enables the server-side H-NEW-1 two-pass date-windowed
// recall: the server parses temporal anchors from questionText against questionDate
// and unions a date-filtered pass with the unfiltered pass. questionText/questionDate
// are advisory — the server falls back to baseline single-pass recall when no window
// resolves (e.g. "how many X ago").
func (c *Client) RecallWithTemporalWindow(ctx context.Context, project, query string, topK int, questionText, questionDate string) ([]string, error) {
	hits, err := c.recallScoredWithParams(ctx, recallParams{
		project: project, query: query, topK: topK,
		temporalWindow: true, questionText: questionText, questionDate: questionDate,
	})
	if err != nil {
		return nil, err
	}
	return IDsFromScoredRecall(hits), nil
}

func (c *Client) RecallWithTemporalWindowResult(ctx context.Context, project, query string, topK int, questionText, questionDate string) (RecallResult, error) {
	return c.recallResultWithParams(ctx, recallParams{
		project: project, query: query, topK: topK,
		temporalWindow: true, questionText: questionText, questionDate: questionDate,
	})
}

func (c *Client) RecallResultsWithTemporalWindow(ctx context.Context, project, query string, topK int, questionText, questionDate string) ([]types.SearchResult, error) {
	result, err := c.recallResultWithParams(ctx, recallParams{
		project: project, query: query, topK: topK,
		temporalWindow: true, questionText: questionText, questionDate: questionDate,
		mode: "full",
	})
	if err != nil {
		return nil, err
	}
	return result.Results, nil
}

// RecallWithOpts calls memory_recall with additional server-side options.
// topicAnchorBoost=true sets topic_anchor_boost on the server (H-TAB, LME exp #3).
func (c *Client) RecallWithOpts(ctx context.Context, project, query string, topK int, since, before *time.Time, topicAnchorBoost bool) ([]string, error) {
	hits, err := c.RecallScoredWithOpts(ctx, project, query, topK, since, before, topicAnchorBoost)
	if err != nil {
		return nil, err
	}
	return IDsFromScoredRecall(hits), nil
}

func (c *Client) RecallWithOptsResult(ctx context.Context, project, query string, topK int, since, before *time.Time, topicAnchorBoost bool) (RecallResult, error) {
	return c.recallResultWithParams(ctx, recallParams{
		project: project, query: query, topK: topK, since: since, before: before,
		topicAnchorBoost: topicAnchorBoost,
	})
}

func (c *Client) RecallScoredWithOpts(ctx context.Context, project, query string, topK int, since, before *time.Time, topicAnchorBoost bool) ([]ScoredMemoryID, error) {
	diversityN := 0
	if v := os.Getenv("ENGRAM_SESSION_DIVERSITY_N"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			diversityN = n
		}
	}
	return c.recallScoredWithParams(ctx, recallParams{
		project: project, query: query, topK: topK, since: since, before: before,
		topicAnchorBoost:  topicAnchorBoost,
		sessionDiversityN: diversityN,
	})
}

// RecallWithAtomRecall calls memory_recall and preserves any returned atom preamble.
func (c *Client) RecallWithAtomRecall(ctx context.Context, project, query string, topK int, since, before *time.Time, topicAnchorBoost, exactFactBoost bool, atomRecallAsOf *time.Time) (RecallResult, error) {
	return c.recallResultWithParams(ctx, recallParams{
		project: project, query: query, topK: topK, since: since, before: before,
		topicAnchorBoost: topicAnchorBoost,
		exactFactBoost:   exactFactBoost,
		atomRecall:       true,
		atomRecallAsOf:   atomRecallAsOf,
	})
}

func (c *Client) RecallResultsWithOpts(ctx context.Context, project, query string, topK int, since, before *time.Time, topicAnchorBoost bool) ([]types.SearchResult, error) {
	result, err := c.recallResultWithParams(ctx, recallParams{
		project: project, query: query, topK: topK, since: since, before: before,
		topicAnchorBoost: topicAnchorBoost, mode: "full",
	})
	if err != nil {
		return nil, err
	}
	return result.Results, nil
}


// recallParams carries the optional knobs for a single memory_recall call.
type recallParams struct {
	project           string
	query             string
	topK              int
	mode              string
	since             *time.Time
	before            *time.Time
	temporalWindow    bool
	questionText      string
	questionDate      string
	exactFactBoost    bool
	topicAnchorBoost  bool
	atomRecall        bool
	atomRecallAsOf    *time.Time
	sessionDiversityN int
}

func (c *Client) recallResultWithParams(ctx context.Context, p recallParams) (RecallResult, error) {
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		if attempt > 0 {
			// Reconnect — previous connection may be dead (SSE drop race).
			if err := c.Connect(ctx); err != nil {
				return RecallResult{}, fmt.Errorf("reconnect on retry %d: %w", attempt, err)
			}
			select {
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
				return RecallResult{}, ctx.Err()
			}
		}
		result, err := c.recall(ctx, p)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return RecallResult{}, lastErr
}

func (c *Client) recallScoredWithParams(ctx context.Context, p recallParams) ([]ScoredMemoryID, error) {
	result, err := c.recallResultWithParams(ctx, p)
	if err != nil {
		return nil, err
	}
	return result.Hits, nil
}

// RecallOpts holds optional parameters for the LongMemEval recall client.
type RecallOpts struct {
	// ExactFactBoost passes exact_fact_boost=true to the server-side memory_recall
	// handler, enabling the entity-identifier scoring boost (LME #938 improvement #3).
	ExactFactBoost bool
}

// RecallWithExactBoost calls recall with exact_fact_boost enabled.
// Convenience wrapper for the longmemeval run command.
func (c *Client) RecallWithExactBoost(ctx context.Context, project, query string, topK int, since, before *time.Time) ([]string, error) {
	hits, err := c.RecallScoredWithExactBoost(ctx, project, query, topK, since, before)
	if err != nil {
		return nil, err
	}
	return IDsFromScoredRecall(hits), nil
}

func (c *Client) RecallWithExactBoostResult(ctx context.Context, project, query string, topK int, since, before *time.Time) (RecallResult, error) {
	return c.recallResultWithParams(ctx, recallParams{
		project: project, query: query, topK: topK, since: since, before: before,
		exactFactBoost: true,
	})
}

func (c *Client) RecallScoredWithExactBoost(ctx context.Context, project, query string, topK int, since, before *time.Time) ([]ScoredMemoryID, error) {
	return c.recallScoredWithParams(ctx, recallParams{
		project: project, query: query, topK: topK, since: since, before: before,
		exactFactBoost: true,
	})
}

func (c *Client) RecallResultsWithExactBoost(ctx context.Context, project, query string, topK int, since, before *time.Time) ([]types.SearchResult, error) {
	result, err := c.recallResultWithParams(ctx, recallParams{
		project: project, query: query, topK: topK, since: since, before: before,
		exactFactBoost: true, mode: "full",
	})
	if err != nil {
		return nil, err
	}
	return result.Results, nil
}

func (c *Client) recall(ctx context.Context, p recallParams) (RecallResult, error) {
	args := map[string]any{
		"query":   p.query,
		"project": p.project,
		"top_k":   p.topK,
		"detail":  "summary",
		// Benchmark retrieval must not mutate retrieval telemetry while
		// measuring recall quality.
		"record_event": false,
	}
	mode := p.mode
	if mode == "" {
		mode = "handle"
	}
	args["mode"] = mode
	if p.exactFactBoost {
		args["exact_fact_boost"] = true
	}
	if p.since != nil {
		args["since"] = p.since.UTC().Format(time.RFC3339)
	}
	if p.before != nil {
		args["before"] = p.before.UTC().Format(time.RFC3339)
	}
	if p.temporalWindow {
		args["temporal_window_recall"] = true
		args["question_text"] = p.questionText
		args["question_date"] = p.questionDate
	}
	// H-TAB (LME exp #3): pass topic-anchor boost flag to server.
	if p.topicAnchorBoost {
		args["topic_anchor_boost"] = true
	}
	if p.atomRecall {
		args["atom_recall"] = true
	}
	if p.atomRecallAsOf != nil {
		args["atom_recall_as_of"] = p.atomRecallAsOf.UTC().Format(time.RFC3339)
	}
	if p.sessionDiversityN > 0 {
		args["session_diversity_n"] = p.sessionDiversityN
	}
	result, err := c.mcp.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "memory_recall",
			Arguments: args,
		},
	})
	if err != nil {
		return RecallResult{}, err
	}
	if result.IsError {
		return RecallResult{}, toolErrorMsg(result, "memory_recall")
	}
	if len(result.Content) == 0 {
		return RecallResult{}, fmt.Errorf("memory_recall returned no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		return RecallResult{}, fmt.Errorf("unexpected content type from memory_recall: %T", result.Content[0])
	}
	if os.Getenv("LME_DEBUG_RECALL") != "" {
		preview := tc.Text
		if len(preview) > 800 {
			preview = preview[:800] + "…"
		}
		fmt.Fprintf(os.Stderr, "DEBUG recall raw: %s\n", preview)
	}
	// Server may respond in either of two shapes depending on configured default
	// mode (ENGRAM_RECALL_DEFAULT_MODE):
	//   full:   {"results":[{"memory":{"id":"..."},"score":...}, ...]}
	//   handle: {"handles":[{"id":"...","score":...}, ...]}
	// Parse both and prefer whichever is populated.
	var resp struct {
		AtomPreamble string               `json:"atom_preamble"`
		Results      []types.SearchResult `json:"results"`
		Handles      []struct {
			ID    string   `json:"id"`
			Score float64  `json:"score"`
			Tags  []string `json:"tags"`
		} `json:"handles"`
	}
	if err := json.Unmarshal([]byte(tc.Text), &resp); err != nil {
		return RecallResult{}, fmt.Errorf("parse recall response: %w", err)
	}
	hits := make([]ScoredMemoryID, 0, len(resp.Results)+len(resp.Handles))
	memoryMap := make(map[string]string, len(resp.Handles))
	for _, r := range resp.Results {
		if r.Memory != nil && r.Memory.ID != "" {
			hits = append(hits, ScoredMemoryID{ID: r.Memory.ID, Score: r.Score})
		}
	}
	for _, h := range resp.Handles {
		if h.ID != "" {
			hits = append(hits, ScoredMemoryID{ID: h.ID, Score: h.Score})
			if len(h.Tags) > 0 {
				memoryMap[h.ID] = search.ExtractSessionID(h.Tags)
			}
		}
	}
	return RecallResult{IDs: IDsFromScoredRecall(hits), AtomPreamble: resp.AtomPreamble, Hits: hits, MemoryMap: memoryMap, Results: resp.Results}, nil
}

// FetchContent fetches the full content of a memory by ID within a project.
// The project argument is required: the server-side memory_fetch handler
// scopes by project (default "default") and will return "not found" if the
// memory lives in a different project than the one the call targets.
func (c *Client) FetchContent(ctx context.Context, project, id string) (string, error) {
	result, err := c.mcp.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "memory_fetch",
			Arguments: map[string]any{
				"project": project,
				"id":      id,
				"detail":  "full",
			},
		},
	})
	if err != nil {
		return "", err
	}
	if result.IsError {
		return "", toolErrorMsg(result, "memory_fetch")
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("memory_fetch returned no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		return "", fmt.Errorf("unexpected content type from memory_fetch: %T", result.Content[0])
	}
	if os.Getenv("LME_DEBUG_FETCH") != "" {
		preview := tc.Text
		if len(preview) > 600 {
			preview = preview[:600] + "…"
		}
		fmt.Fprintf(os.Stderr, "DEBUG fetch raw: %s\n", preview)
	}
	var resp struct {
		Memory struct {
			Content string `json:"content"`
		} `json:"memory"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(tc.Text), &resp); err != nil {
		return "", fmt.Errorf("parse fetch response: %w", err)
	}
	if resp.Memory.Content != "" {
		return resp.Memory.Content, nil
	}
	return resp.Content, nil
}

// DeleteProject calls memory_delete_project to clean up an isolation project.
func (c *Client) DeleteProject(ctx context.Context, project string) error {
	result, err := c.mcp.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "memory_delete_project",
			Arguments: map[string]any{"project": project},
		},
	})
	if err != nil {
		if IsStaleSessionError(err) {
			// Bug #642: SSE session expired server-side; cleanup is moot, not an error.
			return nil
		}
		return err
	}
	if result.IsError {
		return toolErrorMsg(result, "memory_delete_project")
	}
	return nil
}

// Close shuts down the underlying MCP SSE connection.
func (c *Client) Close() error {
	return c.mcp.Close()
}

// RestClient calls engram's sessionless REST endpoints (/quick-store,
// /quick-recall) directly over plain HTTP — no MCP SSE session, no 60s
// transport timeout. Used by the ingest stage for large haystack items.
type RestClient struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewRestClient constructs a RestClient pointed at baseURL with Bearer auth.
// The sessionless REST endpoints (/quick-store, /atoms, /quick-recall) live at
// the SERVER ROOT, never under /mcp or /sse. Callers commonly pass the MCP
// endpoint URL (e.g. https://host/mcp) from the Claude config, so normalize the
// base here via baseServerURL — otherwise QuickStore would POST to
// /mcp/quick-store, which returns a non-object body and fails decoding with the
// opaque "cannot unmarshal number" error (#1185).
func NewRestClient(baseURL, token string) *RestClient {
	return &RestClient{
		baseURL: baseServerURL(baseURL),
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// QuickStore stores a single memory via POST /quick-store and returns its ID.
// When expiresAt is non-nil, the server stamps project_ttl so the project can
// be swept later by lme prune. Retries on 429 and 5xx with exponential backoff.
func (r *RestClient) QuickStore(ctx context.Context, project, content string, tags []string, expiresAt *time.Time) (string, error) {
	body := map[string]any{
		"content": content,
		"project": project,
		"tags":    tags,
	}
	if expiresAt != nil {
		body["expires_at"] = expiresAt.UTC().Format(time.RFC3339)
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal QuickStore body: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s, 8s… capped at 16s.
			// 429s resolve quickly once the token bucket refills.
			backoff := time.Duration(1<<min(attempt-1, 4)) * time.Second
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
		req.Header.Set("Authorization", "Bearer "+r.token)

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

// QuickRecall calls POST /quick-recall and returns memory IDs ranked by score.
func (r *RestClient) QuickRecall(ctx context.Context, project, query string, limit int) ([]string, error) {
	body := map[string]any{
		"query":   query,
		"project": project,
		"limit":   limit,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal QuickRecall body: %w", err) // #710
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/quick-recall", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.token)

	resp, err := r.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("quick-recall decode: %w", err)
	}
	return result.IDs, nil
}

// SessionContent concatenates all turns of a session into a single string,
// preserving role labels so the retriever and generator can distinguish
// user questions from assistant responses. Dropping assistant turns would
// make single-session-assistant questions unanswerable (the gold content
// lives in the assistant's reply).
//
// C0/C1 control characters (except \t, \n, \r) are stripped — the server's
// validateContent rejects them and, because memory_store_batch is all-or-
// nothing, one offender tanks an entire 100-item batch. See LongMemEval
// session 158 of question 7e00a6cb for a real-world offender (\x0B).
func SessionContent(turns []Turn) string {
	var sb strings.Builder
	for _, t := range turns {
		if t.Content == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteByte('\n')
		}
		role := t.Role
		if role == "" {
			role = "user"
		}
		sb.WriteString(role)
		sb.WriteString(": ")
		sb.WriteString(sanitizeControlChars(t.Content))
	}
	return sb.String()
}

// sanitizeControlChars removes C0 (0x00-0x1F except \t \n \r) and C1 (0x7F-0x9F)
// control characters from s.
func sanitizeControlChars(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	for _, r := range s {
		if r == '\t' || r == '\n' || r == '\r' {
			sb.WriteRune(r)
			continue
		}
		if r < 0x20 || (r >= 0x7F && r <= 0x9F) {
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// FetchAtoms retrieves active preference atoms for a project via the REST
// /atoms endpoint. This is the Milestone 1 (#938) atom recall path used by
// the --atom-mode flag in cmd/longmemeval/run.go.
//
// The /atoms endpoint does not exist in the current server — it is a
// forward-reference for the M2 server-side implementation. Until M2, this
// method returns an empty slice (non-fatal; the run continues without atoms).
// This enables the --atom-mode code path to be exercised in unit tests.
//
// topK: maximum number of atoms to return (0 = server default).
func (c *Client) FetchAtoms(ctx context.Context, project string, atomType string, topK int) ([]atom.Atom, error) {
	body := map[string]any{
		"project":   project,
		"atom_type": atomType,
		"top_k":     topK,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal FetchAtoms body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(c.serverURL, "/")+"/atoms", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		// Non-fatal: endpoint not yet implemented; caller logs a warning.
		return nil, fmt.Errorf("fetch atoms: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNotImplemented {
		// Endpoint not yet deployed — treat as empty result, not an error.
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch atoms: unexpected status %d", resp.StatusCode)
	}

	var result struct {
		Atoms []atom.Atom `json:"atoms"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("fetch atoms decode: %w", err)
	}
	return result.Atoms, nil
}

// IsStaleSessionError returns true when err represents an MCP session that
// has already expired server-side. The Engram MCP server drops SSE sessions
// after a timeout; cleanup calls on expired sessions are not an error.
func IsStaleSessionError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "invalid session id")
}
