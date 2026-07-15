package search_test

// embed_timeout_zero_test.go — #973: SetEmbedRecallTimeout(0) semantics.
//
// Verifies that passing ms==0 to SetEmbedRecallTimeout disables the
// engine-level embed deadline, so the parent context governs directly.
// This is the "Option A" behaviour selected by the ADV.5 advisory gate.
//
// #1408: these tests previously asserted on real wall-clock elapsed time
// (e.g. "must return within ~400ms"), which flakes under load on shared CI
// runners — the assertion measures scheduler/runner latency, not the
// behavior under test. They now use the same injected-clock / manual-cancel
// technique as embed_deadline_test.go: a hangingEmbedder that blocks until
// its context is cancelled, a goroutine + result channel to observe
// completion deterministically, and either the fake embedRecallClock (for
// the ms>0 path, which calls embedRecallClock.WithTimeout) or a
// manually-cancelled parent context (for the ms==0 path, which bypasses the
// clock entirely and is governed directly by the parent ctx — see
// engine.go's `if e.embedRecallTimeout == noEmbedTimeout` branch). The only
// wall-clock use left is a generous safety-net timeout guarding against a
// genuine hang; it is never the pass/fail assertion.

import (
	"context"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// zeroTestRecallResult carries the outcome of an async RecallWithOpts call so
// tests can synchronize on a channel instead of a wall-clock elapsed bound.
type zeroTestRecallResult struct {
	results []types.SearchResult
	err     error
}

// runRecallAsync starts RecallWithOpts in a goroutine and returns a channel
// that receives its result once it completes.
func runRecallAsync(eng *search.SearchEngine, ctx context.Context) <-chan zeroTestRecallResult {
	done := make(chan zeroTestRecallResult, 1)
	go func() {
		results, err := eng.RecallWithOpts(ctx, "test query", 5, "summary", search.RecallOpts{})
		done <- zeroTestRecallResult{results: results, err: err}
	}()
	return done
}

// requireStillBlocked confirms the recall call has not yet completed, using
// a non-blocking select (no sleep — a false negative here would mean the
// call raced ahead of the test, which the subsequent assertions would still
// catch).
func requireStillBlocked(t *testing.T, done <-chan zeroTestRecallResult) {
	t.Helper()
	select {
	case result := <-done:
		t.Fatalf("RecallWithOpts returned before the deadline was triggered: %v", result.err)
	default:
	}
}

// awaitRecall waits for the async recall to complete, bounded by a generous
// safety-net timeout that only guards against a genuine hang — it is not the
// behavior assertion.
func awaitRecall(t *testing.T, done <-chan zeroTestRecallResult) zeroTestRecallResult {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("RecallWithOpts did not return within the safety-net window")
		return zeroTestRecallResult{}
	}
}

// TestSetEmbedRecallTimeout_ZeroDisablesDeadline verifies that when
// SetEmbedRecallTimeout(0) is called the engine no longer applies a
// 500ms embed-specific deadline. Instead the parent context deadline
// governs: cancelling the parent context cancels the embed directly (via the
// same ctx, since ms==0 makes embedCtx==ctx — see engine.go), not a 500ms
// internal timer.
func TestSetEmbedRecallTimeout_ZeroDisablesDeadline(t *testing.T) {
	proj := uniqueProject("embed-timeout-zero")
	// Use a hanging embedder so completion is driven purely by context
	// cancellation, never by real embed work finishing.
	eng := newEngineWithEmbedder(t, proj, &hangingEmbedder{dims: 768})
	// Disable the engine-level embed deadline (#973).
	eng.SetEmbedRecallTimeout(0)

	// Manually-cancelled parent context (not a real-time deadline): with the
	// old bug (ms==0 → preserve 500ms default) the embed would be cancelled
	// by an internal timer regardless of when we cancel here. With the fix
	// (ms==0 → no extra deadline) the embed is cancelled only when we cancel
	// the parent — deterministic, no wall-clock race.
	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := runRecallAsync(eng, parentCtx)

	// Confirm the call is genuinely blocked on the embed before we cancel —
	// proves no internal deadline fired on its own.
	requireStillBlocked(t, done)

	cancel()

	result := awaitRecall(t, done)
	require.Error(t, result.err, "RecallWithOpts must return error when parent ctx is cancelled with no embed deadline")
}

// TestSetEmbedRecallTimeout_ZeroVsPositive contrasts zero vs a short positive
// value to confirm the two paths are distinct.
//
// - ms>0: engine applies a short embed deadline; embed times out early and
//   RecallWithOpts degrades to BM25 (no error) once that budget is spent.
// - ms==0: engine applies no embed deadline; the parent ctx must be cancelled
//   externally or the caller will block. This test cancels the parent itself
//   so the expiry propagates as an error deterministically.
func TestSetEmbedRecallTimeout_ZeroVsPositive(t *testing.T) {
	t.Run("positive_ms_applies_budget", func(t *testing.T) {
		proj := uniqueProject("embed-timeout-pos")
		eng := newEngineWithEmbedder(t, proj, &hangingEmbedder{dims: 768})
		eng.SetEmbedRecallTimeout(100) // 100ms budget

		clock := newFakeEmbedClock()
		eng.SetEmbedRecallClock(clock)

		parentCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := runRecallAsync(eng, parentCtx)

		// The engine must arm the embed deadline with exactly the configured
		// 100ms budget — this is the deterministic stand-in for "the budget
		// was applied", replacing the old wall-clock elapsed assertion.
		require.Equal(t, 100*time.Millisecond, <-clock.started)
		requireStillBlocked(t, done)

		// Advance the fake clock: fires the embed deadline exactly as a real
		// 100ms timer would, but on our schedule, not the runner's.
		clock.Advance(t)

		result := awaitRecall(t, done)
		require.NoError(t, result.err, "positive ms: must degrade gracefully, not error")
		require.NotNil(t, result.results)
		// Parent must still be alive — the 100ms embed deadline must not bleed
		// into the parent context.
		require.NoError(t, parentCtx.Err())
	})

	t.Run("zero_ms_no_embed_deadline", func(t *testing.T) {
		proj := uniqueProject("embed-timeout-zero2")
		eng := newEngineWithEmbedder(t, proj, &hangingEmbedder{dims: 768})
		eng.SetEmbedRecallTimeout(0) // no embed deadline

		// Manually-cancelled parent forces a bounded call deterministically —
		// no embedRecallClock.WithTimeout call happens on the ms==0 path (see
		// engine.go), so there is nothing to inject a fake clock into; the
		// parent ctx itself is the only knob.
		parentCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := runRecallAsync(eng, parentCtx)

		requireStillBlocked(t, done)

		cancel()

		result := awaitRecall(t, done)
		require.Error(t, result.err, "zero ms: parent cancellation must propagate as error")
	})
}
