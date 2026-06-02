package search_test

// LME Experiment #4 — exact-fact / entity-identifier scoring boost.
// Tests written BEFORE implementation (TDD). All tests must fail until
// the implementation is wired in score.go, query_signal.go, and engine.go.
//
// Issue #938 improvement #3.

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/search"
	"github.com/stretchr/testify/require"
)

// ── Identifier detection (query_signal.go) ───────────────────────────────────

// TestIsIdentifierQuery_DetectsURLs verifies that URLs in a query signal
// high-precision identifier recall — not just semantic similarity.
func TestIsIdentifierQuery_DetectsURLs(t *testing.T) {
	require.True(t, search.IsIdentifierQuery("what is https://example.com"),
		"URL in query must be detected as identifier query")
	require.True(t, search.IsIdentifierQuery("visit http://foo.bar/path"),
		"http URL must be detected as identifier query")
}

// TestIsIdentifierQuery_DetectsPhoneNumbers verifies that phone-number patterns
// trigger identifier detection.
func TestIsIdentifierQuery_DetectsPhoneNumbers(t *testing.T) {
	require.True(t, search.IsIdentifierQuery("call +1-800-555-1234"),
		"phone number with country code must be detected")
	require.True(t, search.IsIdentifierQuery("phone (415) 555-0123"),
		"parenthesized area code must be detected")
	require.True(t, search.IsIdentifierQuery("reach me at 650-555-9999"),
		"plain dash-format phone must be detected")
}

// TestIsIdentifierQuery_DetectsProperNouns verifies that a query containing
// at least two consecutive Title-Cased words is treated as an identifier query.
// Single capitalised words (start-of-sentence) are NOT flagged.
func TestIsIdentifierQuery_DetectsProperNouns(t *testing.T) {
	require.True(t, search.IsIdentifierQuery("who is John Smith"),
		"two consecutive title-case tokens must be detected as named entity")
	require.True(t, search.IsIdentifierQuery("where does Alice Johnson work"),
		"named entity (first + last name) must trigger identifier mode")
	require.False(t, search.IsIdentifierQuery("who is the president"),
		"single-capitalised-word-only query must NOT trigger identifier mode")
}

// TestIsIdentifierQuery_FalseNegativeSafety verifies that generic prose queries
// are not misclassified as identifier queries.
func TestIsIdentifierQuery_FalseNegativeSafety(t *testing.T) {
	require.False(t, search.IsIdentifierQuery("what did I do yesterday"),
		"plain prose query must not be flagged as identifier query")
	require.False(t, search.IsIdentifierQuery("tell me about my preferences"),
		"preference query must not be flagged as identifier query")
}

// ── Content hit detection (query_signal.go) ──────────────────────────────────

// TestExactIdentifierHit_ContentContainsQueryToken verifies that when a query
// token appears verbatim (case-insensitive) in memory content, the function
// returns true.
func TestExactIdentifierHit_ContentContainsQueryToken(t *testing.T) {
	require.True(t,
		search.ExactIdentifierHit("John Smith called about the contract", "who is John Smith"),
		"verbatim name in content must produce a hit")
	require.True(t,
		search.ExactIdentifierHit("visit https://example.com for details", "what is https://example.com"),
		"verbatim URL in content must produce a hit")
}

// TestExactIdentifierHit_NegativeCase verifies no false-positive when the
// content does not contain the query's identifier tokens.
func TestExactIdentifierHit_NegativeCase(t *testing.T) {
	require.False(t,
		search.ExactIdentifierHit("Alice Johnson is unavailable", "where is John Smith"),
		"different name in content must not produce a hit")
}

// ── Scoring integration: boost applied / not applied ─────────────────────────

// TestCompositeScore_ExactIdentifierBoost_Applied verifies that a memory where
// the ExactIdentifierMatch flag is set scores strictly higher than the same
// memory without the flag, using default weights.
func TestCompositeScore_ExactIdentifierBoost_Applied(t *testing.T) {
	w := search.DefaultWeights()

	base := search.ScoreInput{
		Cosine:     0.55,
		BM25:       0.40,
		HoursSince: 24.0,
		Importance: 2,
	}
	withBoost := base
	withBoost.ExactIdentifierMatch = true

	scoreBase := search.CompositeScoreWithWeights(base, w)
	scoreBoosted := search.CompositeScoreWithWeights(withBoost, w)

	require.Greater(t, scoreBoosted, scoreBase,
		"ExactIdentifierMatch=true must increase the composite score")
}

// TestCompositeScore_ExactIdentifierBoost_AboveNearVectorMatch verifies the
// primary LME #938 target: an exact-name match with only modest vector
// similarity must outrank a near-vector match that does NOT contain the
// identifier, simulating the case where vector similarity ranks the wrong
// memory above the correct named-entity memory.
func TestCompositeScore_ExactIdentifierBoost_AboveNearVectorMatch(t *testing.T) {
	w := search.DefaultWeights()

	// "Wrong" memory: semantically close (high cosine) but no identifier match.
	wrongMem := search.ScoreInput{
		Cosine:               0.82,
		BM25:                 0.50,
		HoursSince:           12.0,
		Importance:           2,
		ExactIdentifierMatch: false,
	}

	// "Right" memory: exact name match, but vector similarity only moderate.
	rightMem := search.ScoreInput{
		Cosine:               0.60,
		BM25:                 0.55,
		HoursSince:           12.0,
		Importance:           2,
		ExactIdentifierMatch: true,
	}

	scoreWrong := search.CompositeScoreWithWeights(wrongMem, w)
	scoreRight := search.CompositeScoreWithWeights(rightMem, w)

	require.Greater(t, scoreRight, scoreWrong,
		"exact identifier match must outrank high-cosine non-match (LME #938 target)")
}

// TestCompositeScore_ExactIdentifierBoost_FlagOff_BaselineIdentical verifies
// the ablation contract: when ExactIdentifierMatch is false, the score is
// identical to the pre-flag baseline — no score change introduced by the new
// field when it is not set.
func TestCompositeScore_ExactIdentifierBoost_FlagOff_BaselineIdentical(t *testing.T) {
	w := search.DefaultWeights()

	in := search.ScoreInput{
		Cosine:               0.70,
		BM25:                 0.60,
		HoursSince:           5.0,
		Importance:           2,
		ExactIdentifierMatch: false,
	}

	scoreViaWeights := search.CompositeScoreWithWeights(in, w)
	scoreViaDefault := search.CompositeScore(in)

	require.InDelta(t, scoreViaDefault, scoreViaWeights, 1e-9,
		"when ExactIdentifierMatch is false, CompositeScoreWithWeights must be unchanged")
}

// TestCompositeScore_ExactIdentifierBoost_NonIdentifierOrdering_Unchanged
// verifies that for generic memories (neither flagged as ExactIdentifierMatch),
// relative ordering is undisturbed.
func TestCompositeScore_ExactIdentifierBoost_NonIdentifierOrdering_Unchanged(t *testing.T) {
	w := search.DefaultWeights()

	memA := search.ScoreInput{Cosine: 0.9, BM25: 0.8, HoursSince: 0, Importance: 2}
	memB := search.ScoreInput{Cosine: 0.4, BM25: 0.3, HoursSince: 0, Importance: 2}

	scoreA := search.CompositeScoreWithWeights(memA, w)
	scoreB := search.CompositeScoreWithWeights(memB, w)

	require.Greater(t, scoreA, scoreB,
		"non-identifier query ordering must not be disrupted by ExactIdentifierMatch path")
}

// TestCompositeScoreRRF_ExactIdentifierBoost_Applied verifies the boost also
// applies to the RRF composite path.
func TestCompositeScoreRRF_ExactIdentifierBoost_Applied(t *testing.T) {
	w := search.DefaultWeights()
	rrfBase := search.RRFScore(3, 4, 60)

	base := search.ScoreInput{
		HoursSince:           6.0,
		Importance:           2,
		ExactIdentifierMatch: false,
	}
	withBoost := base
	withBoost.ExactIdentifierMatch = true

	scoreBase := search.CompositeScoreRRF(base, w, rrfBase)
	scoreBoosted := search.CompositeScoreRRF(withBoost, w, rrfBase)

	require.Greater(t, scoreBoosted, scoreBase,
		"ExactIdentifierMatch boost must also apply via the RRF scoring path")
}

// TestExactFactBoostDisabled_RecallOptsFlag verifies that ExactFactBoost
// exists on RecallOpts and defaults to false (ablation: default OFF).
func TestExactFactBoostDisabled_RecallOptsFlag(t *testing.T) {
	var opts search.RecallOpts
	require.False(t, opts.ExactFactBoost,
		"ExactFactBoost must default to false (flag-gated, default OFF)")
}
