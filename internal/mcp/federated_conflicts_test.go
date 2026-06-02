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
// Before the fix in #154 this test failed because the federated branch
// returned immediately without reading include_conflicts, so
// conflicting_results was never populated.
//
// Three setup requirements (mirroring TestHandleMemoryRecall_IncludeConflicts_Integration):
//
//  1. FlushPendingEmbeddings must be called for BOTH projects: Store() writes
//     chunks with nil embeddings and relies on the background reembed worker to
//     fill them in; tests don't run that worker, so flushing synchronously is
//     required for vector search to return any results at all.
//
//  2. top_k must be 1: EnrichWithConflicts skips memories already present in
//     the primary results slice (to avoid duplicating the same memory in both
//     results and conflicting_results). With two memories and top_k≥2 both
//     entries land in primary, and the contradicts edge is never surfaced.
//     Using top_k=1 ensures exactly one memory wins the primary slot, leaving
//     the other available to be discovered via the contradicts edge.
//
//  3. Bidirectional relationship storage: EnrichWithConflicts calls
//     GetRelationships(ctx, r.Memory.Project, r.Memory.ID). In the federated
//     case, memories live in different projects, and StoreRelationship forces
//     project=b.project on the stored row. Storing only in proj1 means
//     GetRelationships(ctx, proj2, memB.ID) returns nothing when memB wins
//     the primary slot — the edge's project column is proj1, not proj2.
//     Storing the reverse edge (memB→memA, project=proj2) via proj2's backend
//     makes the assertion order-independent: whichever memory wins the primary
//     slot, its contradiction edge is findable via its own project's rows.
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

	// Store the contradicts edge BIDIRECTIONALLY — once per project.
	// EnrichWithConflicts calls GetRelationships(ctx, r.Memory.Project, r.Memory.ID).
	// StoreRelationship forces project=b.project on the row, so a single edge stored
	// via proj1's backend has project=proj1. If memB wins the primary slot, looking
	// up GetRelationships(ctx, proj2, memB.ID) finds nothing. The reverse edge
	// (stored via proj2's backend, project=proj2) covers that ordering.
	//
	// StoreRelationship's source/target existence checks carry no project filter
	// since #430, so both stores succeed across project boundaries.
	relAtoB := &types.Relationship{
		ID:       types.NewMemoryID(),
		SourceID: memA.ID,
		TargetID: memB.ID,
		RelType:  types.RelTypeContradicts,
		Strength: 0.9,
	}
	require.NoError(t, h1.Engine.Backend().StoreRelationship(ctx, relAtoB))

	relBtoA := &types.Relationship{
		ID:       types.NewMemoryID(),
		SourceID: memB.ID,
		TargetID: memA.ID,
		RelType:  types.RelTypeContradicts,
		Strength: 0.9,
	}
	require.NoError(t, h2.Engine.Backend().StoreRelationship(ctx, relBtoA))

	// Backfill NULL embeddings for both projects synchronously so vector search
	// returns results. Production relies on the background reembed worker;
	// tests must flush manually (see FlushPendingEmbeddings doc in export_test.go).
	internalmcp.FlushPendingEmbeddings(t, ctx, dsn, proj1)
	internalmcp.FlushPendingEmbeddings(t, ctx, dsn, proj2)

	// Invoke the federated path with include_conflicts=true and top_k=1 so
	// exactly one memory wins the primary slot and the other is surfaced as a
	// conflict. The assertion is order-independent: either ordering is valid.
	out := internalmcp.CallHandleMemoryRecallFederated(ctx, t, pool, []string{proj1, proj2}, "Friday fast iteration", 1, true)

	conflicts, ok := out["conflicting_results"]
	require.True(t, ok, "conflicting_results key must be present in federated recall when include_conflicts=true")

	conflictSlice, ok := conflicts.([]types.ConflictingResult)
	require.True(t, ok, "conflicting_results must unmarshal to []types.ConflictingResult")
	require.NotEmpty(t, conflictSlice, "expected at least one conflicting result")

	// Build the set of IDs that appear in conflicting_results.
	conflictMemIDs := make(map[string]string, len(conflictSlice)) // memID → contradictsID
	for _, c := range conflictSlice {
		if c.Memory != nil {
			conflictMemIDs[c.Memory.ID] = c.ContradictsID
		}
	}

	// The invariant: exactly one of the two memories is in primary (top_k=1);
	// the other must appear in conflicting_results with its ContradictsID pointing
	// back to the primary memory. We accept either ordering.
	pairIDs := map[string]string{memA.ID: memB.ID, memB.ID: memA.ID}
	found := false
	for conflictID, contradictsID := range conflictMemIDs {
		peer, inPair := pairIDs[conflictID]
		if !inPair {
			continue
		}
		if contradictsID == peer {
			found = true
			break
		}
	}
	assert.True(t, found,
		"one member of the contradicting pair must appear in conflicting_results "+
			"with its ContradictsID pointing to the primary memory; "+
			"conflict IDs seen: %v", conflictMemIDs)
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

	out := internalmcp.CallHandleMemoryRecallFederated(ctx, t, pool, []string{proj1, proj2}, "test before shipping", 10, false)

	_, present := out["conflicting_results"]
	assert.False(t, present, "conflicting_results must be absent when include_conflicts=false on federated path")
}
