package search_test

// LME Phase 3 — evidence-first packing and exact-signal scoring helpers.
//
// Extracted from cmd/longmemeval/run.go so the MCP recall tool can apply the
// same context-ordering logic without importing the benchmark binary.
//
// The env flag ENGRAM_EVIDENCE_FIRST_PACK=true enables server-wide evidence-first
// ordering; the per-call argument evidence_first_pack=true overrides per-request.
// Default OFF — no behavior change until explicitly enabled.

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// ── ExtractExactSignals ──────────────────────────────────────────────────────

// TestExtractExactSignals_URLs verifies URL extraction from query text.
func TestExtractExactSignals_URLs(t *testing.T) {
	sigs := search.ExtractExactSignals("visit https://example.com for details")
	require.Contains(t, sigs, "https://example.com",
		"URL must be extracted as an exact signal")
}

// TestExtractExactSignals_PhoneNumbers verifies phone number extraction.
func TestExtractExactSignals_PhoneNumbers(t *testing.T) {
	sigs := search.ExtractExactSignals("call 650-555-9999 now")
	require.NotEmpty(t, sigs, "phone number must be extracted as an exact signal")
}

// TestExtractExactSignals_QuotedPhrases verifies quoted-phrase extraction.
func TestExtractExactSignals_QuotedPhrases(t *testing.T) {
	sigs := search.ExtractExactSignals(`remember "project alpha" deadline`)
	require.Contains(t, sigs, "project alpha",
		"quoted phrase must be extracted as an exact signal (without quotes)")
}

// TestExtractExactSignals_NoSignals returns empty slice for generic query.
func TestExtractExactSignals_NoSignals(t *testing.T) {
	sigs := search.ExtractExactSignals("what did I do last week")
	require.Empty(t, sigs, "generic query without identifiers must return no exact signals")
}

// ── ScoreExactSignals ────────────────────────────────────────────────────────

// TestScoreExactSignals_HitReturnsPositive verifies that content containing
// a query exact signal scores > 0.
func TestScoreExactSignals_HitReturnsPositive(t *testing.T) {
	score := search.ScoreExactSignals(
		"I visited https://example.com last Tuesday",
		"visit https://example.com for details",
	)
	require.Greater(t, score, 0,
		"content containing the query URL must score above zero")
}

// TestScoreExactSignals_MissReturnsZero verifies that content with no matching
// exact signals scores 0.
func TestScoreExactSignals_MissReturnsZero(t *testing.T) {
	score := search.ScoreExactSignals(
		"generic memory about weather and food",
		"visit https://example.com for details",
	)
	require.Equal(t, 0, score,
		"content with no matching signal must score zero")
}

// TestScoreExactSignals_MultipleSignalsAccumulate verifies that multiple hits
// produce a higher score than a single hit.
func TestScoreExactSignals_MultipleSignalsAccumulate(t *testing.T) {
	query := `visit https://example.com and call "project alpha" team`
	contentSingle := "went to https://example.com"
	contentBoth := `went to https://example.com regarding "project alpha"`

	scoreSingle := search.ScoreExactSignals(contentSingle, query)
	scoreBoth := search.ScoreExactSignals(contentBoth, query)

	require.Greater(t, scoreBoth, scoreSingle,
		"content with more signal matches must score higher")
}

// ── OrderResultsEvidenceFirst ────────────────────────────────────────────────

// TestOrderResultsEvidenceFirst_PlacesHitsFirst verifies that results whose
// memory content contains an exact signal from the query are moved to the front.
func TestOrderResultsEvidenceFirst_PlacesHitsFirst(t *testing.T) {
	q := "visit https://example.com"

	hit := newSearchResult("id-hit", "I visited https://example.com last week")
	miss := newSearchResult("id-miss", "I went to the grocery store")

	// Put miss first so we can see the reordering.
	results := []types.SearchResult{miss, hit}
	ordered := search.OrderResultsEvidenceFirst(results, q)

	require.Equal(t, 2, len(ordered), "all results must be returned")
	require.Equal(t, "id-hit", ordered[0].Memory.ID,
		"result containing the exact signal must come first")
	require.Equal(t, "id-miss", ordered[1].Memory.ID,
		"non-matching result must be placed after the match")
}

// TestOrderResultsEvidenceFirst_StableWhenNoSignals verifies that when the
// query has no exact signals, the result order is unchanged.
func TestOrderResultsEvidenceFirst_StableWhenNoSignals(t *testing.T) {
	q := "what did I eat for dinner"
	a := newSearchResult("id-a", "had pizza")
	b := newSearchResult("id-b", "had salad")
	results := []types.SearchResult{a, b}
	ordered := search.OrderResultsEvidenceFirst(results, q)

	require.Equal(t, "id-a", ordered[0].Memory.ID,
		"stable sort: original order must be preserved when no signals match")
	require.Equal(t, "id-b", ordered[1].Memory.ID)
}

// TestOrderResultsEvidenceFirst_NilMemorySkipped verifies nil Memory entries
// are not panicked on and are preserved in place.
func TestOrderResultsEvidenceFirst_NilMemorySkipped(t *testing.T) {
	q := "visit https://example.com"
	nilEntry := types.SearchResult{Score: 0.5, Memory: nil}
	hit := newSearchResult("id-hit", "I visited https://example.com")
	results := []types.SearchResult{nilEntry, hit}
	require.NotPanics(t, func() {
		search.OrderResultsEvidenceFirst(results, q)
	}, "nil Memory entries must not panic")
}

// ── EvidenceFirstPack RecallOpts flag ────────────────────────────────────────

// TestEvidenceFirstPackDefault_OffByDefault verifies the field exists and
// defaults to false (ablation contract).
func TestEvidenceFirstPackDefault_OffByDefault(t *testing.T) {
	var opts search.RecallOpts
	require.False(t, opts.EvidenceFirstPack,
		"EvidenceFirstPack must default to false (flag-gated, default OFF)")
}

// ── helpers ──────────────────────────────────────────────────────────────────

func newSearchResult(id, content string) types.SearchResult {
	m := &types.Memory{
		ID:      id,
		Content: content,
	}
	return types.SearchResult{Memory: m, Score: 0.5}
}
