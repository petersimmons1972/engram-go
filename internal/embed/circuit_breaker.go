package embed

import (
	"errors"
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
	mu                sync.Mutex
	state             CircuitState
	failures          []time.Time // sliding window of failure timestamps
	openedAt          time.Time
	nextProbeAt       time.Time
	consecutiveOpens  int
	cfg               CircuitConfig
	probeInFlight     bool
	onStateChange     func(from, to CircuitState) // callback for transitions
	now               func() time.Time             // injectable time for testing
}

// NewCircuitBreaker creates a new circuit breaker with the given config.
func NewCircuitBreaker(cfg CircuitConfig) *CircuitBreaker {
	return &CircuitBreaker{
		state:        StateClosed,
		failures:     make([]time.Time, 0),
		cfg:          cfg,
		probeInFlight: false,
		now:          time.Now,
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
			// Transition to Open
			cb.transitionTo(StateOpen)
			cb.openedAt = now
			cb.consecutiveOpens = 1
			cb.updateNextProbeTime()

		case StateHalfOpen:
			// Probe failed; reopen with exponential backoff
			cb.transitionTo(StateOpen)
			cb.openedAt = now
			cb.consecutiveOpens++
			cb.updateNextProbeTime()
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
func (cb *CircuitBreaker) transitionTo(newState CircuitState) {
	oldState := cb.state
	cb.state = newState
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
