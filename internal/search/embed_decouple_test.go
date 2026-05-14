package search_test

// embed_decouple_test.go — issue #611 fix#2: env-var config and metrics coverage.
//
// Tests:
//   T1: ENGRAM_EMBED_RECALL_TIMEOUT_MS shortens the recall embed budget.
//   T2: ENGRAM_STORE_EMBED_MODE=sync embeds inline (embedder called during Store).
//   T3: ENGRAM_STORE_EMBED_MODE=async (default) does NOT call embedder during Store.

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingBlockingEmbedder counts Embed calls and blocks until context is cancelled.
type countingBlockingEmbedder struct {
	dims  int
	calls atomic.Int64
}

func (e *countingBlockingEmbedder) Embed(ctx context.Context, _ string) ([]float32, error) {
	e.calls.Add(1)
	<-ctx.Done()
	return nil, ctx.Err()
}
func (e *countingBlockingEmbedder) Name() string    { return "counting-blocking" }
func (e *countingBlockingEmbedder) Dimensions() int { return e.dims }

var _ embed.Client = (*countingBlockingEmbedder)(nil)

// syncCountingEmbedder counts Embed calls and returns a real (fake) vector.
type syncCountingEmbedder struct {
	dims  int
	calls atomic.Int64
}

func (e *syncCountingEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	e.calls.Add(1)
	return make([]float32, e.dims), nil
}
func (e *syncCountingEmbedder) Name() string    { return "sync-counting" }
func (e *syncCountingEmbedder) Dimensions() int { return e.dims }

var _ embed.Client = (*syncCountingEmbedder)(nil)

// newEngineWithEmbedderAndDSN creates a SearchEngine backed by a real database.
func newEngineWithEmbedderAndDSN(t *testing.T, project string, embedder embed.Client) *search.SearchEngine {
	t.Helper()
	ctx := context.Background()
	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })
	eng := search.New(ctx, backend, embedder, project,
		"http://ollama:11434", "llama3.2", false, nil, 0)
	t.Cleanup(func() { eng.Close() })
	return eng
}

// TestRecallEmbedTimeout_ShortBudget verifies that setting
// ENGRAM_EMBED_RECALL_TIMEOUT_MS=100 causes recall to return quickly
// (within ~300ms) when the embedder blocks.
//
// This is an integration test: it requires TEST_DATABASE_URL.
func TestRecallEmbedTimeout_ShortBudget(t *testing.T) {
	proj := uniqueProject("embed-decouple-short-budget")
	t.Setenv("ENGRAM_EMBED_RECALL_TIMEOUT_MS", "100")

	blocker := &countingBlockingEmbedder{dims: 768}
	eng := newEngineWithEmbedderAndDSN(t, proj, blocker)

	parentCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	results, err := eng.RecallWithOpts(parentCtx, "test query", 5, "summary", search.RecallOpts{})
	elapsed := time.Since(start)

	require.NoError(t, err, "RecallWithOpts must degrade to BM25+recency on embed timeout, not return error")
	require.NotNil(t, results)

	// 100ms budget + BM25 slack => should complete well within 500ms.
	require.Less(t, elapsed, 500*time.Millisecond,
		"with 100ms embed budget, recall must complete in <500ms; got %s", elapsed)

	// Parent context must still be alive.
	require.NoError(t, parentCtx.Err(),
		"parent context must not be cancelled by the embed timeout")
}

// TestStoreEmbedMode_Sync verifies that ENGRAM_STORE_EMBED_MODE=sync causes
// Store to call the embedder inline. The embedder call count must be > 0
// after Store returns.
//
// Integration: requires TEST_DATABASE_URL.
func TestStoreEmbedMode_Sync(t *testing.T) {
	proj := uniqueProject("embed-decouple-sync")
	t.Setenv("ENGRAM_STORE_EMBED_MODE", "sync")

	counter := &syncCountingEmbedder{dims: 4}
	eng := newEngineWithEmbedderAndDSN(t, proj, counter)

	m := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "Sync mode: embedding must be computed inline during Store.",
		MemoryType:  types.MemoryTypeContext,
		Importance:  2,
		StorageMode: "focused",
	}
	require.NoError(t, eng.Store(context.Background(), m))

	assert.Greater(t, int(counter.calls.Load()), 0,
		"ENGRAM_STORE_EMBED_MODE=sync: embedder must be called inline during Store; got 0 calls")
}

// TestStoreEmbedMode_Async verifies that ENGRAM_STORE_EMBED_MODE=async (default)
// does NOT call the embedder during Store. The reembed worker handles embedding.
//
// Integration: requires TEST_DATABASE_URL.
func TestStoreEmbedMode_Async(t *testing.T) {
	proj := uniqueProject("embed-decouple-async")
	t.Setenv("ENGRAM_STORE_EMBED_MODE", "async")

	counter := &syncCountingEmbedder{dims: 4}
	eng := newEngineWithEmbedderAndDSN(t, proj, counter)

	m := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "Async mode: embedder must NOT be called during Store.",
		MemoryType:  types.MemoryTypeContext,
		Importance:  2,
		StorageMode: "focused",
	}
	require.NoError(t, eng.Store(context.Background(), m))

	assert.Equal(t, int64(0), counter.calls.Load(),
		"ENGRAM_STORE_EMBED_MODE=async: embedder must NOT be called during Store; got %d calls",
		counter.calls.Load())
}
