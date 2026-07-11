// Package aggq holds the fleet's CANONICAL multi-fact composition predicate.
// Multi-fact composition questions require combining multiple stored facts,
// including counts, sums, differences, and other derived values. Issue #1345's
// 12/12 analysis established this framing. Duramind mirrors the predicate
// verbatim, and both repositories pin parity with the frozen composition vector
// file.
//
// The predicate is shared by the longmemeval harness and the Layer B
// deterministic-aggregation post-pass.
// It is a leaf package (stdlib-only) specifically so that internal/layerb can
// use these helpers without importing internal/longmemeval, whose engram
// client pulls in internal/search and internal/db — the exact import cycle
// db → layerb → longmemeval → search → db that this package exists to break.
package aggq

import (
	"regexp"
	"strings"
)

// StopWords is a small set of English function words that carry no domain
// signal. Callers strip these when building a subject anchor so the remaining
// tokens are more likely to match the gold session via BM25.
var StopWords = map[string]bool{
	"a": true, "an": true, "the": true, "for": true, "on": true,
	"in": true, "of": true, "to": true, "and": true, "or": true,
	"about": true, "with": true, "at": true, "by": true, "from": true,
	"near": true, "around": true, "into": true, "through": true,
	"i": true, "me": true, "my": true, "you": true, "your": true,
	"do": true, "is": true, "are": true, "some": true, "any": true,
	"can": true, "could": true, "would": true, "should": true,
	"what": true, "which": true, "how": true, "when": true, "where": true,
	"recommend": true, "suggest": true, "advise": true,
	"like": true, "likes": true, "love": true, "loves": true,
	"enjoy": true, "enjoys": true, "prefer": true, "prefers": true,
	"favorite": true, "favourite": true, "avoid": true, "avoids": true,
}

// aggregationRe (H8) matches exhaustive retrieval questions that need all
// matching sessions, including list/every-time phrasing.
var aggregationRe = regexp.MustCompile(
	`(?i)\b(how many(?: times)?|how often|how much total|total number of|sum of|count of|list (?:all|every|everything)|every time|all occasions?)\b`)

// temporalQuantityRe excludes relative-time arithmetic questions like
// "how many days ago", which are temporal reasoning rather than aggregation.
var temporalQuantityRe = regexp.MustCompile(
	`(?i)\bhow many (days?|weeks?|months?|years?) (ago|before|after)\b`)

// monetaryAggRe (2026-06-21) catches "how much" questions that require summing
// or differencing values across sessions. A plain "how much does X cost" price
// lookup carries none of these verbs and is intentionally NOT matched.
var monetaryAggRe = regexp.MustCompile(
	`(?i)\bhow much\b.*\b(save|saved|spend|spent|raised?|earn|earned|in total|altogether)\b`)

// aggregationStripRe (H8) strips the counting interrogative phrase so that the
// remaining tokens describe the object being counted.
var aggregationStripRe = regexp.MustCompile(
	`(?i)^(how many (times )?|how often |how much total |what is the total (number of )?|what is the sum of |give me a count of |count of )`)

// IsMultiFactComposition reports whether answering question requires combining
// multiple stored facts.
func IsMultiFactComposition(question string) bool {
	if temporalQuantityRe.MatchString(question) {
		return false
	}
	return aggregationRe.MatchString(question) || monetaryAggRe.MatchString(question)
}

// IsAggregationQuestion reports whether question is a multi-fact composition.
// Deprecated: use IsMultiFactComposition.
func IsAggregationQuestion(question string) bool {
	return IsMultiFactComposition(question)
}

// ExtractAggregationAnchor (H8) strips the counting interrogative prefix and
// removes stop-words from the object noun phrase, producing a concise
// BM25-friendly query for the exhaustive sweep recall.
func ExtractAggregationAnchor(question string) string {
	stripped := aggregationStripRe.ReplaceAllString(question, "")
	stripped = strings.TrimRight(stripped, "?!.,;:")
	if stripped == "" {
		stripped = question
	}
	tokens := strings.Fields(stripped)
	var keep []string
	for _, tok := range tokens {
		lower := strings.ToLower(strings.TrimRight(tok, "?!.,;:"))
		if !StopWords[lower] {
			keep = append(keep, tok)
		}
	}
	if len(keep) == 0 {
		return stripped
	}
	return strings.Join(keep, " ")
}
