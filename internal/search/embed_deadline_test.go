package search_test

// embed_deadline_test.go — E5 regression tests.
//
// RecallWithOpts: embed failure must degrade to BM25+recency, never return an
// error and never hang. RecallWithinMemory has no BM25 fallback (vector-only),
// so it must return an error — but within 3s, not 15s.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/stretchr/testify/require"
)

// blockingClient is an embed.Client whose Embed method blocks until its context
// is cancelled or the supplied holdFor duration elapses, then returns an error.
// Name and Dimensions return stub values so the engine can initialise.
type blockingClient struct {
	dims    int
	holdFor time.Duration
}

func (b *blockingClient) Embed(ctx context.Context, _ string) ([]float32, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(b.holdFor):
		return nil, errors.New("blockingClient: timeout simulated")
	}
}
func (b *blockingClient) EmbedWithModel(ctx context.Context, text string) ([]float32, string, error) {
	vec, err := b.Embed(ctx, text)
	return vec, b.Name(), err
}
func (b *blockingClient) Name() string    { return "blocking-fake" }
func (b *blockingClient) Dimensions() int { return b.dims }

// compile-time check
var _ embed.Client = (*blockingClient)(nil)

func newEngineWithEmbedder(t *testing.T, project string, embedder embed.Client) *search.SearchEngine {
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

// TestEmbedDeadline_RecallWithOpts verifies that RecallWithOpts degrades to
// BM25+recency when Ollama is unavailable — no error, returns within 1s.
func TestEmbedDeadline_RecallWithOpts(t *testing.T) {
	proj := uniqueProject("embed-deadline-recall")
	// Embedder blocks for 60s — far longer than the 500ms embed deadline.
	eng := newEngineWithEmbedder(t, proj, &blockingClient{dims: 768, holdFor: 60 * time.Second})

	callerCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	results, err := eng.RecallWithOpts(callerCtx, "test query", 5, "summary", search.RecallOpts{})
	elapsed := time.Since(start)

	require.NoError(t, err, "RecallWithOpts must degrade to BM25+recency on embed failure, not return an error")
	require.NotNil(t, results, "results slice must be non-nil even when degraded")
	require.Less(t, elapsed, 1*time.Second,
		"RecallWithOpts must return within 1s when embed times out (500ms deadline); got %s", elapsed)
}

// TestEmbedDeadline_RecallWithinMemory verifies that RecallWithinMemory degrades
// to keyword search (not an error) when Ollama is unavailable, and returns
// within the embed deadline window (not 15s+).
func TestEmbedDeadline_RecallWithinMemory(t *testing.T) {
	proj := uniqueProject("embed-deadline-within")
	// Embedder blocks for 60s — far longer than the 500ms embed deadline.
	eng := newEngineWithEmbedder(t, proj, &blockingClient{dims: 768, holdFor: 60 * time.Second})

	callerCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	results, err := eng.RecallWithinMemory(callerCtx, "test query", "some-memory-id", 5, "summary")
	elapsed := time.Since(start)

	require.NoError(t, err, "RecallWithinMemory must degrade to keyword search on embed failure, not return an error")
	require.NotNil(t, results, "results slice must be non-nil even when degraded")
	require.Less(t, elapsed, 1*time.Second,
		"RecallWithinMemory must return within 1s when embed times out (500ms deadline); got %s", elapsed)
}

// ── #973 regression: SetEmbedRecallTimeout(0) must disable the embed deadline ──

// TestSetEmbedRecallTimeout_Zero_DisablesDeadline_RecallWithOpts verifies that
// SetEmbedRecallTimeout(0) disables the 500ms embed deadline so the embed call
// runs until the PARENT context cancels it. Before this fix, passing 0 was a
// no-op and the 500ms default remained in effect.
func TestSetEmbedRecallTimeout_Zero_DisablesDeadline_RecallWithOpts(t *testing.T) {
	proj := uniqueProject("set-timeout-zero-recall")
	// Embedder blocks for 60s — longer than the 500ms default but shorter than
	// the parent deadline used below. With the old bug the embed would be
	// cancelled at ~500ms and fall back to BM25 before the parent deadline
	// fires. With the fix the embed blocks until the parent deadline fires.
	eng := newEngineWithEmbedder(t, proj, &blockingClient{dims: 768, holdFor: 60 * time.Second})
	eng.SetEmbedRecallTimeout(0) // documented: 0 = no timeout

	// Parent deadline: 800ms. With a real no-timeout embed the call will block
	// until this fires; if the old 500ms guard is still in effect the call
	// would return in <600ms (before the parent deadline).
	parentCtx, parentCancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer parentCancel()

	start := time.Now()
	_, _ = eng.RecallWithOpts(parentCtx, "test query", 5, "summary", search.RecallOpts{})
	elapsed := time.Since(start)

	// Must NOT return before the parent deadline fires (i.e., must have waited
	// for the parent context, not been cut off by a 500ms internal deadline).
	require.GreaterOrEqual(t, elapsed, 700*time.Millisecond,
		"SetEmbedRecallTimeout(0) must disable the embed deadline; call must block until parent ctx fires (got %s, want ≥700ms)", elapsed)
	require.ErrorIs(t, parentCtx.Err(), context.DeadlineExceeded,
		"parent ctx must have expired (not the internal embed deadline)")
}

// TestSetEmbedRecallTimeout_Zero_DisablesDeadline_RecallWithinMemory mirrors
// the above for RecallWithinMemory (#973 covers both recall paths).
func TestSetEmbedRecallTimeout_Zero_DisablesDeadline_RecallWithinMemory(t *testing.T) {
	proj := uniqueProject("set-timeout-zero-within")
	eng := newEngineWithEmbedder(t, proj, &blockingClient{dims: 768, holdFor: 60 * time.Second})
	eng.SetEmbedRecallTimeout(0)

	parentCtx, parentCancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer parentCancel()

	start := time.Now()
	_, _ = eng.RecallWithinMemory(parentCtx, "test query", "some-memory-id", 5, "summary")
	elapsed := time.Since(start)

	require.GreaterOrEqual(t, elapsed, 700*time.Millisecond,
		"SetEmbedRecallTimeout(0) must disable the embed deadline in RecallWithinMemory; got %s", elapsed)
	require.ErrorIs(t, parentCtx.Err(), context.DeadlineExceeded,
		"parent ctx must have expired (not the internal embed deadline)")
}

// TestSetEmbedRecallTimeout_Positive_StillApplies verifies that a positive ms
// value still enforces the shorter internal deadline (regression guard).
func TestSetEmbedRecallTimeout_Positive_StillApplies(t *testing.T) {
	proj := uniqueProject("set-timeout-positive-recall")
	eng := newEngineWithEmbedder(t, proj, &blockingClient{dims: 768, holdFor: 60 * time.Second})
	eng.SetEmbedRecallTimeout(200) // 200ms internal deadline

	parentCtx, parentCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer parentCancel()

	start := time.Now()
	results, err := eng.RecallWithOpts(parentCtx, "test query", 5, "summary", search.RecallOpts{})
	elapsed := time.Since(start)

	// Must return quickly (internal 200ms deadline fires before parent 5s deadline).
	require.NoError(t, err, "positive timeout must still degrade gracefully to BM25+recency")
	require.NotNil(t, results)
	require.Less(t, elapsed, 400*time.Millisecond,
		"positive 200ms timeout must cut the embed short; got %s", elapsed)
	require.NoError(t, parentCtx.Err(), "parent ctx must remain alive")
}
