package search_test

// embed_degradation_observability_test.go — #917: silent degradation
// observability increment.
//
// Verifies that:
//   1. RecallWithOpts sets EmbedDegraded=true via RecallOpts when the embed
//      call fails (timeout or immediate error).
//   2. metrics.RecallEmbedTimeoutTotal is incremented on each degraded recall.
//
// These tests cover the clearly-safe observability increment shipped in this
// PR.  The larger #917 architectural fix (adaptive back-pressure / dedicated
// GPU lane / concurrency throttle) is teed up via the opus-advisor
// recommendation in the PR description and tracked in issue #917.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/metrics"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

// TestEmbedDegradedFlag_SetOnTimeout verifies that RecallOpts.EmbedDegraded
// is set to true when the embed call times out (hanging embedder + 100ms
// budget).
func TestEmbedDegradedFlag_SetOnTimeout(t *testing.T) {
	proj := uniqueProject("embed-degraded-flag-timeout")
	eng := newEngineWithEmbedder(t, proj, &hangingEmbedder{dims: 768})
	eng.SetEmbedRecallTimeout(100) // short budget to force a quick timeout

	parentCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var degraded bool
	results, err := eng.RecallWithOpts(parentCtx, "test", 5, "summary",
		search.RecallOpts{EmbedDegraded: &degraded})

	require.NoError(t, err, "RecallWithOpts must degrade gracefully, not error, on embed timeout")
	require.NotNil(t, results)
	require.True(t, degraded, "EmbedDegraded must be true when embed timed out")
}

// TestEmbedDegradedFlag_SetOnImmediateError verifies that EmbedDegraded is set
// to true when the embedder returns an error immediately (no timeout needed).
func TestEmbedDegradedFlag_SetOnImmediateError(t *testing.T) {
	proj := uniqueProject("embed-degraded-flag-error")
	eng := newEngineWithEmbedder(t, proj,
		&errorEmbedder{dims: 768, err: errors.New("simulated embed failure")})

	parentCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var degraded bool
	results, err := eng.RecallWithOpts(parentCtx, "test", 5, "summary",
		search.RecallOpts{EmbedDegraded: &degraded})

	require.NoError(t, err)
	require.NotNil(t, results)
	require.True(t, degraded, "EmbedDegraded must be true when embedder returns error immediately")
}

// TestEmbedDegradedFlag_NotSetOnSuccess verifies that EmbedDegraded is false
// when the embed call succeeds (no degradation).
func TestEmbedDegradedFlag_NotSetOnSuccess(t *testing.T) {
	proj := uniqueProject("embed-degraded-flag-success")
	// syncCountingEmbedder returns a real (zero) vector immediately — no error.
	eng := newEngineWithEmbedder(t, proj, &syncCountingEmbedder{dims: 768})

	parentCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var degraded bool
	_, err := eng.RecallWithOpts(parentCtx, "test", 5, "summary",
		search.RecallOpts{EmbedDegraded: &degraded})

	// err may be non-nil for dimension mismatch or vector search issues, but
	// degraded must be false.
	_ = err
	require.False(t, degraded, "EmbedDegraded must be false when embed succeeds")
}

// TestRecallEmbedTimeoutMetric_IncrementedOnDegradation verifies that
// metrics.RecallEmbedTimeoutTotal is incremented when recall degrades due
// to embed failure (#917 observability increment).
func TestRecallEmbedTimeoutMetric_IncrementedOnDegradation(t *testing.T) {
	proj := uniqueProject("embed-timeout-metric")
	eng := newEngineWithEmbedder(t, proj,
		&errorEmbedder{dims: 768, err: errors.New("embed backend down")})

	before := testutil.ToFloat64(metrics.RecallEmbedTimeoutTotal)

	parentCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := eng.RecallWithOpts(parentCtx, "test", 5, "summary", search.RecallOpts{})
	require.NoError(t, err, "must degrade gracefully")

	after := testutil.ToFloat64(metrics.RecallEmbedTimeoutTotal)
	require.Equal(t, before+1, after,
		"RecallEmbedTimeoutTotal must be incremented by 1 on embed-degraded recall")
}
