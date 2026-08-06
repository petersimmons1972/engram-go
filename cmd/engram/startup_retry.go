package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/petersimmons1972/engram/internal/db"
)

// Defaults for wait-for-dependency at process start (#1423).
//
// Max wait is sized to the deploy startupProbe window
// (failureThreshold 30 × periodSeconds 10 = 300s) so Kubernetes does not
// CrashLoopBackOff while Postgres (or secret-injected env) is still
// recovering from an infra reboot. Operators can override via
// ENGRAM_STARTUP_DEPENDENCY_WAIT.
const (
	defaultStartupDependencyWait = 5 * time.Minute
	defaultStartupRetryInitial   = 1 * time.Second
	defaultStartupRetryMax       = 30 * time.Second
)

// startupRetryConfig controls bounded exponential backoff while waiting for
// hard startup dependencies (Postgres shared pool, optional gateway pool).
type startupRetryConfig struct {
	MaxWait        time.Duration
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	// Sleep waits for d or until ctx is cancelled. Production uses sleepContext;
	// tests inject a no-op / virtual clock.
	Sleep func(ctx context.Context, d time.Duration) error
	// Now returns the current time. Production uses time.Now; tests inject a
	// controllable clock so MaxWait can be exercised without real wall time.
	Now func() time.Time
	Log *slog.Logger
}

func defaultStartupRetryConfig() startupRetryConfig {
	return startupRetryConfig{
		MaxWait:        envDuration("ENGRAM_STARTUP_DEPENDENCY_WAIT", defaultStartupDependencyWait),
		InitialBackoff: defaultStartupRetryInitial,
		MaxBackoff:     defaultStartupRetryMax,
		Sleep:          sleepContext,
		Now:            time.Now,
		Log:            slog.Default(),
	}
}

// sleepContext waits for d, returning ctx.Err() if the context is cancelled first.
func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// isPermanentDBStartupError reports config/security errors that will not heal
// by waiting (invalid DSN, well-known default password). Transient network /
// "connection refused" / migration races are NOT permanent — those are retried.
func isPermanentDBStartupError(err error) bool {
	if err == nil {
		return false
	}
	// Parent cancellation is handled by the retry loop's ctx check; treat as
	// non-permanent so we surface the context error rather than "permanent".
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "invalid DSN"):
		return true
	case strings.Contains(msg, "well-known default or unset password"):
		return true
	default:
		return false
	}
}

// retryDependency repeatedly calls attempt until it succeeds, returns a
// permanent error, the parent context is cancelled, or MaxWait elapses.
//
// On transient failures it logs at WARN (never fatal) with attempt number and
// next backoff, then sleeps with capped exponential backoff. Exhausting the
// window returns the last attempt error wrapped with the dependency name —
// callers still exit nonzero; only the crash-loop path is removed (#1423).
func retryDependency(
	ctx context.Context,
	name string,
	cfg startupRetryConfig,
	isPermanent func(error) bool,
	attempt func(context.Context) error,
) error {
	if cfg.Sleep == nil {
		cfg.Sleep = sleepContext
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.InitialBackoff <= 0 {
		cfg.InitialBackoff = defaultStartupRetryInitial
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = defaultStartupRetryMax
	}
	if cfg.MaxWait <= 0 {
		// MaxWait <= 0 means a single attempt (fail-fast). Useful in tests and
		// as an explicit operator escape hatch.
		return attempt(ctx)
	}

	start := cfg.Now()
	deadline := start.Add(cfg.MaxWait)
	backoff := cfg.InitialBackoff
	var lastErr error

	for attemptN := 1; ; attemptN++ {
		if err := ctx.Err(); err != nil {
			if lastErr != nil {
				return fmt.Errorf("%s: aborted while waiting (%v); last error: %w", name, err, lastErr)
			}
			return fmt.Errorf("%s: aborted while waiting: %w", name, err)
		}

		// Bound each attempt by the remaining overall wait so a hung Ping
		// cannot overrun MaxWait.
		remaining := deadline.Sub(cfg.Now())
		if remaining <= 0 {
			break
		}
		attemptCtx, cancel := context.WithTimeout(ctx, remaining)
		err := attempt(attemptCtx)
		cancel()
		if err == nil {
			if attemptN > 1 {
				cfg.Log.Info("startup dependency available",
					"dependency", name,
					"attempts", attemptN,
					"elapsed", cfg.Now().Sub(start).Round(time.Millisecond).String(),
				)
			}
			return nil
		}
		lastErr = err

		if isPermanent != nil && isPermanent(err) {
			return fmt.Errorf("%s: permanent configuration error (not retrying): %w", name, err)
		}

		// Context cancelled during the attempt — do not retry.
		if errors.Is(err, context.Canceled) && ctx.Err() != nil {
			return fmt.Errorf("%s: aborted while waiting: %w", name, ctx.Err())
		}

		remaining = deadline.Sub(cfg.Now())
		if remaining <= 0 {
			break
		}

		sleepFor := backoff
		if sleepFor > remaining {
			sleepFor = remaining
		}
		cfg.Log.Warn("startup dependency unavailable — retrying",
			"dependency", name,
			"attempt", attemptN,
			"err", err,
			"retry_in", sleepFor.String(),
			"elapsed", cfg.Now().Sub(start).Round(time.Millisecond).String(),
			"max_wait", cfg.MaxWait.String(),
		)
		if sleepErr := cfg.Sleep(ctx, sleepFor); sleepErr != nil {
			return fmt.Errorf("%s: aborted while waiting (%v); last error: %w", name, sleepErr, lastErr)
		}

		// Capped exponential backoff: 1s, 2s, 4s, … up to MaxBackoff.
		next := backoff * 2
		if next > cfg.MaxBackoff || next <= 0 {
			next = cfg.MaxBackoff
		}
		backoff = next
	}

	if lastErr == nil {
		return fmt.Errorf("%s: unavailable after %s", name, cfg.MaxWait)
	}
	return fmt.Errorf("%s: unavailable after %s: %w", name, cfg.MaxWait, lastErr)
}

// openSharedPoolWithRetry creates the process-wide Postgres pool, retrying
// transient connect/migration failures for the configured wait window (#1423).
func openSharedPoolWithRetry(ctx context.Context, dsn string, cfg startupRetryConfig) (*pgxpool.Pool, error) {
	var pool *pgxpool.Pool
	err := retryDependency(ctx, "shared pool", cfg, isPermanentDBStartupError, func(attemptCtx context.Context) error {
		p, err := db.NewSharedPool(attemptCtx, dsn)
		if err != nil {
			return err
		}
		pool = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return pool, nil
}

// openGatewayPoolWithRetry creates the embed-gateway pool with the same
// wait-for-dependency policy as the shared pool (#1423).
func openGatewayPoolWithRetry(ctx context.Context, dsn string, cfg startupRetryConfig) (*pgxpool.Pool, error) {
	var pool *pgxpool.Pool
	err := retryDependency(ctx, "embed gateway pool", cfg, isPermanentDBStartupError, func(attemptCtx context.Context) error {
		p, err := db.NewGatewayPool(attemptCtx, dsn)
		if err != nil {
			return err
		}
		pool = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return pool, nil
}
