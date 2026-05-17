package embed

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TestCircuitBreaker_LogsTransitions — #676: every state transition produces
// a slog line so operators can correlate recall degradation with log events.
func TestCircuitBreaker_LogsTransitions(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	cb := NewCircuitBreaker(defaultTestConfig())

	// Trigger CLOSED → OPEN
	cb.transitionTo(StateOpen)
	out := buf.String()
	if !strings.Contains(out, `"msg":"embed circuit breaker OPEN`) {
		t.Errorf("missing OPEN warn line; got: %s", out)
	}
	if !strings.Contains(out, `"level":"WARN"`) {
		t.Errorf("OPEN should log at WARN level; got: %s", out)
	}
	buf.Reset()

	// OPEN → HALF_OPEN
	cb.transitionTo(StateHalfOpen)
	out = buf.String()
	if !strings.Contains(out, `"msg":"embed circuit breaker HALF_OPEN`) {
		t.Errorf("missing HALF_OPEN info line; got: %s", out)
	}
	buf.Reset()

	// HALF_OPEN → CLOSED
	cb.transitionTo(StateClosed)
	out = buf.String()
	if !strings.Contains(out, `"msg":"embed circuit breaker CLOSED`) {
		t.Errorf("missing CLOSED info line; got: %s", out)
	}
	buf.Reset()

	// Same-state transitionTo must NOT log (no state change)
	cb.transitionTo(StateClosed)
	if buf.Len() != 0 {
		t.Errorf("same-state transition should be silent; got: %s", buf.String())
	}
}

func defaultTestConfig() CircuitConfig {
	return CircuitConfig{Enabled: true}
}
