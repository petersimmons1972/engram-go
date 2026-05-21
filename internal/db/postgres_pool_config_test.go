package db

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPoolConfig_HasLifetimeSettings verifies that configurePool sets all
// connection lifetime fields to non-zero values. This guards against regressions
// where stale connections survive PostgreSQL restarts or network flaps because
// MaxConnLifetime, MaxConnIdleTime, or HealthCheckPeriod are zero (which disables
// them in pgxpool).
//
// configurePool is used by CLI tools (cmd/reembed-worker, cmd/engram-setup) that
// create a single project-scoped pool. The server uses configureSharedPool instead.
func TestPoolConfig_HasLifetimeSettings(t *testing.T) {
	cfg, err := pgxpool.ParseConfig("postgres://user:password@localhost:5432/engram")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	configurePool(cfg)

	if cfg.MaxConns <= 0 {
		t.Errorf("MaxConns: got %d, want > 0", cfg.MaxConns)
	}
	if cfg.MaxConnLifetime == 0 {
		t.Error("MaxConnLifetime is zero — stale connections will never be evicted")
	}
	if cfg.MaxConnIdleTime == 0 {
		t.Error("MaxConnIdleTime is zero — idle connections will never be reaped")
	}
	if cfg.HealthCheckPeriod == 0 {
		t.Error("HealthCheckPeriod is zero — dead connections will not be detected proactively")
	}
}

// TestPoolConfig_LifetimeValues verifies the exact durations for the CLI tool
// pool (configurePool): MaxConnLifetime=30m, MaxConnIdleTime=5m, HealthCheckPeriod=1m.
func TestPoolConfig_LifetimeValues(t *testing.T) {
	cfg, err := pgxpool.ParseConfig("postgres://user:password@localhost:5432/engram")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	configurePool(cfg)

	if got, want := cfg.MaxConnLifetime, 30*time.Minute; got != want {
		t.Errorf("MaxConnLifetime: got %v, want %v", got, want)
	}
	if got, want := cfg.MaxConnIdleTime, 5*time.Minute; got != want {
		t.Errorf("MaxConnIdleTime: got %v, want %v", got, want)
	}
	if got, want := cfg.HealthCheckPeriod, 1*time.Minute; got != want {
		t.Errorf("HealthCheckPeriod: got %v, want %v", got, want)
	}
}

// TestSharedPoolConfig_HasLifetimeSettings verifies that configureSharedPool
// sets all connection lifetime fields to non-zero values. The shared pool is
// used by the server process (including the GlobalReembedder); a zero
// HealthCheckPeriod would disable proactive detection of dead connections after
// a Postgres restart (#645).
func TestSharedPoolConfig_HasLifetimeSettings(t *testing.T) {
	cfg, err := pgxpool.ParseConfig("postgres://user:password@localhost:5432/engram")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	configureSharedPool(cfg)

	if cfg.MaxConns <= 0 {
		t.Errorf("MaxConns: got %d, want > 0", cfg.MaxConns)
	}
	if cfg.MaxConnLifetime == 0 {
		t.Error("MaxConnLifetime is zero — stale connections will never be evicted")
	}
	if cfg.MaxConnIdleTime == 0 {
		t.Error("MaxConnIdleTime is zero — idle connections will never be reaped")
	}
	if cfg.HealthCheckPeriod == 0 {
		t.Error("HealthCheckPeriod is zero — dead connections will not be detected proactively")
	}
}

// TestSharedPoolConfig_HealthCheckPeriod verifies that HealthCheckPeriod is at
// most 15 seconds for the shared pool. A shorter period means stale connections
// (from a Postgres restart) are detected within one GlobalReembedder poll
// interval, preventing the silent-hang described in #645.
func TestSharedPoolConfig_HealthCheckPeriod(t *testing.T) {
	cfg, err := pgxpool.ParseConfig("postgres://user:password@localhost:5432/engram")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	configureSharedPool(cfg)

	const maxAllowed = 15 * time.Second
	if cfg.HealthCheckPeriod > maxAllowed {
		t.Errorf("HealthCheckPeriod: got %v, want ≤ %v — too long to detect post-restart stale connections promptly (#645)",
			cfg.HealthCheckPeriod, maxAllowed)
	}
}
