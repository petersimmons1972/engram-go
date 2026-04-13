package db_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	return dsn
}

func uniqueProject(base string) string {
	return fmt.Sprintf("%s-%d", base, time.Now().UnixNano())
}

func newTestBackend(t *testing.T, project string) db.Backend {
	t.Helper()
	ctx := context.Background()
	b, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { b.Close() })
	return b
}

func storeMemory(t *testing.T, b db.Backend, proj string, content string) *types.Memory {
	t.Helper()
	m := &types.Memory{
		ID:         types.NewMemoryID(),
		Content:    content,
		Project:    proj,
		MemoryType: types.MemoryTypeContext,
		Importance: 1,
	}
	require.NoError(t, b.StoreMemory(context.Background(), m))
	return m
}

// TestGetRelationshipsBatch_Empty verifies that an empty input returns an empty map
// without hitting the database.
func TestGetRelationshipsBatch_Empty(t *testing.T) {
	proj := uniqueProject("relsbatch-empty")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	result, err := b.GetRelationshipsBatch(ctx, proj, nil)
	require.NoError(t, err)
	require.Empty(t, result, "empty ids slice must return empty map")
}

// TestGetRelationshipsBatch_NoRelationships verifies that known IDs with no
// relationships return an empty slice per ID (not a missing key).
func TestGetRelationshipsBatch_NoRelationships(t *testing.T) {
	proj := uniqueProject("relsbatch-norells")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	m1 := storeMemory(t, b, proj, "no relationships here")
	m2 := storeMemory(t, b, proj, "also no relationships")

	result, err := b.GetRelationshipsBatch(ctx, proj, []string{m1.ID, m2.ID})
	require.NoError(t, err)
	// Keys must be present with empty slices, not absent.
	require.Contains(t, result, m1.ID)
	require.Empty(t, result[m1.ID])
	require.Contains(t, result, m2.ID)
	require.Empty(t, result[m2.ID])
}

// TestGetRelationshipsBatch_ReturnsRelationships verifies that relationships
// are returned grouped by source memory ID, including edges where the memory
// is the target (bidirectional lookup matches GetRelationships semantics).
func TestGetRelationshipsBatch_ReturnsRelationships(t *testing.T) {
	proj := uniqueProject("relsbatch-rels")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	m1 := storeMemory(t, b, proj, "source memory")
	m2 := storeMemory(t, b, proj, "target memory")
	m3 := storeMemory(t, b, proj, "unrelated memory")

	// Create a relationship m1 → m2.
	rel := types.Relationship{
		ID:       types.NewMemoryID(),
		SourceID: m1.ID,
		TargetID: m2.ID,
		RelType:  types.RelTypeRelatesTo,
		Strength: 0.9,
		Project:  proj,
	}
	require.NoError(t, b.StoreRelationship(ctx, &rel))

	result, err := b.GetRelationshipsBatch(ctx, proj, []string{m1.ID, m2.ID, m3.ID})
	require.NoError(t, err)

	// m1 is source: should have 1 relationship.
	require.Contains(t, result, m1.ID)
	require.Len(t, result[m1.ID], 1)
	require.Equal(t, rel.ID, result[m1.ID][0].ID)

	// m2 is target: GetRelationships returns edges for both source and target,
	// so batch must match that behavior.
	require.Contains(t, result, m2.ID)
	require.Len(t, result[m2.ID], 1)
	require.Equal(t, rel.ID, result[m2.ID][0].ID)

	// m3 has no relationships: key must be present with empty slice.
	require.Contains(t, result, m3.ID)
	require.Empty(t, result[m3.ID])
}

// TestGetRelationshipsBatch_DeduplicatesEdges verifies that edges shared between
// two queried IDs (both source and target are in the batch) appear under both
// keys but are not duplicated within a single key's slice.
func TestGetRelationshipsBatch_DeduplicatesEdges(t *testing.T) {
	proj := uniqueProject("relsbatch-dedup")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	m1 := storeMemory(t, b, proj, "node A")
	m2 := storeMemory(t, b, proj, "node B")

	rel := types.Relationship{
		ID:       types.NewMemoryID(),
		SourceID: m1.ID,
		TargetID: m2.ID,
		RelType:  types.RelTypeRelatesTo,
		Strength: 0.8,
		Project:  proj,
	}
	require.NoError(t, b.StoreRelationship(ctx, &rel))

	// Both m1 and m2 are in the batch — edge appears in both but must not
	// be duplicated within either key's slice.
	result, err := b.GetRelationshipsBatch(ctx, proj, []string{m1.ID, m2.ID})
	require.NoError(t, err)

	require.Len(t, result[m1.ID], 1, "m1 slice must have exactly 1 entry, not duplicated")
	require.Len(t, result[m2.ID], 1, "m2 slice must have exactly 1 entry, not duplicated")
}

// TestGetRelationshipsBatch_MatchesSingleLookup verifies that results from
// GetRelationshipsBatch are equivalent to calling GetRelationships individually
// for each ID.
func TestGetRelationshipsBatch_MatchesSingleLookup(t *testing.T) {
	proj := uniqueProject("relsbatch-equiv")
	b := newTestBackend(t, proj)
	ctx := context.Background()

	m1 := storeMemory(t, b, proj, "first")
	m2 := storeMemory(t, b, proj, "second")
	m3 := storeMemory(t, b, proj, "third")

	// m1 → m2 and m1 → m3.
	for _, target := range []*types.Memory{m2, m3} {
		rel := types.Relationship{
			ID:       types.NewMemoryID(),
			SourceID: m1.ID,
			TargetID: target.ID,
			RelType:  types.RelTypeRelatesTo,
			Strength: 0.7,
			Project:  proj,
		}
		require.NoError(t, b.StoreRelationship(ctx, &rel))
	}

	ids := []string{m1.ID, m2.ID, m3.ID}
	batchResult, err := b.GetRelationshipsBatch(ctx, proj, ids)
	require.NoError(t, err)

	for _, id := range ids {
		single, err := b.GetRelationships(ctx, proj, id)
		require.NoError(t, err)
		batchRels := batchResult[id]
		require.Equal(t, len(single), len(batchRels),
			"batch result for %s must have same count as single-lookup", id)
	}
}
