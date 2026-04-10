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
	require.InDelta(t, 0.0/3.0, search.ImportanceBoost(0), 0.001)
	require.InDelta(t, 1.0, search.ImportanceBoost(3), 0.001)
	require.InDelta(t, 4.0/3.0, search.ImportanceBoost(4), 0.001)
}

func TestCompositeScore(t *testing.T) {
	s := search.CompositeScore(search.ScoreInput{
		Cosine:     1.0,
		BM25:       1.0,
		HoursSince: 0,
		Importance: 3,
	})
	// cosine(0.5) + bm25(0.35) + recency(0.15) * importanceBoost(1.0)
	require.InDelta(t, 1.0, s, 0.001)

	// Zero cosine and BM25 still gives a recency contribution
	s2 := search.CompositeScore(search.ScoreInput{
		Cosine:     0,
		BM25:       0,
		HoursSince: 0,
		Importance: 3,
	})
	require.InDelta(t, 0.15, s2, 0.001)
}
