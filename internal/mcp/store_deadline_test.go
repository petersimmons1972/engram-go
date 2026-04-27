package mcp

// store_deadline_test.go — regression tests for 10-second store deadlines.
//
// Verifies that each MCP store handler (handleMemoryStore,
// handleMemoryStoreDocument, handleMemoryStoreBatch, handleMemoryCorrect)
// wraps its engine call with a context.WithTimeout so a stalled Postgres
// connection cannot block an MCP response indefinitely.
//
// Strategy: override the package-level storeTimeout to 50ms, then inject a
// blockingStoreBackend whose Begin / UpdateMemory block until the context is
// cancelled.  The handler's own deadline fires at 50ms, the blocking call
// unblocks, and the handler returns a context deadline error within < 1s.
// Without the deadline wrapping the call would hang until the test's own
// timeout (60s), which would be caught as a test failure.
//
// All tests are in-package (no _test suffix) because they read storeTimeout,
// which is an unexported package-level var.

import (
	"context"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// blockingStoreBackend embeds noopBackend and overrides the two methods that
// are the first DB calls on each store path:
//
//   - Begin — blocks on Store / StoreBatch / StoreDocument paths.
//   - UpdateMemory — blocks on the Correct path.
//
// Both block until ctx is done, then return the context error.
type blockingStoreBackend struct {
	noopBackend
}

func (b blockingStoreBackend) Begin(ctx context.Context) (db.Tx, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (b blockingStoreBackend) UpdateMemory(ctx context.Context, _ string, _ *string, _ []string, _ *int) (*types.Memory, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// newBlockingPool returns an EnginePool backed by blockingStoreBackend.
func newBlockingPool(t *testing.T) *EnginePool {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, blockingStoreBackend{}, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	return NewEnginePool(factory)
}

// setShortStoreTimeout overrides storeTimeout for the duration of the test,
// restoring it via t.Cleanup.
func setShortStoreTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	orig := storeTimeout
	storeTimeout = d
	t.Cleanup(func() { storeTimeout = orig })
}

// ── handleMemoryStore ────────────────────────────────────────────────────────

// TestStoreDeadline_MemoryStore verifies that handleMemoryStore returns a
// context deadline error within the storeTimeout window rather than blocking.
func TestStoreDeadline_MemoryStore(t *testing.T) {
	setShortStoreTimeout(t, 50*time.Millisecond)
	pool := newBlockingPool(t)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":     "test",
		"content":     "nanosecond wire — one billionth of a second",
		"memory_type": "context",
	}

	start := time.Now()
	_, err := handleMemoryStore(context.Background(), pool, req)
	elapsed := time.Since(start)

	require.Error(t, err, "handleMemoryStore must return an error when the engine stalls past the deadline")
	require.Less(t, elapsed, 1*time.Second,
		"handleMemoryStore must return within 1s (storeTimeout=50ms); got %s", elapsed)
}

// ── handleMemoryStoreBatch ───────────────────────────────────────────────────

// TestStoreDeadline_MemoryStoreBatch verifies that handleMemoryStoreBatch is
// bounded by the store deadline.
func TestStoreDeadline_MemoryStoreBatch(t *testing.T) {
	setShortStoreTimeout(t, 50*time.Millisecond)
	pool := newBlockingPool(t)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"memories": []any{
			map[string]any{
				"content":     "first actual case of a bug being found",
				"memory_type": "context",
			},
		},
	}

	start := time.Now()
	_, err := handleMemoryStoreBatch(context.Background(), pool, req)
	elapsed := time.Since(start)

	require.Error(t, err, "handleMemoryStoreBatch must return an error when the engine stalls past the deadline")
	require.Less(t, elapsed, 1*time.Second,
		"handleMemoryStoreBatch must return within 1s (storeTimeout=50ms); got %s", elapsed)
}

// ── handleMemoryStoreDocument ────────────────────────────────────────────────

// TestStoreDeadline_MemoryStoreDocument verifies that handleMemoryStoreDocument
// is bounded by the store deadline.
func TestStoreDeadline_MemoryStoreDocument(t *testing.T) {
	setShortStoreTimeout(t, 50*time.Millisecond)
	pool := newBlockingPool(t)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":     "test",
		"content":     "it is much easier for most people to write an English statement than it is to use symbols",
		"memory_type": "context",
	}

	start := time.Now()
	_, err := handleMemoryStoreDocument(context.Background(), pool, req, Config{})
	elapsed := time.Since(start)

	require.Error(t, err, "handleMemoryStoreDocument must return an error when the engine stalls past the deadline")
	require.Less(t, elapsed, 1*time.Second,
		"handleMemoryStoreDocument must return within 1s (storeTimeout=50ms); got %s", elapsed)
}

// ── handleMemoryCorrect ──────────────────────────────────────────────────────

// TestStoreDeadline_MemoryCorrect verifies that handleMemoryCorrect is bounded
// by the store deadline (via the UpdateMemory path).
func TestStoreDeadline_MemoryCorrect(t *testing.T) {
	setShortStoreTimeout(t, 50*time.Millisecond)
	pool := newBlockingPool(t)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":   "test",
		"memory_id": "mem_test_001",
		"content":   "compilers can do more than arithmetic",
	}

	start := time.Now()
	_, err := handleMemoryCorrect(context.Background(), pool, req)
	elapsed := time.Since(start)

	require.Error(t, err, "handleMemoryCorrect must return an error when the engine stalls past the deadline")
	require.Less(t, elapsed, 1*time.Second,
		"handleMemoryCorrect must return within 1s (storeTimeout=50ms); got %s", elapsed)
}
