package search_test

import (
	"math"
	"testing"

	"github.com/petersimmons1972/engram/internal/search"
	"github.com/stretchr/testify/require"
)

func TestRecencyDecay(t *testing.T) {
	// At 0 hours: decay = 1.0
	require.InDelta(t, 1.0, search.RecencyDecay(0), 0.0001)
	// At 69.3 hours (~ln(2)/0.01): decay ≈ 0.5
	require.InDelta(t, 0.5, search.RecencyDecay(math.Log(2)/0.01), 0.01)
	// Decay is monotonically decreasing
	require.Greater(t, search.RecencyDecay(10), search.RecencyDecay(100))
}

func TestImportanceBoost(t *testing.T) {
	// importance scale: 0=critical (highest boost), 4=trivial (lowest boost)
	require.InDelta(t, 5.0/3.0, search.ImportanceBoost(0), 0.001) // critical
	require.InDelta(t, 1.0, search.ImportanceBoost(2), 0.001)      // neutral
	require.InDelta(t, 1.0/3.0, search.ImportanceBoost(4), 0.001)  // trivial
	// boost decreases as importance value increases (more trivial)
	require.Greater(t, search.ImportanceBoost(0), search.ImportanceBoost(4))
}

func TestCompositeScore(t *testing.T) {
	// importance=2 → neutral boost of 1.0; nil precision → cold-start neutral 0.5
	// raw = cosine(0.45) + bm25(0.30) + recency(0.10) + precision(0.15*0.5) = 0.925; boost=1.0
	s := search.CompositeScore(search.ScoreInput{
		Cosine:     1.0,
		BM25:       1.0,
		HoursSince: 0,
		Importance: 2,
	})
	require.InDelta(t, 0.925, s, 0.001)

	// Zero cosine and BM25: recency + cold-start precision contribute; boost=1.0
	// raw = recency(0.15)*1.0 + precision(0.15)*0.5 = 0.225
	s2 := search.CompositeScore(search.ScoreInput{
		Cosine:     0,
		BM25:       0,
		HoursSince: 0,
		Importance: 2,
	})
	require.InDelta(t, 0.225, s2, 0.001)

	// Critical memory (importance=0) scores higher than trivial (importance=4)
	sCritical := search.CompositeScore(search.ScoreInput{Cosine: 0.5, BM25: 0.5, HoursSince: 0, Importance: 0})
	sTrivial := search.CompositeScore(search.ScoreInput{Cosine: 0.5, BM25: 0.5, HoursSince: 0, Importance: 4})
	require.Greater(t, sCritical, sTrivial)
}

func TestCompositeScoreWithWeights_EpisodeBoost_Applied(t *testing.T) {
	w := search.DefaultWeights()
	input := search.ScoreInput{
		Cosine:       0.8,
		BM25:         0.6,
		HoursSince:   1.0,
		Importance:   2,
		EpisodeMatch: true,
	}
	inputNoBoost := input
	inputNoBoost.EpisodeMatch = false

	withBoost := search.CompositeScoreWithWeights(input, w)
	noBoost := search.CompositeScoreWithWeights(inputNoBoost, w)

	if withBoost <= noBoost {
		t.Fatalf("expected episode boost to increase score: with=%f no=%f", withBoost, noBoost)
	}
	ratio := withBoost / noBoost
	if ratio < 1.14 || ratio > 1.16 {
		t.Fatalf("expected ~1.15× boost, got ratio=%f", ratio)
	}
}

func TestCompositeScoreWithWeights_NoEpisodeMatch_NoPenalty(t *testing.T) {
	w := search.DefaultWeights()
	input := search.ScoreInput{
		Cosine:       0.7,
		BM25:         0.5,
		HoursSince:   24.0,
		Importance:   2,
		EpisodeMatch: false,
	}
	score := search.CompositeScoreWithWeights(input, w)
	if score <= 0 {
		t.Fatalf("expected positive score, got %f", score)
	}
}

func TestCompositeScore_EpisodeMatch_Shorthand(t *testing.T) {
	matched := search.CompositeScore(search.ScoreInput{Cosine: 0.9, BM25: 0.8, HoursSince: 0.5, Importance: 2, EpisodeMatch: true})
	unmatched := search.CompositeScore(search.ScoreInput{Cosine: 0.9, BM25: 0.8, HoursSince: 0.5, Importance: 2, EpisodeMatch: false})
	if matched <= unmatched {
		t.Fatalf("matched should outscore unmatched: %f vs %f", matched, unmatched)
	}
}

func TestRRFScore(t *testing.T) {
	const k = 60

	// Both ranks absent → zero
	require.InDelta(t, 0.0, search.RRFScore(0, 0, k), 0.0001)

	// Single vector leg, rank 1 → 1/(k+1)
	require.InDelta(t, 1.0/float64(k+1), search.RRFScore(1, 0, k), 0.0001)

	// Single BM25 leg, rank 1 → 1/(k+1)
	require.InDelta(t, 1.0/float64(k+1), search.RRFScore(0, 1, k), 0.0001)

	// Both legs rank 1 → 2/(k+1)
	require.InDelta(t, 2.0/float64(k+1), search.RRFScore(1, 1, k), 0.0001)

	// Higher rank number (worse position) → lower score
	require.Greater(t, search.RRFScore(1, 0, k), search.RRFScore(10, 0, k))
	require.Greater(t, search.RRFScore(0, 1, k), search.RRFScore(0, 20, k))

	// Both legs present, mixed ranks
	want := 1.0/float64(k+10) + 1.0/float64(k+5)
	require.InDelta(t, want, search.RRFScore(10, 5, k), 0.0001)

	// Larger k → smaller score magnitude but flatter rank curve
	scoreSmallK := search.RRFScore(1, 1, 10)
	scoreLargeK := search.RRFScore(1, 1, 100)
	require.Greater(t, scoreSmallK, scoreLargeK)
}

func TestCompositeScoreRRF(t *testing.T) {
	w := search.DefaultWeights()
	const k = 60

	// Best case: both legs rank 1 with default weights + neutral inputs.
	// rrfBase = 2/61; scaled = (2/61)*(61/2)*(0.40+0.30) = 0.70
	// raw = 0.70 + 0.15*recency(0) + 0.15*0.5 = 0.70+0.15+0.075 = 0.925; boost=1.0
	bestRRF := search.RRFScore(1, 1, k)
	score := search.CompositeScoreRRF(search.ScoreInput{HoursSince: 0, Importance: 2}, w, bestRRF)
	require.InDelta(t, 0.925, score, 0.001)

	// Single-leg (vector rank 1 only) scores lower than both-leg rank 1.
	singleRRF := search.RRFScore(1, 0, k)
	scoreSingle := search.CompositeScoreRRF(search.ScoreInput{HoursSince: 0, Importance: 2}, w, singleRRF)
	require.Less(t, scoreSingle, score)

	// Episode boost applies: ~1.15×
	noBoostInput := search.ScoreInput{HoursSince: 0, Importance: 2, EpisodeMatch: false}
	boostInput := search.ScoreInput{HoursSince: 0, Importance: 2, EpisodeMatch: true}
	noBoost := search.CompositeScoreRRF(noBoostInput, w, bestRRF)
	withBoost := search.CompositeScoreRRF(boostInput, w, bestRRF)
	require.InDelta(t, 1.15, withBoost/noBoost, 0.001)

	// Preference boost applies: ~1.8× for preference-typed memory on preference query.
	prefInput := search.ScoreInput{HoursSince: 0, Importance: 2, IsPreferenceQuery: true, MemoryType: "preference"}
	noPrefInput := search.ScoreInput{HoursSince: 0, Importance: 2, IsPreferenceQuery: true, MemoryType: "context"}
	withPref := search.CompositeScoreRRF(prefInput, w, bestRRF)
	withoutPref := search.CompositeScoreRRF(noPrefInput, w, bestRRF)
	require.InDelta(t, 1.8, withPref/withoutPref, 0.001)

	// RRF=0 (no hits in either leg) still returns a positive score from recency+precision.
	zeroRRF := search.CompositeScoreRRF(search.ScoreInput{HoursSince: 0, Importance: 2}, w, 0)
	require.Greater(t, zeroRRF, 0.0)

	// Critical memory (importance=0) outscores trivial (importance=4) at same RRF.
	critical := search.CompositeScoreRRF(search.ScoreInput{HoursSince: 0, Importance: 0}, w, bestRRF)
	trivial := search.CompositeScoreRRF(search.ScoreInput{HoursSince: 0, Importance: 4}, w, bestRRF)
	require.Greater(t, critical, trivial)
}
