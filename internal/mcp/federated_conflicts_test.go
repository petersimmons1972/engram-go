package mcp_test

// federated_conflicts_test.go verifies that include_conflicts=true is NOT
// silently ignored on the federated recall path (GitHub issue #154).
//
// Tests here follow the same integration-test pattern used throughout
// conflicts_test.go: they skip when TEST_DATABASE_URL is unset, compile
// against the real handleMemoryRecall code path, and fail before the fix
// is in place (include_conflicts was not read on the federated branch).

import (
	"context"
	"fmt"
	"testing"
	"time"

	internalmcp "github.com/petersimmons1972/engram/internal/mcp"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFederatedRecall_IncludeConflicts_NotSilentlyDropped verifies that when
// memory_recall is called with a "projects" list (federated path) and
// include_conflicts=true, the response contains a "conflicting_results" key
// with at least one entry for a known contradiction.
//
// Before the fix in #154 this test will fail because the federated branch
// returned immediately without reading include_conflicts, so
// conflicting_results was never populated.
func TestFederatedRecall_IncludeConflicts_NotSilentlyDropped(t *testing.T) {
	dsn := testRecallDSN(t) // defined in conflicts_test.go; skips without TEST_DATABASE_URL
	ctx := context.Background()

	proj1 := fmt.Sprintf("fed-conflicts-p1-%d", time.Now().UnixNano())
	proj2 := fmt.Sprintf("fed-conflicts-p2-%d", time.Now().UnixNano())

	pool := internalmcp.NewTestPoolWithDSN(t, ctx, dsn, proj1)

	// Ensure proj2 is also in the pool and cleaned up.
	h2, err := pool.Get(ctx, proj2)
	require.NoError(t, err)
	t.Cleanup(func() {
		if h2 != nil && h2.Engine != nil {
			h2.Engine.Close()
		}
	})

	h1, err := pool.Get(ctx, proj1)
	require.NoError(t, err)

	// memA lives in proj1, memB in proj2. They contradict each other.
	memA := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "Federated test: deploy every Friday for fast iteration.",
		MemoryType:  types.MemoryTypeDecision,
		Importance:  2,
		StorageMode: "focused",
	}
	memB := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "Federated test: never deploy on Friday — too risky.",
		MemoryType:  types.MemoryTypeDecision,
		Importance:  2,
		StorageMode: "focused",
	}

	require.NoError(t, h1.Engine.Store(ctx, memA))
	require.NoError(t, h2.Engine.Store(ctx, memB))

	// Wire the contradiction via the backend of proj1 (shared Postgres instance).
	rel := &types.Relationship{
		ID:       types.NewMemoryID(),
		SourceID: memA.ID,
		TargetID: memB.ID,
		RelType:  types.RelTypeContradicts,
		Strength: 0.9,
		Project:  proj1,
	}
	require.NoError(t, h1.Engine.Backend().StoreRelationship(ctx, rel))

	// Invoke the federated path with include_conflicts=true.
	out := internalmcp.CallHandleMemoryRecallFederated(ctx, t, pool, []string{proj1, proj2}, "deploy Friday", true)

	conflicts, ok := out["conflicting_results"]
	require.True(t, ok, "conflicting_results key must be present in federated recall when include_conflicts=true")

	conflictSlice, ok := conflicts.([]types.ConflictingResult)
	require.True(t, ok, "conflicting_results must unmarshal to []types.ConflictingResult")
	require.NotEmpty(t, conflictSlice, "expected at least one conflicting result")
}

// TestFederatedRecall_IncludeConflicts_FalseNoConflicts verifies that when
// include_conflicts=false (the default) the federated path does NOT return a
// conflicting_results key, so callers are not surprised by an empty array.
func TestFederatedRecall_IncludeConflicts_FalseNoConflicts(t *testing.T) {
	dsn := testRecallDSN(t)
	ctx := context.Background()

	proj1 := fmt.Sprintf("fed-noconflicts-p1-%d", time.Now().UnixNano())
	proj2 := fmt.Sprintf("fed-noconflicts-p2-%d", time.Now().UnixNano())

	pool := internalmcp.NewTestPoolWithDSN(t, ctx, dsn, proj1)

	h2, err := pool.Get(ctx, proj2)
	require.NoError(t, err)
	t.Cleanup(func() {
		if h2 != nil && h2.Engine != nil {
			h2.Engine.Close()
		}
	})

	h1, err := pool.Get(ctx, proj1)
	require.NoError(t, err)

	memA := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "Federated test no-conflicts: always test before shipping.",
		MemoryType:  types.MemoryTypePattern,
		Importance:  1,
		StorageMode: "focused",
	}
	require.NoError(t, h1.Engine.Store(ctx, memA))

	out := internalmcp.CallHandleMemoryRecallFederated(ctx, t, pool, []string{proj1, proj2}, "test before shipping", false)

	_, present := out["conflicting_results"]
	assert.False(t, present, "conflicting_results must be absent when include_conflicts=false on federated path")
}
