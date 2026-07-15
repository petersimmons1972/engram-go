package db_test

// cross_project_isolation_test.go — regression tests for #1217.
//
// RecallEpisode and TouchMemory previously lacked a project predicate,
// allowing callers to read or mutate memories belonging to a different
// project.  These integration tests require TEST_DATABASE_URL and verify
// that both methods are now scoped to the backend's own project.

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/types"
)

func testCrossProjectDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping cross-project isolation integration test")
	}
	return dsn
}

func uniqueTestProject(prefix string) string {
	return prefix + "-" + uuid.New().String()[:8]
}

// TestRecallEpisode_CrossProjectIsolation stores a memory in project A with an
// episode_id, then opens project B and calls RecallEpisode with that same
// episode ID.  Post-fix, project B must return zero memories.
func TestRecallEpisode_CrossProjectIsolation(t *testing.T) {
	dsn := testCrossProjectDSN(t)
	ctx := context.Background()

	projA := uniqueTestProject("iso-recall-a")
	projB := uniqueTestProject("iso-recall-b")

	backendA, err := db.NewPostgresBackend(ctx, projA, dsn)
	if err != nil {
		t.Fatalf("NewPostgresBackend(projA): %v", err)
	}
	defer backendA.Close()

	backendB, err := db.NewPostgresBackend(ctx, projB, dsn)
	if err != nil {
		t.Fatalf("NewPostgresBackend(projB): %v", err)
	}
	defer backendB.Close()

	// Create an episode in project A.
	ep, err := backendA.StartEpisode(ctx, projA, "cross-project isolation episode")
	if err != nil {
		t.Fatalf("StartEpisode: %v", err)
	}

	pool := backendA.Pool()
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM memories WHERE project = $1`, projA) //nolint:errcheck
		pool.Exec(ctx, `DELETE FROM memories WHERE project = $1`, projB) //nolint:errcheck
		pool.Exec(ctx, `DELETE FROM episodes WHERE id = $1`, ep.ID)      //nolint:errcheck
	})

	// Store a memory in project A linked to the episode.
	m := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "secret content belonging to project A",
		MemoryType:  types.MemoryTypeContext,
		StorageMode: "focused",
		EpisodeID:   ep.ID,
	}
	if err := backendA.StoreMemory(ctx, m); err != nil {
		t.Fatalf("StoreMemory(projA): %v", err)
	}

	// Project A can recall its own episode memory — sanity check.
	mems, err := backendA.RecallEpisode(ctx, ep.ID)
	if err != nil {
		t.Fatalf("RecallEpisode(projA): %v", err)
	}
	if len(mems) == 0 {
		t.Fatal("expected project A to recall its own episode memory; got none")
	}

	// Project B must NOT be able to retrieve project A's memories via the episode ID.
	leaked, err := backendB.RecallEpisode(ctx, ep.ID)
	if err != nil {
		t.Fatalf("RecallEpisode(projB): %v", err)
	}
	if len(leaked) != 0 {
		t.Fatalf("cross-project data leakage: project B retrieved %d memory(ies) belonging to project A via RecallEpisode", len(leaked))
	}
}

// TestTouchMemory_CrossProjectIsolation stores a memory in project A, then
// calls TouchMemory from a project B backend using project A's memory ID.
// Post-fix, project B's touch must not affect project A's access_count.
func TestTouchMemory_CrossProjectIsolation(t *testing.T) {
	dsn := testCrossProjectDSN(t)
	ctx := context.Background()

	projA := uniqueTestProject("iso-touch-a")
	projB := uniqueTestProject("iso-touch-b")

	backendA, err := db.NewPostgresBackend(ctx, projA, dsn)
	if err != nil {
		t.Fatalf("NewPostgresBackend(projA): %v", err)
	}
	defer backendA.Close()

	backendB, err := db.NewPostgresBackend(ctx, projB, dsn)
	if err != nil {
		t.Fatalf("NewPostgresBackend(projB): %v", err)
	}
	defer backendB.Close()

	pool := backendA.Pool()
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM memories WHERE project = $1`, projA) //nolint:errcheck
		pool.Exec(ctx, `DELETE FROM memories WHERE project = $1`, projB) //nolint:errcheck
	})

	// Store a memory in project A.
	m := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "project A private memory",
		MemoryType:  types.MemoryTypeContext,
		StorageMode: "focused",
	}
	if err := backendA.StoreMemory(ctx, m); err != nil {
		t.Fatalf("StoreMemory(projA): %v", err)
	}

	// Read the initial access_count.
	var initialCount int
	err = pool.QueryRow(ctx, `SELECT access_count FROM memories WHERE id = $1`, m.ID).Scan(&initialCount)
	if err != nil {
		t.Fatalf("reading initial access_count: %v", err)
	}

	// Project B attempts to touch project A's memory ID.
	if err := backendB.TouchMemory(ctx, m.ID); err != nil {
		// A no-op (0 rows affected) is correct; an error is also acceptable.
		// We only care that the count was not incremented.
		_ = err
	}

	// The access_count for project A's memory must be unchanged.
	var countAfter int
	err = pool.QueryRow(ctx, `SELECT access_count FROM memories WHERE id = $1`, m.ID).Scan(&countAfter)
	if err != nil {
		t.Fatalf("reading access_count after cross-project touch: %v", err)
	}

	if countAfter != initialCount {
		t.Fatalf("cross-project touch mutated project A memory: access_count changed from %d to %d", initialCount, countAfter)
	}

	// Sanity: touching via project A's own backend DOES increment the count.
	if err := backendA.TouchMemory(ctx, m.ID); err != nil {
		t.Fatalf("TouchMemory(projA): %v", err)
	}
	var countAfterOwn int
	err = pool.QueryRow(ctx, `SELECT access_count FROM memories WHERE id = $1`, m.ID).Scan(&countAfterOwn)
	if err != nil {
		t.Fatalf("reading access_count after own-project touch: %v", err)
	}
	expectedAfterOwn := initialCount + 1
	// Allow for up to 1 second of last_accessed staleness but require count match.
	_ = time.Second // imported for cleanup only
	if countAfterOwn != expectedAfterOwn {
		t.Fatalf("expected access_count=%d after own-project touch; got %d", expectedAfterOwn, countAfterOwn)
	}
}
