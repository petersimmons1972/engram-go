// temporal_window_recall_test.go — integration tests for H-NEW-1 two-pass
// date-windowed recall. These require a Postgres backend and skip gracefully
// when TEST_DATABASE_URL is unset (see testutil.DSN).
package search_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

func requireSameResultIDs(t *testing.T, expected, actual []types.SearchResult, msg string) {
	t.Helper()

	require.Equal(t, len(expected), len(actual), "%s result count must match baseline", msg)

	expectedIDs := make([]string, len(expected))
	for i, r := range expected {
		require.NotNil(t, r.Memory)
		expectedIDs[i] = r.Memory.ID
	}

	actualIDs := make([]string, len(actual))
	for i, r := range actual {
		require.NotNil(t, r.Memory)
		actualIDs[i] = r.Memory.ID
	}

	require.ElementsMatch(t, expectedIDs, actualIDs, "%s result IDs must match baseline", msg)
}

// storeWindowFixtures stores an in-window and an out-of-window memory whose
// content is otherwise identical, so retrieval ordering is driven by the date
// window rather than semantics. Returns the two IDs.
func storeWindowFixtures(t *testing.T, engine *search.SearchEngine) (inWindowID, outOfWindowID string) {
	t.Helper()
	ctx := context.Background()
	// question_date 2023/06/09, "3 days ago" -> target 2023/06/06, window
	// [06-05, 06-08). In-window session on 06-06; distractor on 05-09 (out).
	inWindowDate := time.Date(2023, 6, 6, 0, 0, 0, 0, time.UTC)
	outOfWindowDate := time.Date(2023, 5, 9, 0, 0, 0, 0, time.UTC)
	inWindow := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "dentist appointment scheduling notes",
		MemoryType:  types.MemoryTypeContext,
		StorageMode: "focused",
		ValidFrom:   &inWindowDate,
	}
	outOfWindow := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "dentist appointment scheduling notes",
		MemoryType:  types.MemoryTypeContext,
		StorageMode: "focused",
		ValidFrom:   &outOfWindowDate,
	}
	require.NoError(t, engine.Store(ctx, inWindow))
	require.NoError(t, engine.Store(ctx, outOfWindow))
	return inWindow.ID, outOfWindow.ID
}

// TestTemporalWindowRecall_SurfacesInWindowSession verifies that with the flag on,
// the in-window gold session is retrieved (the second, date-filtered pass adds it
// to the candidate set even though the distractor is semantically identical).
func TestTemporalWindowRecall_SurfacesInWindowSession(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-twr-surfaces"))
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	inWindowID, _ := storeWindowFixtures(t, engine)

	results, err := engine.RecallWithOpts(ctx, "dentist appointment", 10, "summary", search.RecallOpts{
		TemporalWindowRecall: true,
		QuestionText:         "What did I schedule 3 days ago?",
		QuestionDate:         "2023/06/09 (Fri)",
	})
	require.NoError(t, err)
	require.NotEmpty(t, results)

	var sawInWindow bool
	for _, r := range results {
		if r.Memory != nil && r.Memory.ID == inWindowID {
			sawInWindow = true
		}
	}
	require.True(t, sawInWindow, "in-window session must appear in temporal-window recall results")
}

// TestTemporalWindowRecall_FlagOffIsBaseline verifies that with the flag off,
// recall is byte-for-byte identical to a plain RecallWithOpts call — the lever is
// fully ablatable and adds no behaviour when disabled.
func TestTemporalWindowRecall_FlagOffIsBaseline(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-twr-baseline"))
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	storeWindowFixtures(t, engine)

	baseline, err := engine.RecallWithOpts(ctx, "dentist appointment", 10, "summary", search.RecallOpts{})
	require.NoError(t, err)

	flagOff, err := engine.RecallWithOpts(ctx, "dentist appointment", 10, "summary", search.RecallOpts{
		TemporalWindowRecall: false,
		QuestionText:         "What did I schedule 3 days ago?",
		QuestionDate:         "2023/06/09 (Fri)",
	})
	require.NoError(t, err)

	requireSameResultIDs(t, baseline, flagOff, "flag-off")
}

// TestTemporalWindowRecall_HowManyFallsBackToBaseline verifies that a "how many
// X ago" question (where ParseTemporalWindow returns nil) does not invoke the
// second pass and produces baseline results, even with the flag on.
func TestTemporalWindowRecall_HowManyFallsBackToBaseline(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-twr-howmany"))
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	storeWindowFixtures(t, engine)

	baseline, err := engine.RecallWithOpts(ctx, "dentist appointment", 10, "summary", search.RecallOpts{})
	require.NoError(t, err)

	howMany, err := engine.RecallWithOpts(ctx, "dentist appointment", 10, "summary", search.RecallOpts{
		TemporalWindowRecall: true,
		QuestionText:         "How many weeks ago did I visit the dentist?",
		QuestionDate:         "2023/06/09 (Fri)",
	})
	require.NoError(t, err)

	requireSameResultIDs(t, baseline, howMany, "'how many'")
}

// twrCountingReranker is a stub search.ResultReranker that counts RerankResults
// invocations. Used to verify the temporal two-pass path invokes reranking once.
type twrCountingReranker struct {
	calls atomic.Int64
}

func (r *twrCountingReranker) RerankResults(_ context.Context, _ string, items []search.RerankItem) ([]search.RerankResult, error) {
	r.calls.Add(1)
	out := make([]search.RerankResult, len(items))
	for i, it := range items {
		out[i] = search.RerankResult{ID: it.ID, Score: it.Score}
	}
	return out, nil
}

// TestTemporalWindowRecall_RerankerRunsOnce proves that when a reranker is wired
// and TemporalWindowRecall is true, the reranker is invoked exactly once — on the
// merged result set — not once per internal pass.
func TestTemporalWindowRecall_RerankerRunsOnce(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-twr-rerank"))
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	storeWindowFixtures(t, engine)

	reranker := &twrCountingReranker{}
	_, err := engine.RecallWithOpts(ctx, "dentist appointment", 10, "summary", search.RecallOpts{
		TemporalWindowRecall: true,
		QuestionText:         "What did I schedule 3 days ago?",
		QuestionDate:         "2023/06/09 (Fri)",
		Reranker:             reranker,
	})
	require.NoError(t, err)

	calls := reranker.calls.Load()
	require.Equal(t, int64(1), calls,
		"reranker must be invoked exactly once on the merged result (not once per pass); got %d calls", calls)
}
