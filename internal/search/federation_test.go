package search_test

// Feature 4: Cross-Project Knowledge Federation
// All tests written BEFORE implementation (TDD).
// They must fail (compile or runtime) until Feature 4 is implemented.

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingRecallBackend struct {
	err error
	db.Backend
}

func (b *failingRecallBackend) VectorSearchWithDateRange(_ context.Context, _ string, _ []float32, _ int, _, _ *time.Time) ([]db.VectorHit, error) {
	return nil, b.err
}

func (b *failingRecallBackend) FTSSearch(_ context.Context, _ string, _ string, _ int, _, _ *time.Time) ([]db.FTSResult, error) {
	return nil, b.err
}

func (b *failingRecallBackend) GetMeta(ctx context.Context, project string, key string) (string, bool, error) {
	return b.Backend.GetMeta(ctx, project, key)
}

func (b *failingRecallBackend) SetMeta(ctx context.Context, project string, key string, val string) error {
	return b.Backend.SetMeta(ctx, project, key, val)
}

func (b *failingRecallBackend) SetMetaTx(ctx context.Context, tx db.Tx, project string, key string, val string) error {
	return b.Backend.SetMetaTx(ctx, tx, project, key, val)
}

func newEngineWithFailingBackend(t *testing.T, project string, err error) *search.SearchEngine {
	t.Helper()
	ctx := context.Background()
	base, dsnErr := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, dsnErr)
	backend := &failingRecallBackend{err: err, Backend: base}
	t.Cleanup(base.Close)
	t.Cleanup(func() {
		if delErr := base.DeleteProject(context.Background(), project); delErr != nil {
			t.Logf("cleanup delete project %q: %v", project, delErr)
		}
	})
	return search.New(ctx, backend, &fakeClient{dims: 1024}, project,
		"http://ollama:11434", "llama3.2", false, nil, 0)
}

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
		if math.IsNaN(results[i-1].Score) {
			assert.True(t, math.IsNaN(results[i].Score),
				"NaN scores must sort after finite scores; results[%d].Score=%v, results[%d].Score=%v",
				i-1, results[i-1].Score, i, results[i].Score)
			continue
		}
		if math.IsNaN(results[i].Score) {
			continue
		}
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

func TestFederatedRecall_AllEnginesFailReturnsError(t *testing.T) {
	engA := newTestEngine(t, uniqueProject("fed-fail-a"))
	t.Cleanup(func() { engA.Close() })
	engB := newTestEngine(t, uniqueProject("fed-fail-b"))
	t.Cleanup(func() { engB.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results, err := search.RecallAcrossEngines(ctx, []*search.SearchEngine{engA, engB}, "distributed consensus", 10, "normal")
	require.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "federated recall failed across")
}

func TestFederatedRecall_PartialFailureReturnsMetadataAndResults(t *testing.T) {
	engGood := newTestEngine(t, uniqueProject("fed-partial-good"))
	t.Cleanup(func() { engGood.Close() })
	engBad := newEngineWithFailingBackend(t, uniqueProject("fed-partial-bad"), errors.New("failing engine"))
	t.Cleanup(func() { engBad.Close() })
	ctx := context.Background()

	m := &types.Memory{
		Content:    "Federation mixed-engine result with one failing tenant",
		MemoryType: types.MemoryTypePattern,
		Project:    engGood.Project(), Importance: 2, StorageMode: "focused",
	}
	require.NoError(t, engGood.Store(ctx, m))

	results, failed, err := search.RecallAcrossEnginesWithEventsAndOpts(ctx, []*search.SearchEngine{engGood, engBad}, "mixed-engine recall", 10, "normal", search.RecallOpts{}, false)
	require.NoError(t, err, "partial federation failure should still return partial results")
	require.Len(t, failed, 1, "partial failure should include exactly one failed project")
	require.Equal(t, engBad.Project(), failed[0].Project)
	require.Equal(t, "failing engine", failed[0].Error)
	require.Greater(t, len(results), 0, "good engine should contribute recall results")
}
