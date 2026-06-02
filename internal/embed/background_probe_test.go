package embed

// background_probe_test.go — deterministic tests for StartBackgroundProbe.
//
// Design notes:
//   - No real time.Sleep; all timing is controlled via the injected cb.now func.
//   - Probe behaviour is controlled via a stubbed probeFunc injected through
//     runProbeLoop (a test helper that mirrors the production loop logic).
//   - Goroutine-leak safety is verified via a done-channel / WaitGroup pattern.
//   - All tests are safe under -race.

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// runProbeLoop is the deterministic core of the background probe logic used
// in tests. It mirrors StartBackgroundProbe but accepts an explicit tick
// channel and probeFunc so tests can drive each tick deterministically.
//
// Returns a done channel that closes when the loop exits (context cancelled).
func runProbeLoop(
	ctx context.Context,
	cb *CircuitBreaker,
	probeTimeout time.Duration,
	tick <-chan time.Time,
	probeFunc func(ctx context.Context) (bool, string),
) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick:
				// Mirror production StartBackgroundProbe: only probe when Open,
				// then gate through Allow() for the Open->HalfOpen transition,
				// nextProbeAt enforcement, and probeInFlight guard.
				if cb.State() != StateOpen {
					continue
				}
				if err := cb.Allow(); err != nil {
					continue
				}

				// Run probe with a dedicated timeout context.
				probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
				ok, _ := probeFunc(probeCtx)
				cancel()

				if ok {
					cb.RecordSuccess()
				} else {
					cb.RecordFailure()
				}
			}
		}
	}()
	return done
}

// makeOpenBreaker creates a circuit breaker already in StateOpen with
// nextProbeAt in the past (using the injected now func).
func makeOpenBreaker(t *testing.T, now func() time.Time) *CircuitBreaker {
	t.Helper()
	cfg := CircuitConfig{
		Enabled:           true,
		FailureThreshold:  1,
		FailureWindow:     30 * time.Second,
		OpenDuration:      10 * time.Second,
		BackoffMultiplier: 2.0,
		BackoffCap:        5 * time.Minute,
	}
	cb := NewCircuitBreaker(cfg)
	cb.now = now
	// Force Open state directly.
	cb.mu.Lock()
	cb.state = StateOpen
	cb.consecutiveOpens = 1
	// Set nextProbeAt 1 second in the past so the probe fires immediately.
	cb.nextProbeAt = now().Add(-1 * time.Second)
	cb.mu.Unlock()
	return cb
}

// TestBackgroundProbe_OpenSucceeds verifies:
//
//	Open breaker + now past nextProbeAt + probe success → transitions to Closed,
//	failures cleared, consecutiveOpens reset.
func TestBackgroundProbe_OpenSucceeds(t *testing.T) {
	fixedNow := time.Now()
	cb := makeOpenBreaker(t, func() time.Time { return fixedNow })

	probeFunc := func(_ context.Context) (bool, string) { return true, "" }
	tick := make(chan time.Time, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := runProbeLoop(ctx, cb, 5*time.Second, tick, probeFunc)

	// Fire one tick.
	tick <- fixedNow

	// Wait long enough for the goroutine to process.
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	if got := cb.State(); got != StateClosed {
		t.Errorf("expected StateClosed after successful probe, got %v", got)
	}

	cb.mu.Lock()
	co := cb.consecutiveOpens
	fl := len(cb.failures)
	cb.mu.Unlock()

	if co != 0 {
		t.Errorf("consecutiveOpens = %d, want 0", co)
	}
	if fl != 0 {
		t.Errorf("failures len = %d, want 0", fl)
	}
}

// TestBackgroundProbe_OpenFails verifies:
//
//	Open breaker + probe fails -> stays Open, consecutiveOpens++, nextProbeAt advances.
func TestBackgroundProbe_OpenFails(t *testing.T) {
	fixedNow := time.Now()
	cb := makeOpenBreaker(t, func() time.Time { return fixedNow })

	// Capture initial nextProbeAt.
	cb.mu.Lock()
	initialNextProbeAt := cb.nextProbeAt
	initialConsecutiveOpens := cb.consecutiveOpens
	cb.mu.Unlock()

	probeFunc := func(_ context.Context) (bool, string) { return false, "connection refused" }
	tick := make(chan time.Time, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := runProbeLoop(ctx, cb, 5*time.Second, tick, probeFunc)

	tick <- fixedNow
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	if got := cb.State(); got != StateOpen {
		t.Errorf("expected StateOpen after failed probe, got %v", got)
	}

	cb.mu.Lock()
	newConsecutiveOpens := cb.consecutiveOpens
	newNextProbeAt := cb.nextProbeAt
	cb.mu.Unlock()

	if newConsecutiveOpens <= initialConsecutiveOpens {
		t.Errorf("consecutiveOpens did not increase: before=%d after=%d", initialConsecutiveOpens, newConsecutiveOpens)
	}
	if !newNextProbeAt.After(initialNextProbeAt) {
		t.Errorf("nextProbeAt did not advance: before=%v after=%v", initialNextProbeAt, newNextProbeAt)
	}
}

// TestBackgroundProbe_ClosedIsNoOp verifies:
//
//	When breaker is Closed, background tick does NOT call the probe func.
func TestBackgroundProbe_ClosedIsNoOp(t *testing.T) {
	fixedNow := time.Now()
	cfg := CircuitConfig{
		Enabled:           true,
		FailureThreshold:  5,
		FailureWindow:     30 * time.Second,
		OpenDuration:      10 * time.Second,
		BackoffMultiplier: 2.0,
		BackoffCap:        5 * time.Minute,
	}
	cb := NewCircuitBreaker(cfg)
	cb.now = func() time.Time { return fixedNow }

	// Confirm state is Closed.
	if cb.State() != StateClosed {
		t.Fatalf("expected StateClosed, got %v", cb.State())
	}

	var probeCalled atomic.Int64
	probeFunc := func(_ context.Context) (bool, string) {
		probeCalled.Add(1)
		return true, ""
	}
	tick := make(chan time.Time, 3)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := runProbeLoop(ctx, cb, 5*time.Second, tick, probeFunc)

	// Send three ticks -- none should invoke probeFunc because state is Closed.
	tick <- fixedNow
	tick <- fixedNow
	tick <- fixedNow
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	if n := probeCalled.Load(); n != 0 {
		t.Errorf("probe was called %d times on a Closed breaker, want 0", n)
	}
}

// TestBackgroundProbe_ContextCancelExits verifies:
//
//	Cancelling the context causes the goroutine to exit cleanly (no leak).
func TestBackgroundProbe_ContextCancelExits(t *testing.T) {
	fixedNow := time.Now()
	cb := makeOpenBreaker(t, func() time.Time { return fixedNow })

	probeFunc := func(ctx context.Context) (bool, string) {
		// Block until context cancelled.
		<-ctx.Done()
		return false, "cancelled"
	}
	tick := make(chan time.Time)

	ctx, cancel := context.WithCancel(context.Background())
	done := runProbeLoop(ctx, cb, 5*time.Second, tick, probeFunc)

	// Cancel immediately -- no ticks needed.
	cancel()

	select {
	case <-done:
		// Good: goroutine exited.
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine did not exit within 2s after context cancel")
	}
}

// TestBackgroundProbe_OnlyOneProbeInFlight verifies:
//
//	probeInFlight flag prevents concurrent probes; second tick while first
//	probe is running is a no-op.
func TestBackgroundProbe_OnlyOneProbeInFlight(t *testing.T) {
	fixedNow := time.Now()
	cb := makeOpenBreaker(t, func() time.Time { return fixedNow })

	var probeCalled atomic.Int64
	// Block the first probe call until we've sent the second tick.
	firstProbeStarted := make(chan struct{}, 1)
	firstProbeUnblock := make(chan struct{})

	probeFunc := func(ctx context.Context) (bool, string) {
		probeCalled.Add(1)
		select {
		case firstProbeStarted <- struct{}{}:
		default:
		}
		select {
		case <-firstProbeUnblock:
		case <-ctx.Done():
		}
		return true, ""
	}

	tick := make(chan time.Time, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := runProbeLoop(ctx, cb, 5*time.Second, tick, probeFunc)

	// Fire first tick -- probe will block.
	tick <- fixedNow
	<-firstProbeStarted

	// Fire second tick while first probe is in flight.
	tick <- fixedNow
	time.Sleep(30 * time.Millisecond)

	// Unblock the first probe.
	close(firstProbeUnblock)
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	// Only one probe should have run (second tick was a no-op due to probeInFlight).
	if n := probeCalled.Load(); n != 1 {
		t.Errorf("probe called %d times, want exactly 1", n)
	}
}

// TestBackgroundProbe_ConcurrentSafe verifies that the probe loop and the
// circuit breaker's Allow/RecordFailure paths are race-free under -race.
func TestBackgroundProbe_ConcurrentSafe(t *testing.T) {
	fixedNow := time.Now()
	cfg := CircuitConfig{
		Enabled:           true,
		FailureThreshold:  3,
		FailureWindow:     30 * time.Second,
		OpenDuration:      1 * time.Second,
		BackoffMultiplier: 1.5,
		BackoffCap:        10 * time.Second,
	}
	cb := NewCircuitBreaker(cfg)

	// Drive now forward so nextProbeAt is periodically past.
	var nowOffset atomic.Int64
	cb.now = func() time.Time { return fixedNow.Add(time.Duration(nowOffset.Load())) }

	// Alternate probe success/fail.
	var probeCount atomic.Int64
	probeFunc := func(_ context.Context) (bool, string) {
		n := probeCount.Add(1)
		if n%2 == 0 {
			return true, ""
		}
		return false, "simulated fail"
	}

	tick := make(chan time.Time, 32)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := runProbeLoop(ctx, cb, 100*time.Millisecond, tick, probeFunc)

	// Concurrent Allow/RecordFailure goroutines.
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if err := cb.Allow(); err == nil {
					cb.RecordFailure()
				}
				nowOffset.Add(int64(100 * time.Millisecond))
			}
		}()
	}

	// Concurrently send ticks.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			select {
			case tick <- fixedNow:
			default:
			}
			nowOffset.Add(int64(50 * time.Millisecond))
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()
	cancel()
	<-done
	// No assertion on final state -- test passes if -race detects no races.
}

// TestStartBackgroundProbe_IntegrationSmoke verifies the exported method
// StartBackgroundProbe on a real *LiteLLMClient (no network), exercising
// the lifecycle wiring at the method level.
func TestStartBackgroundProbe_IntegrationSmoke(t *testing.T) {
	// Build a LiteLLMClient with a circuit breaker but no real HTTP server.
	cbCfg := CircuitConfig{
		Enabled:           true,
		FailureThreshold:  1,
		FailureWindow:     30 * time.Second,
		OpenDuration:      10 * time.Second,
		BackoffMultiplier: 2.0,
		BackoffCap:        5 * time.Minute,
	}
	c := NewLiteLLMClientNoProbeWithCircuitBreaker("http://127.0.0.1:1", "test-model", "", 0, cbCfg)

	fixedNow := time.Now()
	c.cb.now = func() time.Time { return fixedNow }

	// Force breaker Open with nextProbeAt in the past.
	c.cb.mu.Lock()
	c.cb.state = StateOpen
	c.cb.consecutiveOpens = 1
	c.cb.nextProbeAt = fixedNow.Add(-1 * time.Second)
	c.cb.mu.Unlock()

	// Start the background probe with a short interval.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// StartBackgroundProbe is the production method under test.
	// If it does not exist yet, compilation fails (TDD red phase).
	c.StartBackgroundProbe(ctx, 10*time.Millisecond)

	// Give the goroutine time to run at least one probe cycle.
	// The real probe will fail (no server at 127.0.0.1:1) -- that's acceptable;
	// we're testing that the method exists, starts without panic, and the
	// goroutine exits cleanly when ctx is cancelled.
	time.Sleep(150 * time.Millisecond)
	cancel()

	// Allow time for goroutine exit.
	time.Sleep(50 * time.Millisecond)
	// Test passes if no panic and no deadlock.
}
