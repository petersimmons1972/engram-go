package embed

import (
	"errors"
	"log/slog"
	"sync"
	"time"
)

// CircuitState represents the current state of the circuit breaker.
type CircuitState int

const (
	StateClosed CircuitState = iota
	StateOpen
	StateHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}

var errCircuitOpen = errors.New("circuit breaker is open")

// errProbeDisabled is returned by AllowProbe when circuit breaking is
// disabled (cfg.Enabled == false). It is distinct from errCircuitOpen so the
// "feature off" case is not conflated with "circuit currently open". The
// background-probe loop discards the specific error; this is for clarity.
var errProbeDisabled = errors.New("circuit breaker probing disabled")

// CircuitConfig holds configuration for the circuit breaker.
type CircuitConfig struct {
	Enabled           bool
	FailureThreshold  int
	FailureWindow     time.Duration
	OpenDuration      time.Duration
	BackoffMultiplier float64
	BackoffCap        time.Duration
}

// CircuitBreaker implements the circuit breaker pattern with three states:
// Closed (normal operation), Open (short-circuit), and HalfOpen (testing recovery).
type CircuitBreaker struct {
	mu               sync.Mutex
	state            CircuitState
	failures         []time.Time // sliding window of failure timestamps
	openedAt         time.Time
	nextProbeAt      time.Time
	consecutiveOpens int
	cfg              CircuitConfig
	probeInFlight    bool
	onStateChange    func(from, to CircuitState) // callback for transitions
	now              func() time.Time            // injectable time for testing
}

// NewCircuitBreaker creates a new circuit breaker with the given config.
func NewCircuitBreaker(cfg CircuitConfig) *CircuitBreaker {
	return &CircuitBreaker{
		state:         StateClosed,
		failures:      make([]time.Time, 0),
		cfg:           cfg,
		probeInFlight: false,
		now:           time.Now,
	}
}

// Allow checks if a request is allowed. Returns errCircuitOpen if the breaker
// is open or if a probe is already in flight during half-open state.
func (cb *CircuitBreaker) Allow() error {
	if !cb.cfg.Enabled {
		return nil
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := cb.now()

	switch cb.state {
	case StateClosed:
		return nil

	case StateOpen:
		// Check if we should transition to half-open
		if now.After(cb.nextProbeAt) {
			cb.transitionTo(StateHalfOpen)
			cb.probeInFlight = true
			return nil
		}
		return errCircuitOpen

	case StateHalfOpen:
		// Only allow one probe at a time
		if cb.probeInFlight {
			return errCircuitOpen
		}
		cb.probeInFlight = true
		return nil
	}

	return errCircuitOpen
}

// AllowProbe is the background-recovery counterpart to Allow. It atomically
// decides whether the caller may run a recovery probe, returning nil only when
// the circuit is degraded and a probe slot is available:
//
//   - Closed: returns errCircuitOpen — a healthy circuit needs no background
//     probe (this is what makes a Closed-state tick a no-op).
//   - Open and past nextProbeAt: transitions to HalfOpen, claims the probe
//     slot, returns nil.
//   - Open within the backoff window: returns errCircuitOpen.
//   - HalfOpen with no probe in flight: claims the probe slot, returns nil.
//   - HalfOpen with a probe already in flight: returns errCircuitOpen.
//
// Unlike Allow, AllowProbe never returns nil for a Closed circuit. This single
// lock acquisition replaces the racy State()+Allow() two-step in the background
// probe loop, eliminating the TOCTOU window where a demand-path Allow could
// interleave and trigger a second concurrent probe.
func (cb *CircuitBreaker) AllowProbe() error {
	if !cb.cfg.Enabled {
		return errProbeDisabled
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := cb.now()

	switch cb.state {
	case StateClosed:
		// Healthy: no background probe needed.
		return errCircuitOpen

	case StateOpen:
		if now.After(cb.nextProbeAt) {
			cb.transitionTo(StateHalfOpen)
			cb.probeInFlight = true
			return nil
		}
		return errCircuitOpen

	case StateHalfOpen:
		if cb.probeInFlight {
			return errCircuitOpen
		}
		cb.probeInFlight = true
		return nil
	}

	return errCircuitOpen
}

// RecordSuccess records a successful request. Transitions to Closed if in HalfOpen.
func (cb *CircuitBreaker) RecordSuccess() {
	if !cb.cfg.Enabled {
		return
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.probeInFlight = false

	if cb.state == StateHalfOpen {
		// Probe succeeded; transition back to Closed
		cb.transitionTo(StateClosed)
		cb.failures = make([]time.Time, 0)
		cb.consecutiveOpens = 0
	}
}

// RecordFailure records a failed request. May transition to Open.
func (cb *CircuitBreaker) RecordFailure() {
	if !cb.cfg.Enabled {
		return
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := cb.now()
	cb.probeInFlight = false

	// Prune failures older than the window
	cutoff := now.Add(-cb.cfg.FailureWindow)
	validFailures := make([]time.Time, 0, len(cb.failures))
	for _, t := range cb.failures {
		if t.After(cutoff) {
			validFailures = append(validFailures, t)
		}
	}
	cb.failures = validFailures

	// Add the new failure
	cb.failures = append(cb.failures, now)

	// Check if threshold is met
	if len(cb.failures) >= cb.cfg.FailureThreshold {
		switch cb.state {
		case StateClosed:
			// Compute backoff state BEFORE transitionTo so the OPEN log line
			// reports the freshly-computed next_probe_at rather than a stale
			// (here: zero) value (#1000 review).
			cb.openedAt = now
			cb.consecutiveOpens = 1
			cb.updateNextProbeTime()
			cb.transitionTo(StateOpen)

		case StateHalfOpen:
			// Probe failed; reopen with exponential backoff. Compute backoff
			// state BEFORE transitionTo so the OPEN log reports the fresh
			// next_probe_at (#1000 review).
			cb.openedAt = now
			cb.consecutiveOpens++
			cb.updateNextProbeTime()
			cb.transitionTo(StateOpen)
			cb.failures = make([]time.Time, 0) // reset window
		}
	}
}

// State returns the current state of the circuit breaker.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// transitionTo changes state and calls the onStateChange callback if set.
//
// #676: every state transition is logged at slog.Warn (OPEN) or slog.Info
// (HALF_OPEN, CLOSED) so operators can see in the log stream when the
// embed circuit toggled — previously transitions were Prometheus-only and
// silent recall degradation looked indistinguishable from normal operation.
func (cb *CircuitBreaker) transitionTo(newState CircuitState) {
	oldState := cb.state
	cb.state = newState
	if oldState != newState {
		switch newState {
		case StateOpen:
			slog.Warn("embed circuit breaker OPEN — recall will degrade to BM25 until cooldown",
				"from", oldState.String(),
				"consecutive_opens", cb.consecutiveOpens,
				"next_probe_at", cb.nextProbeAt.Format(time.RFC3339))
		case StateHalfOpen:
			slog.Info("embed circuit breaker HALF_OPEN — probing upstream",
				"from", oldState.String())
		case StateClosed:
			slog.Info("embed circuit breaker CLOSED — upstream recovered",
				"from", oldState.String())
		}
	}
	if cb.onStateChange != nil {
		// Call outside the lock to avoid deadlock
		go cb.onStateChange(oldState, newState)
	}
}

// updateNextProbeTime calculates the next probe time with exponential backoff.
func (cb *CircuitBreaker) updateNextProbeTime() {
	now := cb.now()

	// base * multiplier^(consecutiveOpens-1), capped at BackoffCap
	baseDuration := cb.cfg.OpenDuration
	multiplier := cb.cfg.BackoffMultiplier

	cooldown := baseDuration
	for i := 1; i < cb.consecutiveOpens; i++ {
		cooldown = time.Duration(float64(cooldown) * multiplier)
		if cooldown > cb.cfg.BackoffCap {
			cooldown = cb.cfg.BackoffCap
			break
		}
	}

	cb.nextProbeAt = now.Add(cooldown)
}

// AbandonProbe releases a probe slot that was granted by Allow() or AllowProbe()
// but never resolved — because the caller's context was cancelled before any
// network I/O was attempted. This is a NEUTRAL release: it clears probeInFlight
// and returns the state from HalfOpen to Open WITHOUT incrementing
// consecutiveOpens or advancing nextProbeAt.
//
// Rationale (ADV.1, #1003): a client-side context cancellation is not evidence
// the backend is unhealthy. Calling RecordFailure on cancel would spuriously
// inflate consecutiveOpens and extend the exponential backoff window — during
// a load-shedding or graceful-drain event this could keep the breaker stuck
// Open on a healthy upstream indefinitely. AbandonProbe returns the breaker to
// exactly the Open state it was in before the probe slot was claimed, allowing
// the background-probe goroutine (#1000) to conduct the next real health check
// on its own schedule.
//
// No-op when circuit breaking is disabled (cfg.Enabled == false).
func (cb *CircuitBreaker) AbandonProbe() {
	if !cb.cfg.Enabled {
		return
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.probeInFlight = false
	if cb.state == StateHalfOpen {
		// Return to Open without modifying consecutiveOpens or nextProbeAt.
		// transitionTo is intentionally NOT used here: we do not want the
		// state-change log line ("HALF_OPEN → OPEN") for an abandoned probe —
		// it would be misleading to operators (no backend decision was made).
		cb.state = StateOpen
	}
}
