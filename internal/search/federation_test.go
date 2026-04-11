package search_test

// Feature 4: Cross-Project Knowledge Federation
// All tests written BEFORE implementation (TDD).
// They must fail (compile or runtime) until Feature 4 is implemented.

import (
	"context"
	"testing"

	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFederatedRecall_MergesResults verifies that RecallAcrossEngines returns
// memories from multiple projects in a single result set sorted by score.
func TestFederatedRecall_MergesResults(t *testing.T) {
	engA := newTestEngine(t, uniqueProject("fed-a"))
	t.Cleanup(func() { engA.Close() })
	engB := newTestEngine(t, uniqueProject("fed-b"))
	t.Cleanup(func() { engB.Close() })
	ctx := context.Background()

	mA := &types.Memory{
		Content:    "Federation cross-project: alpha project pattern about distributed consensus",
		MemoryType: types.MemoryTypePattern,
		Project:    engA.Project(), Importance: 2, StorageMode: "focused",
	}
	mB := &types.Memory{
		Content:    "Federation cross-project: beta project pattern about distributed consensus",
		MemoryType: types.MemoryTypePattern,
		Project:    engB.Project(), Importance: 2, StorageMode: "focused",
	}
	require.NoError(t, engA.Store(ctx, mA))
	require.NoError(t, engB.Store(ctx, mB))

	results, err := search.RecallAcrossEngines(ctx, []*search.SearchEngine{engA, engB}, "distributed consensus", 10, "normal")
	require.NoError(t, err)
	assert.NotEmpty(t, results, "federated recall must return results from both projects")

	projectsSeen := make(map[string]bool)
	for _, r := range results {
		projectsSeen[r.Memory.Project] = true
	}
	assert.True(t, projectsSeen[engA.Project()],
		"results must include memories from project A (%s)", engA.Project())
	assert.True(t, projectsSeen[engB.Project()],
		"results must include memories from project B (%s)", engB.Project())
}

// TestFederatedRecall_ResultsAreSortedByScore verifies that the merged result
// set is ordered by composite score descending.
func TestFederatedRecall_ResultsAreSortedByScore(t *testing.T) {
	engA := newTestEngine(t, uniqueProject("fed-sort-a"))
	t.Cleanup(func() { engA.Close() })
	engB := newTestEngine(t, uniqueProject("fed-sort-b"))
	t.Cleanup(func() { engB.Close() })
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		for j, eng := range []*search.SearchEngine{engA, engB} {
			m := &types.Memory{
				Content:    "Federated sort test: important cross-project information for ranking validation",
				MemoryType: types.MemoryTypeContext,
				Project:    eng.Project(), Importance: i + j, StorageMode: "focused",
			}
			_ = eng.Store(ctx, m)
		}
	}

	results, err := search.RecallAcrossEngines(ctx, []*search.SearchEngine{engA, engB}, "cross-project information", 20, "normal")
	require.NoError(t, err)

	for i := 1; i < len(results); i++ {
		assert.GreaterOrEqual(t, results[i-1].Score, results[i].Score,
			"results[%d].Score (%v) must be >= results[%d].Score (%v)", i-1, results[i-1].Score, i, results[i].Score)
	}
}

// TestFederatedRecall_SingleEngine verifies no regression when only one engine
// is passed (should behave identically to a single-project recall).
func TestFederatedRecall_SingleEngine(t *testing.T) {
	eng := newTestEngine(t, uniqueProject("fed-single"))
	t.Cleanup(func() { eng.Close() })
	ctx := context.Background()

	m := &types.Memory{
		Content:    "Single-engine federation passthrough: verify no regression",
		MemoryType: types.MemoryTypeContext,
		Project:    eng.Project(), Importance: 2, StorageMode: "focused",
	}
	require.NoError(t, eng.Store(ctx, m))

	results, err := search.RecallAcrossEngines(ctx, []*search.SearchEngine{eng}, "federation passthrough", 5, "normal")
	require.NoError(t, err)
	assert.NotEmpty(t, results)
}
