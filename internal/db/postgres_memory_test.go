package db_test

import (
	"context"
	"sync"
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

func TestMergeMemoriesAtomic_ConcurrentSameWinner(t *testing.T) {
	proj := uniqueProject("merge-concurrent")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	winner := storeMemory(t, b, proj, "winner-before-merge")
	loserA := storeMemory(t, b, proj, "loser-a")
	loserB := storeMemory(t, b, proj, "loser-b")

	const contentA = "winner-after-merge-a"
	const contentB = "winner-after-merge-b"

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup

	runMerge := func(loserID, content string) {
		defer wg.Done()
		<-start
		errs <- b.MergeMemoriesAtomic(ctx, proj, winner.ID, loserID, content)
	}

	wg.Add(2)
	go runMerge(loserA.ID, contentA)
	go runMerge(loserB.ID, contentB)
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}

	finalWinner, err := b.GetMemory(ctx, winner.ID)
	require.NoError(t, err)
	require.NotNil(t, finalWinner)
	require.Contains(t, []string{contentA, contentB}, finalWinner.Content)

	for _, loserID := range []string{loserA.ID, loserB.ID} {
		loser, err := b.GetMemory(ctx, loserID)
		require.NoError(t, err)
		require.Nil(t, loser, "loser %s must be deleted", loserID)
	}

	history, err := b.GetMemoryHistory(ctx, proj, winner.ID)
	require.NoError(t, err)
	require.Len(t, history, 2, "winner should capture both pre-merge states across serialized merges")

	versionedContents := map[string]bool{}
	for _, version := range history {
		versionedContents[version.Content] = true
		require.Equal(t, types.VersionChangeUpdate, version.ChangeType)
	}

	require.True(t, versionedContents[winner.Content], "original winner content must be versioned before the first merge")

	intermediate := contentA
	if finalWinner.Content == contentA {
		intermediate = contentB
	}
	require.True(t, versionedContents[intermediate], "the winner state from the earlier merge must be versioned before the later merge")
}

func TestGetMemoryByIDInProject_SameProject(t *testing.T) {
	proj := uniqueProject("get-mem-proj-same")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	mem := storeMemory(t, b, proj, "same-project scoped memory")

	retrieved, err := b.GetMemoryByIDInProject(ctx, mem.ID, proj)
	require.NoError(t, err)
	require.NotNil(t, retrieved, "GetMemoryByIDInProject must return the memory in the matching project")
	require.Equal(t, mem.ID, retrieved.ID)
	require.Equal(t, mem.Content, retrieved.Content)
}

func TestGetMemoryByIDInProject_CrossProjectRejected(t *testing.T) {
	projA := uniqueProject("get-mem-proj-cross-a")
	projB := uniqueProject("get-mem-proj-cross-b")
	bA := newTestBackend(t, projA)
	ctx := context.Background()

	memA := storeMemory(t, bA, projA, "cross-project scoped memory")

	retrieved, err := bA.GetMemoryByIDInProject(ctx, memA.ID, projB)
	require.NoError(t, err)
	require.Nil(t, retrieved, "GetMemoryByIDInProject must not return a memory from another project")
}
