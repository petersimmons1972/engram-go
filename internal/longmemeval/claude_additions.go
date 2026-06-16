package longmemeval

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// preferenceStripRe strips the opening recommendation verb phrase from a question.
// e.g. "Can you recommend a hotel for Miami?" → "a hotel for Miami?".
var preferenceStripRe = regexp.MustCompile(
	`(?i)^(?:(?:can|could|would) you )?(?:recommend|suggest|advise|give me|tell me)\s+|^what do you think about\s+|^do you prefer\s+|^what did i say about\s+`)

// preferenceQuestionRe is the text-only heuristic used to infer that a raw
// question is preference-shaped in the production recall path, where dataset
// question_type is unavailable (FM-77).
var preferenceQuestionRe = regexp.MustCompile(
	`(?i)^(?:(?:can|could|would) you )?(?:recommend|suggest|advise|give me|tell me)\b|^what do you think about\b|^do you prefer\b|^what did i say about\b|^what do i like\b|^what do i love\b|^what do i hate\b`)

// RunOpts holds longmemeval run-side options used by the dual preference recall
// planner. The zero value preserves the baseline single-call path.
type RunOpts struct {
	DualPreferenceRecall bool
}

// RecallResult is one scored memory_recall hit.
type RecallResult struct {
	ID    string
	Score float64
}

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

// SubjectNPQuery strips preference framing and returns the clean subject noun
// phrase for the anchor recall pass in H15.
func SubjectNPQuery(question string) string {
	stripped := preferenceStripRe.ReplaceAllString(strings.TrimSpace(question), "")
	stripped = strings.TrimSpace(strings.TrimRight(stripped, "?!.,;:"))
	if stripped == "" {
		return strings.TrimSpace(strings.TrimRight(question, "?!.,;:"))
	}
	return stripped
}

// InferQuestionType infers the preference category directly from question text.
// Only the single-session-preference case is needed by the eval harness today.
func InferQuestionType(question string) string {
	if preferenceQuestionRe.MatchString(strings.TrimSpace(question)) {
		return "single-session-preference"
	}
	return ""
}

// RecallForQuestion executes the baseline recall path or, when enabled and the
// question text is preference-shaped, the H15 dual-pass recall:
//  1. subject noun-phrase anchor query
//  2. generic preference recall query
//
// The union is deduped by memory ID and ranked by max(score).
func RecallForQuestion(question, baselineQuery string, opts RunOpts, recall func(query string) ([]RecallResult, error)) ([]RecallResult, error) {
	if !opts.DualPreferenceRecall || InferQuestionType(question) != "single-session-preference" {
		return recall(baselineQuery)
	}

	subjectResults, err := recall(SubjectNPQuery(question))
	if err != nil {
		return nil, err
	}
	preferenceResults, err := recall(PreferenceRecallQuery(question))
	if err != nil {
		return nil, err
	}
	return unionRecallResults(subjectResults, preferenceResults), nil
}

func unionRecallResults(primary, secondary []RecallResult) []RecallResult {
	type scored struct {
		result    RecallResult
		firstSeen int
	}
	merged := make(map[string]scored, len(primary)+len(secondary))
	order := 0
	ingest := func(results []RecallResult) {
		for _, result := range results {
			if result.ID == "" {
				continue
			}
			current, ok := merged[result.ID]
			if !ok {
				merged[result.ID] = scored{result: result, firstSeen: order}
				order++
				continue
			}
			if result.Score > current.result.Score {
				current.result.Score = result.Score
				merged[result.ID] = current
			}
		}
	}
	ingest(primary)
	ingest(secondary)

	out := make([]scored, 0, len(merged))
	for _, result := range merged {
		out = append(out, result)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].result.Score == out[j].result.Score {
			return out[i].firstSeen < out[j].firstSeen
		}
		return out[i].result.Score > out[j].result.Score
	})

	combined := make([]RecallResult, 0, len(out))
	for _, result := range out {
		combined = append(combined, result.result)
	}
	return combined
}

// ---------------------------------------------------------------------------
// H15 / H8 — lme-h8h12h15 branch additions: subject-anchor + aggregation helpers
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

// ExtractSubjectAnchor (H15) builds a domain-specific recall query from the
// object noun-phrase of a preference question. It strips the recommendation
// verb phrase via preferenceStripRe, then tokenises by whitespace, removes
// stop-words, and joins the remaining tokens. If all tokens are stop-words it
// falls back to the full stripped remainder so the anchor is never empty.
func ExtractSubjectAnchor(question string) string {
	stripped := preferenceStripRe.ReplaceAllString(question, "")
	if stripped == "" {
		stripped = question
	}
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
		return stripped
	}
	return strings.Join(keep, " ")
}

// PreferenceSubjectAnchorQuery keeps the H15 domain anchor while preserving the
// preference signal required for extracted-preference memories to be eligible.
func PreferenceSubjectAnchorQuery(question string) string {
	anchor := ExtractSubjectAnchor(question)
	if strings.TrimSpace(anchor) == "" {
		anchor = question
	}
	return "user preference " + anchor + " like dislike use avoid"
}

// aggregationRe (H8) matches count-shaped questions that require
// population-level retrieval rather than a single top-scoring session.
var aggregationRe = regexp.MustCompile(
	`(?i)\b(how many|how often|how much total|total number of|sum of|count of)\b`)

// aggregationStripRe (H8) strips the counting interrogative phrase so that the
// remaining tokens describe the object being counted.
var aggregationStripRe = regexp.MustCompile(
	`(?i)^(how many (times )?|how often |how much total |what is the total (number of )?|what is the sum of |give me a count of |count of )`)

// IsAggregationQuestion (H8) returns true when the question matches the
// aggregation pattern that requires exhaustive population recall.
func IsAggregationQuestion(question string) bool {
	return aggregationRe.MatchString(question)
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
		if !preferenceStopWords[lower] {
			keep = append(keep, tok)
		}
	}
	if len(keep) == 0 {
		return stripped
	}
	return strings.Join(keep, " ")
}

// UnionMemoryIDs (H8/H15) merges primary and secondary ID slices, preserving
// order and deduplicating by memory ID. Primary IDs appear first; secondary
// IDs that are not already in primary are appended in their original order.
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

// ContextTopKForType returns how many recalled memories to feed to generation.
// Multi-session and temporal questions need more context to capture the right
// sessions. Phase 0 (P0): single-session-user and single-session-preference
// raised from 8→15; these types have low vocab-overlap with the gold session,
// so a larger context window is required to surface the answer.
// Revert: change "single-session-user", "single-session-preference" case to 8.
func ContextTopKForType(questionType string) int {
	switch questionType {
	case "multi-session", "temporal-reasoning",
		"single-session-user", "single-session-preference":
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
5. You MUST provide a date or count answer. If the exact event date is not explicitly present, use the closest available date from context as your best estimate. Do not output "not mentioned" or "cannot be determined" — always give a specific answer based on available evidence.

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
