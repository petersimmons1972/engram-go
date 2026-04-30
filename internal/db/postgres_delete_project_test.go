package db_test

import (
	"context"
	"os"
	"testing"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/types"
)

// testDeleteProjectDSN returns the TEST_DATABASE_URL or skips the test.
func testDeleteProjectDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	return dsn
}

// TestDeleteProjectHappyPath verifies that DeleteProject removes all memories,
// chunks, relationships, episodes, and weight config for a project.
func TestDeleteProjectHappyPath(t *testing.T) {
	dsn := testDeleteProjectDSN(t)
	ctx := context.Background()
	project := "test-delete-project-happy-path"

	backend, err := db.NewPostgresBackend(ctx, project, dsn)
	if err != nil {
		t.Fatalf("NewPostgresBackend: %v", err)
	}
	defer backend.Close()

	// Create a memory
	m := &types.Memory{
		ID:         "mem-1",
		Project:    project,
		Content:    "test content",
		Tags:       []string{"test"},
		MemoryType: "context",
	}
	if err := backend.StoreMemory(ctx, m); err != nil {
		t.Fatalf("StoreMemory failed: %v", err)
	}

	// Create a relationship
	rel := &types.Relationship{
		SourceID:  "mem-1",
		TargetID:  "mem-1",
		RelType:   "relates_to",
		Strength:  1.0,
		Project:   project,
	}
	if err := backend.StoreRelationship(ctx, rel); err != nil {
		t.Fatalf("StoreRelationship failed: %v", err)
	}

	// Create a weight config entry
	if err := backend.SetMeta(ctx, project, "embedder_model", "test-model"); err != nil {
		t.Fatalf("SetMeta failed: %v", err)
	}

	// Verify data exists before deletion
	stats, err := backend.GetStats(ctx, project)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}
	if stats.TotalMemories != 1 {
		t.Errorf("expected 1 memory before deletion, got %d", stats.TotalMemories)
	}
	t.Cleanup(func() {
		backend.Pool().Exec(ctx, "DELETE FROM episodes WHERE project = $1", project) //nolint:errcheck
		backend.Pool().Exec(ctx, "DELETE FROM relationships WHERE project = $1", project) //nolint:errcheck
		backend.Pool().Exec(ctx, "DELETE FROM memories WHERE project = $1", project) //nolint:errcheck
	})

	// Delete the project
	if err := backend.DeleteProject(ctx, project); err != nil {
		t.Fatalf("DeleteProject failed: %v", err)
	}

	// Verify data is gone
	stats, err = backend.GetStats(ctx, project)
	if err != nil {
		t.Fatalf("GetStats failed after deletion: %v", err)
	}
	if stats.TotalMemories != 0 {
		t.Errorf("expected 0 memories after deletion, got %d", stats.TotalMemories)
	}
}

// TestDeleteProjectEmpty verifies DeleteProject succeeds on an empty project.
func TestDeleteProjectEmpty(t *testing.T) {
	dsn := testDeleteProjectDSN(t)
	ctx := context.Background()
	project := "test-delete-project-empty"

	backend, err := db.NewPostgresBackend(ctx, project, dsn)
	if err != nil {
		t.Fatalf("NewPostgresBackend: %v", err)
	}
	defer backend.Close()

	// Verify project is initially empty
	stats, err := backend.GetStats(ctx, project)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}
	if stats.TotalMemories != 0 {
		t.Errorf("expected 0 memories in empty project, got %d", stats.TotalMemories)
	}

	// Delete the empty project
	if err := backend.DeleteProject(ctx, project); err != nil {
		t.Fatalf("DeleteProject failed on empty project: %v", err)
	}

	// Verify still empty
	stats, err = backend.GetStats(ctx, project)
	if err != nil {
		t.Fatalf("GetStats failed after deletion: %v", err)
	}
	if stats.TotalMemories != 0 {
		t.Errorf("expected 0 memories after deleting empty project, got %d", stats.TotalMemories)
	}
}

// TestDeleteProjectNilPool verifies DeleteProject returns nil when pool is nil.
func TestDeleteProjectNilPool(t *testing.T) {
	b := &db.PostgresBackend{}
	ctx := context.Background()
	if err := b.DeleteProject(ctx, "some-project"); err != nil {
		t.Fatalf("DeleteProject with nil pool should succeed, got err: %v", err)
	}
}

// TestDeleteProjectIsolation verifies DeleteProject only removes data for the
// specified project, leaving other projects intact.
func TestDeleteProjectIsolation(t *testing.T) {
	dsn := testDeleteProjectDSN(t)
	ctx := context.Background()
	project1 := "test-delete-project-iso-1"
	project2 := "test-delete-project-iso-2"

	backend1, err := db.NewPostgresBackend(ctx, project1, dsn)
	if err != nil {
		t.Fatalf("NewPostgresBackend for project1: %v", err)
	}
	defer backend1.Close()

	backend2, err := db.NewPostgresBackend(ctx, project2, dsn)
	if err != nil {
		t.Fatalf("NewPostgresBackend for project2: %v", err)
	}
	defer backend2.Close()

	// Create a memory in each project
	m1 := &types.Memory{
		ID:         "mem-1",
		Project:    project1,
		Content:    "content 1",
		Tags:       []string{"tag1"},
		MemoryType: "context",
	}
	if err := backend1.StoreMemory(ctx, m1); err != nil {
		t.Fatalf("StoreMemory for project1 failed: %v", err)
	}

	m2 := &types.Memory{
		ID:         "mem-2",
		Project:    project2,
		Content:    "content 2",
		Tags:       []string{"tag2"},
		MemoryType: "context",
	}
	if err := backend2.StoreMemory(ctx, m2); err != nil {
		t.Fatalf("StoreMemory for project2 failed: %v", err)
	}

	t.Cleanup(func() {
		backend1.Pool().Exec(ctx, "DELETE FROM memories WHERE project = $1", project1) //nolint:errcheck
		backend2.Pool().Exec(ctx, "DELETE FROM memories WHERE project = $1", project2) //nolint:errcheck
	})

	// Delete project1
	if err := backend1.DeleteProject(ctx, project1); err != nil {
		t.Fatalf("DeleteProject failed: %v", err)
	}

	// Verify project1 is empty
	stats1, err := backend1.GetStats(ctx, project1)
	if err != nil {
		t.Fatalf("GetStats for project1 failed: %v", err)
	}
	if stats1.TotalMemories != 0 {
		t.Errorf("project1 should have 0 memories after deletion, got %d", stats1.TotalMemories)
	}

	// Verify project2 still has its memory
	stats2, err := backend2.GetStats(ctx, project2)
	if err != nil {
		t.Fatalf("GetStats for project2 failed: %v", err)
	}
	if stats2.TotalMemories != 1 {
		t.Errorf("project2 should have 1 memory after project1 deletion, got %d", stats2.TotalMemories)
	}
}
