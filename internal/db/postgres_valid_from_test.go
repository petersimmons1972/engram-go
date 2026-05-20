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
