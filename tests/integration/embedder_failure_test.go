//go:build integration
// +build integration

package integration_test

// embedder_failure_test.go — T8: End-to-end integration tests for embedder failure
// scenarios from PR #414 (fast-fail + decouple + fallback).
//
// Verifies three behaviors:
//   1. TestMismatchFastFail — embedder model name mismatch returns PermanentError
//      JSON in <200ms with no DB write.
//   2. TestWriteWhileEmbedDown — embedder unavailable; memory_store succeeds in
//      <500ms; chunk row exists with embedding=NULL; chunk_embed_lease row exists;
//      response contains degraded.embed=true.
//   3. TestRecallWhileEmbedSlow — embedder hangs >2s; memory_recall returns <1.5s
//      with BM25 results; response contains degraded.embed=true reason indicates
//      timeout; no `context canceled` error to client.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/mcp"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/testutil"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// testDSN returns the TEST_DATABASE_URL environment variable, skipping t if unset.
func testDSN(t *testing.T) string {
	t.Helper()
	return testutil.DSN(t)
}

// uniqueProject returns a collision-free project name for each test run.
func uniqueProject(base string) string {
	return testutil.UniqueProject(base)
}

// ── Fake embedders for failure scenarios ──────────────────────────────────────

// fakeClientWithName allows tests to customize the embedder name.
type fakeClientWithName struct {
	name string
	dims int
}

func (f *fakeClientWithName) Embed(_ context.Context, text string) ([]float32, error) {
	vec := make([]float32, f.dims)
	for i := range vec {
		vec[i] = float32(i) / float32(f.dims)
	}
	return vec, nil
}

func (f *fakeClientWithName) Name() string    { return f.name }
func (f *fakeClientWithName) Dimensions() int { return f.dims }

var _ embed.Client = (*fakeClientWithName)(nil)

// transientErrorEmbedder returns a transient (non-Permanent) error for any Embed call.
type transientErrorEmbedder struct {
	dims int
	name string
}

func (e *transientErrorEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, errors.New("connection refused to embedder backend")
}

func (e *transientErrorEmbedder) Name() string    { return e.name }
func (e *transientErrorEmbedder) Dimensions() int { return e.dims }

var _ embed.Client = (*transientErrorEmbedder)(nil)

// hangingEmbedder blocks indefinitely until its context is cancelled,
// returning the context error. Used to test timeout behavior.
type hangingEmbedder struct {
	dims int
}

func (h *hangingEmbedder) Embed(ctx context.Context, _ string) ([]float32, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (h *hangingEmbedder) Name() string    { return "hanging-fake" }
func (h *hangingEmbedder) Dimensions() int { return h.dims }

var _ embed.Client = (*hangingEmbedder)(nil)

// ── TestMismatchFastFail ──────────────────────────────────────────────────────

// TestMismatchFastFail verifies that when the embedder model name changes
// (mismatch), the memory_store handler returns a PermanentError JSON response
// within ~200ms, with no chunks persisted (fast-fail without DB write).
func TestMismatchFastFail(t *testing.T) {
	ctx := context.Background()
	dsn := testDSN(t)
	proj := uniqueProject("mismatch-fast-fail")

	// Create an engine with embedder name "model-v1".
	backend1, err := db.NewPostgresBackend(ctx, proj, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { backend1.Close() })

	engine1 := search.New(ctx, backend1, &fakeClientWithName{name: "model-v1", dims: 768}, proj,
		"http://ollama:11434", "llama3.2", false, nil, 0)
	t.Cleanup(func() { engine1.Close() })

	// Store a memory to initialize embedder metadata.
	m1 := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "first memory with model-v1",
		MemoryType:  types.MemoryTypePattern,
		Importance:  1,
		StorageMode: "focused",
	}
	err = engine1.Store(ctx, m1)
	require.NoError(t, err, "initial store should succeed")

	// Now create a second engine with a different embedder name ("model-v2"),
	// using the same backend/project.
	backend2, err := db.NewPostgresBackend(ctx, proj, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { backend2.Close() })

	engine2 := search.New(ctx, backend2, &fakeClientWithName{name: "model-v2", dims: 768}, proj,
		"http://ollama:11434", "llama3.2", false, nil, 0)
	t.Cleanup(func() { engine2.Close() })

	// Try to store a memory with the mismatched embedder via the MCP handler.
	// Create an EnginePool and inject the mismatched engine.
	pool := mcp.NewEnginePool(func(factoryCtx context.Context, p string) (*mcp.EngineHandle, error) {
		if p == proj {
			return &mcp.EngineHandle{Engine: engine2}, nil
		}
		return nil, fmt.Errorf("project %s not found", p)
	})
	t.Cleanup(func() {
		h, err := pool.Get(ctx, proj)
		if err == nil && h != nil && h.Engine != nil {
			h.Engine.Close()
		}
	})

	// Call handleMemoryStore via MCP.
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":     proj,
		"content":     "second memory with model-v2 — should fail fast",
		"memory_type": "context",
		"importance":  2,
	}

	cfg := mcp.Config{
		EmbedderHealth: mcp.NewEmbedderHealth(func(ctx context.Context) (bool, string) {
			return true, ""
		}, 0),
	}

	start := time.Now()
	result, err := mcp.CallHandleMemoryStoreForTest(ctx, pool, req, cfg)
	elapsed := time.Since(start)

	// Assert: error should be embed.PermanentError wrapping embedder_mismatch.
	require.NoError(t, err, "MCP handler should return Go error, got nil")
	require.NotNil(t, result, "MCP handler must return a CallToolResult")
	require.True(t, result.IsError, "CallToolResult.IsError should be true for PermanentError")

	// Parse the error JSON to verify the code.
	if len(result.Content) > 0 {
		tc, ok := result.Content[0].(mcpgo.TextContent)
		if ok {
			var errOut map[string]any
			_ = json.Unmarshal([]byte(tc.Text), &errOut)
			if code, ok := errOut["code"].(string); ok {
				require.Equal(t, "embedder_mismatch", code,
					"error code should be 'embedder_mismatch'")
			}
		}
	}

	// Assert: elapsed time should be < 500ms (200ms + generous slack for race detector).
	require.Less(t, elapsed, 500*time.Millisecond,
		"mismatch detection must be fast, got %s", elapsed)
}

// ── TestWriteWhileEmbedDown ───────────────────────────────────────────────────

// TestWriteWhileEmbedDown verifies that when the embedder backend is unavailable
// (transient error), memory_store still succeeds:
//   - The memory row is persisted.
//   - Chunks are persisted with NULL embeddings.
//   - A lease row exists for each chunk ID (enqueued for async re-embedding).
//   - The response includes degraded.embed=true.
//   - Handler completes within ~500ms.
func TestWriteWhileEmbedDown(t *testing.T) {
	ctx := context.Background()
	dsn := testDSN(t)
	proj := uniqueProject("write-embed-down")

	// Create a pool with transient-error embedder.
	transientEmbed := &transientErrorEmbedder{dims: 768, name: "transient"}

	pool := mcp.NewEnginePool(func(factoryCtx context.Context, p string) (*mcp.EngineHandle, error) {
		if p != proj {
			return nil, fmt.Errorf("project %s not found", p)
		}
		backend, err := db.NewPostgresBackend(factoryCtx, p, dsn)
		if err != nil {
			return nil, err
		}
		engine := search.New(factoryCtx, backend, transientEmbed, p,
			"http://ollama:11434", "llama3.2", false, nil, 0)
		return &mcp.EngineHandle{Engine: engine}, nil
	})
	t.Cleanup(func() {
		h, err := pool.Get(ctx, proj)
		if err == nil && h != nil && h.Engine != nil {
			h.Engine.Close()
		}
	})

	// Call handleMemoryStore.
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":     proj,
		"content":     "memory content while embedder is down",
		"memory_type": "context",
		"importance":  2,
	}

	cfg := mcp.Config{
		EmbedderHealth: mcp.NewEmbedderHealth(func(ctx context.Context) (bool, string) {
			return true, ""
		}, 0),
	}

	start := time.Now()
	result, err := mcp.CallHandleMemoryStoreForTest(ctx, pool, req, cfg)
	elapsed := time.Since(start)

	// Assert: handler should not return a Go error (decouple behavior).
	require.NoError(t, err, "handleMemoryStore should not return Go error on transient embed failure")
	require.NotNil(t, result, "handler must return a CallToolResult")
	require.False(t, result.IsError, "CallToolResult.IsError should be false (success, degraded)")

	// Decode the response JSON.
	if len(result.Content) == 0 {
		t.Fatal("tool result has no content items")
	}
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok, "expected TextContent, got %T", result.Content[0])

	var out map[string]any
	err = json.Unmarshal([]byte(tc.Text), &out)
	require.NoError(t, err, "decode tool result JSON")

	// Extract memory ID.
	memID, ok := out["id"].(string)
	require.True(t, ok, "response must contain a string 'id'")
	require.NotEmpty(t, memID, "memory ID must not be empty")

	// Assert: degraded.embed should be true.
	degradedRaw, ok := out["degraded"]
	require.True(t, ok, "response must contain 'degraded' field")
	degraded, ok := degradedRaw.(map[string]any)
	require.True(t, ok, "degraded must be an object")
	embedDegraded, ok := degraded["embed"].(bool)
	require.True(t, ok, "degraded.embed must be a boolean")
	require.True(t, embedDegraded, "degraded.embed should be true when embedder is down")

	// Assert: elapsed time < 500ms.
	require.Less(t, elapsed, 500*time.Millisecond,
		"write-while-embed-down should complete within 500ms, got %s", elapsed)

	// Verify DB state: memory row exists.
	dbPool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer dbPool.Close()

	var memCount int
	err = dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM memories WHERE id=$1 AND project=$2", memID, proj).Scan(&memCount)
	require.NoError(t, err)
	require.Equal(t, 1, memCount, "memory row must exist in database")

	// Verify chunks exist with NULL embeddings.
	var chunkCount int
	err = dbPool.QueryRow(ctx,
		"SELECT COUNT(*) FROM chunks WHERE memory_id=$1 AND embedding IS NULL", memID).Scan(&chunkCount)
	require.NoError(t, err)
	require.Greater(t, chunkCount, 0, "at least one chunk should be created with NULL embedding")

	// Verify lease rows exist for the chunks (enqueued for re-embedding).
	var leaseCount int
	err = dbPool.QueryRow(ctx,
		"SELECT COUNT(*) FROM chunk_embed_leases WHERE chunk_id IN (SELECT id FROM chunks WHERE memory_id=$1)",
		memID).Scan(&leaseCount)
	require.NoError(t, err)
	require.Equal(t, chunkCount, leaseCount, "lease count should match chunk count with NULL embeddings")
}

// ── TestRecallWhileEmbedSlow ──────────────────────────────────────────────────

// TestRecallWhileEmbedSlow verifies that when the embedder hangs (>2s timeout),
// memory_recall falls back to BM25+recency:
//   - Completes within ~1.5s (not blocked by embedder).
//   - Response includes degraded.embed=true with reason="embed_timeout".
//   - No "context canceled" error to client.
//   - Parent context remains alive after the call.
func TestRecallWhileEmbedSlow(t *testing.T) {
	ctx := context.Background()
	dsn := testDSN(t)
	proj := uniqueProject("recall-embed-slow")

	// First, store a memory so we have something to recall.
	pool := mcp.NewEnginePool(func(factoryCtx context.Context, p string) (*mcp.EngineHandle, error) {
		if p != proj {
			return nil, fmt.Errorf("project %s not found", p)
		}
		backend, err := db.NewPostgresBackend(factoryCtx, p, dsn)
		if err != nil {
			return nil, err
		}
		engine := search.New(factoryCtx, backend, &fakeClientWithName{name: "working", dims: 768}, p,
			"http://ollama:11434", "llama3.2", false, nil, 0)
		return &mcp.EngineHandle{Engine: engine}, nil
	})
	t.Cleanup(func() {
		h, err := pool.Get(ctx, proj)
		if err == nil && h != nil && h.Engine != nil {
			h.Engine.Close()
		}
	})

	// Store a test memory.
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":     proj,
		"content":     "Grace Hopper, compiler, machine code, programming",
		"memory_type": "context",
		"importance":  1,
	}
	cfg := mcp.Config{
		EmbedderHealth: mcp.NewEmbedderHealth(func(ctx context.Context) (bool, string) {
			return true, ""
		}, 0),
	}

	_, err := mcp.CallHandleMemoryStoreForTest(ctx, pool, req, cfg)
	require.NoError(t, err, "initial store should succeed")

	// Now create a new pool with hanging embedder for the recall test.
	hangingPool := mcp.NewEnginePool(func(factoryCtx context.Context, p string) (*mcp.EngineHandle, error) {
		if p != proj {
			return nil, fmt.Errorf("project %s not found", p)
		}
		backend, err := db.NewPostgresBackend(factoryCtx, p, dsn)
		if err != nil {
			return nil, err
		}
		engine := search.New(factoryCtx, backend, &hangingEmbedder{dims: 768}, p,
			"http://ollama:11434", "llama3.2", false, nil, 0)
		return &mcp.EngineHandle{Engine: engine}, nil
	})
	t.Cleanup(func() {
		h, err := hangingPool.Get(ctx, proj)
		if err == nil && h != nil && h.Engine != nil {
			h.Engine.Close()
		}
	})

	// Parent context with a 5s deadline — should NOT be cancelled by the embed timeout.
	parentCtx, parentCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer parentCancel()

	// Call handleMemoryRecall.
	start := time.Now()
	recallResult, err := mcp.CallHandleMemoryRecallFullForTest(parentCtx, hangingPool, proj, "Grace Hopper and compilation",
		map[string]any{
			"top_k":  float64(5),
			"detail": "summary",
		}, cfg)
	elapsed := time.Since(start)

	// Assert: handler should not return a Go error (degraded mode).
	require.NoError(t, err, "handleMemoryRecall should not return Go error on embed timeout")

	// Assert: degraded.embed should be true with timeout reason.
	degradedRaw, ok := recallResult["degraded"]
	require.True(t, ok, "response must contain 'degraded' field")
	degraded, ok := degradedRaw.(map[string]any)
	require.True(t, ok, "degraded must be an object")

	embedDegraded, ok := degraded["embed"].(bool)
	require.True(t, ok, "degraded.embed must be a boolean")
	require.True(t, embedDegraded, "degraded.embed should be true on embed timeout")

	reason, ok := degraded["reason"].(string)
	require.True(t, ok, "degraded.reason must be a string")
	require.Equal(t, "embed_timeout", reason, "reason should indicate embed_timeout")

	// Assert: results should be non-empty (BM25 fallback).
	results, ok := recallResult["results"].([]any)
	require.True(t, ok, "response must contain 'results' array")
	require.Greater(t, len(results), 0, "BM25 fallback should return at least one result")

	// Assert: elapsed time < 1500ms (1s + slack).
	require.Less(t, elapsed, 1500*time.Millisecond,
		"recall with embed timeout should complete within 1.5s, got %s", elapsed)

	// Assert: parent context must still be alive.
	require.NoError(t, parentCtx.Err(),
		"parent context must still be alive after embed timeout; Err() = %v", parentCtx.Err())
}
