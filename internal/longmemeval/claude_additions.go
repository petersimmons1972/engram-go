package longmemeval

import (
	"fmt"
	"regexp"
	"strings"
)

// ---------------------------------------------------------------------------
// H15 — Dual-query preference recall helpers
// ---------------------------------------------------------------------------

// preferenceStopWords is a small set of English function words that carry no
// domain signal. We strip these when building a subject anchor so the remaining
// tokens are more likely to match the gold preference session via BM25.
var preferenceStopWords = map[string]bool{
	"a": true, "an": true, "the": true, "for": true, "on": true,
	"in": true, "of": true, "to": true, "and": true, "or": true,
	"i": true, "me": true, "my": true, "you": true, "your": true,
	"do": true, "is": true, "are": true, "some": true, "any": true,
	"can": true, "could": true, "would": true, "should": true,
	"what": true, "which": true, "how": true, "when": true, "where": true,
}

// ExtractSubjectAnchor builds a domain-specific recall query from the object
// noun-phrase of a preference question. It strips the recommendation verb phrase
// via preferenceStripRe, then tokenises by whitespace, removes stop-words, and
// joins the remaining tokens.
//
// If all tokens are stop-words (fully generic question such as "What do I like?")
// it falls back to the full stripped remainder so the anchor is never empty.
func ExtractSubjectAnchor(question string) string {
	stripped := preferenceStripRe.ReplaceAllString(question, "")
	if stripped == "" {
		stripped = question
	}
	// Remove trailing punctuation from the stripped string.
	stripped = strings.TrimRight(stripped, "?!.,;:")

	tokens := strings.Fields(stripped)
	var keep []string
	for _, tok := range tokens {
		lower := strings.ToLower(tok)
		lower = strings.TrimRight(lower, "?!.,;:")
		if !preferenceStopWords[lower] {
			keep = append(keep, tok)
		}
	}
	if len(keep) == 0 {
		// Fallback: return the fully stripped string so anchor is always non-empty.
		return stripped
	}
	return strings.Join(keep, " ")
}

// UnionMemoryIDs merges primary and secondary ID slices, preserving order and
// deduplicating by memory ID. Primary IDs appear first; secondary IDs that are
// not already in primary are appended in their original order.
func UnionMemoryIDs(primary, secondary []string) []string {
	seen := make(map[string]bool, len(primary))
	result := make([]string, 0, len(primary)+len(secondary))
	for _, id := range primary {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	for _, id := range secondary {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}

// preferenceStripRe strips the opening recommendation verb phrase from a question.
// e.g. "Can you recommend a hotel for Miami?" → "a hotel for Miami?"
var preferenceStripRe = regexp.MustCompile(
	`(?i)^(can you |could you |would you )?(recommend|suggest|advise|give me|tell me) `)

// PreferenceRecallQuery rewrites a preference question into a recall query
// targeting sessions where the user expressed preferences, not sessions that
// would answer the literal question. Only the RECALL QUERY changes — the
// benchmark question itself is never modified.
func PreferenceRecallQuery(question string) string {
	stripped := preferenceStripRe.ReplaceAllString(question, "")
	if stripped == "" {
		stripped = question
	}
	return "user preference " + stripped + " like dislike use avoid"
}

// ContextTopKForType returns how many recalled memories to feed to generation.
// Multi-session and temporal questions need more context to capture the right
// sessions; other types are fine with the baseline of 8.
func ContextTopKForType(questionType string) int {
	switch questionType {
	case "multi-session", "temporal-reasoning":
		return 15
	default:
		return 8
	}
}

// ContextTopKForTypeWithBump is like ContextTopKForType but raises all
// categories to 15 when bump is true — equalising single-session types with
// multi-session types for broader-retrieval reruns.
func ContextTopKForTypeWithBump(questionType string, bump bool) int {
	if bump {
		return 15
	}
	return ContextTopKForType(questionType)
}

// temporalGenerationPrompt returns a step-by-step date-arithmetic prompt
// for temporal-reasoning questions, explicitly forbidding event invention.
func temporalGenerationPrompt(question, questionDate string, contextBlocks []string) string {
	ctx := strings.Join(contextBlocks, "\n\n---\n\n")
	return fmt.Sprintf(`You are answering a time-based question about a person's conversation history.

Each memory block begins with "Session date: YYYY-MM-DD". The question was asked on %s.

Relevant memory context:
%s

Question (asked on %s): %s

Step-by-step:
1. Identify the relevant event in the memory blocks above.
2. Note its exact Session date.
3. Compute elapsed time: %s minus the Session date.
4. Express the result as the question requests (days, weeks, months, or ordered list).
5. If the event is not present in the memory blocks, say so. Do not invent dates or events. Do not fabricate trips, locations, or timestamps not in the context.

Answer concisely. Show the date subtraction if computing a count (e.g., "2024-03-15 minus 2024-02-22 = 21 days = 3 weeks").`,
		questionDate, ctx, questionDate, question, questionDate)
}
