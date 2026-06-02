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
	"net/http"
	"net/http/httptest"
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

// TestDemandSlot_CancelledDuringBackoffReleasesSlot tests the second unguarded
// path: ctx expires DURING the backoff sleep (attempt > 0). Same wedge, different
// code location (~line 299 before the fix).
func TestDemandSlot_CancelledDuringBackoffReleasesSlot(t *testing.T) {
	// Server always returns 503 to trigger the retry/backoff path.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
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

	// Context with a very tight deadline so it expires during the backoff select.
	tightCtx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	_, _, err := client.EmbedWithModel(tightCtx, "test text")
	if err == nil {
		t.Fatal("expected error from EmbedWithModel with tight ctx, got nil")
	}

	client.cb.mu.Lock()
	pif := client.cb.probeInFlight
	client.cb.mu.Unlock()

	if pif {
		t.Error("#1003 WEDGE (backoff path): probeInFlight is still true after ctx expiry during backoff — breaker is wedged")
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
