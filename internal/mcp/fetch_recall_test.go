package mcp

// Unit tests for handle-mode handleMemoryRecall and the two execFetch boundary
// cases not covered by fetch_exec_test.go (GetMemory error propagation and the
// maxBytes=0 no-truncation guarantee).
//
// Uses newTestNoopPool from explore_handler_test.go for the recall tests.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// ── minimal fetcher stub ──────────────────────────────────────────────────────
// Named recallStubFetcher to avoid collision with fakeFetcher in fetch_exec_test.go.

type recallStubFetcher struct {
	mem    *types.Memory
	memErr error
	chunks []*types.Chunk
}

func (s *recallStubFetcher) GetMemory(_ context.Context, _ string) (*types.Memory, error) {
	return s.mem, s.memErr
}

func (s *recallStubFetcher) GetChunksForMemory(_ context.Context, _ string) ([]*types.Chunk, error) {
	return s.chunks, nil
}

// ── execFetch boundary cases not in fetch_exec_test.go ───────────────────────

// TestExecFetch_GetMemoryError: GetMemory returns a non-nil error → error is
// propagated unchanged (not swallowed or wrapped with different semantics).
func TestExecFetch_GetMemoryError(t *testing.T) {
	sentinel := errors.New("db exploded")
	f := &recallStubFetcher{memErr: sentinel}
	_, err := execFetch(context.Background(), f, "any-id", "summary", 0, nil)
	require.ErrorIs(t, err, sentinel)
}

// TestExecFetch_FullDetail_ZeroMaxBytes: maxBytes=0 must disable truncation
// regardless of content length, and truncated must be false.
func TestExecFetch_FullDetail_ZeroMaxBytes(t *testing.T) {
	longContent := string(make([]byte, 200_000)) // 200 KB
	f := &recallStubFetcher{mem: &types.Memory{
		ID:      "big-mem",
		Project: "p",
		Content: longContent,
	}}
	out, err := execFetch(context.Background(), f, "big-mem", "full", 0, nil)
	require.NoError(t, err)
	content, ok := out["content"].(string)
	require.True(t, ok)
	require.Equal(t, len(longContent), len(content), "content must not be truncated when maxBytes=0")
	truncated, _ := out["truncated"].(bool)
	require.False(t, truncated)
}

// ── handleMemoryRecall handle-mode tests ─────────────────────────────────────

// parseRecallResult decodes the first text content item of a non-error
// CallToolResult into a map[string]any.
func parseRecallResult(t *testing.T, res *mcpgo.CallToolResult) map[string]any {
	t.Helper()
	require.NotNil(t, res)
	require.False(t, res.IsError, "expected non-error tool result, got: %+v", res.Content)
	require.NotEmpty(t, res.Content, "result content must not be empty")
	text, ok := res.Content[0].(mcpgo.TextContent)
	require.True(t, ok, "first content item must be TextContent, got %T", res.Content[0])
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(text.Text), &out))
	return out
}

// TestMemoryRecall_EmptyQuery_ReturnsValidationError: empty query must return a
// clean MCP tool error (IsError=true) and a nil Go error — not a WARN log.
// The handler must not reach the DB layer for caller input mistakes.
func TestMemoryRecall_EmptyQuery_ReturnsValidationError(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"query":   "",
	}
	cfg := Config{RecallDefaultMode: "handle"}

	res, err := handleMemoryRecall(context.Background(), pool, req, cfg)
	require.NoError(t, err, "caller input error must NOT be a Go error (would produce WARN log)")
	require.NotNil(t, res)
	require.True(t, res.IsError, "empty query must return an MCP tool error result")
	require.NotEmpty(t, res.Content)
	text, ok := res.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	require.Contains(t, text.Text, "query")
}

// TestMemoryRecall_MissingQuery_ReturnsValidationError: missing query key (not
// supplied at all) must also return a clean MCP tool error, not a Go error.
func TestMemoryRecall_MissingQuery_ReturnsValidationError(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		// query key omitted entirely
	}
	cfg := Config{RecallDefaultMode: "handle"}

	res, err := handleMemoryRecall(context.Background(), pool, req, cfg)
	require.NoError(t, err, "caller input error must NOT be a Go error (would produce WARN log)")
	require.NotNil(t, res)
	require.True(t, res.IsError, "missing query must return an MCP tool error result")
}

// TestHandleMemoryRecall_HandleMode_EmptyResults: valid query against noopBackend
// returns zero results; response must contain handles + count + fetch_hint and
// must NOT contain a results key.
func TestHandleMemoryRecall_HandleMode_EmptyResults(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"query":   "what is the meaning of life",
	}
	cfg := Config{RecallDefaultMode: "handle"}

	res, err := handleMemoryRecall(context.Background(), pool, req, cfg)
	require.NoError(t, err)

	out := parseRecallResult(t, res)

	// Required keys.
	_, hasHandles := out["handles"]
	require.True(t, hasHandles, "handle mode must return 'handles' key")
	count, hasCount := out["count"]
	require.True(t, hasCount, "handle mode must return 'count' key")
	require.Equal(t, float64(0), count, "empty backend → count must be 0")
	_, hasFetchHint := out["fetch_hint"]
	require.True(t, hasFetchHint, "handle mode must return 'fetch_hint' key")

	// Forbidden key.
	_, hasResults := out["results"]
	require.False(t, hasResults, "handle mode must NOT return 'results' key")
}

// TestHandleMemoryRecall_DefaultMode_ReturnsResultsKey: RecallDefaultMode="" (default
// full mode) must return results key and must not return handles key.
// Scope: single-project, non-rerank, non-federated, no conflicts enrichment.
func TestHandleMemoryRecall_DefaultMode_ReturnsResultsKey(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"query":   "what is the meaning of life",
	}
	cfg := Config{} // RecallDefaultMode="" → full results path

	res, err := handleMemoryRecall(context.Background(), pool, req, cfg)
	require.NoError(t, err)

	out := parseRecallResult(t, res)

	_, hasResults := out["results"]
	require.True(t, hasResults, "default mode must return 'results' key")

	_, hasHandles := out["handles"]
	require.False(t, hasHandles, "default mode must NOT return 'handles' key")
}

// TestHandleMemoryRecall_EpisodeContextInjected verifies that
// episodeIDFromContext correctly extracts an episode ID placed by withEpisodeID,
// which is the mechanism handleMemoryRecall relies on for the Phase 3 episode
// boost. This is a unit-level test of the context plumbing; integration
// coverage lives in auto_episode_test.go.
func TestHandleMemoryRecall_EpisodeContextInjected(t *testing.T) {
	// Inject episode ID into context.
	ctx := withEpisodeID(context.Background(), "ep-recall-phase3")

	// Verify extraction succeeds and returns the correct ID.
	id, ok := episodeIDFromContext(ctx)
	if !ok || id != "ep-recall-phase3" {
		t.Fatalf("episodeIDFromContext failed: ok=%v id=%q", ok, id)
	}

	// Verify that a context without an episode ID returns ok=false.
	_, okEmpty := episodeIDFromContext(context.Background())
	if okEmpty {
		t.Fatal("episodeIDFromContext must return ok=false on a plain context")
	}

	// Verify that handleMemoryRecall runs without error when episode context is
	// present — the noopBackend returns empty results, so the episode path
	// exits cleanly without event recording.
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"query":   "episode boost smoke test",
	}
	cfg := Config{} // full results mode
	res, err := handleMemoryRecall(ctx, pool, req, cfg)
	if err != nil {
		t.Fatalf("handleMemoryRecall returned unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("handleMemoryRecall returned nil result")
	}
}
