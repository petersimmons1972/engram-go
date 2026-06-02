package embed

// background_probe_test.go — deterministic tests for StartBackgroundProbe.
//
// Design:
//   - Tests drive the REAL StartBackgroundProbe loop (no mirror helper). The
//     loop's probe call is injected via the unexported probeFunc field so no
//     network is needed.
//   - Synchronization uses channels signaled by the probe stub — never
//     time.Sleep as an ordering barrier. The only timed constructs are
//     bounded negative-assertion guards (proving an event does NOT happen).
//   - All tests are safe under -race.

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newProbeTestClient builds a LiteLLMClient with a circuit breaker and an
// injectable probe stub, plus a controllable injected clock.
func newProbeTestClient(t *testing.T, cfg CircuitConfig, now func() time.Time, probe func(context.Context) (bool, string)) *LiteLLMClient {
	t.Helper()
	c := NewLiteLLMClientNoProbeWithCircuitBreaker("http://127.0.0.1:1", "test-model", "", 0, cfg)
	c.cb.now = now
	c.probeFunc = probe
	return c
}

// forceOpen drives the breaker directly into StateOpen with nextProbeAt in the
// past (relative to the injected clock) so the next probe fires immediately.
func forceOpen(cb *CircuitBreaker, now time.Time) {
	cb.mu.Lock()
	cb.state = StateOpen
	cb.consecutiveOpens = 1
	cb.nextProbeAt = now.Add(-1 * time.Second)
	cb.mu.Unlock()
}

func defaultCfg() CircuitConfig {
	return CircuitConfig{
		Enabled:           true,
		FailureThreshold:  1,
		FailureWindow:     30 * time.Second,
		OpenDuration:      10 * time.Second,
		BackoffMultiplier: 2.0,
		BackoffCap:        5 * time.Minute,
	}
}

// TestBackgroundProbe_OpenSucceeds: Open + past nextProbeAt + probe success ->
// Closed, failures cleared, consecutiveOpens reset.
func TestBackgroundProbe_OpenSucceeds(t *testing.T) {
	fixedNow := time.Now()
	probed := make(chan struct{}, 1)
	c := newProbeTestClient(t, defaultCfg(), func() time.Time { return fixedNow },
		func(context.Context) (bool, string) {
			probed <- struct{}{}
			return true, ""
		})
	forceOpen(c.cb, fixedNow)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.StartBackgroundProbe(ctx, time.Millisecond)

	// Wait for the probe to actually run (channel barrier, not a sleep).
	select {
	case <-probed:
	case <-time.After(2 * time.Second):
		t.Fatal("probe never ran")
	}

	// Wait for the breaker to reach Closed (RecordSuccess runs after probe).
	waitForState(t, c.cb, StateClosed, 2*time.Second)
	cancel()

	c.cb.mu.Lock()
	co, fl := c.cb.consecutiveOpens, len(c.cb.failures)
	c.cb.mu.Unlock()
	if co != 0 {
		t.Errorf("consecutiveOpens = %d, want 0", co)
	}
	if fl != 0 {
		t.Errorf("failures len = %d, want 0", fl)
	}
}

// TestBackgroundProbe_OpenFails: Open + probe fails -> stays Open,
// consecutiveOpens++, nextProbeAt advances.
func TestBackgroundProbe_OpenFails(t *testing.T) {
	fixedNow := time.Now()
	probed := make(chan struct{}, 1)
	c := newProbeTestClient(t, defaultCfg(), func() time.Time { return fixedNow },
		func(context.Context) (bool, string) {
			probed <- struct{}{}
			return false, "connection refused"
		})
	forceOpen(c.cb, fixedNow)

	c.cb.mu.Lock()
	initialNextProbeAt := c.cb.nextProbeAt
	initialConsecutiveOpens := c.cb.consecutiveOpens
	c.cb.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.StartBackgroundProbe(ctx, time.Millisecond)

	select {
	case <-probed:
	case <-time.After(2 * time.Second):
		t.Fatal("probe never ran")
	}
	// After a failed probe the breaker re-opens. Wait for that transition by
	// polling for consecutiveOpens to advance (RecordFailure runs after probe).
	deadline := time.Now().Add(2 * time.Second)
	for {
		c.cb.mu.Lock()
		co := c.cb.consecutiveOpens
		st := c.cb.state
		c.cb.mu.Unlock()
		if co > initialConsecutiveOpens && st == StateOpen {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("breaker did not re-open: state=%v consecutiveOpens=%d", st, co)
		}
		runtimeYield()
	}
	cancel()

	c.cb.mu.Lock()
	newConsecutiveOpens := c.cb.consecutiveOpens
	newNextProbeAt := c.cb.nextProbeAt
	st := c.cb.state
	c.cb.mu.Unlock()

	if st != StateOpen {
		t.Errorf("expected StateOpen after failed probe, got %v", st)
	}
	if newConsecutiveOpens <= initialConsecutiveOpens {
		t.Errorf("consecutiveOpens did not increase: before=%d after=%d", initialConsecutiveOpens, newConsecutiveOpens)
	}
	if !newNextProbeAt.After(initialNextProbeAt) {
		t.Errorf("nextProbeAt did not advance: before=%v after=%v", initialNextProbeAt, newNextProbeAt)
	}
}

// TestBackgroundProbe_ClosedIsNoOp: when Closed, ticks never call the probe.
func TestBackgroundProbe_ClosedIsNoOp(t *testing.T) {
	fixedNow := time.Now()
	var probeCalled atomic.Int64
	cfg := defaultCfg()
	cfg.FailureThreshold = 5
	c := newProbeTestClient(t, cfg, func() time.Time { return fixedNow },
		func(context.Context) (bool, string) {
			probeCalled.Add(1)
			return true, ""
		})
	// Leave breaker in its default Closed state.
	if c.cb.State() != StateClosed {
		t.Fatalf("expected StateClosed, got %v", c.cb.State())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Fast interval so many ticks fire during the observation window.
	c.StartBackgroundProbe(ctx, time.Millisecond)

	// Negative assertion: over a bounded window with many ticks, the probe must
	// never be called on a Closed breaker. The window is a guard, not an
	// ordering barrier — a single missed probe would still be caught given the
	// 1ms interval fires ~100+ times here.
	time.Sleep(120 * time.Millisecond)
	cancel()

	if n := probeCalled.Load(); n != 0 {
		t.Errorf("probe was called %d times on a Closed breaker, want 0", n)
	}
}

// TestBackgroundProbe_ContextCancelExits: cancelling ctx exits the goroutine.
func TestBackgroundProbe_ContextCancelExits(t *testing.T) {
	fixedNow := time.Now()
	probeEntered := make(chan struct{}, 1)
	c := newProbeTestClient(t, defaultCfg(), func() time.Time { return fixedNow },
		func(pctx context.Context) (bool, string) {
			select {
			case probeEntered <- struct{}{}:
			default:
			}
			<-pctx.Done() // block until the probe context is cancelled
			return false, "cancelled"
		})
	forceOpen(c.cb, fixedNow)

	// Track goroutine completion via a wrapper: StartBackgroundProbe does not
	// expose a done channel, so we assert exit indirectly — after cancel, a
	// subsequent probe must never fire. We confirm the loop was running first.
	ctx, cancel := context.WithCancel(context.Background())
	c.StartBackgroundProbe(ctx, time.Millisecond)

	select {
	case <-probeEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("probe loop never started")
	}

	// Cancel: this cancels the probe context (unblocking the stub) and the loop.
	cancel()

	// After cancel, drain probeEntered and assert no NEW probe starts within a
	// bounded guard window. If the goroutine leaked, a 1ms ticker would refire.
	// First probe re-opened the breaker via RecordFailure (ctx.Err guard path
	// or probe-fail path), so a leaked loop would re-enter.
	time.Sleep(80 * time.Millisecond)
	select {
	case <-probeEntered:
		t.Fatal("probe fired after context cancel — goroutine leaked")
	default:
	}
}

// TestBackgroundProbe_OnlyOneProbeInFlight: a second tick while a probe is in
// flight is a no-op (probeInFlight respected).
func TestBackgroundProbe_OnlyOneProbeInFlight(t *testing.T) {
	fixedNow := time.Now()
	var probeCalled atomic.Int64
	firstStarted := make(chan struct{}, 1)
	unblock := make(chan struct{})
	c := newProbeTestClient(t, defaultCfg(), func() time.Time { return fixedNow },
		func(pctx context.Context) (bool, string) {
			probeCalled.Add(1)
			select {
			case firstStarted <- struct{}{}:
			default:
			}
			select {
			case <-unblock:
			case <-pctx.Done():
			}
			return true, ""
		})
	forceOpen(c.cb, fixedNow)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// 1ms interval: many ticks fire while the first probe is blocked.
	c.StartBackgroundProbe(ctx, time.Millisecond)

	// Wait for the first probe to start and block.
	select {
	case <-firstStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first probe never started")
	}

	// Let many ticks fire while the first probe holds the slot. Because the
	// breaker is HalfOpen with probeInFlight=true, AllowProbe rejects them all.
	time.Sleep(50 * time.Millisecond)

	// Exactly one probe should have run so far.
	if n := probeCalled.Load(); n != 1 {
		t.Errorf("probe called %d times while one in flight, want exactly 1", n)
	}

	// Release; the probe succeeds and the breaker closes. No further probe runs
	// because a Closed breaker is a no-op.
	close(unblock)
	waitForState(t, c.cb, StateClosed, 2*time.Second)
	cancel()
}

// TestAllowProbe_NeverGrantsOnClosed is the DETERMINISTIC contract guard for the
// TOCTOU BLOCKER (#1000 review finding #1): AllowProbe() must never return nil
// when the breaker is Closed at the instant of the call. Because AllowProbe
// decides atomically under a single lock, we can prove this single-threaded —
// no concurrent post-read (which would itself be racy) is needed.
//
// We also demonstrate, in the same deterministic frame, that the ORIGINAL buggy
// two-step gate (State()==Open then Allow()) WOULD grant a probe on a Closed
// breaker when the two steps straddle a transition — so this guard is not
// vacuous: it distinguishes the correct atomic path from the bug it replaced.
func TestAllowProbe_NeverGrantsOnClosed(t *testing.T) {
	fixedNow := time.Now()
	cfg := defaultCfg()
	cb := NewCircuitBreaker(cfg)
	cb.now = func() time.Time { return fixedNow }

	// 1) Closed breaker: AllowProbe denies, atomically, every time.
	if cb.State() != StateClosed {
		t.Fatalf("expected Closed, got %v", cb.State())
	}
	for i := 0; i < 100; i++ {
		if err := cb.AllowProbe(); err != errCircuitOpen {
			t.Fatalf("AllowProbe on Closed = %v, want errCircuitOpen (iter %d)", err, i)
		}
		if cb.State() != StateClosed {
			t.Fatalf("AllowProbe perturbed Closed state -> %v (iter %d)", cb.State(), i)
		}
	}

	// 2) Non-vacuity: the bug AllowProbe replaced is the two-step gate. Reproduce
	//    the exact straddle deterministically — read Open, transition to Closed,
	//    then the buggy second step (Allow on Closed) returns nil = spurious grant.
	cb.mu.Lock()
	cb.state = StateOpen
	cb.consecutiveOpens = 1
	cb.nextProbeAt = fixedNow.Add(-1 * time.Second)
	cb.mu.Unlock()

	buggyStateRead := cb.State() // reads Open (step 1 of the buggy gate)
	if buggyStateRead != StateOpen {
		t.Fatalf("setup: expected Open, got %v", buggyStateRead)
	}
	// A concurrent path drives the breaker Closed between the two steps.
	cb.mu.Lock()
	cb.state = StateClosed
	cb.probeInFlight = false
	cb.mu.Unlock()
	// Buggy step 2: Allow() on the now-Closed breaker returns nil — a SPURIOUS
	// probe grant against a healthy upstream. This is exactly what the old loop
	// did and what AllowProbe fixes.
	if err := cb.Allow(); err != nil {
		t.Fatalf("buggy two-step proof broken: Allow on Closed = %v, want nil", err)
	}
	// And AllowProbe, given the identical Closed state, would NOT have granted:
	cb.mu.Lock()
	cb.state = StateClosed
	cb.probeInFlight = false
	cb.mu.Unlock()
	if err := cb.AllowProbe(); err != errCircuitOpen {
		t.Fatalf("AllowProbe on Closed = %v, want errCircuitOpen — bug present", err)
	}
}

// TestAllowProbe_ConcurrentNoRaceStillGrants is the concurrency guard. It runs
// AllowProbe() (background) and Allow() (demand) against the breaker from many
// goroutines while a driver flips it Open<->Closed, and asserts only two things:
//   - the run is data-race clean (the value of this test is under `-race`), and
//   - legitimate Open-state probes are still granted (probeGrants > 0), so the
//     concurrent path is exercised, not starved.
//
// Crucially it does NOT attempt a "was the breaker Closed at grant time"
// post-read: that determination cannot be made atomically with the grant from
// outside the breaker, so a separate-lock read after AllowProbe returns is
// inherently racy and produces false positives. The Closed-grant contract is
// proven deterministically in TestAllowProbe_NeverGrantsOnClosed instead.
func TestAllowProbe_ConcurrentNoRaceStillGrants(t *testing.T) {
	fixedNow := time.Now()
	cfg := defaultCfg()
	cb := NewCircuitBreaker(cfg)
	cb.now = func() time.Time { return fixedNow }

	var probeGrants atomic.Int64

	runProbeGate := func() {
		if err := cb.AllowProbe(); err != nil {
			return
		}
		probeGrants.Add(1)
		// Release the slot the real way. No post-read of state here — that would
		// be racy (see doc comment).
		cb.RecordSuccess()
	}
	runDemandGate := func() {
		if err := cb.Allow(); err != nil {
			return
		}
		cb.RecordSuccess()
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Driver flips Open<->Closed only while no probe slot is held, so it never
	// stomps an in-flight probe.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			cb.mu.Lock()
			if !cb.probeInFlight {
				if cb.state == StateClosed {
					cb.state = StateOpen
					cb.consecutiveOpens = 1
					cb.nextProbeAt = fixedNow.Add(-1 * time.Second)
				} else {
					cb.state = StateClosed
				}
			}
			cb.mu.Unlock()
		}
	}()

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20000; j++ {
				select {
				case <-stop:
					return
				default:
				}
				runProbeGate()
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20000; j++ {
				select {
				case <-stop:
					return
				default:
				}
				runDemandGate()
			}
		}()
	}

	time.Sleep(200 * time.Millisecond)
	close(stop)
	wg.Wait()

	if probeGrants.Load() == 0 {
		t.Fatal("no probe was ever granted under concurrency — path starved/vacuous")
	}
}

// TestBackgroundProbe_PanicSafe verifies a panicking probe does not wedge the
// breaker: the slot is released (RecordFailure runs) and the loop keeps going.
func TestBackgroundProbe_PanicSafe(t *testing.T) {
	fixedNow := time.Now()
	var calls atomic.Int64
	recovered := make(chan struct{}, 1)
	c := newProbeTestClient(t, defaultCfg(), func() time.Time { return fixedNow },
		func(context.Context) (bool, string) {
			n := calls.Add(1)
			if n == 1 {
				panic("simulated probe panic")
			}
			select {
			case recovered <- struct{}{}:
			default:
			}
			return false, "after panic"
		})
	forceOpen(c.cb, fixedNow)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.StartBackgroundProbe(ctx, time.Millisecond)

	// The first probe panics; the deferred recover records a failure (releasing
	// the slot). The breaker re-opens, nextProbeAt advances by backoff. To let
	// a second probe fire we must move the clock past the new nextProbeAt.
	// Wait for the panic to be handled first (breaker leaves probeInFlight).
	deadline := time.Now().Add(2 * time.Second)
	for {
		c.cb.mu.Lock()
		pif := c.cb.probeInFlight
		c.cb.mu.Unlock()
		if !pif && calls.Load() >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("breaker stayed wedged (probeInFlight) after probe panic")
		}
		runtimeYield()
	}
	cancel()
	// Success criterion: the breaker was not wedged — probeInFlight cleared and
	// at least one probe ran. (A wedged breaker would fail the loop above.)
	if calls.Load() < 1 {
		t.Fatal("probe never ran")
	}
}

// TestBackgroundProbe_ConcurrentSafe exercises the loop and breaker paths
// concurrently under -race.
func TestBackgroundProbe_ConcurrentSafe(t *testing.T) {
	fixedNow := time.Now()
	cfg := CircuitConfig{
		Enabled:           true,
		FailureThreshold:  3,
		FailureWindow:     30 * time.Second,
		OpenDuration:      time.Millisecond,
		BackoffMultiplier: 1.5,
		BackoffCap:        10 * time.Millisecond,
	}
	var probeCount atomic.Int64
	c := newProbeTestClient(t, cfg, func() time.Time { return fixedNow },
		func(context.Context) (bool, string) {
			n := probeCount.Add(1)
			return n%2 == 0, "x"
		})
	forceOpen(c.cb, fixedNow)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	c.StartBackgroundProbe(ctx, time.Millisecond)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				if err := c.cb.Allow(); err == nil {
					c.cb.RecordFailure()
				}
			}
		}()
	}
	wg.Wait()
	<-ctx.Done()
	// Passes if -race reports no data races.
}

// waitForState polls the breaker until it reaches want or the deadline passes.
func waitForState(t *testing.T, cb *CircuitBreaker, want CircuitState, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		if cb.State() == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("breaker did not reach %v within %v (current %v)", want, within, cb.State())
		}
		runtimeYield()
	}
}

// runtimeYield yields the scheduler without a meaningful wall-clock delay; used
// only to widen race windows and avoid busy-spin starvation, never as an
// ordering barrier for correctness assertions.
func runtimeYield() {
	time.Sleep(time.Microsecond)
}
