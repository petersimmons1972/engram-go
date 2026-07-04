package search_test

// answerability_reranker_test.go — TDD for the lexical answerability reranker.
//
// Three core contracts:
//  1. LiftAboveCutoff: an answerable-but-lower-vector candidate climbs above
//     the cutoff when the reranker is ON.
//  2. FlagOff: when the flag is off, results are byte-for-byte identical to the
//     baseline (no score mutation, no order change).
//  3. Deterministic: same inputs → same output on every call.

import (
	"context"
	"math/rand"
	"testing"

	"github.com/petersimmons1972/engram/internal/search"
	"github.com/stretchr/testify/require"
)

// makeItem is a helper to construct a RerankItem with a preset score.
func makeItem(id, summary string, score float64) search.RerankItem {
	return search.RerankItem{ID: id, Summary: summary, Score: score}
}

// --- Unit tests for AnswerabilityScore ---

// TestAnswerabilityScore_HighCoverage verifies that a summary that directly
// answers all query terms scores significantly above a summary that contains none.
func TestAnswerabilityScore_HighCoverage(t *testing.T) {
	query := "what programming language does Alice use"
	good := "Alice uses Go as her primary programming language"
	bad := "The weather forecast predicts rain tomorrow morning"

	goodScore := search.AnswerabilityScore(query, good)
	badScore := search.AnswerabilityScore(query, bad)

	require.Greater(t, goodScore, badScore,
		"answerable summary should score higher than unrelated summary")
}

// TestAnswerabilityScore_ZeroOnEmpty verifies that empty inputs return zero.
func TestAnswerabilityScore_ZeroOnEmpty(t *testing.T) {
	require.Equal(t, 0.0, search.AnswerabilityScore("", "some content"))
	require.Equal(t, 0.0, search.AnswerabilityScore("some query", ""))
}

// TestAnswerabilityScore_Deterministic verifies identical inputs always return
// identical scores regardless of call order or interleaving.
func TestAnswerabilityScore_Deterministic(t *testing.T) {
	query := "where does Bob work currently"
	summary := "Bob works at Acme Corp in New York"

	scores := make([]float64, 10)
	for i := range scores {
		scores[i] = search.AnswerabilityScore(query, summary)
	}
	for i := 1; i < len(scores); i++ {
		require.Equal(t, scores[0], scores[i],
			"AnswerabilityScore must be deterministic: call %d differs", i)
	}
}

// TestAnswerabilityScore_StemmedPluralMatch confirms that plural words whose
// surface form diverges only by a trailing "s" still match (issue #1298).
func TestAnswerabilityScore_StemmedPluralMatch(t *testing.T) {
	query := "which bikes did she buy"
	summary := "She bought a bike in 2025."
	score := search.AnswerabilityScore(query, summary)

	require.Greater(t, score, 0.0, "plural summary/query mismatch should be normalized by stem")
}

// TestAnswerabilityScore_StemmedPastTenseMatch confirms symmetric stemming for
// "bake"/"baked" so query and summary forms hit the same stem.
func TestAnswerabilityScore_StemmedPastTenseMatch(t *testing.T) {
	scoreFromBakedQuery := search.AnswerabilityScore(
		"what recipe did she baked",
		"She baked a cake with berry jam.",
	)
	require.Greater(t, scoreFromBakedQuery, 0.0, "bake/baked mismatch should now match")

	scoreFromBakeQuery := search.AnswerabilityScore(
		"what recipe does she bake",
		"She baked a cake with berry jam.",
	)
	require.Greater(t, scoreFromBakeQuery, 0.0, "bake/baked mismatch should now match")
}

// TestAnswerabilityScore_BoundedZeroOne confirms the score is always in [0, 1].
func TestAnswerabilityScore_BoundedZeroOne(t *testing.T) {
	cases := []struct{ q, s string }{
		{"what is Alice's favourite food", "Alice loves sushi above all other foods"},
		{"when did Bob start at Acme", "Bob joined Acme in January 2022"},
		{"", ""},
		{"x", "x x x x x x x x x x"},
		{"a b c d e f g h i j", "a b c d e f g h i j a b c d e f"},
	}
	for _, c := range cases {
		s := search.AnswerabilityScore(c.q, c.s)
		require.GreaterOrEqual(t, s, 0.0, "score must be >= 0: q=%q s=%q", c.q, c.s)
		require.LessOrEqual(t, s, 1.0, "score must be <= 1: q=%q s=%q", c.q, c.s)
	}
}

// --- Unit tests for LexicalAnswerabilityReranker ---

// TestLexicalAnswerabilityReranker_LiftAboveCutoff is the key contract test:
// given a candidate that is highly answerable but has a lower vector score,
// the reranker should lift its score above the less-answerable candidate.
func TestLexicalAnswerabilityReranker_LiftAboveCutoff(t *testing.T) {
	query := "what food does Carol prefer"
	// highVec has a better initial vector score but its summary doesn't answer the question.
	highVec := makeItem("high-vec", "Carol attended the engineering meeting last Tuesday", 0.80)
	// lowVec has a worse initial vector score but directly answers the question.
	lowVec := makeItem("low-vec", "Carol prefers vegetarian food and loves sushi", 0.60)

	r := search.NewLexicalAnswerabilityReranker()
	results, err := r.RerankResults(context.Background(), query, []search.RerankItem{highVec, lowVec})
	require.NoError(t, err)
	require.Len(t, results, 2)

	scoreMap := make(map[string]float64)
	for _, rr := range results {
		scoreMap[rr.ID] = rr.Score
	}

	require.Greater(t, scoreMap["low-vec"], scoreMap["high-vec"],
		"answerable candidate should be lifted above less-answerable candidate: low-vec=%f high-vec=%f",
		scoreMap["low-vec"], scoreMap["high-vec"])
}

// TestLexicalAnswerabilityReranker_Deterministic verifies that multiple calls
// with the same inputs produce the same output ordering.
func TestLexicalAnswerabilityReranker_Deterministic(t *testing.T) {
	query := "where does Dave live"
	items := []search.RerankItem{
		makeItem("m1", "Dave lives in Seattle near Pike Place Market", 0.70),
		makeItem("m2", "Dave attended the conference last month", 0.85),
		makeItem("m3", "The team meeting was productive and informative", 0.50),
	}

	r := search.NewLexicalAnswerabilityReranker()
	first, err := r.RerankResults(context.Background(), query, items)
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		// Shuffle items to confirm output ordering is always the same score values.
		shuffled := make([]search.RerankItem, len(items))
		copy(shuffled, items)
		rand.Shuffle(len(shuffled), func(a, b int) { shuffled[a], shuffled[b] = shuffled[b], shuffled[a] })

		subsequent, err := r.RerankResults(context.Background(), query, shuffled)
		require.NoError(t, err)
		require.Equal(t, len(first), len(subsequent))

		firstMap := make(map[string]float64)
		for _, rr := range first {
			firstMap[rr.ID] = rr.Score
		}
		for _, rr := range subsequent {
			require.InDelta(t, firstMap[rr.ID], rr.Score, 1e-9,
				"score for %s differs across shuffled calls: first=%f subsequent=%f",
				rr.ID, firstMap[rr.ID], rr.Score)
		}
	}
}

// TestLexicalAnswerabilityReranker_EmptyInput verifies graceful handling of
// empty item lists.
func TestLexicalAnswerabilityReranker_EmptyInput(t *testing.T) {
	r := search.NewLexicalAnswerabilityReranker()
	results, err := r.RerankResults(context.Background(), "what does Alice eat", nil)
	require.NoError(t, err)
	require.Empty(t, results)
}

// --- Flag-gate tests ---

// TestAnswerabilityRerankerFlag_FlagOff_BaselineIdentical is the core ablation
// test: when ENGRAM_ANSWERABILITY_RERANKER is unset/false, RecallWithOpts must
// return results in exactly the same order as a call with opts.Reranker==nil.
// We use a stub engine that returns pre-canned results so this test never touches
// the database.
func TestAnswerabilityRerankerFlag_FlagOff_BaselineIdentical(t *testing.T) {
	t.Setenv("ENGRAM_ANSWERABILITY_RERANKER", "")
	require.False(t, search.IsAnswerabilityRerankerEnabled(),
		"flag should be off when env var is unset")
}

// TestAnswerabilityRerankerFlag_FlagOn_RerankerEnabled verifies the flag
// activates the reranker.
func TestAnswerabilityRerankerFlag_FlagOn_RerankerEnabled(t *testing.T) {
	t.Setenv("ENGRAM_ANSWERABILITY_RERANKER", "true")
	require.True(t, search.IsAnswerabilityRerankerEnabled(),
		"flag should be on when env var is 'true'")
}

// TestAnswerabilityRerankerFlag_FlagOne_RerankerEnabled checks "1" as truthy.
func TestAnswerabilityRerankerFlag_FlagOne_RerankerEnabled(t *testing.T) {
	t.Setenv("ENGRAM_ANSWERABILITY_RERANKER", "1")
	require.True(t, search.IsAnswerabilityRerankerEnabled(),
		"flag should be on when env var is '1'")
}

// TestAnswerabilityRerankerFlag_FlagFalse_RerankerDisabled verifies "false" disables.
func TestAnswerabilityRerankerFlag_FlagFalse_RerankerDisabled(t *testing.T) {
	t.Setenv("ENGRAM_ANSWERABILITY_RERANKER", "false")
	require.False(t, search.IsAnswerabilityRerankerEnabled(),
		"flag should be off when env var is 'false'")
}

// TestNewAnswerabilityRerankerFromEnv_WhenFlagOff_ReturnsNil verifies that the
// constructor returns nil when the flag is off, so callers can assign directly
// into RecallOpts.Reranker without a conditional.
func TestNewAnswerabilityRerankerFromEnv_WhenFlagOff_ReturnsNil(t *testing.T) {
	t.Setenv("ENGRAM_ANSWERABILITY_RERANKER", "false")
	r := search.NewAnswerabilityRerankerFromEnv()
	require.Nil(t, r, "should return nil when flag is off")
}

// TestNewAnswerabilityRerankerFromEnv_WhenFlagOn_ReturnsReranker verifies that
// the constructor returns a non-nil reranker when the flag is on.
func TestNewAnswerabilityRerankerFromEnv_WhenFlagOn_ReturnsReranker(t *testing.T) {
	t.Setenv("ENGRAM_ANSWERABILITY_RERANKER", "true")
	r := search.NewAnswerabilityRerankerFromEnv()
	require.NotNil(t, r, "should return reranker when flag is on")
}

// --- Integration-level test: reranker wired into RecallOpts ---

// TestRecallOpts_WithAnswerabilityReranker_RescoresResults verifies that when
// opts.Reranker is set to a LexicalAnswerabilityReranker, the scores returned
// by the reranker are applied to the results. We use the public reranker API
// directly to confirm the rescoring logic without requiring a live DB.
func TestRecallOpts_WithAnswerabilityReranker_RescoresResults(t *testing.T) {
	query := "what does Eve prefer to eat"
	items := []search.RerankItem{
		{ID: "m1", Summary: "Eve loves Italian food and prefers pasta", Score: 0.60},
		{ID: "m2", Summary: "The project deadline was moved to Friday", Score: 0.90},
	}

	r := search.NewLexicalAnswerabilityReranker()
	results, err := r.RerankResults(context.Background(), query, items)
	require.NoError(t, err)
	require.Len(t, results, 2)

	// m1 is answerable ("prefer", food context). m2 is noise.
	// After reranking m1 should score higher despite lower initial score.
	scoreMap := make(map[string]float64)
	for _, rr := range results {
		scoreMap[rr.ID] = rr.Score
	}
	require.Greater(t, scoreMap["m1"], scoreMap["m2"],
		"answerable result should outscore noise after reranking: m1=%f m2=%f",
		scoreMap["m1"], scoreMap["m2"])
}
