package search_test

// embed_timeout_zero_test.go — #973: SetEmbedRecallTimeout(0) semantics.
//
// Verifies that passing ms==0 to SetEmbedRecallTimeout disables the
// engine-level embed deadline, so the parent context governs directly.
// This is the "Option A" behaviour selected by the ADV.5 advisory gate.

import (
	"context"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/search"
	"github.com/stretchr/testify/require"
)

// TestSetEmbedRecallTimeout_ZeroDisablesDeadline verifies that when
// SetEmbedRecallTimeout(0) is called the engine no longer applies a
// 500ms embed-specific deadline.  Instead the parent context deadline
// governs: a 200ms parent deadline cancels the hanging embed after ≈200ms,
// not 500ms.
func TestSetEmbedRecallTimeout_ZeroDisablesDeadline(t *testing.T) {
	proj := uniqueProject("embed-timeout-zero")
	// Use a hanging embedder so we can see exactly when the embed unblocks.
	eng := newEngineWithEmbedder(t, proj, &hangingEmbedder{dims: 768})
	// Disable the engine-level embed deadline (#973).
	eng.SetEmbedRecallTimeout(0)

	// Parent context with a 200ms deadline.  With the old code (ms==0 →
	// preserve 500ms default) the embed would unblock at ~500ms and the call
	// would succeed (degraded).  With the new code (ms==0 → no extra deadline)
	// the embed is cancelled by the parent at 200ms, at which point ctx.Err()
	// is non-nil, so RecallWithOpts propagates context.DeadlineExceeded — the
	// call must return within ~250ms.
	parentCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := eng.RecallWithOpts(parentCtx, "test query", 5, "summary", search.RecallOpts{})
	elapsed := time.Since(start)

	// The parent ctx expired before the embed could finish: the error must be
	// context.DeadlineExceeded (propagated because ctx.Err() != nil).
	require.Error(t, err, "RecallWithOpts must return error when parent ctx expires with no embed deadline")
	require.Less(t, elapsed, 350*time.Millisecond,
		"RecallWithOpts must return within ~350ms when parent ctx has 200ms deadline; got %s", elapsed)
	// Critically: must complete BEFORE the old 500ms default would have fired.
	require.Less(t, elapsed, 450*time.Millisecond,
		"elapsed %s ≥ 450ms — indicates the 500ms default deadline was applied instead of the parent ctx", elapsed)
}

// TestSetEmbedRecallTimeout_ZeroVsPositive contrasts zero vs a short positive
// value to confirm the two paths are distinct.
//
//   - ms>0: engine applies a short embed deadline; embed times out early and
//     RecallWithOpts degrades to BM25 (no error) within that budget.
//   - ms==0: engine applies no embed deadline; a bounded parent ctx is required
//     or the caller will block.  This test gives a 150ms parent so the parent
//     expiry propagates as an error.
func TestSetEmbedRecallTimeout_ZeroVsPositive(t *testing.T) {
	t.Run("positive_ms_applies_budget", func(t *testing.T) {
		proj := uniqueProject("embed-timeout-pos")
		eng := newEngineWithEmbedder(t, proj, &hangingEmbedder{dims: 768})
		eng.SetEmbedRecallTimeout(100) // 100ms budget

		parentCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		start := time.Now()
		results, err := eng.RecallWithOpts(parentCtx, "query", 5, "summary", search.RecallOpts{})
		elapsed := time.Since(start)

		require.NoError(t, err, "positive ms: must degrade gracefully, not error")
		require.NotNil(t, results)
		require.Less(t, elapsed, 400*time.Millisecond,
			"positive ms=100: must return within ~400ms; got %s", elapsed)
		// Parent must still be alive — the 100ms embed deadline must not bleed.
		require.NoError(t, parentCtx.Err())
	})

	t.Run("zero_ms_no_embed_deadline", func(t *testing.T) {
		proj := uniqueProject("embed-timeout-zero2")
		eng := newEngineWithEmbedder(t, proj, &hangingEmbedder{dims: 768})
		eng.SetEmbedRecallTimeout(0) // no embed deadline

		// Short parent forces a bounded call.
		parentCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		start := time.Now()
		_, err := eng.RecallWithOpts(parentCtx, "query", 5, "summary", search.RecallOpts{})
		elapsed := time.Since(start)

		require.Error(t, err, "zero ms: parent deadline must propagate as error")
		require.Less(t, elapsed, 300*time.Millisecond,
			"zero ms: must return within ~300ms when parent has 100ms deadline; got %s", elapsed)
	})
}
