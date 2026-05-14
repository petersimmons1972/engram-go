package search_test

import (
	"context"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTemporalVersioning_SoftDelete verifies that Forget soft-deletes a memory
// (sets valid_to), preserves it in GetMemoryHistory, and excludes it from recall.
func TestTemporalVersioning_SoftDelete(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-softdelete"))
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	// Store a memory.
	m := &types.Memory{
		Content:     "The auth service uses JWT tokens",
		MemoryType:  types.MemoryTypeDecision,
		Project:     engine.Project(),
		Tags:        []string{"auth", "jwt"},
		Importance:  2,
		StorageMode: "focused",
	}
	require.NoError(t, engine.Store(ctx, m))
	require.NotEmpty(t, m.ID)

	// Confirm it is retrievable before deletion.
	fetched, err := engine.Backend().GetMemory(ctx, m.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, m.ID, fetched.ID)

	// Soft-delete it.
	deleted, err := engine.Forget(ctx, m.ID, "superseded by new auth design")
	require.NoError(t, err)
	assert.True(t, deleted)

	// GetMemory must return nil — active-filter excludes soft-deleted records.
	afterDelete, err := engine.Backend().GetMemory(ctx, m.ID)
	require.NoError(t, err)
	assert.Nil(t, afterDelete, "GetMemory must exclude soft-deleted memories from active results")

	// GetMemoryHistory must contain the invalidation snapshot with the correct reason.
	history, err := engine.MemoryHistory(ctx, m.ID)
	require.NoError(t, err)
	require.NotEmpty(t, history, "GetMemoryHistory must return at least one version entry")
	assert.Equal(t, types.VersionChangeInvalidate, history[0].ChangeType)
	require.NotNil(t, history[0].ChangeReason)
	assert.Equal(t, "superseded by new auth design", *history[0].ChangeReason)

	// Second forget on the same ID should return false (already invalidated).
	deleted2, err := engine.Forget(ctx, m.ID, "duplicate")
	require.NoError(t, err)
	assert.False(t, deleted2, "soft-deleting an already-invalidated memory must return false")
}

// TestTemporalVersioning_History verifies that GetMemoryHistory returns version
// snapshots created by both UpdateMemory (change_type=update) and soft-delete
// (change_type=invalidate).
func TestTemporalVersioning_History(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-history"))
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	m := &types.Memory{
		Content:     "We deploy on Fridays",
		MemoryType:  types.MemoryTypeDecision,
		Project:     engine.Project(),
		Tags:        []string{"deploy"},
		Importance:  1,
		StorageMode: "focused",
	}
	require.NoError(t, engine.Store(ctx, m))

	// First update — should create a version snapshot.
	newContent := "We no longer deploy on Fridays"
	updated, err := engine.Correct(ctx, m.ID, &newContent, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, updated)

	// Second update.
	newContent2 := "Deployments happen Monday through Thursday only"
	updated2, err := engine.Correct(ctx, m.ID, &newContent2, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, updated2)

	// Soft-delete — should create an invalidate snapshot.
	_, err = engine.Forget(ctx, m.ID, "policy changed")
	require.NoError(t, err)

	// Check history.
	history, err := engine.MemoryHistory(ctx, m.ID)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(history), 3, "expected at least 3 version entries (2 updates + 1 invalidate)")

	// Most recent entry should be the invalidation.
	assert.Equal(t, types.VersionChangeInvalidate, history[0].ChangeType)
	// Second most recent should be an update.
	assert.Equal(t, types.VersionChangeUpdate, history[1].ChangeType)

	// All entries must belong to this memory and project.
	for _, v := range history {
		assert.Equal(t, m.ID, v.MemoryID)
		assert.Equal(t, engine.Project(), v.Project)
		assert.NotEmpty(t, v.Content)
		assert.False(t, v.SystemFrom.IsZero())
	}
}

// TestTemporalVersioning_AsOf verifies that GetMemoriesAsOf returns memories
// that were active at a specific timestamp and excludes later-added or later-deleted ones.
func TestTemporalVersioning_AsOf(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-asof"))
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	// Store a memory, record the time, then soft-delete it.
	m := &types.Memory{
		Content:     "Candidate truth at T1",
		MemoryType:  types.MemoryTypeContext,
		Project:     engine.Project(),
		Tags:        []string{"temporal"},
		Importance:  3,
		StorageMode: "focused",
	}
	require.NoError(t, engine.Store(ctx, m))

	checkpoint := time.Now().UTC()
	time.Sleep(5 * time.Millisecond) // ensure checkpoint is before soft-delete

	_, err := engine.Forget(ctx, m.ID, "test soft-delete")
	require.NoError(t, err)

	// AsOf checkpoint: memory should be present (it existed and valid_to > checkpoint).
	atCheckpoint, err := engine.MemoryAsOf(ctx, checkpoint, 50)
	require.NoError(t, err)
	found := false
	for _, mem := range atCheckpoint {
		if mem.ID == m.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "memory should appear in AsOf query at checkpoint (before soft-delete)")

	// AsOf now: memory should be absent (valid_to <= now).
	atNow, err := engine.MemoryAsOf(ctx, time.Now().UTC(), 50)
	require.NoError(t, err)
	for _, mem := range atNow {
		assert.NotEqual(t, m.ID, mem.ID, "soft-deleted memory should NOT appear in current AsOf query")
	}
}

// TestTemporalVersioning_ActiveFilter verifies that soft-deleted memories are
// excluded from list/recall results while hard data remains in the database.
func TestTemporalVersioning_ActiveFilter(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-activefilter"))
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	mActive := &types.Memory{
		Content:     "Active memory — should always appear",
		MemoryType:  types.MemoryTypePattern,
		Project:     engine.Project(),
		Tags:        []string{"active"},
		Importance:  2,
		StorageMode: "focused",
	}
	mDeleted := &types.Memory{
		Content:     "Deleted memory — must not appear in recall",
		MemoryType:  types.MemoryTypePattern,
		Project:     engine.Project(),
		Tags:        []string{"deleted"},
		Importance:  2,
		StorageMode: "focused",
	}
	require.NoError(t, engine.Store(ctx, mActive))
	require.NoError(t, engine.Store(ctx, mDeleted))

	deleted, err := engine.Forget(ctx, mDeleted.ID, "filter test")
	require.NoError(t, err)
	require.True(t, deleted)

	// List should not contain the soft-deleted memory.
	list, err := engine.List(ctx, nil, nil, nil, 100, 0)
	require.NoError(t, err)
	for _, mem := range list {
		assert.NotEqual(t, mDeleted.ID, mem.ID, "soft-deleted memory must not appear in List")
	}

	// Active memory must still appear in the list.
	found := false
	for _, mem := range list {
		if mem.ID == mActive.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "active memory must appear in List after sibling soft-delete")
}
