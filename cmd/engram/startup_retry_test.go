package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// virtualClock is a controllable clock + sleep for unit tests. Sleep advances
// the clock by d without real wall time so MaxWait/backoff can be asserted
// deterministically.
type virtualClock struct {
	mu  sync.Mutex
	now time.Time
}

func newVirtualClock(start time.Time) *virtualClock {
	return &virtualClock{now: start}
}

func (c *virtualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *virtualClock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
	return nil
}

func testRetryConfig(clk *virtualClock, maxWait time.Duration) startupRetryConfig {
	var buf bytes.Buffer
	return startupRetryConfig{
		MaxWait:        maxWait,
		InitialBackoff: time.Second,
		MaxBackoff:     30 * time.Second,
		Sleep:          clk.Sleep,
		Now:            clk.Now,
		Log:            slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}
}

func TestRetryDependency_SucceedsFirstAttempt(t *testing.T) {
	clk := newVirtualClock(time.Unix(0, 0).UTC())
	cfg := testRetryConfig(clk, 5*time.Minute)
	var calls atomic.Int32

	err := retryDependency(context.Background(), "db", cfg, isPermanentDBStartupError, func(context.Context) error {
		calls.Add(1)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("attempts = %d, want 1", calls.Load())
	}
}

func TestRetryDependency_SucceedsAfterTransientFailures(t *testing.T) {
	clk := newVirtualClock(time.Unix(0, 0).UTC())
	cfg := testRetryConfig(clk, 5*time.Minute)
	var calls atomic.Int32
	transient := errors.New("cannot connect to PostgreSQL: connection refused")

	err := retryDependency(context.Background(), "db", cfg, isPermanentDBStartupError, func(context.Context) error {
		n := calls.Add(1)
		if n < 4 {
			return transient
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected eventual success, got: %v", err)
	}
	if calls.Load() != 4 {
		t.Fatalf("attempts = %d, want 4", calls.Load())
	}
	// 3 sleeps of 1s+2s+4s = 7s of virtual time before success.
	if elapsed := clk.Now().Sub(time.Unix(0, 0).UTC()); elapsed < 7*time.Second {
		t.Fatalf("elapsed virtual time %v, want >= 7s of backoff", elapsed)
	}
}

func TestRetryDependency_ExhaustsWindowReturnsLastError(t *testing.T) {
	clk := newVirtualClock(time.Unix(0, 0).UTC())
	// Short window: first fail, sleep 1s, second fail then remaining budget is
	// exhausted (or a later fail after backoff fills the window).
	cfg := testRetryConfig(clk, 3*time.Second)
	cfg.InitialBackoff = time.Second
	cfg.MaxBackoff = time.Second
	transient := errors.New("dial tcp: connection refused")

	err := retryDependency(context.Background(), "shared pool", cfg, isPermanentDBStartupError, func(context.Context) error {
		return transient
	})
	if err == nil {
		t.Fatal("expected error after exhausting window, got nil")
	}
	if !errors.Is(err, transient) && !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("error should wrap last attempt: %v", err)
	}
	if !strings.Contains(err.Error(), "unavailable after") {
		t.Fatalf("error should mention exhausted wait window: %v", err)
	}
	// Must not have spun unbounded — virtual clock should stop near MaxWait.
	if elapsed := clk.Now().Sub(time.Unix(0, 0).UTC()); elapsed > 10*time.Second {
		t.Fatalf("elapsed virtual time %v looks unbounded", elapsed)
	}
}

func TestRetryDependency_PermanentErrorFailsImmediately(t *testing.T) {
	clk := newVirtualClock(time.Unix(0, 0).UTC())
	cfg := testRetryConfig(clk, 5*time.Minute)
	var calls atomic.Int32
	permanent := errors.New("invalid DSN: cannot parse")

	err := retryDependency(context.Background(), "shared pool", cfg, isPermanentDBStartupError, func(context.Context) error {
		calls.Add(1)
		return permanent
	})
	if err == nil {
		t.Fatal("expected permanent error, got nil")
	}
	if calls.Load() != 1 {
		t.Fatalf("attempts = %d, want 1 (no retry on permanent error)", calls.Load())
	}
	if !strings.Contains(err.Error(), "permanent") {
		t.Fatalf("error should mark permanent config: %v", err)
	}
	// No backoff sleep should have advanced the clock.
	if !clk.Now().Equal(time.Unix(0, 0).UTC()) {
		t.Fatalf("clock advanced on permanent error: %v", clk.Now())
	}
}

func TestRetryDependency_DefaultPasswordIsPermanent(t *testing.T) {
	clk := newVirtualClock(time.Unix(0, 0).UTC())
	cfg := testRetryConfig(clk, 5*time.Minute)
	var calls atomic.Int32
	errDefault := errors.New("SECURITY: PostgreSQL is using a well-known default or unset password — set a strong POSTGRES_PASSWORD")

	err := retryDependency(context.Background(), "shared pool", cfg, isPermanentDBStartupError, func(context.Context) error {
		calls.Add(1)
		return errDefault
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls.Load() != 1 {
		t.Fatalf("attempts = %d, want 1", calls.Load())
	}
}

func TestRetryDependency_ContextCancelAborts(t *testing.T) {
	clk := newVirtualClock(time.Unix(0, 0).UTC())
	cfg := testRetryConfig(clk, 5*time.Minute)
	// Sleep that observes cancellation instead of advancing forever.
	cfg.Sleep = func(ctx context.Context, d time.Duration) error {
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32

	errCh := make(chan error, 1)
	go func() {
		errCh <- retryDependency(ctx, "db", cfg, isPermanentDBStartupError, func(context.Context) error {
			n := calls.Add(1)
			if n == 1 {
				cancel() // cancel after first failure so sleep/next loop aborts
			}
			return errors.New("connection refused")
		})
	}()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected abort error, got nil")
		}
		if !strings.Contains(err.Error(), "aborted") && !errors.Is(err, context.Canceled) {
			t.Fatalf("want cancelled/aborted error, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("retryDependency did not return after context cancel")
	}
}

func TestRetryDependency_MaxWaitZeroIsSingleAttempt(t *testing.T) {
	clk := newVirtualClock(time.Unix(0, 0).UTC())
	cfg := testRetryConfig(clk, 0)
	var calls atomic.Int32

	err := retryDependency(context.Background(), "db", cfg, isPermanentDBStartupError, func(context.Context) error {
		calls.Add(1)
		return errors.New("connection refused")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls.Load() != 1 {
		t.Fatalf("attempts = %d, want 1 (MaxWait=0 fail-fast)", calls.Load())
	}
}

func TestRetryDependency_BackoffCapsAtMax(t *testing.T) {
	clk := newVirtualClock(time.Unix(0, 0).UTC())
	cfg := testRetryConfig(clk, 20*time.Second)
	cfg.InitialBackoff = 4 * time.Second
	cfg.MaxBackoff = 5 * time.Second

	var sleeps []time.Duration
	cfg.Sleep = func(ctx context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		return clk.Sleep(ctx, d)
	}

	_ = retryDependency(context.Background(), "db", cfg, isPermanentDBStartupError, func(context.Context) error {
		return errors.New("connection refused")
	})

	if len(sleeps) < 2 {
		t.Fatalf("expected multiple sleeps, got %v", sleeps)
	}
	for i, d := range sleeps {
		if d > cfg.MaxBackoff {
			// Final sleep may be truncated to remaining budget, still ≤ MaxBackoff.
			t.Fatalf("sleep[%d]=%v exceeds MaxBackoff %v", i, d, cfg.MaxBackoff)
		}
	}
	// After first 4s sleep, next should be capped at 5s (not 8s).
	if sleeps[0] != 4*time.Second {
		t.Fatalf("first sleep = %v, want 4s", sleeps[0])
	}
	if sleeps[1] != 5*time.Second {
		t.Fatalf("second sleep = %v, want 5s (capped)", sleeps[1])
	}
}

// Adversarial: a dependency that flips success→failure mid-wait must not be
// reported as available after a later failure (we return on first success).
// Also ensure we do not treat a mid-flight timeout of the overall budget as
// success.
func TestRetryDependency_Adversarial_NoFalseSuccessOnTimeout(t *testing.T) {
	clk := newVirtualClock(time.Unix(0, 0).UTC())
	cfg := testRetryConfig(clk, time.Second)
	cfg.InitialBackoff = 500 * time.Millisecond
	cfg.MaxBackoff = 500 * time.Millisecond

	err := retryDependency(context.Background(), "db", cfg, isPermanentDBStartupError, func(ctx context.Context) error {
		// Never succeed; always refuse.
		return fmt.Errorf("cannot connect to PostgreSQL — check DATABASE_URL: %w",
			errors.New("dial tcp 192.168.0.189:5434: connect: connection refused"))
	})
	if err == nil {
		t.Fatal("ADVERSARIAL FAIL: exhausted wait returned nil success")
	}
	if strings.Contains(err.Error(), "permanent") {
		t.Fatalf("connection refused must not be classified permanent: %v", err)
	}
}

func TestIsPermanentDBStartupError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("cannot connect to PostgreSQL: connection refused"), false},
		{errors.New("shared pool: cannot connect to PostgreSQL — check DATABASE_URL: dial tcp: i/o timeout"), false},
		{errors.New("schema migration failed: deadlock detected"), false},
		{errors.New("invalid DSN: failed to parse as keyword/value"), true},
		{errors.New("SECURITY: PostgreSQL is using a well-known default or unset password — set a strong POSTGRES_PASSWORD"), true},
		{context.Canceled, false},
		{context.DeadlineExceeded, false},
	}
	for _, tc := range cases {
		got := isPermanentDBStartupError(tc.err)
		if got != tc.want {
			t.Errorf("isPermanentDBStartupError(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestDefaultStartupRetryConfig_EnvOverride(t *testing.T) {
	t.Setenv("ENGRAM_STARTUP_DEPENDENCY_WAIT", "90s")
	cfg := defaultStartupRetryConfig()
	if cfg.MaxWait != 90*time.Second {
		t.Fatalf("MaxWait = %v, want 90s from env", cfg.MaxWait)
	}
}

func TestDefaultStartupRetryConfig_Defaults(t *testing.T) {
	t.Setenv("ENGRAM_STARTUP_DEPENDENCY_WAIT", "")
	cfg := defaultStartupRetryConfig()
	if cfg.MaxWait != defaultStartupDependencyWait {
		t.Fatalf("MaxWait = %v, want %v", cfg.MaxWait, defaultStartupDependencyWait)
	}
	if cfg.InitialBackoff != defaultStartupRetryInitial {
		t.Fatalf("InitialBackoff = %v, want %v", cfg.InitialBackoff, defaultStartupRetryInitial)
	}
	if cfg.MaxBackoff != defaultStartupRetryMax {
		t.Fatalf("MaxBackoff = %v, want %v", cfg.MaxBackoff, defaultStartupRetryMax)
	}
}

// Adversarial: permanent-looking substring in a transient network error message
// must not trip fail-fast. Only the known permanent phrases qualify.
func TestIsPermanentDBStartupError_Adversarial_SubstringTrap(t *testing.T) {
	// "password" alone, or "DSN" in an unrelated message, must not be permanent.
	traps := []error{
		errors.New("password authentication failed for user engram"), // wrong password — could be rotated secret; not our permanent set
		errors.New("could not translate host name to address: Name or service not known"),
		errors.New("server closed the connection unexpectedly"),
	}
	for _, err := range traps {
		if isPermanentDBStartupError(err) {
			t.Errorf("unexpected permanent classification for %q", err)
		}
	}
	// Wrong password is NOT in isPermanentDBStartupError by design: a secret
	// rotation race can heal when Infisical re-injects. We still bound the
	// wait window so a truly wrong password exits after MaxWait rather than
	// looping forever.
}
