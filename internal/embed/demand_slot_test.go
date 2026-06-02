package embed

// demand_slot_test.go — regression tests for #1003: demand-path HalfOpen probe
// slot wedge when the caller's context is already cancelled.
//
// THE BUG: EmbedWithModel calls Allow() at the top. When the breaker is HalfOpen
// with probeInFlight==false, Allow() sets probeInFlight=true and returns nil.
// The function then hits a ctx.Err()!=nil check and returns early WITHOUT calling
// RecordSuccess or RecordFailure. probeInFlight stays true forever: Allow() and
// AllowProbe() both return errCircuitOpen indefinitely — the breaker can never
// recover.
//
// THE FIX: A deferred guard in EmbedWithModel calls AbandonProbe() when no
// explicit outcome was recorded. AbandonProbe() clears probeInFlight and returns
// the state to Open without incrementing consecutiveOpens (neutral release: a
// cancelled client request is NOT evidence the backend is unhealthy).
//
// Test design:
//   - Pure circuit-breaker tests (TestAbandonProbe_*): exercised at the
//     CircuitBreaker level, no HTTP, no real network.
//   - Integration tests (TestDemandSlot_*): drive the real EmbedWithModel with a
//     test HTTP server and a pre-seeded HalfOpen breaker.
//
// All tests are safe under -race and deterministic (injected now, no wall-clock
// sleeps as ordering barriers).

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ── Circuit-breaker unit tests ────────────────────────────────────────────────

// TestAbandonProbe_ReleasesSlotAndReturnsToOpen verifies that AbandonProbe():
//   - clears probeInFlight
//   - returns the state to Open (not HalfOpen, not Closed)
//   - does NOT increment consecutiveOpens
//   - does NOT update nextProbeAt (preserves the existing backoff window)
func TestAbandonProbe_ReleasesSlotAndReturnsToOpen(t *testing.T) {
	now := time.Now()
	cfg := CircuitConfig{
		Enabled:           true,
		FailureThreshold:  2,
		FailureWindow:     30 * time.Second,
		OpenDuration:      10 * time.Second,
		BackoffMultiplier: 2.0,
		BackoffCap:        5 * time.Minute,
	}
	cb := NewCircuitBreaker(cfg)
	cb.now = func() time.Time { return now }

	// Manually put the breaker into HalfOpen with probeInFlight=false.
	cb.mu.Lock()
	cb.state = StateHalfOpen
	cb.consecutiveOpens = 1
	cb.nextProbeAt = now.Add(-1 * time.Second)
	cb.probeInFlight = false
	cb.mu.Unlock()

	// Allow() grants the probe slot.
	if err := cb.Allow(); err != nil {
		t.Fatalf("Allow() in HalfOpen with free slot = %v, want nil", err)
	}

	cb.mu.Lock()
	if !cb.probeInFlight {
		t.Fatal("expected probeInFlight=true after Allow() grant")
	}
	prevConsecutiveOpens := cb.consecutiveOpens
	prevNextProbeAt := cb.nextProbeAt
	cb.mu.Unlock()

	// Verify a second Allow() is blocked (slot is held).
	if err := cb.Allow(); err != errCircuitOpen {
		t.Fatalf("second Allow() while probe in flight = %v, want errCircuitOpen", err)
	}

	// AbandonProbe: release the slot without penalizing the backend.
	cb.AbandonProbe()

	cb.mu.Lock()
	st := cb.state
	pif := cb.probeInFlight
	co := cb.consecutiveOpens
	npa := cb.nextProbeAt
	cb.mu.Unlock()

	if pif {
		t.Error("AbandonProbe: probeInFlight should be false after abandon")
	}
	if st != StateOpen {
		t.Errorf("AbandonProbe: state = %v, want StateOpen", st)
	}
	if co != prevConsecutiveOpens {
		t.Errorf("AbandonProbe: consecutiveOpens = %d, want %d (must not inflate on client cancel)", co, prevConsecutiveOpens)
	}
	if npa != prevNextProbeAt {
		t.Errorf("AbandonProbe: nextProbeAt changed from %v to %v (must not advance on client cancel)", prevNextProbeAt, npa)
	}
}

// TestAbandonProbe_NextAllowCanBeAdmitted verifies that after AbandonProbe, a
// subsequent AllowProbe() can be admitted (the breaker is not wedged).
func TestAbandonProbe_NextAllowCanBeAdmitted(t *testing.T) {
	now := time.Now()
	cfg := CircuitConfig{
		Enabled:           true,
		FailureThreshold:  2,
		FailureWindow:     30 * time.Second,
		OpenDuration:      10 * time.Second,
		BackoffMultiplier: 2.0,
		BackoffCap:        5 * time.Minute,
	}
	cb := NewCircuitBreaker(cfg)
	cb.now = func() time.Time { return now }

	// Seed HalfOpen with free slot.
	cb.mu.Lock()
	cb.state = StateHalfOpen
	cb.consecutiveOpens = 1
	cb.nextProbeAt = now.Add(-1 * time.Second)
	cb.probeInFlight = false
	cb.mu.Unlock()

	// Grab and abandon the slot.
	if err := cb.Allow(); err != nil {
		t.Fatalf("Allow() = %v, want nil", err)
	}
	cb.AbandonProbe()

	// State should now be Open (not HalfOpen, not Closed).
	if cb.State() != StateOpen {
		t.Fatalf("after AbandonProbe: state = %v, want StateOpen", cb.State())
	}

	// Since the state returned to Open (nextProbeAt is in the past), AllowProbe
	// from the background goroutine should be admitted on the next tick.
	if err := cb.AllowProbe(); err != nil {
		t.Errorf("AllowProbe() after AbandonProbe (past nextProbeAt) = %v, want nil (breaker must not be wedged)", err)
	}
}

// TestAbandonProbe_IsNoOpWhenDisabled verifies that AbandonProbe on a disabled
// circuit breaker does not panic and leaves state unchanged.
func TestAbandonProbe_IsNoOpWhenDisabled(t *testing.T) {
	cb := NewCircuitBreaker(CircuitConfig{Enabled: false})
	// Should not panic.
	cb.AbandonProbe()
	if cb.State() != StateClosed {
		t.Errorf("state = %v, want Closed (disabled breaker unchanged)", cb.State())
	}
}

// TestAbandonProbe_NoSlotHeldIsNoOp verifies the probeInFlight guard: when no
// probe slot is actually held, AbandonProbe must be a true no-op — it must NOT
// kick HalfOpen->Open or perturb consecutiveOpens / nextProbeAt. Without the
// guard, a stray call on HalfOpen-without-slot would silently corrupt state.
func TestAbandonProbe_NoSlotHeldIsNoOp(t *testing.T) {
	now := time.Now()
	cfg := CircuitConfig{
		Enabled:           true,
		FailureThreshold:  2,
		FailureWindow:     30 * time.Second,
		OpenDuration:      10 * time.Second,
		BackoffMultiplier: 2.0,
		BackoffCap:        5 * time.Minute,
	}

	// Case A: Closed breaker (no slot). AbandonProbe is a no-op.
	t.Run("Closed", func(t *testing.T) {
		cb := NewCircuitBreaker(cfg)
		cb.now = func() time.Time { return now }
		if cb.State() != StateClosed {
			t.Fatalf("setup: state = %v, want Closed", cb.State())
		}
		cb.AbandonProbe()
		if cb.State() != StateClosed {
			t.Errorf("AbandonProbe on Closed changed state to %v, want Closed", cb.State())
		}
	})

	// Case B: HalfOpen but probeInFlight=false (no slot held). AbandonProbe must
	// NOT transition to Open — that would be silent state corruption.
	t.Run("HalfOpenWithoutSlot", func(t *testing.T) {
		cb := NewCircuitBreaker(cfg)
		cb.now = func() time.Time { return now }
		cb.mu.Lock()
		cb.state = StateHalfOpen
		cb.consecutiveOpens = 3
		cb.nextProbeAt = now.Add(7 * time.Second)
		cb.probeInFlight = false
		prevCO := cb.consecutiveOpens
		prevNPA := cb.nextProbeAt
		cb.mu.Unlock()

		cb.AbandonProbe()

		cb.mu.Lock()
		st := cb.state
		pif := cb.probeInFlight
		co := cb.consecutiveOpens
		npa := cb.nextProbeAt
		cb.mu.Unlock()

		if st != StateHalfOpen {
			t.Errorf("AbandonProbe on HalfOpen-without-slot changed state to %v, want HalfOpen (no slot to release)", st)
		}
		if pif {
			t.Error("probeInFlight became true — should remain false")
		}
		if co != prevCO {
			t.Errorf("consecutiveOpens changed %d -> %d on a no-op abandon", prevCO, co)
		}
		if npa != prevNPA {
			t.Errorf("nextProbeAt changed %v -> %v on a no-op abandon", prevNPA, npa)
		}
	})

	// Case C: HalfOpen WITH probeInFlight=true (slot held). AbandonProbe still
	// releases: probeInFlight->false, state->Open, consecutiveOpens/nextProbeAt
	// untouched. Proves the guard does not break the legitimate path.
	t.Run("HalfOpenWithSlotStillReleases", func(t *testing.T) {
		cb := NewCircuitBreaker(cfg)
		cb.now = func() time.Time { return now }
		cb.mu.Lock()
		cb.state = StateHalfOpen
		cb.consecutiveOpens = 3
		cb.nextProbeAt = now.Add(7 * time.Second)
		cb.probeInFlight = true
		prevCO := cb.consecutiveOpens
		prevNPA := cb.nextProbeAt
		cb.mu.Unlock()

		cb.AbandonProbe()

		cb.mu.Lock()
		st := cb.state
		pif := cb.probeInFlight
		co := cb.consecutiveOpens
		npa := cb.nextProbeAt
		cb.mu.Unlock()

		if pif {
			t.Error("probeInFlight should be false after releasing a held slot")
		}
		if st != StateOpen {
			t.Errorf("state = %v, want Open after releasing a HalfOpen slot", st)
		}
		if co != prevCO {
			t.Errorf("consecutiveOpens changed %d -> %d (must not inflate on abandon)", prevCO, co)
		}
		if npa != prevNPA {
			t.Errorf("nextProbeAt changed %v -> %v (must not advance on abandon)", prevNPA, npa)
		}
	})
}

// ── Demand-path integration tests ────────────────────────────────────────────

// TestDemandSlot_CancelledCtxReleasesSlot is THE wedge regression test (#1003).
//
// Sequence:
//  1. Breaker in HalfOpen, probeInFlight=false.
//  2. EmbedWithModel called with an already-cancelled ctx.
//  3. Allow() grants the probe slot (probeInFlight=true).
//  4. The ctx.Err() != nil pre-flight check fires -> returns early.
//  5. ASSERT (currently FAILS without the fix): probeInFlight is false and a
//     subsequent AllowProbe() can be admitted.
func TestDemandSlot_CancelledCtxReleasesSlot(t *testing.T) {
	// Minimal server: should never be reached in this test because ctx is
	// already cancelled before the HTTP request is issued.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server was reached — ctx-cancel guard did not fire before network I/O")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	now := time.Now()
	cfg := CircuitConfig{
		Enabled:           true,
		FailureThreshold:  2,
		FailureWindow:     30 * time.Second,
		OpenDuration:      10 * time.Second,
		BackoffMultiplier: 2.0,
		BackoffCap:        5 * time.Minute,
	}
	client := NewLiteLLMClientNoProbeWithCircuitBreaker(server.URL, "test-model", "", 0, cfg)
	client.cb.now = func() time.Time { return now }

	// Seed the breaker into HalfOpen with probeInFlight=false.
	client.cb.mu.Lock()
	client.cb.state = StateHalfOpen
	client.cb.consecutiveOpens = 1
	client.cb.nextProbeAt = now.Add(-1 * time.Second)
	client.cb.probeInFlight = false
	client.cb.mu.Unlock()

	// Create a context that is already cancelled.
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// EmbedWithModel should return an error (cancelled context) but MUST release
	// the probe slot.
	_, _, err := client.EmbedWithModel(cancelledCtx, "test text")
	if err == nil {
		t.Fatal("expected error from EmbedWithModel with cancelled ctx, got nil")
	}

	// CRITICAL ASSERTIONS — these fail before the fix is applied:

	client.cb.mu.Lock()
	pif := client.cb.probeInFlight
	co := client.cb.consecutiveOpens
	client.cb.mu.Unlock()

	if pif {
		t.Error("#1003 WEDGE: probeInFlight is still true after EmbedWithModel returned with cancelled ctx — breaker is wedged")
	}

	// consecutiveOpens must NOT be incremented: a cancelled client request is not
	// evidence of backend failure (neutral release, not penalize-on-cancel).
	if co != 1 {
		t.Errorf("#1003 NEUTRAL-RELEASE: consecutiveOpens = %d, want 1 (must not inflate on client cancel)", co)
	}

	// After the slot is released, AllowProbe must admit a new probe.
	if err := client.cb.AllowProbe(); err != nil {
		t.Errorf("#1003 WEDGE: AllowProbe() after cancelled-ctx return = %v, want nil (breaker must not be permanently wedged)", err)
	}
}

// retryThenCancelRoundTripper returns a synthetic, retryable 503 with a complete
// in-memory body on its FIRST call and cancels the request context as part of the
// same return. Because the body is already buffered (no streaming read), the
// cancellation cannot race a network read: EmbedWithModel reads the 503 body
// synchronously, sees a retryable status, loops, and enters the backoff select
// with ctx.Done() already closed. A 2nd RoundTrip call would mean the backoff arm
// did NOT abort the retry — the test asserts that never happens (calls == 1).
type retryThenCancelRoundTripper struct {
	cancel context.CancelFunc
	calls  int
}

func (rt *retryThenCancelRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.calls++
	// Cancel the context now so the subsequent backoff select fires ctx.Done().
	rt.cancel()
	return &http.Response{
		StatusCode: http.StatusServiceUnavailable, // retryable -> triggers backoff
		Body:       io.NopCloser(strings.NewReader("service unavailable")),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

// TestDemandSlot_CancelledDuringBackoffReleasesSlot tests the second unguarded
// path: ctx is cancelled and the backoff select's ctx.Done() arm fires on the
// retry (attempt > 0). Same wedge, different code location (the backoff select).
//
// Deterministic by construction (no real timeout race, no network-read race): a
// custom RoundTripper returns a buffered retryable 503 and cancels the context in
// the same step. On the retry, EmbedWithModel enters the backoff select with
// ctx.Done() already closed; Go's select picks the only ready case, so the
// ctx.Done() arm — the path under test — fires every time regardless of load. The
// time.After(backoff) arm (~75-125ms) is never selected. Asserting calls == 1
// proves the retry aborted IN the backoff select rather than issuing a 2nd
// request, i.e. the intended arm was exercised.
func TestDemandSlot_CancelledDuringBackoffReleasesSlot(t *testing.T) {
	now := time.Now()
	cfg := CircuitConfig{
		Enabled:           true,
		FailureThreshold:  2,
		FailureWindow:     30 * time.Second,
		OpenDuration:      10 * time.Second,
		BackoffMultiplier: 2.0,
		BackoffCap:        5 * time.Minute,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt := &retryThenCancelRoundTripper{cancel: cancel}

	client := NewLiteLLMClientNoProbeWithCircuitBreaker("http://127.0.0.1:1", "test-model", "", 0, cfg)
	client.cb.now = func() time.Time { return now }
	client.http = &http.Client{Transport: rt}

	// Seed the breaker into HalfOpen with probeInFlight=false.
	client.cb.mu.Lock()
	client.cb.state = StateHalfOpen
	client.cb.consecutiveOpens = 1
	client.cb.nextProbeAt = now.Add(-1 * time.Second)
	client.cb.probeInFlight = false
	client.cb.mu.Unlock()

	_, _, err := client.EmbedWithModel(ctx, "test text")
	if err == nil {
		t.Fatal("expected error from EmbedWithModel after ctx cancel during backoff, got nil")
	}
	// Confirm we actually reached the backoff arm (not the network-error or
	// pre-flight ctx.Err() path): the error must be the backoff-select message.
	if !strings.Contains(err.Error(), "canceled during backoff") {
		t.Fatalf("expected the backoff-select cancel path, got: %v", err)
	}
	// Exactly one RoundTrip: the first 503, then the retry aborted in the backoff
	// select before issuing a second request.
	if rt.calls != 1 {
		t.Errorf("RoundTrip calls = %d, want 1 (retry must abort in the backoff select, not re-issue)", rt.calls)
	}

	client.cb.mu.Lock()
	pif := client.cb.probeInFlight
	co := client.cb.consecutiveOpens
	client.cb.mu.Unlock()

	if pif {
		t.Error("#1003 WEDGE (backoff path): probeInFlight is still true after ctx cancel during backoff — breaker is wedged")
	}
	// Neutral release: cancel during backoff must not inflate consecutiveOpens.
	if co != 1 {
		t.Errorf("consecutiveOpens = %d after backoff-cancel, want 1 (neutral release)", co)
	}
}

// TestDemandSlot_SuccessRecordsOnce verifies that a successful EmbedWithModel
// still calls RecordSuccess exactly once (regression guard: the deferred guard
// must not double-record on success paths).
func TestDemandSlot_SuccessRecordsOnce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encodeEmbeddingResponse(w, []float32{0.1, 0.2, 0.3})
	}))
	defer server.Close()

	now := time.Now()
	cfg := CircuitConfig{
		Enabled:           true,
		FailureThreshold:  2,
		FailureWindow:     30 * time.Second,
		OpenDuration:      10 * time.Second,
		BackoffMultiplier: 2.0,
		BackoffCap:        5 * time.Minute,
	}
	client := NewLiteLLMClientNoProbeWithCircuitBreaker(server.URL, "test-model", "", 0, cfg)
	client.cb.now = func() time.Time { return now }

	// Seed HalfOpen.
	client.cb.mu.Lock()
	client.cb.state = StateHalfOpen
	client.cb.consecutiveOpens = 1
	client.cb.nextProbeAt = now.Add(-1 * time.Second)
	client.cb.probeInFlight = false
	client.cb.mu.Unlock()

	_, _, err := client.EmbedWithModel(context.Background(), "test text")
	if err != nil {
		t.Fatalf("EmbedWithModel returned unexpected error: %v", err)
	}

	// Success should transition to Closed.
	if client.cb.State() != StateClosed {
		t.Errorf("after successful embed in HalfOpen: state = %v, want Closed", client.cb.State())
	}

	client.cb.mu.Lock()
	pif := client.cb.probeInFlight
	co := client.cb.consecutiveOpens
	client.cb.mu.Unlock()

	if pif {
		t.Error("probeInFlight should be false after RecordSuccess")
	}
	if co != 0 {
		t.Errorf("consecutiveOpens = %d after RecordSuccess (should reset to 0)", co)
	}
}

// TestDemandSlot_FailureRecordsOnce verifies that a request that fails (non-retryable
// error) in HalfOpen still calls RecordFailure exactly once (no double-record from
// the deferred guard + explicit path).
func TestDemandSlot_FailureRecordsOnce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest) // non-retryable
	}))
	defer server.Close()

	now := time.Now()
	cfg := CircuitConfig{
		Enabled:           true,
		FailureThreshold:  1, // threshold=1: a single failure from HalfOpen triggers re-open
		FailureWindow:     30 * time.Second,
		OpenDuration:      10 * time.Second,
		BackoffMultiplier: 2.0,
		BackoffCap:        5 * time.Minute,
	}
	client := NewLiteLLMClientNoProbeWithCircuitBreaker(server.URL, "test-model", "", 0, cfg)
	client.cb.now = func() time.Time { return now }

	// Seed HalfOpen with consecutiveOpens=1. RecordFailure should increment to 2.
	client.cb.mu.Lock()
	client.cb.state = StateHalfOpen
	client.cb.consecutiveOpens = 1
	client.cb.nextProbeAt = now.Add(-1 * time.Second)
	client.cb.probeInFlight = false
	client.cb.mu.Unlock()

	_, _, err := client.EmbedWithModel(context.Background(), "test text")
	if err == nil {
		t.Fatal("expected error from EmbedWithModel on 400, got nil")
	}

	// After RecordFailure in HalfOpen: state=Open, consecutiveOpens incremented.
	if client.cb.State() != StateOpen {
		t.Errorf("after failed embed in HalfOpen: state = %v, want Open", client.cb.State())
	}

	client.cb.mu.Lock()
	pif := client.cb.probeInFlight
	co := client.cb.consecutiveOpens
	client.cb.mu.Unlock()

	if pif {
		t.Error("probeInFlight should be false after RecordFailure")
	}
	// RecordFailure in HalfOpen increments consecutiveOpens (1 -> 2). The deferred
	// guard must not cause a second RecordFailure (which would increment it to 3).
	if co != 2 {
		t.Errorf("consecutiveOpens = %d after RecordFailure in HalfOpen, want 2 (must be incremented exactly once, not double-recorded)", co)
	}
}

// panicRoundTripper panics on RoundTrip, simulating a mid-request panic that
// lands on the stack AFTER Allow() granted the HalfOpen probe slot but BEFORE
// any recordCBOutcome call in EmbedWithModel.
type panicRoundTripper struct{ msg string }

func (p panicRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	panic(p.msg)
}

// TestDemandSlot_PanicMidRequestReleasesSlot verifies that a panic between the
// Allow() grant and any record call does NOT wedge the breaker: Go runs deferred
// funcs during panic unwind, so the #1003 record-once guard fires AbandonProbe
// and releases the slot. The test recovers the propagated panic and asserts the
// breaker is recoverable (probeInFlight cleared, a subsequent Allow/AllowProbe is
// admitted). This guards our reliance on defer-runs-during-panic.
func TestDemandSlot_PanicMidRequestReleasesSlot(t *testing.T) {
	now := time.Now()
	cfg := CircuitConfig{
		Enabled:           true,
		FailureThreshold:  2,
		FailureWindow:     30 * time.Second,
		OpenDuration:      10 * time.Second,
		BackoffMultiplier: 2.0,
		BackoffCap:        5 * time.Minute,
	}
	client := NewLiteLLMClientNoProbeWithCircuitBreaker("http://127.0.0.1:1", "test-model", "", 0, cfg)
	client.cb.now = func() time.Time { return now }
	// Inject a transport that panics inside c.http.Do(req) — i.e. after Allow()
	// granted the slot and before any recordCBOutcome call.
	client.http = &http.Client{Transport: panicRoundTripper{msg: "simulated mid-request panic"}}

	// Seed HalfOpen with a free slot.
	client.cb.mu.Lock()
	client.cb.state = StateHalfOpen
	client.cb.consecutiveOpens = 1
	client.cb.nextProbeAt = now.Add(-1 * time.Second)
	client.cb.probeInFlight = false
	client.cb.mu.Unlock()

	// Call EmbedWithModel inside a recover() so the propagated panic does not fail
	// the test. The deferred AbandonProbe in EmbedWithModel runs during unwind
	// BEFORE this recover sees the panic.
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected EmbedWithModel to propagate the panic, got none")
			}
		}()
		_, _, _ = client.EmbedWithModel(context.Background(), "test text")
	}()

	// The slot must have been released by the deferred guard during unwind.
	client.cb.mu.Lock()
	pif := client.cb.probeInFlight
	co := client.cb.consecutiveOpens
	client.cb.mu.Unlock()

	if pif {
		t.Error("#1003 PANIC PATH: probeInFlight still true after panic — deferred guard did not release the slot")
	}
	// Neutral release: a panic mid-request abandons the slot without inflating
	// consecutiveOpens (state returns to Open, ready for a real probe).
	if co != 1 {
		t.Errorf("consecutiveOpens = %d after panic, want 1 (neutral release, no inflation)", co)
	}

	// Breaker must be recoverable: AllowProbe admitted on the next tick.
	if err := client.cb.AllowProbe(); err != nil {
		t.Errorf("AllowProbe() after panic-released slot = %v, want nil (breaker must not be wedged)", err)
	}
}
