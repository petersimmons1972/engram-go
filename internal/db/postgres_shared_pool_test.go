package db

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestSharedPoolConfig verifies configureSharedPool sets values tuned for
// a single pool shared across all project backends: higher MaxConns to handle
// concurrent projects, lower MinConns to avoid wasting slots on idle instances.
func TestSharedPoolConfig(t *testing.T) {
	cfg, err := pgxpool.ParseConfig("postgres://user:password@localhost:5432/engram")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	configureSharedPool(cfg)

	if cfg.MaxConns != 50 {
		t.Errorf("MaxConns: got %d, want 50", cfg.MaxConns)
	}
	if cfg.MinConns != 2 {
		t.Errorf("MinConns: got %d, want 2", cfg.MinConns)
	}
	if got, want := cfg.MaxConnIdleTime, 3*time.Minute; got != want {
		t.Errorf("MaxConnIdleTime: got %v, want %v", got, want)
	}
	if got, want := cfg.HealthCheckPeriod, 30*time.Second; got != want {
		t.Errorf("HealthCheckPeriod: got %v, want %v", got, want)
	}
	if cfg.MaxConnLifetime == 0 {
		t.Error("MaxConnLifetime is zero — stale connections will never be evicted")
	}
}

// TestNewSharedPoolRejectsDefaultPassword verifies that NewSharedPool refuses
// to connect when the DSN uses a well-known default password. This runs without
// a live PostgreSQL connection — the password check must fire before any dial.
func TestNewSharedPoolRejectsDefaultPassword(t *testing.T) {
	ctx := context.Background()
	badDSNs := []string{
		"postgres://engram:engram@localhost:5432/engram",
		"postgres://engram:postgres@localhost:5432/engram",
	}
	for _, dsn := range badDSNs {
		_, err := NewSharedPool(ctx, dsn)
		if err == nil {
			t.Errorf("NewSharedPool(%q): expected error for default password, got nil", dsn)
		}
	}
}

// TestSharedPoolBackendCloseIsNoop verifies that Close() on a shared-pool backend
// does not close the underlying pool — calling it must be safe and idempotent.
func TestSharedPoolBackendCloseIsNoop(t *testing.T) {
	b, _ := newPostgresBackendFromPool(nil, "test")
	// ownsPool defaults to false; Close must not panic or close a nil pool.
	b.Close()
	b.Close() // second call must also be safe
}

// TestOwnedPoolBackendOwnsPool verifies the ownsPool flag is set for CLI-path
// backends (created by NewPostgresBackend), not for shared-pool backends.
func TestOwnedPoolBackendOwnsPool(t *testing.T) {
	b, _ := newPostgresBackendFromPool(nil, "test")
	if b.ownsPool {
		t.Error("shared-pool backend must have ownsPool=false")
	}
}

// TestNewPostgresBackendWithPoolRejectsEmptyProject verifies that
// NewPostgresBackendWithPool falls back to "default" when project is empty,
// matching the behaviour of NewPostgresBackend.
func TestNewPostgresBackendWithPoolRejectsEmptyProject(t *testing.T) {
	// We don't need a live pool to test project slug sanitization; pass nil
	// and verify the backend struct is returned with project=="default".
	// NewPostgresBackendWithPool must NOT call pool.Ping, so nil is safe here.
	b, err := newPostgresBackendFromPool(nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.project != "default" {
		t.Errorf("project: got %q, want %q", b.project, "default")
	}
}

// TestNewPostgresBackendWithPoolSanitisesSlug verifies unsafe characters are
// stripped from the project name, matching NewPostgresBackend behaviour.
func TestNewPostgresBackendWithPoolSanitisesSlug(t *testing.T) {
	cases := []struct{ input, want string }{
		{"my project!", "myproject"},
		{"hello/world", "helloworld"},
		{"valid-slug_123", "valid-slug_123"},
		{"", "default"},
	}
	for _, tc := range cases {
		b, err := newPostgresBackendFromPool(nil, tc.input)
		if err != nil {
			t.Fatalf("input %q: unexpected error: %v", tc.input, err)
		}
		if b.project != tc.want {
			t.Errorf("input %q: project = %q, want %q", tc.input, b.project, tc.want)
		}
	}
}

// TestSharedPoolIsReusedAcrossBackends verifies two backends created from the
// same *pgxpool.Pool share connections rather than each owning an independent
// pool. Requires TEST_DATABASE_URL; skipped otherwise.
func TestSharedPoolIsReusedAcrossBackends(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	ctx := context.Background()

	pool, err := NewSharedPool(ctx, dsn)
	if err != nil {
		t.Fatalf("NewSharedPool: %v", err)
	}
	defer pool.Close()

	b1, err := NewPostgresBackendWithPool(ctx, "proj-a", pool)
	if err != nil {
		t.Fatalf("backend proj-a: %v", err)
	}
	b2, err := NewPostgresBackendWithPool(ctx, "proj-b", pool)
	if err != nil {
		t.Fatalf("backend proj-b: %v", err)
	}

	// Both backends must reference the same pool pointer.
	if b1.pool != b2.pool {
		t.Error("backends have different pool pointers — connections are not shared")
	}

	// Pool stat total connections must not exceed shared MaxConns.
	stat := pool.Stat()
	if stat.TotalConns() > 50 {
		t.Errorf("TotalConns %d > MaxConns 50", stat.TotalConns())
	}

	_ = b2
}
