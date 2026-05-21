package search_test

import (
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// TestRankNormalizedRecency_BasicOrdering verifies that given 3 memories with
// valid_from dates spanning 2 years, RankNormalizedRecency returns scores in
// ascending order (oldest = lowest score, most recent = highest score).
func TestRankNormalizedRecency_BasicOrdering(t *testing.T) {
	oldest := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
	middle := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	newest := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	minDate := oldest
	maxDate := newest

	scoreOldest := search.RankNormalizedRecency(oldest, minDate, maxDate)
	scoreMiddle := search.RankNormalizedRecency(middle, minDate, maxDate)
	scoreNewest := search.RankNormalizedRecency(newest, minDate, maxDate)

	require.Less(t, scoreOldest, scoreMiddle, "oldest should score lower than middle")
	require.Less(t, scoreMiddle, scoreNewest, "middle should score lower than newest")
	require.InDelta(t, 0.0, scoreOldest, 0.0001, "oldest should score 0.0 (== minDate)")
	require.InDelta(t, 1.0, scoreNewest, 0.0001, "newest should score 1.0 (== maxDate)")
}

// TestRankNormalizedRecency_AllSameDate verifies that when all candidates have
// the same valid_from, every candidate receives a neutral score of 0.5.
func TestRankNormalizedRecency_AllSameDate(t *testing.T) {
	sameDate := time.Date(2023, 6, 15, 12, 0, 0, 0, time.UTC)
	minDate := sameDate
	maxDate := sameDate

	score1 := search.RankNormalizedRecency(sameDate, minDate, maxDate)
	score2 := search.RankNormalizedRecency(sameDate, minDate, maxDate)

	require.InDelta(t, 0.5, score1, 0.0001, "all-same-date should return 0.5")
	require.InDelta(t, 0.5, score2, 0.0001, "all-same-date should return 0.5")
}

// TestRankNormalizedRecency_SingleCandidate verifies that a single candidate
// receives a score of 1.0 via CompositeScoreWithRankNorm when only one memory
// is in the candidate set. The engine detects len(candidates)==1 and returns 1.0
// for the recency component so the single result is always fully recency-boosted.
func TestRankNormalizedRecency_SingleCandidate(t *testing.T) {
	singleDate := time.Date(2023, 3, 10, 0, 0, 0, 0, time.UTC)
	w := search.TemporalWeights()

	m := &types.Memory{
		Content:    "Only memory in candidate set",
		ValidFrom:  &singleDate,
		CreatedAt:  singleDate,
		Importance: 2,
	}
	candidates := []*types.Memory{m}

	score := search.CompositeScoreWithRankNorm(
		search.ScoreInput{Cosine: 0.8, BM25: 0.7, Importance: 2},
		w,
		singleDate,
		candidates,
	)

	// When there's only one candidate, recency should be 1.0 (fully boosted).
	// Compute the expected score manually: recency=1.0, boost=1.0 (importance=2).
	// raw = w.Vector*0.8 + w.BM25*0.7 + w.Recency*1.0 + w.Precision*0.5
	expectedRecency := 1.0
	expectedRaw := w.Vector*0.8 + w.BM25*0.7 + w.Recency*expectedRecency + w.Precision*0.5
	require.InDelta(t, expectedRaw, score, 0.001, "single candidate should use recency=1.0")
}

// TestRankNormalizedRecency_NilValidFrom verifies that when a memory has a zero
// valid_from (nil), the function falls back to RecencyDecay using created_at hours.
func TestRankNormalizedRecency_NilValidFrom(t *testing.T) {
	// A zero time.Time represents a nil/unset valid_from.
	var zeroTime time.Time
	createdAt := time.Now().Add(-48 * time.Hour) // 48 hours ago

	minDate := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
	maxDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// When validFrom is zero, the function should fall back to RecencyDecay
	// using the hours since createdAt. The fallback score should equal RecencyDecay(~48h).
	score := search.RankNormalizedRecencyWithFallback(zeroTime, createdAt, minDate, maxDate)
	expected := search.RecencyDecay(48.0)

	// Allow generous delta since time passes between computing expected and calling the function.
	require.InDelta(t, expected, score, 0.001, "zero valid_from should fall back to RecencyDecay(createdAt hours)")
}

// TestCompositeScore_TemporalWeights_UseRankNorm verifies the end-to-end scoring path:
// when TemporalWeights are in effect and candidates have spread valid_from dates,
// the newer session scores higher than the older session.
func TestCompositeScore_TemporalWeights_UseRankNorm(t *testing.T) {
	w := search.TemporalWeights()

	olderDate := time.Date(2022, 6, 1, 0, 0, 0, 0, time.UTC)
	newerDate := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	// Build two ScoreInputs with identical cosine/BM25 signals — only valid_from differs.
	// Both memories have the same project-level cosine and BM25 scores so the
	// rank-normalized recency is the sole differentiator.
	olderMem := &types.Memory{
		Content:    "Older session memory",
		ValidFrom:  &olderDate,
		CreatedAt:  olderDate,
		Importance: 2,
	}
	newerMem := &types.Memory{
		Content:    "Newer session memory",
		ValidFrom:  &newerDate,
		CreatedAt:  newerDate,
		Importance: 2,
	}

	candidates := []*types.Memory{olderMem, newerMem}

	// CompositeScoreWithRankNorm computes minDate/maxDate across the set
	// and uses RankNormalizedRecency for the recency component.
	scoreOlder := search.CompositeScoreWithRankNorm(
		search.ScoreInput{Cosine: 0.8, BM25: 0.7, Importance: 2},
		w,
		olderDate,
		candidates,
	)
	scoreNewer := search.CompositeScoreWithRankNorm(
		search.ScoreInput{Cosine: 0.8, BM25: 0.7, Importance: 2},
		w,
		newerDate,
		candidates,
	)

	require.Greater(t, scoreNewer, scoreOlder,
		"newer session (valid_from=2024) should score higher than older (valid_from=2022) under TemporalWeights with rank normalization")
}
