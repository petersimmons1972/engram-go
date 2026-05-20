package longmemeval

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// preferenceStripRe strips the opening recommendation verb phrase from a question.
// e.g. "Can you recommend a hotel for Miami?" → "a hotel for Miami?".
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

// isOrderingQuestion reports whether the question is asking about event
// ordering or sequence — the M5 trigger for H-M5 chrono-sort forcing.
// Matches keywords: "order", "earliest to latest", "latest to earliest",
// "which first", "which came first", "what came first", "sequence", "chronological".
// Exported so run.go and tests can call it without re-implementing the pattern.
func isOrderingQuestion(question string) bool {
	lower := strings.ToLower(question)
	orderingPhrases := []string{
		"order",
		"earliest to latest",
		"latest to earliest",
		"which first",
		"what first",
		"came first",
		"came last",
		"which came",
		"what came",
		"sequence",
		"chronological",
	}
	for _, phrase := range orderingPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

// isEntityAmbiguousQuestion reports whether the question is asking "what X
// did at date D" where multiple X events may exist near D — the M1 trigger
// for H-M1 entity enumeration pass. Matches "which X did I" / "what X did I"
// combined with a relative-time anchor, which is the canonical M1 pattern.
func isEntityAmbiguousQuestion(question string) bool {
	lower := strings.ToLower(question)
	timeAnchors := []string{
		"ago", "last week", "last weekend", "last tuesday", "last monday",
		"last wednesday", "last thursday", "last friday", "last saturday",
		"last sunday", "yesterday", "this week", "past week", "past weekend",
		"days ago", "weeks ago", "months ago",
	}
	for _, anchor := range timeAnchors {
		if strings.Contains(lower, anchor) {
			return true
		}
	}
	return false
}

// temporalGenerationPromptWithAug is the Exp-14 H-M5+H-M1 combined prompt
// for temporal-reasoning questions. It conditionally prepends:
//   - H-M5 (ordering questions): "First list all relevant events with their session
//     dates in chronological order before answering."
//   - H-M1 (entity-ambiguous questions): "First enumerate ALL events of type X
//     near date D from context, then commit to the most temporally precise one."
//
// Both augmentations can fire on the same question if it matches both patterns.
// For questions matching neither pattern, the standard step-by-step prompt is
// returned unchanged so non-augmented items serve as a within-run control.
//
// Activated by --temporal-prompt-aug (Config.TemporalPromptAug). Off by default.
func temporalGenerationPromptWithAug(question, questionDate string, contextBlocks []string) string {
	ctx := strings.Join(contextBlocks, "\n\n---\n\n")

	var augLines []string
	if isOrderingQuestion(question) {
		augLines = append(augLines,
			"H-M5: If this question is about event ordering or sequence, first list ALL relevant events with their session dates in chronological order before answering.")
	}
	if isEntityAmbiguousQuestion(question) {
		augLines = append(augLines,
			"H-M1: If this question asks 'what X did at date D', first enumerate ALL events of type X near date D from the context blocks, then commit to the one whose Session date is most temporally precise for the question.")
	}

	augBlock := ""
	if len(augLines) > 0 {
		augBlock = "\n" + strings.Join(augLines, "\n") + "\n"
	}

	return fmt.Sprintf(`You are answering a time-based question about a person's conversation history.

Each memory block begins with "Session date: YYYY-MM-DD". The question was asked on %s.

Relevant memory context:
%s

Question (asked on %s): %s
%s
Step-by-step:
1. Identify the relevant event in the memory blocks above.
2. Note its exact Session date.
3. Compute elapsed time: %s minus the Session date.
4. Express the result as the question requests (days, weeks, months, or ordered list).
5. If the event is not present in the memory blocks, say so. Do not invent dates or events. Do not fabricate trips, locations, or timestamps not in the context.

Answer concisely. Show the date subtraction if computing a count (e.g., "2024-03-15 minus 2024-02-22 = 21 days = 3 weeks").`,
		questionDate, ctx, questionDate, question, augBlock, questionDate)
}

// ---------------------------------------------------------------------------
// H15 — paraphrased multi-pass BM25 union
// ---------------------------------------------------------------------------

// BuildParaphrasePrompt constructs the Haiku prompt that asks for n paraphrased
// query variants emphasising different verbs and aspects. The prompt requests
// one variant per line, numbered, so ParseParaphrases can extract them cleanly.
// Exported for testing.
func BuildParaphrasePrompt(query string, n int) string {
	return fmt.Sprintf(`Generate %d paraphrased search queries for the following user question.
Each paraphrase should emphasise different verbs, synonyms, or aspects (e.g. "bought" vs "acquired" vs "brought home"; "went to" vs "visited" vs "attended").
Output exactly %d lines, one paraphrase per line, numbered 1. through %d.
Do not explain. Do not add extra text. Only the numbered list.

Original query: %s`, n, n, n, query)
}

// ParseParaphrases parses the numbered-list output from the Haiku paraphraser
// into a slice of plain strings. Leading "N." or "N)" prefixes and whitespace
// are stripped. Empty lines are skipped.
// Exported for testing.
func ParseParaphrases(raw string) []string {
	numberedRe := regexp.MustCompile(`^\s*\d+[.)]\s*`)
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = numberedRe.ReplaceAllString(line, "")
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// DeduplicateIDs returns ids with duplicates removed, preserving first-occurrence
// order. Exported for testing and use in run.go union logic.
func DeduplicateIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}

// TextGenerator is a function that calls an LLM with a prompt and returns the
// raw response. It matches the signature of GenerateHaiku so that
// generateParaphrasesWith can be used in tests with a fake implementation.
type TextGenerator func(ctx context.Context, prompt string, retries int) (string, error)

// generateParaphrasesWith is the testable core of GenerateParaphrases.
// gen is called with the formatted paraphrase prompt; the returned text is
// parsed via ParseParaphrases. Exported-ish via the package-level wrapper.
func generateParaphrasesWith(ctx context.Context, query string, n, retries int, gen TextGenerator) ([]string, error) {
	if n <= 0 {
		return nil, nil
	}
	prompt := BuildParaphrasePrompt(query, n)
	raw, err := gen(ctx, prompt, retries)
	if err != nil {
		return nil, fmt.Errorf("paraphrase: %w", err)
	}
	return ParseParaphrases(raw), nil
}

// GenerateParaphrases calls Haiku to produce n paraphrased query variants for
// the given query. Returns an empty slice when n == 0 (default-off behaviour).
// retries is passed through to the Haiku call.
func GenerateParaphrases(ctx context.Context, query string, n, retries int) ([]string, error) {
	return generateParaphrasesWith(ctx, query, n, retries, GenerateHaiku)
}

// temporalGenerationPromptWithDateInjection is the H16 variant of
// temporalGenerationPrompt. It places "Today's date is: {questionDate}" as the
// very first line of the prompt so the model anchors all relative-time
// references ("N days ago", "last Tuesday", "past weekend") before reading any
// memory context.
//
// Each memory block carries a "Session date: YYYY-MM-DD" header written at
// ingest time. Combined with the question_date header, this gives the model
// two explicit reference points for resolving relative temporal anchors.
//
// Activated by --inject-question-date (Config.InjectQuestionDate). Off by
// default so the baseline prompt path is unaffected.
func temporalGenerationPromptWithDateInjection(question, questionDate string, contextBlocks []string) string {
	ctx := strings.Join(contextBlocks, "\n\n---\n\n")
	return fmt.Sprintf(`Today's date is: %s

You are answering a time-based question about a person's conversation history.

Each memory block begins with "Session date: YYYY-MM-DD". Use the session dates together with today's date above to resolve any relative time references (e.g. "last Tuesday", "5 days ago", "past weekend").

Relevant memory context:
%s

Question (asked on %s): %s

Step-by-step:
1. Anchor to today's date: %s.
2. Resolve the relative time reference in the question to a specific calendar date or date range.
3. Identify the memory block whose Session date falls within that resolved range.
4. Extract the answer from that block.
5. If no block matches the resolved date range, say so. Do not invent dates or events.

Answer concisely. Show your date resolution if computing a relative offset (e.g., "today is 2023-03-21, 'last Tuesday' = 2023-03-14").`,
		questionDate, ctx, questionDate, question, questionDate)
}
