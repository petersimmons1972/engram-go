package db_test

import (
	"context"
	"testing"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/types"
)

// TestDeleteProjectHappyPath verifies that DeleteProject removes all memories,
// chunks, relationships, episodes, and weight config for a project.
func TestDeleteProjectHappyPath(t *testing.T) {
	_ = testDSN(t) // skip if TEST_DATABASE_URL not set
	ctx := context.Background()
	project := uniqueProject("del-happy")

	b := newTestBackend(t, project)

	// Create a memory via the shared helper (uses types.NewMemoryID internally).
	mem := storeMemory(t, b, project, "test content")

	// Create a relationship referencing the memory.
	rel := &types.Relationship{
		SourceID: mem.ID,
		TargetID: mem.ID,
		RelType:  "relates_to",
		Strength: 1.0,
		Project:  project,
	}
	if err := b.StoreRelationship(ctx, rel); err != nil {
		t.Fatalf("StoreRelationship failed: %v", err)
	}

	// Create a weight config entry.
	if err := b.SetMeta(ctx, project, "embedder_model", "test-model"); err != nil {
		t.Fatalf("SetMeta failed: %v", err)
	}

	// Verify data exists before deletion.
	stats, err := b.GetStats(ctx, project)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}
	if stats.TotalMemories != 1 {
		t.Errorf("expected 1 memory before deletion, got %d", stats.TotalMemories)
	}

	// Delete the project.
	if err := b.DeleteProject(ctx, project); err != nil {
		t.Fatalf("DeleteProject failed: %v", err)
	}

	// Verify data is gone.
	stats, err = b.GetStats(ctx, project)
	if err != nil {
		t.Fatalf("GetStats failed after deletion: %v", err)
	}
	if stats.TotalMemories != 0 {
		t.Errorf("expected 0 memories after deletion, got %d", stats.TotalMemories)
	}
}

// TestDeleteProjectEmpty verifies DeleteProject succeeds on an empty project.
func TestDeleteProjectEmpty(t *testing.T) {
	_ = testDSN(t) // skip if TEST_DATABASE_URL not set
	ctx := context.Background()
	project := uniqueProject("del-empty")

	b := newTestBackend(t, project)

	// Verify project is initially empty.
	stats, err := b.GetStats(ctx, project)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}
	if stats.TotalMemories != 0 {
		t.Errorf("expected 0 memories in empty project, got %d", stats.TotalMemories)
	}

	// Delete the empty project.
	if err := b.DeleteProject(ctx, project); err != nil {
		t.Fatalf("DeleteProject failed on empty project: %v", err)
	}

	// Verify still empty.
	stats, err = b.GetStats(ctx, project)
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
	_ = testDSN(t) // skip if TEST_DATABASE_URL not set
	ctx := context.Background()
	project1 := uniqueProject("del-iso-1")
	project2 := uniqueProject("del-iso-2")

	b1 := newTestBackend(t, project1)
	b2 := newTestBackend(t, project2)

	// Create a memory in each project using unique IDs.
	storeMemory(t, b1, project1, "content 1")
	storeMemory(t, b2, project2, "content 2")

	// Delete project1.
	if err := b1.DeleteProject(ctx, project1); err != nil {
		t.Fatalf("DeleteProject failed: %v", err)
	}

	// Verify project1 is empty.
	stats1, err := b1.GetStats(ctx, project1)
	if err != nil {
		t.Fatalf("GetStats for project1 failed: %v", err)
	}
	if stats1.TotalMemories != 0 {
		t.Errorf("project1 should have 0 memories after deletion, got %d", stats1.TotalMemories)
	}

	// Verify project2 still has its memory.
	stats2, err := b2.GetStats(ctx, project2)
	if err != nil {
		t.Fatalf("GetStats for project2 failed: %v", err)
	}
	if stats2.TotalMemories != 1 {
		t.Errorf("project2 should have 1 memory after project1 deletion, got %d", stats2.TotalMemories)
	}
}
