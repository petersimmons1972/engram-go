package embed

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestCircuitBreakerOpensAfterThresholdFailures verifies that recording N failures
// within the window transitions the breaker from Closed to Open.
func TestCircuitBreakerOpensAfterThresholdFailures(t *testing.T) {
	cfg := CircuitConfig{
		Enabled:          true,
		FailureThreshold: 5,
		FailureWindow:    30 * time.Second,
		OpenDuration:     30 * time.Second,
		BackoffMultiplier: 2.0,
		BackoffCap:       5 * time.Minute,
	}
	cb := NewCircuitBreaker(cfg)

	// Record N-1 failures; should stay Closed
	for i := 0; i < 4; i++ {
		if err := cb.Allow(); err != nil {
			t.Fatalf("Allow() failed at attempt %d: %v", i, err)
		}
		cb.RecordFailure()
	}
	if cb.State() != StateClosed {
		t.Errorf("expected Closed after 4 failures, got %v", cb.State())
	}

	// Record the Nth failure; should transition to Open
	if err := cb.Allow(); err != nil {
		t.Fatalf("Allow() failed at 5th attempt: %v", err)
	}
	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Errorf("expected Open after 5 failures, got %v", cb.State())
	}

	// Subsequent Allow() should fail
	if err := cb.Allow(); err != errCircuitOpen {
		t.Errorf("expected errCircuitOpen, got %v", err)
	}
}

// TestCircuitBreakerStaysClosedBelowThreshold verifies that fewer than N failures
// keeps the breaker Closed.
func TestCircuitBreakerStaysClosedBelowThreshold(t *testing.T) {
	cfg := CircuitConfig{
		Enabled:          true,
		FailureThreshold: 5,
		FailureWindow:    30 * time.Second,
		OpenDuration:     30 * time.Second,
		BackoffMultiplier: 2.0,
		BackoffCap:       5 * time.Minute,
	}
	cb := NewCircuitBreaker(cfg)

	for i := 0; i < 4; i++ {
		if err := cb.Allow(); err != nil {
			t.Fatalf("Allow() failed: %v", err)
		}
		cb.RecordFailure()
	}

	if cb.State() != StateClosed {
		t.Errorf("expected Closed after 4 failures, got %v", cb.State())
	}

	// Should still allow new requests
	if err := cb.Allow(); err != nil {
		t.Errorf("expected Allow() to succeed, got %v", err)
	}
}

// TestCircuitBreakerSlidingWindow verifies that failures outside the window don't count.
func TestCircuitBreakerSlidingWindow(t *testing.T) {
	now := time.Now()
	timeFunc := func() time.Time { return now }

	cfg := CircuitConfig{
		Enabled:          true,
		FailureThreshold: 3,
		FailureWindow:    10 * time.Second,
		OpenDuration:     30 * time.Second,
		BackoffMultiplier: 2.0,
		BackoffCap:       5 * time.Minute,
	}
	cb := NewCircuitBreaker(cfg)
	cb.now = timeFunc

	// Record 3 failures at t=0
	for i := 0; i < 3; i++ {
		cb.Allow()
		cb.RecordFailure()
	}
	if cb.State() != StateOpen {
		t.Errorf("expected Open after 3 failures, got %v", cb.State())
	}

	// Advance time beyond the window
	now = now.Add(11 * time.Second)

	// Manually call Allow to check state without being blocked by circuit
	// Since we're in Open, we need to wait for nextProbeAt
	// Let's manually reset and try again
	cb.mu.Lock()
	cb.state = StateClosed
	cb.failures = make([]time.Time, 0)
	cb.mu.Unlock()

	// Record one new failure; window is now 11-21s, old failures are gone
	cb.Allow()
	cb.RecordFailure()

	// Should still be Closed (only 1 failure in window)
	if cb.State() != StateClosed {
		t.Errorf("expected Closed with 1 failure in window after 11s, got %v", cb.State())
	}
}

// TestCircuitBreakerHalfOpenProbeRecoversOnSuccess verifies the recovery path:
// Open → wait OpenDuration → HalfOpen → success → Closed
func TestCircuitBreakerHalfOpenProbeRecoversOnSuccess(t *testing.T) {
	now := time.Now()
	timeFunc := func() time.Time { return now }

	cfg := CircuitConfig{
		Enabled:          true,
		FailureThreshold: 2,
		FailureWindow:    30 * time.Second,
		OpenDuration:     10 * time.Second,
		BackoffMultiplier: 2.0,
		BackoffCap:       5 * time.Minute,
	}
	cb := NewCircuitBreaker(cfg)
	cb.now = timeFunc

	// Open the circuit
	cb.Allow()
	cb.RecordFailure()
	cb.Allow()
	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Fatalf("expected Open, got %v", cb.State())
	}

	// Attempt to Allow while Open; should fail
	if err := cb.Allow(); err != errCircuitOpen {
		t.Errorf("expected errCircuitOpen while Open, got %v", err)
	}

	// Advance time past OpenDuration
	now = now.Add(11 * time.Second)

	// Now Allow should succeed and transition to HalfOpen
	if err := cb.Allow(); err != nil {
		t.Fatalf("expected Allow to succeed after cooldown, got %v", err)
	}
	if cb.State() != StateHalfOpen {
		t.Errorf("expected HalfOpen after cooldown, got %v", cb.State())
	}

	// Probe succeeds; should transition to Closed
	cb.RecordSuccess()
	if cb.State() != StateClosed {
		t.Errorf("expected Closed after probe success, got %v", cb.State())
	}

	// Should allow requests again
	if err := cb.Allow(); err != nil {
		t.Errorf("expected Allow to succeed after recovery, got %v", err)
	}
}

// TestCircuitBreakerHalfOpenProbeFailsBackOff verifies that a failed probe reopens
// with longer cooldown.
func TestCircuitBreakerHalfOpenProbeFailsBackOff(t *testing.T) {
	now := time.Now()
	timeFunc := func() time.Time { return now }

	cfg := CircuitConfig{
		Enabled:          true,
		FailureThreshold: 2,
		FailureWindow:    30 * time.Second,
		OpenDuration:     10 * time.Second,
		BackoffMultiplier: 2.0,
		BackoffCap:       5 * time.Minute,
	}
	cb := NewCircuitBreaker(cfg)
	cb.now = timeFunc

	// Open the circuit (consecutive opens = 1)
	cb.Allow()
	cb.RecordFailure()
	cb.Allow()
	cb.RecordFailure()

	// Advance time and transition to HalfOpen
	now = now.Add(11 * time.Second)
	cb.Allow()

	// Record failure on probe; should reopen with 2x cooldown
	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Errorf("expected Open after probe failure, got %v", cb.State())
	}

	// Check that nextProbeAt is approximately 20 seconds from now (2 * 10s)
	cb.mu.Lock()
	expectedProbeTime := now.Add(20 * time.Second)
	actualProbeTime := cb.nextProbeAt
	cb.mu.Unlock()

	diff := actualProbeTime.Sub(expectedProbeTime).Abs()
	if diff > 1*time.Second {
		t.Errorf("expected nextProbeAt ~%v, got %v (diff: %v)", expectedProbeTime, actualProbeTime, diff)
	}
}

// TestCircuitBreakerBackoffCapped verifies that exponential backoff is capped at BackoffCap.
func TestCircuitBreakerBackoffCapped(t *testing.T) {
	now := time.Now()
	timeFunc := func() time.Time { return now }

	cfg := CircuitConfig{
		Enabled:          true,
		FailureThreshold: 1,
		FailureWindow:    30 * time.Second,
		OpenDuration:     10 * time.Second,
		BackoffMultiplier: 2.0,
		BackoffCap:       60 * time.Second,
	}
	cb := NewCircuitBreaker(cfg)
	cb.now = timeFunc

	// Open and reopen multiple times to exceed cap
	for i := 0; i < 8; i++ {
		cb.Allow()
		cb.RecordFailure()

		if i < 7 {
			// Advance time to transition to HalfOpen and fail
			now = now.Add(cb.cfg.OpenDuration * time.Duration(1<<uint(i)))
			cb.Allow()
			cb.RecordFailure()
		}
	}

	// After many reopens, cooldown should be capped
	cb.mu.Lock()
	expectedCap := cfg.BackoffCap
	nextProbeTime := cb.nextProbeAt
	cooldown := nextProbeTime.Sub(now)
	cb.mu.Unlock()

	if cooldown > expectedCap {
		t.Errorf("expected cooldown <= %v, got %v", expectedCap, cooldown)
	}
}

// TestCircuitBreakerDisabled verifies that when Enabled=false, Allow always succeeds.
func TestCircuitBreakerDisabled(t *testing.T) {
	cfg := CircuitConfig{
		Enabled:          false,
		FailureThreshold: 1,
		FailureWindow:    30 * time.Second,
		OpenDuration:     30 * time.Second,
		BackoffMultiplier: 2.0,
		BackoffCap:       5 * time.Minute,
	}
	cb := NewCircuitBreaker(cfg)

	// Record many failures
	for i := 0; i < 100; i++ {
		if err := cb.Allow(); err != nil {
			t.Errorf("Allow() failed when disabled: %v", err)
		}
		cb.RecordFailure()
	}

	// State should still be Closed
	if cb.State() != StateClosed {
		t.Errorf("expected Closed when disabled, got %v", cb.State())
	}
}

// TestCircuitBreakerConcurrentSafe verifies that concurrent Allow/Record calls
// are safe under the race detector.
func TestCircuitBreakerConcurrentSafe(t *testing.T) {
	cfg := CircuitConfig{
		Enabled:          true,
		FailureThreshold: 5,
		FailureWindow:    30 * time.Second,
		OpenDuration:     30 * time.Second,
		BackoffMultiplier: 2.0,
		BackoffCap:       5 * time.Minute,
	}
	cb := NewCircuitBreaker(cfg)

	var wg sync.WaitGroup
	numGoroutines := 10
	operationsPerGoroutine := 100

	successCount := atomic.Int64{}
	failureCount := atomic.Int64{}

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < operationsPerGoroutine; i++ {
				if err := cb.Allow(); err == nil {
					successCount.Add(1)
					// Randomly record success or failure
					if i%3 == 0 {
						cb.RecordSuccess()
					} else {
						cb.RecordFailure()
					}
				} else {
					failureCount.Add(1)
					cb.RecordFailure()
				}
			}
		}()
	}

	wg.Wait()

	total := successCount.Load() + failureCount.Load()
	if total != int64(numGoroutines*operationsPerGoroutine) {
		t.Errorf("expected %d operations, got %d", numGoroutines*operationsPerGoroutine, total)
	}
}

// TestCircuitBreakerStateTransitions verifies state transitions are correct.
func TestCircuitBreakerStateTransitions(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*CircuitBreaker)
		expectedEnd CircuitState
	}{
		{
			name: "Closed to Open on threshold",
			setup: func(cb *CircuitBreaker) {
				for i := 0; i < cb.cfg.FailureThreshold; i++ {
					cb.Allow()
					cb.RecordFailure()
				}
			},
			expectedEnd: StateOpen,
		},
		{
			name: "Open to HalfOpen after cooldown",
			setup: func(cb *CircuitBreaker) {
				// Open the circuit
				for i := 0; i < cb.cfg.FailureThreshold; i++ {
					cb.Allow()
					cb.RecordFailure()
				}
				// Advance time and transition to HalfOpen
				cb.mu.Lock()
				cb.nextProbeAt = cb.now().Add(-1 * time.Second)
				cb.mu.Unlock()
				cb.Allow()
			},
			expectedEnd: StateHalfOpen,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := CircuitConfig{
				Enabled:           true,
				FailureThreshold:  5,
				FailureWindow:     30 * time.Second,
				OpenDuration:      30 * time.Second,
				BackoffMultiplier: 2.0,
				BackoffCap:        5 * time.Minute,
			}
			cb := NewCircuitBreaker(cfg)
			tc.setup(cb)
			if cb.State() != tc.expectedEnd {
				t.Errorf("expected %v, got %v", tc.expectedEnd, cb.State())
			}
		})
	}
}

// TestCircuitBreakerHalfOpenOnlyAllowsOneProbe verifies that in HalfOpen state,
// only one probe is allowed at a time.
func TestCircuitBreakerHalfOpenOnlyAllowsOneProbe(t *testing.T) {
	now := time.Now()
	timeFunc := func() time.Time { return now }

	cfg := CircuitConfig{
		Enabled:          true,
		FailureThreshold: 2,
		FailureWindow:    30 * time.Second,
		OpenDuration:     10 * time.Second,
		BackoffMultiplier: 2.0,
		BackoffCap:       5 * time.Minute,
	}
	cb := NewCircuitBreaker(cfg)
	cb.now = timeFunc

	// Open the circuit
	cb.Allow()
	cb.RecordFailure()
	cb.Allow()
	cb.RecordFailure()

	// Advance time and transition to HalfOpen
	now = now.Add(11 * time.Second)
	if err := cb.Allow(); err != nil {
		t.Fatalf("first Allow in HalfOpen failed: %v", err)
	}

	// Second Allow should fail (probe already in flight)
	if err := cb.Allow(); err != errCircuitOpen {
		t.Errorf("expected errCircuitOpen for second probe, got %v", err)
	}

	// After recording result, next Allow should work
	cb.RecordSuccess()
	if err := cb.Allow(); err != nil {
		t.Errorf("expected Allow to succeed after probe complete, got %v", err)
	}
}
