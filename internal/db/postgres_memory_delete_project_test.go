package db_test

// NOTE: This is an integration test requiring a running Postgres instance.
// Run with: go test ./internal/db/... -run TestDeleteProject -v
// The test is skipped automatically when ENGRAM_TEST_DSN is unset.

import (
	"context"
	"os"
	"testing"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/types"
)

func TestDeleteProject(t *testing.T) {
	dsn := os.Getenv("ENGRAM_TEST_DSN")
	if dsn == "" {
		t.Skip("ENGRAM_TEST_DSN not set")
	}
	ctx := context.Background()
	project := "test-delete-project-" + t.Name()

	b, err := db.NewPostgresBackend(ctx, project, dsn)
	if err != nil {
		t.Fatalf("NewPostgresBackend: %v", err)
	}
	defer b.Close()

	// Store two memories.
	m1 := &types.Memory{Project: project, Content: "hello world", Tags: []string{"a"}}
	m2 := &types.Memory{Project: project, Content: "second memory", Tags: []string{"b"}}
	if err := b.StoreMemory(ctx, m1); err != nil {
		t.Fatalf("StoreMemory m1: %v", err)
	}
	if err := b.StoreMemory(ctx, m2); err != nil {
		t.Fatalf("StoreMemory m2: %v", err)
	}

	// Delete the project.
	n, err := b.DeleteProject(ctx, project)
	if err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	if n != 2 {
		t.Errorf("DeleteProject returned %d, want 2", n)
	}

	// Verify memories are gone.
	got, err := b.GetMemory(ctx, m1.ID)
	if err != nil {
		t.Fatalf("GetMemory after delete: %v", err)
	}
	if got != nil {
		t.Errorf("memory %s still exists after DeleteProject", m1.ID)
	}
}

func TestDeleteProject_Empty(t *testing.T) {
	dsn := os.Getenv("ENGRAM_TEST_DSN")
	if dsn == "" {
		t.Skip("ENGRAM_TEST_DSN not set")
	}
	ctx := context.Background()
	b, err := db.NewPostgresBackend(ctx, "test-dp-empty", dsn)
	if err != nil {
		t.Fatalf("NewPostgresBackend: %v", err)
	}
	defer b.Close()

	n, err := b.DeleteProject(ctx, "nonexistent-project-xyz")
	if err != nil {
		t.Fatalf("DeleteProject on empty project: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 deletions, got %d", n)
	}
}
