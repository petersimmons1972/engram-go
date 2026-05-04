package search_test

// recall_fallback_test.go — T5: Embed timeout fallback tests.
//
// Verifies that RecallWithOpts bounds the embed call with a short timeout
// (500ms) and falls back to BM25+recency WITHOUT propagating the embed
// timeout cancellation to the parent context.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/stretchr/testify/require"
)

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

// errorEmbedder returns an error immediately.
type errorEmbedder struct {
	dims int
	err  error
}

func (e *errorEmbedder) Embed(ctx context.Context, _ string) ([]float32, error) {
	return nil, e.err
}
func (e *errorEmbedder) Name() string    { return "error-fake" }
func (e *errorEmbedder) Dimensions() int { return e.dims }

var _ embed.Client = (*errorEmbedder)(nil)

// TestRecallFallbackOnEmbedTimeout_DoesNotCancelParent verifies that when
// the embed call times out after 500ms, the parent context remains alive
// and RecallWithOpts completes with BM25+recency results (degraded, no error).
func TestRecallFallbackOnEmbedTimeout_DoesNotCancelParent(t *testing.T) {
	proj := uniqueProject("recall-fallback-timeout")
	eng := newEngineWithEmbedder(t, proj, &hangingEmbedder{dims: 768})

	// Parent context with a 5s deadline — should NOT be cancelled by the
	// 500ms embed timeout.
	parentCtx, parentCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer parentCancel()

	start := time.Now()
	results, err := eng.RecallWithOpts(parentCtx, "test query", 5, "summary", search.RecallOpts{})
	elapsed := time.Since(start)

	// Should NOT error; should degrade to BM25+recency.
	require.NoError(t, err, "RecallWithOpts must NOT return error on embed timeout; should degrade to BM25+recency")
	require.NotNil(t, results, "results slice must be non-nil even when degraded")

	// Must return within ~700ms (500ms embed timeout + slack for BM25 search).
	require.Less(t, elapsed, 700*time.Millisecond,
		"RecallWithOpts must return within ~700ms on embed timeout; got %s", elapsed)

	// Parent context must NOT be cancelled.
	require.NoError(t, parentCtx.Err(),
		"parent context must still be alive after embed timeout; Err() = %v", parentCtx.Err())
}

// TestRecallFallbackOnEmbedError_FallsBackToBM25 verifies that when the
// embedder returns an error immediately, RecallWithOpts falls back to
// BM25+recency with no error and completes quickly.
func TestRecallFallbackOnEmbedError_FallsBackToBM25(t *testing.T) {
	proj := uniqueProject("recall-fallback-error")
	eng := newEngineWithEmbedder(t, proj, &errorEmbedder{dims: 768, err: errors.New("simulated embed error")})

	parentCtx, parentCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer parentCancel()

	start := time.Now()
	results, err := eng.RecallWithOpts(parentCtx, "test query", 5, "summary", search.RecallOpts{})
	elapsed := time.Since(start)

	// Should NOT error; should degrade to BM25+recency.
	require.NoError(t, err, "RecallWithOpts must NOT return error on embed failure; should degrade to BM25+recency")
	require.NotNil(t, results, "results slice must be non-nil even when degraded")

	// Must return quickly (BM25 search).
	require.Less(t, elapsed, 500*time.Millisecond,
		"RecallWithOpts must return quickly on immediate embed error; got %s", elapsed)

	// Parent context must NOT be cancelled.
	require.NoError(t, parentCtx.Err(),
		"parent context must still be alive after embed error; Err() = %v", parentCtx.Err())
}
