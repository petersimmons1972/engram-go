package search

// answerability_reranker.go — lexical answerability reranker (LME experiment #7).
//
// # Design
//
// The answerability reranker reorders retrieved candidates by how well each
// candidate's summary could *answer* the incoming query, rather than by raw
// embedding similarity. The scoring model is intentionally cheap and local —
// no network calls, no heavy dependency. It uses three lightweight lexical
// signals:
//
//  1. **Term coverage**: fraction of non-trivial query tokens that appear in
//     the candidate summary (case-insensitive). A summary that mentions the
//     entity and predicate of the question scores higher.
//
//  2. **Answer-bearing word boost**: summaries that contain common
//     answer-introducing patterns ("is", "was", "lives", "works", "prefers",
//     "uses", "has", numeric tokens) receive a small additive boost since
//     these patterns co-occur with answerable facts.
//
//  3. **Score fusion**: the final score blends the reranker's answerability
//     signal with the original vector score so that a modestly-answerable
//     candidate does not displace a very highly relevant one entirely.
//     Formula: blended = answerWeight*answerability + (1-answerWeight)*vectorNorm
//     where vectorNorm is each item's score normalized to [0,1] within the batch.
//
// # Flag
//
// Feature-gated by ENGRAM_ANSWERABILITY_RERANKER=true|1 (default OFF).
// Set RecallOpts.Reranker = NewAnswerabilityRerankerFromEnv() at the call site;
// when the flag is off the function returns nil and the hook is a no-op.
//
// # Ablation
//
// To measure delta on the LME benchmark:
//
//	ENGRAM_ANSWERABILITY_RERANKER=true ./longmemeval run \
//	  --data ~/path/to/longmemeval_m_cleaned.json \
//	  --url http://localhost:8788 \
//	  --llm-url "${LME_LLM_URL}" \
//	  --llm-model "${LME_LLM_MODEL}" \
//	  --workers 8 \
//	  --out ~/benchmarks/lme-exp7-reranker \
//	  --recall-topk 100 \
//	  --context-topk 8
//
// Run with a fresh run_id (never re-use run_id 7a87fd — that is the golden baseline).
// Compare score distribution against the golden snapshot at
// results/golden-snapshot-20260602T1810Z/ (367/566 CORRECT, run_id 7a87fd).

import (
	"context"
	"math"
	"os"
	"strings"
	"unicode"
)

// answerWeight controls how much the answerability signal blends into the
// final score relative to the original vector score. At 0.4 the reranker can
// lift a moderately answerable but lower-vector candidate above a high-vector
// but unanswerable candidate without fully discarding vector relevance.
const answerWeight = 0.40

// answerBearingVerbs are common English verbs and auxiliaries that frequently
// introduce factual statements. Their presence in a summary is a weak signal
// that the summary *states* something (as opposed to merely asking or describing).
var answerBearingVerbs = map[string]struct{}{
	"is": {}, "was": {}, "are": {}, "were": {}, "has": {}, "have": {}, "had": {},
	"works": {}, "worked": {}, "lives": {}, "lived": {}, "prefers": {}, "preferred": {},
	"uses": {}, "used": {}, "likes": {}, "liked": {}, "loves": {}, "loved": {},
	"hates": {}, "hated": {}, "eats": {}, "ate": {}, "drives": {}, "drove": {},
	"plays": {}, "played": {}, "enjoys": {}, "enjoyed": {},
}

// stopWords are high-frequency function words excluded from coverage scoring
// so that a summary padded with "the/a/an/of/in/to" doesn't inflate coverage.
var stopWords = map[string]struct{}{
	"the": {}, "a": {}, "an": {}, "of": {}, "in": {}, "to": {}, "for": {},
	"and": {}, "or": {}, "but": {}, "at": {}, "on": {}, "by": {}, "with": {},
	"is": {}, "was": {}, "are": {}, "were": {}, "be": {}, "been": {},
	"he": {}, "she": {}, "it": {}, "they": {}, "we": {}, "i": {},
	"his": {}, "her": {}, "its": {}, "their": {}, "our": {}, "my": {},
	"what": {}, "who": {}, "when": {}, "where": {}, "why": {}, "how": {},
	"does": {}, "do": {}, "did": {}, "has": {}, "have": {}, "had": {},
	"that": {}, "this": {}, "which": {}, "not": {}, "no": {},
}

// isNumeric returns true when s consists entirely of digits (e.g., years, counts).
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// tokenize splits text into lowercase, punctuation-stripped word tokens that
// are longer than one character, excluding stop words.
func tokenize(text string) []string {
	raw := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	out := make([]string, 0, len(raw))
	for _, tok := range raw {
		if len(tok) <= 1 {
			continue
		}
		if _, ok := stopWords[tok]; ok {
			continue
		}
		out = append(out, tok)
	}
	return out
}

// AnswerabilityScore returns a [0,1] score reflecting how likely the given
// summary can answer the given query. The score is a weighted combination of:
//
//   - Term coverage: fraction of non-trivial query tokens present in summary.
//   - Answer-bearing verb boost: additive bonus when summary contains common
//     factual verbs.
//
// Both query and summary are empty-guarded; either empty returns 0.0.
// The function is pure and deterministic — no state, no randomness.
func AnswerabilityScore(query, summary string) float64 {
	if query == "" || summary == "" {
		return 0.0
	}

	queryToks := tokenize(query)
	if len(queryToks) == 0 {
		return 0.0
	}

	summaryToks := tokenize(summary)
	if len(summaryToks) == 0 {
		return 0.0
	}

	// Build a set from summary tokens for O(1) lookup.
	summarySet := make(map[string]struct{}, len(summaryToks))
	for _, tok := range summaryToks {
		summarySet[tok] = struct{}{}
	}

	// Term coverage: fraction of query tokens that appear in summary.
	// Uses prefix-stem matching: a query token "prefer" matches summary token
	// "prefers", "preferred", etc. (query token is a prefix of summary token,
	// or summary token is a prefix of query token, min 4 chars to avoid noise).
	hits := 0
	for _, qTok := range queryToks {
		matched := false
		if _, ok := summarySet[qTok]; ok {
			matched = true
		} else if len(qTok) >= 4 {
			// Prefix-stem: check if any summary token has qTok as a prefix,
			// or if qTok has any summary token as a prefix.
			for sTok := range summarySet {
				if strings.HasPrefix(sTok, qTok) || (len(sTok) >= 4 && strings.HasPrefix(qTok, sTok)) {
					matched = true
					break
				}
			}
		}
		if matched {
			hits++
		}
	}
	coverage := float64(hits) / float64(len(queryToks))

	// Answer-bearing verb bonus: check the ORIGINAL (un-stop-word-filtered)
	// lowercase summary for answer-bearing verbs. We use the raw lowercase
	// token split here so stop-word filtering doesn't remove "is/was/has".
	rawSummaryToks := strings.Fields(strings.ToLower(summary))
	verbBonus := 0.0
	for _, tok := range rawSummaryToks {
		// Strip light punctuation.
		tok = strings.Trim(tok, ".,;:!?\"'()[]{}–—")
		if _, ok := answerBearingVerbs[tok]; ok {
			verbBonus = 0.15 // additive, capped — only counted once
			break
		}
		// Numeric token bonus: summaries with year/count literals are more
		// likely to be answerable for temporal and knowledge-update questions.
		if isNumeric(tok) {
			verbBonus = math.Max(verbBonus, 0.08)
		}
	}

	// Combine: coverage is the primary signal; verb bonus is additive.
	raw := coverage*(1.0-verbBonus) + verbBonus
	// Clamp to [0, 1] — defensive against floating-point edge cases.
	if raw < 0 {
		return 0.0
	}
	if raw > 1 {
		return 1.0
	}
	return raw
}

// LexicalAnswerabilityReranker implements ResultReranker using purely lexical
// answerability scoring. No external calls. Zero dependencies beyond stdlib.
// Instantiate via NewLexicalAnswerabilityReranker or NewAnswerabilityRerankerFromEnv.
type LexicalAnswerabilityReranker struct{}

// NewLexicalAnswerabilityReranker returns a new LexicalAnswerabilityReranker.
func NewLexicalAnswerabilityReranker() *LexicalAnswerabilityReranker {
	return &LexicalAnswerabilityReranker{}
}

// RerankResults reorders items by blending their answerability score (relative
// to query) with their original vector score. The blend formula is:
//
//	blended = answerWeight * answerability + (1-answerWeight) * vectorNorm
//
// where vectorNorm normalises each item's original Score to [0,1] within
// the batch (max-normalisation). This preserves relative ordering when all
// items are equally (un)answerable.
//
// The function never returns an error; the error return satisfies the
// ResultReranker interface.
func (r *LexicalAnswerabilityReranker) RerankResults(
	ctx context.Context,
	query string,
	items []RerankItem,
) ([]RerankResult, error) {
	if len(items) == 0 {
		return nil, nil
	}

	// Find max original score for normalisation.
	maxScore := 0.0
	for _, it := range items {
		if it.Score > maxScore {
			maxScore = it.Score
		}
	}

	results := make([]RerankResult, len(items))
	for i, it := range items {
		answerability := AnswerabilityScore(query, it.Summary)

		vectorNorm := 0.0
		if maxScore > 0 {
			vectorNorm = it.Score / maxScore
		}

		blended := answerWeight*answerability + (1.0-answerWeight)*vectorNorm
		// Clamp defensively.
		if blended < 0 {
			blended = 0
		}
		if blended > 1 {
			blended = 1
		}
		results[i] = RerankResult{ID: it.ID, Score: blended}
	}

	return results, nil
}

// Compile-time check: LexicalAnswerabilityReranker satisfies ResultReranker.
var _ ResultReranker = (*LexicalAnswerabilityReranker)(nil)

// IsAnswerabilityRerankerEnabled returns true when ENGRAM_ANSWERABILITY_RERANKER
// is set to "true" or "1" (case-insensitive). Default is OFF.
//
// This is evaluated at call time (not cached) so that t.Setenv works correctly
// in tests and the flag can be toggled at runtime without restart.
func IsAnswerabilityRerankerEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("ENGRAM_ANSWERABILITY_RERANKER")))
	return v == "true" || v == "1"
}

// NewAnswerabilityRerankerFromEnv returns a *LexicalAnswerabilityReranker when
// ENGRAM_ANSWERABILITY_RERANKER is true, or nil when it is off. Assign directly
// to RecallOpts.Reranker:
//
//	opts := search.RecallOpts{
//	    Reranker: search.NewAnswerabilityRerankerFromEnv(),
//	}
//
// When nil, RecallWithOpts skips re-ranking entirely — baseline unchanged.
func NewAnswerabilityRerankerFromEnv() ResultReranker {
	if !IsAnswerabilityRerankerEnabled() {
		return nil
	}
	return NewLexicalAnswerabilityReranker()
}
