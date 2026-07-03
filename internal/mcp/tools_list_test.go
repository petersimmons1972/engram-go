package mcp

// Regression tests for #1280: memory_list ignored limit/offset because the
// empty MCP input schema (#1281) let schema-driven clients stringify numeric
// args, and getInt's typed cast then silently fell back to its default.
//
// listCapturingBackend captures both the stored memories (for a genuine
// store→list round trip) and the db.ListOptions passed to ListMemories, so
// these tests prove the values reach the backend call — not just that no Go
// error occurred.

import (
	"context"
	"sync"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// listCapturingBackend records StoreMemoryTx writes and ListMemories calls,
// returning the stored memories (most-recent-first) from ListMemories so a
// store→list round trip can be verified without a real Postgres instance.
type listCapturingBackend struct {
	storeCapableBackend
	mu           sync.Mutex
	stored       []*types.Memory
	lastListOpts db.ListOptions
	listCalls    int
}

func (b *listCapturingBackend) StoreMemoryTx(_ context.Context, _ db.Tx, m *types.Memory) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stored = append(b.stored, m)
	return nil
}

func (b *listCapturingBackend) ListMemories(_ context.Context, _ string, opts db.ListOptions) ([]*types.Memory, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastListOpts = opts
	b.listCalls++
	// Return up to opts.Limit items starting at opts.Offset, mirroring a real
	// paginated backend closely enough to prove limit/offset actually reached
	// this call.
	if opts.Offset >= len(b.stored) {
		return []*types.Memory{}, nil
	}
	end := opts.Offset + opts.Limit
	if end > len(b.stored) || opts.Limit <= 0 {
		end = len(b.stored)
	}
	return b.stored[opts.Offset:end], nil
}

func newListCapturingPool(t *testing.T) (*EnginePool, *listCapturingBackend) {
	t.Helper()
	back := &listCapturingBackend{}
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, back, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	return NewEnginePool(factory), back
}

// seedMemories stores n memories via handleMemoryStore so a real store→list
// round trip can be exercised.
func seedMemories(t *testing.T, pool *EnginePool, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		req := mcpgo.CallToolRequest{}
		req.Params.Arguments = map[string]any{
			"project": "test",
			"content": "seed memory",
		}
		res, err := handleMemoryStore(context.Background(), pool, req, testConfig())
		require.NoError(t, err)
		require.False(t, res.IsError, "seed store %d failed: %+v", i, res.Content)
	}
}

// TestMemoryList_LimitOffset_NativeNumbers_ReachBackend is the happy path a
// schema-respecting client produces once #1281 lands: limit/offset arrive as
// real JSON numbers and must reach db.ListOptions unchanged (#1280).
func TestMemoryList_LimitOffset_NativeNumbers_ReachBackend(t *testing.T) {
	pool, back := newListCapturingPool(t)
	seedMemories(t, pool, 10)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"limit":   5.0,
		"offset":  2.0,
	}
	res, err := handleMemoryList(context.Background(), pool, req)
	require.NoError(t, err)
	require.False(t, res.IsError, "expected non-error result, got: %+v", res.Content)

	require.Equal(t, 1, back.listCalls)
	require.Equal(t, 5, back.lastListOpts.Limit, "limit must reach ListMemories unchanged — see #1280")
	require.Equal(t, 2, back.lastListOpts.Offset, "offset must reach ListMemories unchanged — see #1280")

	out := parseSimpleResult(t, res)
	require.Equal(t, float64(5), out["count"], "5 of the 10 seeded memories must be returned")
}

// TestMemoryList_LimitOffset_JSONEncodedStringFallback covers the
// defense-in-depth coercion path: a stringified numeric limit/offset (what a
// pre-#1281 schema-less client actually sent) must still reach the backend
// correctly instead of silently falling back to the default (50/0).
func TestMemoryList_LimitOffset_JSONEncodedStringFallback(t *testing.T) {
	pool, back := newListCapturingPool(t)
	seedMemories(t, pool, 10)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"limit":   "5",
		"offset":  "2",
	}
	res, err := handleMemoryList(context.Background(), pool, req)
	require.NoError(t, err)
	require.False(t, res.IsError, "expected non-error result, got: %+v", res.Content)

	require.Equal(t, 5, back.lastListOpts.Limit,
		"stringified limit must be coerced, not silently fall back to the default 50 — this is the exact #1280 repro")
	require.Equal(t, 2, back.lastListOpts.Offset,
		"stringified offset must be coerced, not silently fall back to the default 0 — this is the exact #1280 repro")
}

// TestMemoryList_DefaultLimit_WhenOmitted verifies the documented default
// (50) still applies when limit is genuinely absent.
func TestMemoryList_DefaultLimit_WhenOmitted(t *testing.T) {
	pool, back := newListCapturingPool(t)
	seedMemories(t, pool, 3)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{"project": "test"}
	res, err := handleMemoryList(context.Background(), pool, req)
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Equal(t, 50, back.lastListOpts.Limit)
	require.Equal(t, 0, back.lastListOpts.Offset)
}

// TestMemoryList_LimitWrongType_ReturnsLoudErrorNotSilentDefault is the
// direct regression guard: a present-but-uncoercible limit must be a tool
// error, never a silent fallback to 50 (the exact shape of #1280).
func TestMemoryList_LimitWrongType_ReturnsLoudErrorNotSilentDefault(t *testing.T) {
	pool, back := newListCapturingPool(t)
	seedMemories(t, pool, 3)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"limit":   []any{"oops"},
	}
	res, err := handleMemoryList(context.Background(), pool, req)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.IsError, "wrongly-typed limit must produce a tool error, not silently default to 50")
	require.Contains(t, res.Content[0].(mcpgo.TextContent).Text, "limit")
	require.Equal(t, 0, back.listCalls, "the backend must not be queried on a rejected request")
}

// TestMemoryList_OffsetWrongType_ReturnsLoudErrorNotSilentDefault mirrors the
// limit case for offset.
func TestMemoryList_OffsetWrongType_ReturnsLoudErrorNotSilentDefault(t *testing.T) {
	pool, back := newListCapturingPool(t)
	seedMemories(t, pool, 3)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"offset":  []any{"oops"},
	}
	res, err := handleMemoryList(context.Background(), pool, req)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.IsError, "wrongly-typed offset must produce a tool error, not silently default to 0")
	require.Contains(t, res.Content[0].(mcpgo.TextContent).Text, "offset")
	require.Equal(t, 0, back.listCalls, "the backend must not be queried on a rejected request")
}

// TestMemoryList_LimitOutOfRange_StillClampsToDefault verifies the existing
// range-clamp behavior (limit<1 or >500 → 50) is preserved by the new strict
// parsing path — this is a boundary condition, not an error condition.
func TestMemoryList_LimitOutOfRange_StillClampsToDefault(t *testing.T) {
	pool, back := newListCapturingPool(t)
	seedMemories(t, pool, 3)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"limit":   5000.0,
	}
	res, err := handleMemoryList(context.Background(), pool, req)
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Equal(t, 50, back.lastListOpts.Limit, "out-of-range limit must still clamp to the default 50")
}
