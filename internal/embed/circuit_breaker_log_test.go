package embed

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
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

// TestCircuitBreaker_OpenLogReportsFreshNextProbeAt is the regression guard for
// #1000 review finding #4: RecordFailure must compute updateNextProbeTime()
// BEFORE transitionTo(StateOpen) so the OPEN log line reports the freshly
// computed next_probe_at, not a stale/zero value.
func TestCircuitBreaker_OpenLogReportsFreshNextProbeAt(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	fixedNow := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	cfg := CircuitConfig{
		Enabled:           true,
		FailureThreshold:  1,
		FailureWindow:     30 * time.Second,
		OpenDuration:      30 * time.Second,
		BackoffMultiplier: 2.0,
		BackoffCap:        5 * time.Minute,
	}
	cb := NewCircuitBreaker(cfg)
	cb.now = func() time.Time { return fixedNow }

	// Closed -> Open via RecordFailure (the real path).
	cb.Allow()
	cb.RecordFailure()

	// Parse the OPEN log line and assert next_probe_at == now + OpenDuration,
	// i.e. the fresh value — not the zero time the stale ordering would log.
	var sawOpen bool
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		msg, _ := rec["msg"].(string)
		if !strings.HasPrefix(msg, "embed circuit breaker OPEN") {
			continue
		}
		sawOpen = true
		npa, _ := rec["next_probe_at"].(string)
		if npa == "" {
			t.Fatalf("OPEN log missing next_probe_at; line: %s", line)
		}
		got, err := time.Parse(time.RFC3339, npa)
		if err != nil {
			t.Fatalf("next_probe_at not RFC3339: %q (%v)", npa, err)
		}
		want := fixedNow.Add(cfg.OpenDuration)
		if !got.Equal(want) {
			t.Errorf("OPEN log next_probe_at = %v, want fresh %v (stale-ordering bug logs zero time)", got, want)
		}
		// Explicitly reject the stale zero-time value.
		if got.IsZero() {
			t.Error("OPEN log next_probe_at is the zero time — stale-ordering bug present")
		}
	}
	if !sawOpen {
		t.Fatalf("no OPEN log line emitted; buffer: %s", buf.String())
	}
}

func defaultTestConfig() CircuitConfig {
	return CircuitConfig{Enabled: true}
}
