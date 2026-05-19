package longmemeval

import (
	"fmt"
	"regexp"
	"strings"
)

// preferenceStripRe strips the opening recommendation verb phrase from a question.
// e.g. "Can you recommend a hotel for Miami?" → "a hotel for Miami?"
var preferenceStripRe = regexp.MustCompile(
	`(?i)^(can you |could you |would you )?(recommend|suggest|advise|give me|tell me) `)

// preferenceLeadStripRe strips filler lead phrases that don't carry topic content,
// e.g. "what is my preferred", "what do I like", "which do I prefer", etc.
var preferenceLeadStripRe = regexp.MustCompile(
	`(?i)^(what('?s| is) my (preferred|favorite|favourite)|what do i (like|prefer|use|enjoy)|which (do i|should i)|how do i prefer|my preferred)\s+`)

// stopWords is the set of common words to exclude when extracting a topic noun-phrase.
var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "for": true, "of": true, "in": true,
	"to": true, "and": true, "or": true, "that": true, "i": true, "my": true,
	"me": true, "some": true, "any": true, "might": true, "find": true,
	"interesting": true, "recently": true, "recent": true, "new": true,
}

// extractTopicNounPhrase extracts the domain noun-phrase from a stripped question.
// It collects all non-stop-word tokens, preserving their relative order,
// and returns them joined by spaces. Falls back to the full stripped string if
// no content tokens are found.
func extractTopicNounPhrase(stripped string) string {
	// Remove trailing punctuation.
	stripped = strings.TrimRight(stripped, "?!.,;:")
	tokens := strings.Fields(stripped)

	var content []string
	for _, tok := range tokens {
		lower := strings.ToLower(strings.TrimRight(tok, ".,;:!?"))
		if !stopWords[lower] && lower != "" {
			content = append(content, tok)
		}
	}
	if len(content) == 0 {
		return stripped
	}
	return strings.Join(content, " ")
}

// PreferenceRecallQuery rewrites a preference question into a recall query
// targeting sessions where the user expressed preferences, not sessions that
// would answer the literal question. Only the RECALL QUERY changes — the
// benchmark question itself is never modified.
//
// The query seeds recall with the domain noun-phrase extracted from the question
// so that topic-specific preferences (e.g. "coffee brewing method") are preferred
// over generic preference terms that match the dominant topic in the haystack.
func PreferenceRecallQuery(question string) string {
	stripped := preferenceStripRe.ReplaceAllString(question, "")
	stripped = preferenceLeadStripRe.ReplaceAllString(stripped, "")
	if stripped == "" {
		stripped = question
	}
	topic := extractTopicNounPhrase(stripped)
	if topic == "" || topic == question {
		// Fall back to legacy behaviour when extraction yields nothing useful.
		return "user preference " + stripped + " like dislike use avoid"
	}
	return "user interested in " + topic + " experience knowledge background"
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
