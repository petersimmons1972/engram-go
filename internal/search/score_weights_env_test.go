package search

// Tests for H6: env-var-driven composite scoring weight overrides.
// These live in the internal package so they can call resetWeightConfigForTesting.

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// setEnvWeights sets all four ENGRAM_W_* env vars and returns a cleanup function
// that unsets them and resets the cached config.
func setEnvWeights(t *testing.T, cosine, bm25, recency, precision string) {
	t.Helper()
	if cosine != "" {
		t.Setenv("ENGRAM_W_COSINE", cosine)
	}
	if bm25 != "" {
		t.Setenv("ENGRAM_W_BM25", bm25)
	}
	if recency != "" {
		t.Setenv("ENGRAM_W_RECENCY", recency)
	}
	if precision != "" {
		t.Setenv("ENGRAM_W_PRECISION", precision)
	}
	resetWeightConfigForTesting()
	t.Cleanup(resetWeightConfigForTesting)
}

// TestDefaultWeights_EnvUnset verifies that with no env vars set the returned
// weights match the compile-time constants.
func TestDefaultWeights_EnvUnset(t *testing.T) {
	resetWeightConfigForTesting()
	t.Cleanup(resetWeightConfigForTesting)

	w := DefaultWeights()
	require.InDelta(t, 0.40, w.Vector, 0.0001, "Vector default")
	require.InDelta(t, 0.30, w.BM25, 0.0001, "BM25 default")
	require.InDelta(t, 0.15, w.Recency, 0.0001, "Recency default")
	require.InDelta(t, 0.15, w.Precision, 0.0001, "Precision default")
}

// TestDefaultWeights_H6 verifies that setting ENGRAM_W_BM25=0.40 and
// ENGRAM_W_COSINE=0.30 (H6 hypothesis values) is reflected in DefaultWeights
// while Recency and Precision fall back to constants.
func TestDefaultWeights_H6(t *testing.T) {
	setEnvWeights(t, "0.30", "0.40", "", "")

	w := DefaultWeights()
	require.InDelta(t, 0.30, w.Vector, 0.0001, "H6 Vector (cosine) should be 0.30")
	require.InDelta(t, 0.40, w.BM25, 0.0001, "H6 BM25 should be 0.40")
	require.InDelta(t, 0.15, w.Recency, 0.0001, "Recency should fall back to default 0.15")
	require.InDelta(t, 0.15, w.Precision, 0.0001, "Precision should fall back to default 0.15")
}

// TestDefaultWeights_InvalidValue verifies that an out-of-range value (1.5) and a
// non-numeric value ("bogus") each fall back to the compile-time constant without
// panicking, and that valid sibling vars are still applied.
func TestDefaultWeights_InvalidOutOfRange(t *testing.T) {
	setEnvWeights(t, "1.5", "0.40", "", "")

	w := DefaultWeights()
	require.InDelta(t, 0.40, w.Vector, 0.0001, "out-of-range cosine should fall back to default 0.40")
	require.InDelta(t, 0.40, w.BM25, 0.0001, "valid BM25 should be accepted")
}

func TestDefaultWeights_InvalidNonNumeric(t *testing.T) {
	setEnvWeights(t, "bogus", "", "", "")

	w := DefaultWeights()
	require.InDelta(t, 0.40, w.Vector, 0.0001, "non-numeric cosine should fall back to default 0.40")
	// Other weights untouched.
	require.InDelta(t, 0.30, w.BM25, 0.0001)
	require.InDelta(t, 0.15, w.Recency, 0.0001)
	require.InDelta(t, 0.15, w.Precision, 0.0001)
}

// --- H7: TemporalWeights env-var overrides ---

// setEnvTemporalWeights sets all four ENGRAM_W_TEMPORAL_* env vars and returns
// a cleanup that unsets them and resets the cached config.
func setEnvTemporalWeights(t *testing.T, cosine, bm25, recency, precision string) {
	t.Helper()
	if cosine != "" {
		t.Setenv("ENGRAM_W_TEMPORAL_COSINE", cosine)
	}
	if bm25 != "" {
		t.Setenv("ENGRAM_W_TEMPORAL_BM25", bm25)
	}
	if recency != "" {
		t.Setenv("ENGRAM_W_TEMPORAL_RECENCY", recency)
	}
	if precision != "" {
		t.Setenv("ENGRAM_W_TEMPORAL_PRECISION", precision)
	}
	resetTemporalWeightConfigForTesting()
	t.Cleanup(resetTemporalWeightConfigForTesting)
}

// setEnvKUWeights sets all four ENGRAM_W_KU_* env vars and returns a cleanup
// that unsets them and resets the cached config.
func setEnvKUWeights(t *testing.T, cosine, bm25, recency, precision string) {
	t.Helper()
	if cosine != "" {
		t.Setenv("ENGRAM_W_KU_COSINE", cosine)
	}
	if bm25 != "" {
		t.Setenv("ENGRAM_W_KU_BM25", bm25)
	}
	if recency != "" {
		t.Setenv("ENGRAM_W_KU_RECENCY", recency)
	}
	if precision != "" {
		t.Setenv("ENGRAM_W_KU_PRECISION", precision)
	}
	resetKUWeightConfigForTesting()
	t.Cleanup(resetKUWeightConfigForTesting)
}

// TestTemporalWeights_EnvUnset verifies that with no env vars set TemporalWeights
// returns the compile-time temporal constants.
func TestTemporalWeights_EnvUnset(t *testing.T) {
	resetTemporalWeightConfigForTesting()
	t.Cleanup(resetTemporalWeightConfigForTesting)

	w := TemporalWeights()
	require.InDelta(t, 0.35, w.Vector, 0.0001, "temporal Vector default")
	require.InDelta(t, 0.25, w.BM25, 0.0001, "temporal BM25 default")
	require.InDelta(t, 0.30, w.Recency, 0.0001, "temporal Recency default")
	require.InDelta(t, 0.10, w.Precision, 0.0001, "temporal Precision default")
}

// TestTemporalWeights_H7 verifies that ENGRAM_W_TEMPORAL_RECENCY=0.45 is reflected
// in TemporalWeights while the other three fall back to their compile-time constants.
func TestTemporalWeights_H7(t *testing.T) {
	setEnvTemporalWeights(t, "", "", "0.45", "")

	w := TemporalWeights()
	require.InDelta(t, 0.35, w.Vector, 0.0001, "temporal Vector should fall back to 0.35")
	require.InDelta(t, 0.25, w.BM25, 0.0001, "temporal BM25 should fall back to 0.25")
	require.InDelta(t, 0.45, w.Recency, 0.0001, "temporal Recency override should be 0.45")
	require.InDelta(t, 0.10, w.Precision, 0.0001, "temporal Precision should fall back to 0.10")
}

// TestTemporalWeights_InvalidOutOfRange verifies that an out-of-range temporal
// override (2.0) falls back to the compile-time constant without panicking.
func TestTemporalWeights_InvalidOutOfRange(t *testing.T) {
	setEnvTemporalWeights(t, "2.0", "", "", "")

	w := TemporalWeights()
	require.InDelta(t, 0.35, w.Vector, 0.0001, "out-of-range temporal cosine should fall back to 0.35")
}

// TestKnowledgeUpdateWeights_EnvUnset verifies that with no env vars set
// KnowledgeUpdateWeights returns the compile-time KU constants.
func TestKnowledgeUpdateWeights_EnvUnset(t *testing.T) {
	resetKUWeightConfigForTesting()
	t.Cleanup(resetKUWeightConfigForTesting)

	w := KnowledgeUpdateWeights()
	require.InDelta(t, 0.38, w.Vector, 0.0001, "KU Vector default")
	require.InDelta(t, 0.27, w.BM25, 0.0001, "KU BM25 default")
	require.InDelta(t, 0.25, w.Recency, 0.0001, "KU Recency default")
	require.InDelta(t, 0.13, w.Precision, 0.0001, "KU Precision default")
}

// TestKnowledgeUpdateWeights_H7 verifies that ENGRAM_W_KU_RECENCY=0.35 is
// reflected in KnowledgeUpdateWeights while the other three fall back.
func TestKnowledgeUpdateWeights_H7(t *testing.T) {
	setEnvKUWeights(t, "", "", "0.35", "")

	w := KnowledgeUpdateWeights()
	require.InDelta(t, 0.38, w.Vector, 0.0001, "KU Vector should fall back to 0.38")
	require.InDelta(t, 0.27, w.BM25, 0.0001, "KU BM25 should fall back to 0.27")
	require.InDelta(t, 0.35, w.Recency, 0.0001, "KU Recency override should be 0.35")
	require.InDelta(t, 0.13, w.Precision, 0.0001, "KU Precision should fall back to 0.13")
}

// TestKnowledgeUpdateWeights_InvalidNonNumeric verifies that a non-numeric KU
// override falls back to the compile-time constant without panicking.
func TestKnowledgeUpdateWeights_InvalidNonNumeric(t *testing.T) {
	setEnvKUWeights(t, "bogus", "", "", "")

	w := KnowledgeUpdateWeights()
	require.InDelta(t, 0.38, w.Vector, 0.0001, "non-numeric KU cosine should fall back to 0.38")
	require.InDelta(t, 0.27, w.BM25, 0.0001)
	require.InDelta(t, 0.25, w.Recency, 0.0001)
	require.InDelta(t, 0.13, w.Precision, 0.0001)
}

// --- H6: existing DefaultWeights tests (unmodified above this line) ---

// TestCompositeScore_H6WeightsRankDifferently verifies that H6 weights (higher BM25,
// lower cosine) change the composite ranking for a synthetic pair where one memory
// has high BM25 / low cosine and the other has low BM25 / high cosine.
//
// With default weights (cosine=0.40, bm25=0.30):
//
//	lexical  = 0.40*0.20 + 0.30*0.90 = 0.08 + 0.27 = 0.35  (higher)
//	semantic = 0.40*0.90 + 0.30*0.20 = 0.36 + 0.06 = 0.42  (lower — wait, semantic wins by default)
//
// With H6 weights (cosine=0.30, bm25=0.40):
//
//	lexical  = 0.30*0.20 + 0.40*0.90 = 0.06 + 0.36 = 0.42
//	semantic = 0.30*0.90 + 0.40*0.20 = 0.27 + 0.08 = 0.35
//
// So under H6 the lexical-heavy memory wins; under defaults the semantic-heavy one wins.
func TestCompositeScore_H6WeightsRankDifferently(t *testing.T) {
	lexicalHeavy := ScoreInput{Cosine: 0.20, BM25: 0.90, HoursSince: 0, Importance: 2}
	semanticHeavy := ScoreInput{Cosine: 0.90, BM25: 0.20, HoursSince: 0, Importance: 2}

	// --- Default weights: semantic-heavy should outscore lexical-heavy ---
	{
		resetWeightConfigForTesting()
		t.Cleanup(resetWeightConfigForTesting)

		defaultW := DefaultWeights()
		scoreLexical := CompositeScoreWithWeights(lexicalHeavy, defaultW)
		scoreSemantic := CompositeScoreWithWeights(semanticHeavy, defaultW)
		require.Greater(t, scoreSemantic, scoreLexical,
			"with defaults: semantic-heavy (cosine=0.90) should outscore lexical-heavy (bm25=0.90)")
	}

	// --- H6 weights: lexical-heavy should now outscore semantic-heavy ---
	{
		setEnvWeights(t, "0.30", "0.40", "", "")

		h6W := DefaultWeights()
		scoreLexical := CompositeScoreWithWeights(lexicalHeavy, h6W)
		scoreSemantic := CompositeScoreWithWeights(semanticHeavy, h6W)
		require.Greater(t, scoreLexical, scoreSemantic,
			"with H6 weights: lexical-heavy (bm25=0.90) should outscore semantic-heavy (cosine=0.90)")
	}
}
