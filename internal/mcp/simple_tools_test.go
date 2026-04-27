package mcp

// Tests for the three simplified front-door MCP tools:
//   - memory_quick_store  (simplified wrapper over memory_store)
//   - memory_query        (simplified wrapper over memory_recall)
//   - memory_expand       (graph neighbourhood explorer)
//
// All tests use the noopBackend + newTestNoopPool helpers defined in
// explore_handler_test.go (same package).

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// parseSimpleResult decodes the first TextContent item from a non-error result.
func parseSimpleResult(t *testing.T, res *mcpgo.CallToolResult) map[string]any {
	t.Helper()
	require.NotNil(t, res)
	require.False(t, res.IsError, "expected non-error tool result, content: %+v", res.Content)
	require.NotEmpty(t, res.Content)
	text, ok := res.Content[0].(mcpgo.TextContent)
	require.True(t, ok, "expected TextContent, got %T", res.Content[0])
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(text.Text), &out))
	return out
}

// ── memory_quick_store tests ──────────────────────────────────────────────────

// TestMemoryQuickStore_HappyPath: content + project → returns id + status "stored".
func TestMemoryQuickStore_HappyPath(t *testing.T) {
	pool := newStorePool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"content": "important observation about the system",
		"project": "test",
	}

	res, err := handleMemoryQuickStore(context.Background(), pool, req)
	require.NoError(t, err)
	out := parseSimpleResult(t, res)
	require.NotEmpty(t, out["id"], "id must be present")
	require.Equal(t, "stored", out["status"])
}

// TestMemoryQuickStore_DefaultMemoryType: when memory_type is absent it must
// default to "context" (verified by no validation error from handleMemoryStore).
func TestMemoryQuickStore_DefaultMemoryType(t *testing.T) {
	pool := newStorePool(t)
	req := mcpgo.CallToolRequest{}
	// No memory_type supplied — handler must inject "context".
	req.Params.Arguments = map[string]any{
		"content": "context-only content",
		"project": "test",
	}

	res, err := handleMemoryQuickStore(context.Background(), pool, req)
	require.NoError(t, err, "default memory_type must not cause a validation error")
	out := parseSimpleResult(t, res)
	require.Equal(t, "stored", out["status"])
}

// TestMemoryQuickStore_DefaultImportance: when importance is absent it must
// default to 2 (no validation error from handleMemoryStore which allows 0–4).
func TestMemoryQuickStore_DefaultImportance(t *testing.T) {
	pool := newStorePool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"content": "importance-default test",
		"project": "test",
	}

	_, err := handleMemoryQuickStore(context.Background(), pool, req)
	require.NoError(t, err, "default importance=2 must be accepted by handleMemoryStore")
}

// TestMemoryQuickStore_MissingContent: missing content → error.
func TestMemoryQuickStore_MissingContent(t *testing.T) {
	pool := newStorePool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		// content intentionally absent
	}

	result, err := handleMemoryQuickStore(context.Background(), pool, req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.IsError)
	require.Contains(t, result.Content[0].(mcpgo.TextContent).Text, "content is required")
}

// TestMemoryQuickStore_DoesNotMutateOriginal: merged args must not bleed back
// into the caller's args map.
func TestMemoryQuickStore_DoesNotMutateOriginal(t *testing.T) {
	pool := newStorePool(t)
	original := map[string]any{
		"content": "test content",
		"project": "test",
	}
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = original

	_, _ = handleMemoryQuickStore(context.Background(), pool, req)

	// Original map must not have acquired new keys injected by the handler.
	_, hasMemoryType := original["memory_type"]
	_, hasImportance := original["importance"]
	require.False(t, hasMemoryType, "handler must not mutate the original args map")
	require.False(t, hasImportance, "handler must not mutate the original args map")
}

// ── memory_query tests ────────────────────────────────────────────────────────

// TestMemoryQuery_HappyPath: query + project → delegates to handleMemoryRecall
// and returns a non-error result.
func TestMemoryQuery_HappyPath(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"query":   "some interesting topic",
		"project": "test",
	}

	res, err := handleMemoryQuery(context.Background(), pool, req, Config{})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.IsError)
}

// TestMemoryQuery_LimitMapsToTopK: limit param must map to top_k before
// delegating to handleMemoryRecall, and limit must be removed from the args.
func TestMemoryQuery_LimitMapsToTopK(t *testing.T) {
	pool := newTestNoopPool(t)
	// We verify this indirectly: if limit is translated to top_k=3 the recall
	// succeeds (noopBackend returns no results, which is valid).
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"query":   "topic",
		"project": "test",
		"limit":   3,
	}

	res, err := handleMemoryQuery(context.Background(), pool, req, Config{})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.IsError)
}

// TestMemoryQuery_DefaultLimit: when neither limit nor top_k are provided,
// top_k defaults to 5 (no error from recall).
func TestMemoryQuery_DefaultLimit(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"query":   "default limit topic",
		"project": "test",
		// neither limit nor top_k
	}

	res, err := handleMemoryQuery(context.Background(), pool, req, Config{})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.IsError)
}

// TestMemoryQuery_MissingQuery: missing query → clean MCP tool error (IsError=true),
// not a Go error. The delegated recall handler returns the validation error at the
// tool boundary so no WARN log is emitted.
func TestMemoryQuery_MissingQuery(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		// query intentionally absent
	}

	res, err := handleMemoryQuery(context.Background(), pool, req, Config{})
	require.NoError(t, err, "missing query must not return a Go error (would produce WARN log)")
	require.NotNil(t, res)
	require.True(t, res.IsError, "missing query must return an MCP tool error result")
}

// TestMemoryQuery_DoesNotMutateOriginal: original args map must not be modified.
func TestMemoryQuery_DoesNotMutateOriginal(t *testing.T) {
	pool := newTestNoopPool(t)
	original := map[string]any{
		"query":   "topic",
		"project": "test",
		"limit":   7,
	}
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = original

	_, _ = handleMemoryQuery(context.Background(), pool, req, Config{})

	// limit must still be in original (handler must work on a copy).
	_, hasLimit := original["limit"]
	require.True(t, hasLimit, "handler must not delete limit from the original args map")
}

// ── memory_expand tests ───────────────────────────────────────────────────────

// TestMemoryExpand_MissingMemoryID: missing memory_id → error.
func TestMemoryExpand_MissingMemoryID(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		// memory_id intentionally absent
	}

	result, err := handleMemoryExpand(context.Background(), pool, req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.IsError)
	require.Contains(t, result.Content[0].(mcpgo.TextContent).Text, "memory_id is required")
}

// TestMemoryExpand_EmptyConnected: noopBackend.GetConnected returns nil → handler
// returns empty slice without error.
func TestMemoryExpand_EmptyConnected(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"memory_id": "mem_abc123",
		"project":   "test",
	}

	res, err := handleMemoryExpand(context.Background(), pool, req)
	require.NoError(t, err)
	out := parseSimpleResult(t, res)

	require.Equal(t, "mem_abc123", out["memory_id"])
	connected, ok := out["connected"].([]any)
	require.True(t, ok, "connected must be a JSON array, got %T", out["connected"])
	require.Empty(t, connected)
}

// TestMemoryExpand_HappyPath: backend returns connected results → all fields
// are present in the output items.
func TestMemoryExpand_HappyPath(t *testing.T) {
	// Build a pool backed by a stub that returns one connected result.
	stub := &connectedStubBackend{
		results: []db.ConnectedResult{
			{
				Memory:    &types.Memory{ID: "mem_neighbor", Content: "linked content"},
				RelType:   "supports",
				Direction: "outgoing",
				Strength:  0.85,
			},
		},
	}
	pool := newPoolWithBackend(t, stub)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"memory_id": "mem_source",
		"project":   "test",
		"depth":     2,
	}

	res, err := handleMemoryExpand(context.Background(), pool, req)
	require.NoError(t, err)
	out := parseSimpleResult(t, res)

	require.Equal(t, "mem_source", out["memory_id"])
	require.InDelta(t, 2.0, out["depth"], 0.001)

	connected, ok := out["connected"].([]any)
	require.True(t, ok)
	require.Len(t, connected, 1)

	item, ok := connected[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "mem_neighbor", item["id"])
	require.Equal(t, "linked content", item["content"])
	require.Equal(t, "supports", item["rel_type"])
	require.Equal(t, "outgoing", item["direction"])
	require.InDelta(t, 0.85, item["strength"], 0.001)
}

// TestMemoryExpand_DepthClamping: depth outside [1,5] must be reset to 2.
func TestMemoryExpand_DepthClamping(t *testing.T) {
	pool := newTestNoopPool(t)

	for _, badDepth := range []any{0, 6, -1, 99} {
		req := mcpgo.CallToolRequest{}
		req.Params.Arguments = map[string]any{
			"memory_id": "mem_clamp_test",
			"project":   "test",
			"depth":     badDepth,
		}

		res, err := handleMemoryExpand(context.Background(), pool, req)
		require.NoError(t, err, "depth=%v must not error (should be clamped to 2)", badDepth)
		out := parseSimpleResult(t, res)
		require.InDelta(t, 2.0, out["depth"], 0.001, "depth=%v must be clamped to 2", badDepth)
	}
}

// ── capturing backend (verifies injected defaults) ───────────────────────────

// capturingBackend records memories passed via StoreMemoryTx for assertion.
type capturingBackend struct {
	storeCapableBackend
	mu     sync.Mutex
	stored []*types.Memory
}

func (c *capturingBackend) StoreMemoryTx(_ context.Context, _ db.Tx, m *types.Memory) error {
	c.mu.Lock()
	c.stored = append(c.stored, m)
	c.mu.Unlock()
	return nil
}

func newCapturingPool(t *testing.T) (*EnginePool, *capturingBackend) {
	t.Helper()
	cap := &capturingBackend{}
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, cap, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	return NewEnginePool(factory), cap
}

// TestMemoryQuickStore_DefaultMemoryType_Injected verifies the handler actually
// writes memory_type="context" to the backend (not just that no error occurs).
func TestMemoryQuickStore_DefaultMemoryType_Injected(t *testing.T) {
	pool, cap := newCapturingPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"content": "context-only content",
		"project": "test",
	}

	_, err := handleMemoryQuickStore(context.Background(), pool, req)
	require.NoError(t, err)
	require.Len(t, cap.stored, 1)
	require.Equal(t, "context", cap.stored[0].MemoryType)
}

// TestMemoryQuickStore_DefaultImportance_Injected verifies the handler actually
// writes importance=2 to the backend when the caller omits the field.
func TestMemoryQuickStore_DefaultImportance_Injected(t *testing.T) {
	pool, cap := newCapturingPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"content": "importance-default test",
		"project": "test",
	}

	_, err := handleMemoryQuickStore(context.Background(), pool, req)
	require.NoError(t, err)
	require.Len(t, cap.stored, 1)
	require.Equal(t, 2, cap.stored[0].Importance)
}

// ── no-op Tx and Store-capable backend ───────────────────────────────────────

// noopTx is a do-nothing implementation of db.Tx.
type noopTx struct{}

func (noopTx) Commit(_ context.Context) error   { return nil }
func (noopTx) Rollback(_ context.Context) error { return nil }

var _ db.Tx = noopTx{}

// storeCapableBackend embeds noopBackend and overrides Begin to return a
// noopTx, allowing engine.Store to succeed without a real database.
type storeCapableBackend struct {
	noopBackend
}

func (storeCapableBackend) Begin(_ context.Context) (db.Tx, error) {
	return noopTx{}, nil
}

// newStorePool builds an EnginePool whose backend supports Store operations.
func newStorePool(t *testing.T) *EnginePool {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, storeCapableBackend{}, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	return NewEnginePool(factory)
}

// ── stub backend for memory_expand happy path ─────────────────────────────────

// connectedStubBackend embeds noopBackend and overrides GetConnected.
type connectedStubBackend struct {
	noopBackend
	results []db.ConnectedResult
}

func (s *connectedStubBackend) GetConnected(_ context.Context, _ string, _ int) ([]db.ConnectedResult, error) {
	return s.results, nil
}

// newPoolWithBackend builds an EnginePool backed by the given db.Backend.
func newPoolWithBackend(t *testing.T, backend db.Backend) *EnginePool {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, backend, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	return NewEnginePool(factory)
}
