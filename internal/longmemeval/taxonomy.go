package longmemeval

import (
	"fmt"
	"regexp"
	"strings"
)

// numericRE matches a bare integer gold answer (1–4 digits, optional whitespace).
var numericRE = regexp.MustCompile(`^\s*\d{1,4}\s*$`)

// wordOverlapThreshold is the minimum IoU word-overlap score to consider a gold
// answer substantially present in the hypothesis.  Below this the item is
// classified as missing_recall (data likely absent from retrieved set).
// Uses Jaccard IoU in [0,1]; a value of 1.0 means exact word-set equality.
const wordOverlapThreshold = 0.5

// TaxonomyInput holds the per-item data required to classify a failure.
type TaxonomyInput struct {
	// GoldAnswer is the reference answer string from the dataset.
	GoldAnswer string
	// Hypothesis is the model's generated answer.  May be empty.
	Hypothesis string
	// GoldSessionIDs are the session IDs that contain the correct answer.
	GoldSessionIDs []string
	// RetrievedSessions are the session IDs in the model's retrieved top-K set.
	RetrievedSessions []string
}

// TaxonomyResult is the output of ClassifyFailure.
type TaxonomyResult struct {
	// Class is one of: "aggregation_failure", "missing_recall", "generation_failure".
	Class    string
	// Evidence is a human-readable explanation of the classification decision.
	Evidence string
}

// ClassifyFailure classifies why a LongMemEval item was not answered correctly.
//
// Classification logic (ordered):
//  1. If GoldAnswer is a bare number (≤4 digits) AND at least one gold session
//     was retrieved → "aggregation_failure": the data was present but the model
//     failed to aggregate across sessions.
//  2. If GoldAnswer is a bare number AND no gold sessions were retrieved →
//     "missing_recall": numeric gold but data not in top-K.
//  3. If word-overlap(GoldAnswer, Hypothesis) ≥ wordOverlapThreshold →
//     "generation_failure": content was retrieved but not surfaced correctly.
//  4. Otherwise → "missing_recall": low overlap, data likely absent.
func ClassifyFailure(in TaxonomyInput) TaxonomyResult {
	gold := strings.TrimSpace(in.GoldAnswer)

	// Path 1 & 2: numeric gold answer.
	if numericRE.MatchString(gold) {
		retrieved := make(map[string]bool, len(in.RetrievedSessions))
		for _, sid := range in.RetrievedSessions {
			retrieved[sid] = true
		}
		var presentCount int
		for _, sid := range in.GoldSessionIDs {
			if retrieved[sid] {
				presentCount++
			}
		}
		if presentCount > 0 {
			return TaxonomyResult{
				Class: "aggregation_failure",
				Evidence: fmt.Sprintf(
					"Gold answer is a count/aggregate (%q). Gold sessions retrieved (%d/%d) but model failed to aggregate across sessions.",
					gold, presentCount, len(in.GoldSessionIDs),
				),
			}
		}
		return TaxonomyResult{
			Class:    "missing_recall",
			Evidence: fmt.Sprintf("Numeric gold answer (%q); gold sessions not in top-K retrieved set.", gold),
		}
	}

	// Path 3: word-overlap check for non-numeric gold.
	score := wordOverlap(gold, in.Hypothesis)
	if score >= wordOverlapThreshold {
		return TaxonomyResult{
			Class:    "generation_failure",
			Evidence: fmt.Sprintf("Word-overlap %.2f >= %.2f; content retrieved but not correctly surfaced.", score, wordOverlapThreshold),
		}
	}

	// Path 4: default — low overlap, missing data.
	return TaxonomyResult{
		Class:    "missing_recall",
		Evidence: fmt.Sprintf("Word-overlap %.2f < %.2f; gold content likely absent from retrieved set.", score, wordOverlapThreshold),
	}
}

// wordOverlap computes a simple intersection-over-union overlap score between
// the word sets of a and b.  Returns 0 when either string is empty.
func wordOverlap(a, b string) float64 {
	wordsA := tokenize(a)
	wordsB := tokenize(b)
	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0
	}
	setA := make(map[string]bool, len(wordsA))
	for _, w := range wordsA {
		setA[w] = true
	}
	var intersection int
	setB := make(map[string]bool, len(wordsB))
	for _, w := range wordsB {
		if setA[w] {
			intersection++
		}
		setB[w] = true
	}
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// tokenize lowercases and splits s into words, discarding punctuation.
func tokenize(s string) []string {
	s = strings.ToLower(s)
	var words []string
	for _, field := range strings.Fields(s) {
		word := strings.Trim(field, ".,!?;:\"'()-")
		if word != "" {
			words = append(words, word)
		}
	}
	return words
}
