package db_test

import (
	"context"
	"testing"

	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// TestGetMemoryByID_FoundInSameProject verifies that GetMemoryByID retrieves
// a memory from the same project without error.
func TestGetMemoryByID_FoundInSameProject(t *testing.T) {
	proj := uniqueProject("get-mem-same")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	// Store a memory in the project.
	mem := storeMemory(t, b, proj, "test memory content")

	// Retrieve it by ID.
	retrieved, err := b.GetMemoryByID(ctx, mem.ID)
	require.NoError(t, err)
	require.NotNil(t, retrieved, "GetMemoryByID must return the memory, not nil")
	require.Equal(t, mem.ID, retrieved.ID)
	require.Equal(t, mem.Content, retrieved.Content)
}

// TestGetMemoryByID_FoundCrossProject verifies that GetMemoryByID retrieves
// a memory from a different project. This is the load-bearing assertion for
// issue #430 — GetMemoryByID must NOT filter by project.
func TestGetMemoryByID_FoundCrossProject(t *testing.T) {
	projA := uniqueProject("get-mem-cross-a")
	projB := uniqueProject("get-mem-cross-b")
	bA := newTestBackend(t, projA)
	bB := newTestBackend(t, projB)
	ctx := context.Background()

	// Store a memory in projA.
	memA := storeMemory(t, bA, projA, "cross-project memory")

	// Retrieve it from projB's backend.
	// GetMemoryByID must return the memory despite the backend belonging to projB.
	retrieved, err := bB.GetMemoryByID(ctx, memA.ID)
	require.NoError(t, err)
	require.NotNil(t, retrieved, "GetMemoryByID must return memory from other project")
	require.Equal(t, memA.ID, retrieved.ID)
	require.Equal(t, memA.Content, retrieved.Content)
}

// TestGetMemoryByID_NotFound verifies that GetMemoryByID returns nil, nil
// for a memory ID that does not exist.
func TestGetMemoryByID_NotFound(t *testing.T) {
	proj := uniqueProject("get-mem-notfound")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	// Try to retrieve a nonexistent ID.
	retrieved, err := b.GetMemoryByID(ctx, types.NewMemoryID())
	require.NoError(t, err)
	require.Nil(t, retrieved, "GetMemoryByID must return nil for nonexistent IDs")
}
