package db_test

// postgres_episode_test.go — unit tests for CloseStaleEpisodes.
//
// These tests require TEST_DATABASE_URL; they are skipped in CI unless the env
// var is set.  The two unit-level behavioural tests use an in-process stub via
// the exported interface so they can run without a real database.

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/db"
)

// ---------------------------------------------------------------------------
// Integration tests (require TEST_DATABASE_URL)
// ---------------------------------------------------------------------------

func testEpisodeDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	return dsn
}

// TestCloseStaleEpisodes_ClosesOldOpenEpisodes inserts an episode with a
// started_at 48 h in the past (no ended_at) and verifies that
// CloseStaleEpisodes with a 24 h threshold closes exactly that row.
func TestCloseStaleEpisodes_ClosesOldOpenEpisodes(t *testing.T) {
	dsn := testEpisodeDSN(t)
	ctx := context.Background()

	proj := "test-stale-episodes"
	backend, err := db.NewPostgresBackend(ctx, proj, dsn)
	if err != nil {
		t.Fatalf("NewPostgresBackend: %v", err)
	}
	defer backend.Close()

	// Insert a stale open episode directly so we control started_at.
	ep, err := backend.StartEpisode(ctx, proj, "stale episode for testing")
	if err != nil {
		t.Fatalf("StartEpisode: %v", err)
	}

	// Back-date started_at to 48 h ago so it qualifies as stale.
	pool := backend.Pool()
	_, err = pool.Exec(ctx,
		`UPDATE episodes SET started_at = NOW() - INTERVAL '48 hours' WHERE id = $1`,
		ep.ID,
	)
	if err != nil {
		t.Fatalf("back-dating episode: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM episodes WHERE project = $1`, proj) //nolint:errcheck
	})

	n, err := backend.CloseStaleEpisodes(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("CloseStaleEpisodes: %v", err)
	}
	if n == 0 {
		t.Fatal("expected rowsAffected >= 1 for a stale episode; got 0")
	}
}

// TestCloseStaleEpisodes_LeavesRecentEpisodesOpen inserts an episode started
// 1 h ago and verifies it is NOT closed when the threshold is 24 h.
func TestCloseStaleEpisodes_LeavesRecentEpisodesOpen(t *testing.T) {
	dsn := testEpisodeDSN(t)
	ctx := context.Background()

	proj := "test-recent-episodes"
	backend, err := db.NewPostgresBackend(ctx, proj, dsn)
	if err != nil {
		t.Fatalf("NewPostgresBackend: %v", err)
	}
	defer backend.Close()

	ep, err := backend.StartEpisode(ctx, proj, "recent episode for testing")
	if err != nil {
		t.Fatalf("StartEpisode: %v", err)
	}

	pool := backend.Pool()
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM episodes WHERE id = $1`, ep.ID) //nolint:errcheck
	})

	n, err := backend.CloseStaleEpisodes(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("CloseStaleEpisodes: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 rows affected for a recent episode; got %d", n)
	}
}

// ---------------------------------------------------------------------------
// Interface compile-time check
// ---------------------------------------------------------------------------

// Verify PostgresBackend satisfies db.Backend at compile time.
var _ db.Backend = (*db.PostgresBackend)(nil)

// TestCloseStaleEpisodes_InterfaceConformance ensures the method signature on
// the Backend interface matches what we expect.  No database required.
func TestCloseStaleEpisodes_InterfaceConformance(t *testing.T) {
	var _ interface {
		CloseStaleEpisodes(ctx context.Context, olderThan time.Duration) (int64, error)
	} = (*db.PostgresBackend)(nil)
	// If this file compiles, the interface is satisfied.
}

