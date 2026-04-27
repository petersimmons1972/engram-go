package search_test

// embed_deadline_test.go — E5 regression tests.
//
// Verifies that embed calls inside RecallWithOpts and RecallWithinMemory honour
// an independent 15s deadline even when the surrounding request context has a
// much longer (or no) deadline.  A blocking embedder simulates a slow Ollama.

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

// TestEmbedDeadline_RecallWithOpts verifies that RecallWithOpts fails within
// ~20 s even when the embedder would block for 60 s, and that the error is a
// context deadline exceeded (not the caller's context cancellation).
func TestEmbedDeadline_RecallWithOpts(t *testing.T) {
	proj := uniqueProject("embed-deadline-recall")
	// Embedder blocks for 60s — longer than the 15s embed deadline.
	eng := newEngineWithEmbedder(t, proj, &blockingClient{dims: 768, holdFor: 60 * time.Second})

	// Caller context has a generous 90s deadline to confirm the embed deadline
	// fires independently — not because the caller cancelled.
	callerCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	start := time.Now()
	_, err := eng.RecallWithOpts(callerCtx, "test query", 5, "summary", search.RecallOpts{})
	elapsed := time.Since(start)

	require.Error(t, err, "expected embed deadline error")
	require.ErrorContains(t, err, "embed query", "error should wrap embed failure")
	require.Less(t, elapsed, 20*time.Second,
		"RecallWithOpts should fail within 20s (embed deadline is 15s); got %s", elapsed)
	require.Greater(t, elapsed, 10*time.Second,
		"RecallWithOpts should not fail before 10s (sanity check); got %s", elapsed)
}

// TestEmbedDeadline_RecallWithinMemory verifies the same behaviour for
// RecallWithinMemory, which has its own independent embed deadline.
func TestEmbedDeadline_RecallWithinMemory(t *testing.T) {
	proj := uniqueProject("embed-deadline-within")
	eng := newEngineWithEmbedder(t, proj, &blockingClient{dims: 768, holdFor: 60 * time.Second})

	callerCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	start := time.Now()
	_, err := eng.RecallWithinMemory(callerCtx, "test query", "some-memory-id", 5, "summary")
	elapsed := time.Since(start)

	require.Error(t, err, "expected embed deadline error")
	require.ErrorContains(t, err, "embed query", "error should wrap embed failure")
	require.Less(t, elapsed, 20*time.Second,
		"RecallWithinMemory should fail within 20s (embed deadline is 15s); got %s", elapsed)
	require.Greater(t, elapsed, 10*time.Second,
		"RecallWithinMemory should not fail before 10s (sanity check); got %s", elapsed)
}
