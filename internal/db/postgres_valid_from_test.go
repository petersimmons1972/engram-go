package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// TestStoreMemory_PersistsValidFrom is the regression guard for issue #747:
// the StoreMemoryTx INSERT statement previously omitted the valid_from column,
// so every memory landed in the DB with NULL valid_from even when the Go struct
// had ValidFrom set from a date: tag. That degraded recency scoring cluster-wide
// — temporalAnchorHours fell back to LastAccessed (ingest wall-clock) for every
// memory. This test stores a memory with an explicit ValidFrom and verifies it
// round-trips through Postgres unchanged.
func TestStoreMemory_PersistsValidFrom(t *testing.T) {
	proj := uniqueProject("valid-from-persist")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	want := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	mem := &types.Memory{
		ID:         types.NewMemoryID(),
		Content:    "session-dated memory for valid_from regression",
		Project:    proj,
		MemoryType: types.MemoryTypeContext,
		Importance: 1,
		Tags:       []string{"date:2024-03-15"},
		ValidFrom:  &want,
	}
	require.NoError(t, b.StoreMemory(ctx, mem))

	got, err := b.GetMemoryByID(ctx, mem.ID)
	require.NoError(t, err)
	require.NotNil(t, got, "stored memory must be retrievable")
	require.NotNil(t, got.ValidFrom, "valid_from must be persisted, not NULL — see issue #747")
	require.True(t, got.ValidFrom.Equal(want),
		"valid_from must round-trip unchanged: got %s, want %s", got.ValidFrom, want)
}

// TestStoreMemory_NilValidFromStaysNil verifies the column accepts NULL when
// ValidFrom is unset on the Go struct (memories without date: tags).
func TestStoreMemory_NilValidFromStaysNil(t *testing.T) {
	proj := uniqueProject("valid-from-nil")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	mem := &types.Memory{
		ID:         types.NewMemoryID(),
		Content:    "memory without a session date",
		Project:    proj,
		MemoryType: types.MemoryTypeContext,
		Importance: 1,
	}
	require.NoError(t, b.StoreMemory(ctx, mem))

	got, err := b.GetMemoryByID(ctx, mem.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Nil(t, got.ValidFrom, "valid_from must remain NULL when not set on the struct")
}

// TestUpdateMemory_PreservesValidFromWhenTagsHaveNoDate verifies the
// "only promote, never nullify" policy: if memory_correct sends tags that
// no longer include a date: tag, the previously-stored valid_from is
// preserved unchanged. Regression guard for issue #765.
func TestUpdateMemory_PreservesValidFromWhenTagsHaveNoDate(t *testing.T) {
	proj := uniqueProject("update-vf-preserve")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	// 1. Store a memory with a date: tag — ValidFrom should be set.
	originalDate := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	m := &types.Memory{
		ID:         types.NewMemoryID(),
		Content:    "test content for preserve",
		Project:    proj,
		MemoryType: types.MemoryTypeContext,
		Importance: 1,
		Tags:       []string{"date:2024-06-15", "foo"},
		ValidFrom:  &originalDate,
	}
	require.NoError(t, b.StoreMemory(ctx, m))

	// 2. Call UpdateMemory with tags that have NO date: tag.
	newTags := []string{"foo", "bar"}
	updated, err := b.UpdateMemory(ctx, m.ID, nil, newTags, nil, nil)
	require.NoError(t, err)

	// 3. ValidFrom must still equal the original date — preserved, not cleared.
	require.NotNil(t, updated.ValidFrom, "ValidFrom must not be nullified when new tags omit date:")
	require.True(t, updated.ValidFrom.Equal(originalDate),
		"ValidFrom must equal original 2024-06-15, got %s — see issue #765 'only promote, never nullify'", updated.ValidFrom)
}

// TestUpdateMemory_PromotesValidFromOnDateTagChange is the paired positive case:
// when new tags include a different date: tag, ValidFrom is updated to match.
func TestUpdateMemory_PromotesValidFromOnDateTagChange(t *testing.T) {
	proj := uniqueProject("update-vf-promote")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	originalDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	m := &types.Memory{
		ID:         types.NewMemoryID(),
		Content:    "test content for promote",
		Project:    proj,
		MemoryType: types.MemoryTypeContext,
		Importance: 1,
		Tags:       []string{"date:2024-01-01"},
		ValidFrom:  &originalDate,
	}
	require.NoError(t, b.StoreMemory(ctx, m))

	// Update with a new date: tag — ValidFrom must be promoted to the new date.
	newTags := []string{"date:2024-12-31"}
	updated, err := b.UpdateMemory(ctx, m.ID, nil, newTags, nil, nil)
	require.NoError(t, err)

	expectedNew := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	require.NotNil(t, updated.ValidFrom)
	require.True(t, updated.ValidFrom.Equal(expectedNew),
		"ValidFrom must equal new 2024-12-31, got %s", updated.ValidFrom)
}
