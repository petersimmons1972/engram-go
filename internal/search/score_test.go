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
	// raw = 0.10*1.0 + 0.15*0.5 = 0.175
	s2 := search.CompositeScore(search.ScoreInput{
		Cosine:     0,
		BM25:       0,
		HoursSince: 0,
		Importance: 2,
	})
	require.InDelta(t, 0.175, s2, 0.001)

	// Critical memory (importance=0) scores higher than trivial (importance=4)
	sCritical := search.CompositeScore(search.ScoreInput{Cosine: 0.5, BM25: 0.5, HoursSince: 0, Importance: 0})
	sTrivial := search.CompositeScore(search.ScoreInput{Cosine: 0.5, BM25: 0.5, HoursSince: 0, Importance: 4})
	require.Greater(t, sCritical, sTrivial)
}
