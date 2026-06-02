package search_test

// embed_deadline_test.go — E5 regression tests + #917 degradation signal tests.
//
// RecallWithOpts: embed failure must degrade to BM25+recency, never return an
// error and never hang. RecallWithinMemory has no BM25 fallback (vector-only),
// so it must return an error — but within 3s, not 15s.
//
// #917: degradation must be observable — EmbedDegraded flag set in RecallOpts
// and the engram_recall_degraded_total metric incremented with reason label.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/metrics"
	"github.com/petersimmons1972/engram/internal/search"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
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

// ── #917: degradation must be observable (EmbedDegraded flag + metric) ────────

// TestRecallDegradedSignal_TimeoutReason verifies that when the embed deadline
// fires, RecallWithOpts sets opts.EmbedDegraded=true AND increments the
// engram_recall_degraded_total metric with reason="embed_timeout". This is the
// primary operator signal for detecting backend saturation (#917).
func TestRecallDegradedSignal_TimeoutReason(t *testing.T) {
	proj := uniqueProject("degraded-signal-timeout")
	eng := newEngineWithEmbedder(t, proj, &blockingClient{dims: 768, holdFor: 60 * time.Second})

	callerCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Capture metric value before call.
	before := promtest.ToFloat64(metrics.RecallDegradedTotal.WithLabelValues("embed_timeout"))

	var degraded bool
	opts := search.RecallOpts{EmbedDegraded: &degraded}
	_, err := eng.RecallWithOpts(callerCtx, "test query", 5, "summary", opts)

	require.NoError(t, err, "timeout degradation must not return an error")
	require.True(t, degraded, "EmbedDegraded must be set to true on embed timeout")

	after := promtest.ToFloat64(metrics.RecallDegradedTotal.WithLabelValues("embed_timeout"))
	require.Equal(t, before+1, after,
		"engram_recall_degraded_total{reason=embed_timeout} must increment by 1 on timeout")
}

// TestRecallDegradedSignal_ErrorReason verifies that when the embedder returns
// a hard error (not a context cancellation), the reason label is "embed_error"
// rather than "embed_timeout" — letting operators distinguish saturation from
// model crashes (#917).
//
// Uses immediateErrorEmbedder (synchronous hard error, no timing dependency)
// so the test is deterministic. The previous holdFor=1ms blockingClient could
// race the 500ms embed deadline and misclassify as embed_timeout (#973 blocker fix).
func TestRecallDegradedSignal_ErrorReason(t *testing.T) {
	proj := uniqueProject("degraded-signal-error")
	// immediateErrorEmbedder returns an error synchronously with ctx.Err()==nil,
	// so the engine must classify the reason as "embed_error", not "embed_timeout".
	eng := newEngineWithEmbedder(t, proj, &immediateErrorEmbedder{dims: 768})

	callerCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	before := promtest.ToFloat64(metrics.RecallDegradedTotal.WithLabelValues("embed_error"))

	var degraded bool
	opts := search.RecallOpts{EmbedDegraded: &degraded}
	_, err := eng.RecallWithOpts(callerCtx, "test query", 5, "summary", opts)

	require.NoError(t, err, "hard embed error must degrade gracefully, not return error")
	require.True(t, degraded, "EmbedDegraded must be true on hard embed error")

	after := promtest.ToFloat64(metrics.RecallDegradedTotal.WithLabelValues("embed_error"))
	require.Equal(t, before+1, after,
		"engram_recall_degraded_total{reason=embed_error} must increment by 1 on hard error")
}

// ── #917/#973: RecallWithinMemory degradation signal (Blocker 3) ─────────────

// TestRecallWithinMemoryDegradedSignal_TimeoutReason verifies that when the embed
// deadline fires during RecallWithinMemory, RecallDegradedTotal is incremented
// exactly once with reason="embed_timeout". This mirrors the RecallWithOpts test
// above for the second recall path (#917 blocker fix — missing coverage).
func TestRecallWithinMemoryDegradedSignal_TimeoutReason(t *testing.T) {
	proj := uniqueProject("within-degraded-timeout")
	// Embedder blocks for 60s — far longer than the 500ms embed deadline.
	eng := newEngineWithEmbedder(t, proj, &blockingClient{dims: 768, holdFor: 60 * time.Second})

	callerCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	before := promtest.ToFloat64(metrics.RecallDegradedTotal.WithLabelValues("embed_timeout"))

	// RecallWithinMemory degrades to keyword fallback on embed timeout — no error.
	_, err := eng.RecallWithinMemory(callerCtx, "test query", "nonexistent-id", 5, "summary")

	require.NoError(t, err, "RecallWithinMemory must degrade gracefully on embed timeout, not return error")

	after := promtest.ToFloat64(metrics.RecallDegradedTotal.WithLabelValues("embed_timeout"))
	require.Equal(t, before+1, after,
		"engram_recall_degraded_total{reason=embed_timeout} must increment exactly once via RecallWithinMemory")
}

// TestRecallWithinMemoryDegradedSignal_ErrorReason verifies that when the embedder
// returns a hard synchronous error during RecallWithinMemory, RecallDegradedTotal
// is incremented exactly once with reason="embed_error" (not "embed_timeout").
// Uses immediateErrorEmbedder for determinism (#973 blocker fix — missing coverage).
func TestRecallWithinMemoryDegradedSignal_ErrorReason(t *testing.T) {
	proj := uniqueProject("within-degraded-error")
	eng := newEngineWithEmbedder(t, proj, &immediateErrorEmbedder{dims: 768})

	callerCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	before := promtest.ToFloat64(metrics.RecallDegradedTotal.WithLabelValues("embed_error"))

	_, err := eng.RecallWithinMemory(callerCtx, "test query", "nonexistent-id", 5, "summary")

	require.NoError(t, err, "RecallWithinMemory must degrade gracefully on hard embed error, not return error")

	after := promtest.ToFloat64(metrics.RecallDegradedTotal.WithLabelValues("embed_error"))
	require.Equal(t, before+1, after,
		"engram_recall_degraded_total{reason=embed_error} must increment exactly once via RecallWithinMemory")
}

// TestNoDoubleCount_RecallWithOpts_MCPLayer verifies that a single degraded
// RecallWithOpts call increments RecallDegradedTotal exactly once, not twice.
// Before the fix, the engine layer and the MCP layer both called .Inc() causing
// a double-count (#973 blocker fix).
//
// This test exercises the engine directly (the MCP layer's addRecallDegradedWarning
// no longer contains a .Inc() call, so invoking it after RecallWithOpts will not
// increment the counter a second time — confirmed by this test).
func TestNoDoubleCount_RecallWithOpts_MCPLayer(t *testing.T) {
	proj := uniqueProject("no-double-count")
	eng := newEngineWithEmbedder(t, proj, &immediateErrorEmbedder{dims: 768})

	callerCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	beforeTimeout := promtest.ToFloat64(metrics.RecallDegradedTotal.WithLabelValues("embed_timeout"))
	beforeError := promtest.ToFloat64(metrics.RecallDegradedTotal.WithLabelValues("embed_error"))

	var degraded bool
	opts := search.RecallOpts{EmbedDegraded: &degraded}
	_, err := eng.RecallWithOpts(callerCtx, "test query", 5, "summary", opts)

	require.NoError(t, err)
	require.True(t, degraded)

	afterTimeout := promtest.ToFloat64(metrics.RecallDegradedTotal.WithLabelValues("embed_timeout"))
	afterError := promtest.ToFloat64(metrics.RecallDegradedTotal.WithLabelValues("embed_error"))

	// embed_timeout must NOT have incremented (this was a hard error, not a timeout).
	require.Equal(t, beforeTimeout, afterTimeout,
		"embed_timeout counter must not increment for a hard embed error")
	// embed_error must increment exactly once — not twice from engine + MCP.
	require.Equal(t, beforeError+1, afterError,
		"engram_recall_degraded_total{reason=embed_error} must increment exactly once (engine is sole owner)")
}

// ── #989: EmbedDegradedReason must reflect actual cause, not hardcoded string ──

// TestEmbedDegradedReason_Timeout verifies that when the embed deadline fires,
// RecallWithOpts populates EmbedDegradedReason with "embed_timeout" — not the
// hardcoded literal that the MCP layer used to supply (#989).
func TestEmbedDegradedReason_Timeout(t *testing.T) {
	proj := uniqueProject("degraded-reason-timeout")
	// Embedder blocks for 60s — far longer than the 500ms default deadline.
	eng := newEngineWithEmbedder(t, proj, &blockingClient{dims: 768, holdFor: 60 * time.Second})

	callerCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var degraded bool
	var reason string
	opts := search.RecallOpts{EmbedDegraded: &degraded, EmbedDegradedReason: &reason}
	_, err := eng.RecallWithOpts(callerCtx, "test query", 5, "summary", opts)

	require.NoError(t, err, "timeout degradation must not return an error")
	require.True(t, degraded, "EmbedDegraded must be true on embed timeout")
	require.Equal(t, "embed_timeout", reason,
		"EmbedDegradedReason must be 'embed_timeout' when the embed deadline fires (#989)")
}

// TestEmbedDegradedReason_HardError verifies that when the embedder returns a
// hard synchronous error (not a context cancellation), RecallWithOpts populates
// EmbedDegradedReason with "embed_error" — allowing operators to distinguish
// backend saturation from model crashes (#989).
func TestEmbedDegradedReason_HardError(t *testing.T) {
	proj := uniqueProject("degraded-reason-error")
	eng := newEngineWithEmbedder(t, proj, &immediateErrorEmbedder{dims: 768})

	callerCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var degraded bool
	var reason string
	opts := search.RecallOpts{EmbedDegraded: &degraded, EmbedDegradedReason: &reason}
	_, err := eng.RecallWithOpts(callerCtx, "test query", 5, "summary", opts)

	require.NoError(t, err, "hard embed error must degrade gracefully, not return error")
	require.True(t, degraded, "EmbedDegraded must be true on hard embed error")
	require.Equal(t, "embed_error", reason,
		"EmbedDegradedReason must be 'embed_error' on hard embed failure, not 'embed_timeout' (#989)")
}

// TestEmbedDegradedReason_NoDegrade verifies that when the embed succeeds,
// EmbedDegradedReason is left empty and EmbedDegraded is false.
func TestEmbedDegradedReason_NoDegrade(t *testing.T) {
	proj := uniqueProject("degraded-reason-nodeg")
	eng := newEngineWithEmbedder(t, proj, &fakeClient{dims: 768})

	callerCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var degraded bool
	var reason string
	opts := search.RecallOpts{EmbedDegraded: &degraded, EmbedDegradedReason: &reason}
	_, err := eng.RecallWithOpts(callerCtx, "test query", 5, "summary", opts)

	require.NoError(t, err)
	require.False(t, degraded, "EmbedDegraded must be false when embed succeeds")
	require.Empty(t, reason, "EmbedDegradedReason must be empty when embed succeeds (#989)")
}

// ── #973 Blocker 2: ENGRAM_EMBED_RECALL_TIMEOUT_MS=0 via New() ───────────────

// TestEnvVarZero_DisablesDeadline_RecallWithOpts verifies that when
// ENGRAM_EMBED_RECALL_TIMEOUT_MS=0 in the environment, New() initialises
// embedRecallTimeout to noEmbedTimeout (not 0) so the embed deadline is disabled.
// Before the fix, env=0 stored 0*ms=0 in embedRecallTimeout, causing
// context.WithTimeout(ctx, 0) which is an immediate cancel (#973 blocker fix).
func TestEnvVarZero_DisablesDeadline_RecallWithOpts(t *testing.T) {
	t.Setenv("ENGRAM_EMBED_RECALL_TIMEOUT_MS", "0")

	proj := uniqueProject("envvar-zero-recall")
	// Embedder blocks for 60s — longer than any reasonable deadline.
	// With env=0 correctly mapped to noEmbedTimeout, the embed must block
	// until the parent deadline fires (not immediately cancel).
	eng := newEngineWithEmbedder(t, proj, &blockingClient{dims: 768, holdFor: 60 * time.Second})

	parentCtx, parentCancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer parentCancel()

	start := time.Now()
	_, _ = eng.RecallWithOpts(parentCtx, "test query", 5, "summary", search.RecallOpts{})
	elapsed := time.Since(start)

	// Must block until the parent deadline fires (~800ms), not return immediately
	// (which would happen if env=0 stored 0 duration → immediate context cancel).
	require.GreaterOrEqual(t, elapsed, 700*time.Millisecond,
		"ENGRAM_EMBED_RECALL_TIMEOUT_MS=0 must disable the embed deadline via New(); got %s, want ≥700ms", elapsed)
	require.ErrorIs(t, parentCtx.Err(), context.DeadlineExceeded,
		"parent ctx must have expired (not the internal embed deadline)")
}
