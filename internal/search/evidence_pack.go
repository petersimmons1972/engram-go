// evidence_pack.go — evidence-first context-packing helpers.
//
// LME Phase 3: production MCP integration of the exact-signal retrieval
// improvements validated in cmd/longmemeval/run.go (issue #938).
//
// The core insight from LME sample-100 analysis: re-ordering the result
// context by the count of verbatim identifier matches (URLs, phone numbers,
// quoted phrases) before presenting to the LLM significantly improves
// answer accuracy when the question names a specific entity.
//
// Exported symbols:
//   - ExtractExactSignals(question string) []string
//   - ScoreExactSignals(text, question string) int
//   - OrderResultsEvidenceFirst(results []types.SearchResult, query string) []types.SearchResult
//
// The internal benchmark equivalents in cmd/longmemeval/run.go are kept for
// backward compatibility; they delegate to these exported functions via thin
// wrappers so the two implementations cannot diverge.
package search

import (
	"regexp"
	"sort"
	"strings"

	"github.com/petersimmons1972/engram/internal/types"
)

// exactSignalURLRe matches http(s) URLs.
var exactSignalURLRe = regexp.MustCompile(`https?://[^\s)]+`)

// exactSignalPhoneRe matches common US phone number formats.
var exactSignalPhoneRe = regexp.MustCompile(`\b(?:\+?1[-.\s]?)?(?:\(?\d{3}\)?[-.\s]?){2}\d{4}\b`)

// exactSignalQuotedRe matches double-quoted phrases.
var exactSignalQuotedRe = regexp.MustCompile(`"([^"]+)"`)

// ExtractExactSignals extracts high-precision identifier tokens from a query:
// URLs, phone numbers, and double-quoted phrases. These are used to score
// memories by verbatim content match when EvidenceFirstPack is active.
//
// Returned strings are deduplicated and non-empty.
func ExtractExactSignals(question string) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}

	for _, m := range exactSignalURLRe.FindAllString(question, -1) {
		add(m)
	}
	for _, m := range exactSignalPhoneRe.FindAllString(question, -1) {
		add(m)
	}
	for _, m := range exactSignalQuotedRe.FindAllStringSubmatch(question, -1) {
		if len(m) > 1 {
			add(m[1])
		}
	}
	return out
}

// ScoreExactSignals returns the number of exact signals from question that
// appear verbatim (case-insensitive) in text. Each matching signal contributes
// 3 to the score (matching the benchmark calibration in run.go).
func ScoreExactSignals(text, question string) int {
	signals := ExtractExactSignals(question)
	if len(signals) == 0 {
		return 0
	}
	score := 0
	lower := strings.ToLower(text)
	for _, sig := range signals {
		if strings.Contains(lower, strings.ToLower(sig)) {
			score += 3
		}
	}
	return score
}

// OrderResultsEvidenceFirst re-orders results so that memories whose content
// contains verbatim identifiers from query come first. Uses a stable sort so
// results with equal scores keep their original order.
//
// Entries with a nil Memory are treated as score-0 and sink to the end
// relative to scored entries, but do not panic.
//
// This is the production equivalent of orderContextEvidenceFirst in
// cmd/longmemeval/run.go. Returns a new slice; the original is not modified.
func OrderResultsEvidenceFirst(results []types.SearchResult, query string) []types.SearchResult {
	signals := ExtractExactSignals(query)
	if len(signals) == 0 {
		// No signals: stable copy, no reordering.
		out := make([]types.SearchResult, len(results))
		copy(out, results)
		return out
	}

	out := make([]types.SearchResult, len(results))
	copy(out, results)

	sort.SliceStable(out, func(i, j int) bool {
		var ci, cj string
		if out[i].Memory != nil {
			ci = out[i].Memory.Content
		}
		if out[j].Memory != nil {
			cj = out[j].Memory.Content
		}
		return ScoreExactSignals(ci, query) > ScoreExactSignals(cj, query)
	})
	return out
}
