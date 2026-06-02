package longmemeval_test

// Tests for the LongMemEval failure taxonomy classifier.
// See issue #748: aggregation/counting questions were misclassified as
// missing_recall because word-overlap with a bare digit gold answer is
// near-zero even when the gold sessions are present in the retrieved set.

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// TestClassifyFailure_AggregationGoldSessionsRetrieved verifies that when the
// gold answer is a bare number and all gold session IDs are present in the
// retrieved set, ClassifyFailure returns "aggregation_failure".
func TestClassifyFailure_AggregationGoldSessionsRetrieved(t *testing.T) {
	result := longmemeval.ClassifyFailure(longmemeval.TaxonomyInput{
		GoldAnswer:        "4",
		GoldSessionIDs:    []string{"sess-1", "sess-2", "sess-3"},
		RetrievedSessions: []string{"sess-1", "sess-2", "sess-3", "sess-4"},
	})
	if result.Class != "aggregation_failure" {
		t.Errorf("ClassifyFailure: got %q, want %q", result.Class, "aggregation_failure")
	}
	if result.Evidence == "" {
		t.Error("ClassifyFailure: Evidence must not be empty")
	}
}

// TestClassifyFailure_NumericGoldSessionsMissing verifies that when the gold
// answer is numeric but no gold session IDs are in the retrieved set, the
// result is missing_recall (data not retrieved).
func TestClassifyFailure_NumericGoldSessionsMissing(t *testing.T) {
	result := longmemeval.ClassifyFailure(longmemeval.TaxonomyInput{
		GoldAnswer:        "4",
		GoldSessionIDs:    []string{"sess-gold"},
		RetrievedSessions: []string{"sess-other-1", "sess-other-2"},
	})
	if result.Class != "missing_recall" {
		t.Errorf("ClassifyFailure: got %q, want %q", result.Class, "missing_recall")
	}
}

// TestClassifyFailure_NonNumericShortGold verifies that non-numeric short
// answers (e.g. "yes", "no") are still classified as missing_recall via the
// legacy short-answer path, not as aggregation_failure.
func TestClassifyFailure_NonNumericShortGold(t *testing.T) {
	result := longmemeval.ClassifyFailure(longmemeval.TaxonomyInput{
		GoldAnswer:        "yes",
		GoldSessionIDs:    []string{"sess-1"},
		RetrievedSessions: []string{"sess-1"},
	})
	if result.Class != "missing_recall" {
		t.Errorf("ClassifyFailure: got %q, want %q", result.Class, "missing_recall")
	}
}

// TestClassifyFailure_NormalWordOverlapLow verifies that a non-numeric gold
// answer with low word-overlap scores missing_recall.
func TestClassifyFailure_NormalWordOverlapLow(t *testing.T) {
	result := longmemeval.ClassifyFailure(longmemeval.TaxonomyInput{
		GoldAnswer:        "the ancient capital of Persia",
		Hypothesis:        "I don't know",
		GoldSessionIDs:    []string{"sess-1"},
		RetrievedSessions: []string{},
	})
	if result.Class != "missing_recall" {
		t.Errorf("ClassifyFailure: got %q, want %q", result.Class, "missing_recall")
	}
}

// TestClassifyFailure_NormalWordOverlapHigh verifies that when the hypothesis
// contains all words from the gold answer (IoU >= 0.5) the item is classified
// as "generation_failure" rather than "missing_recall" — data was retrieved
// but the model failed to surface the answer correctly.
func TestClassifyFailure_NormalWordOverlapHigh(t *testing.T) {
	// Identical strings → IoU = 1.0, well above threshold.
	result := longmemeval.ClassifyFailure(longmemeval.TaxonomyInput{
		GoldAnswer:        "ancient capital Persepolis",
		Hypothesis:        "ancient capital Persepolis was the great ancient capital",
		GoldSessionIDs:    []string{"sess-1"},
		RetrievedSessions: []string{"sess-1"},
	})
	if result.Class == "missing_recall" {
		t.Errorf("ClassifyFailure: high-overlap answer should not be missing_recall, got %q", result.Class)
	}
	if result.Class != "generation_failure" {
		t.Errorf("ClassifyFailure: got %q, want %q", result.Class, "generation_failure")
	}
}

// TestClassifyFailure_FiveDigitNumericAggregation verifies that a five-digit
// numeric gold answer (e.g. "10000") is correctly classified as
// "aggregation_failure" when gold sessions are present in the retrieved set.
//
// Regression guard for #815: the original numericRE cap of 1–4 digits caused
// five-digit counts to fall through to word-overlap classification, where
// word-overlap against a bare number is near-zero, yielding a false
// "missing_recall" even when the gold sessions were retrieved.
func TestClassifyFailure_FiveDigitNumericAggregation(t *testing.T) {
	result := longmemeval.ClassifyFailure(longmemeval.TaxonomyInput{
		GoldAnswer:        "10000",
		GoldSessionIDs:    []string{"sess-a", "sess-b"},
		RetrievedSessions: []string{"sess-a", "sess-b", "sess-c"},
	})
	if result.Class != "aggregation_failure" {
		t.Errorf("ClassifyFailure(gold=%q, sessions retrieved): got %q, want %q — five-digit numeric gold should be aggregation_failure (#815)",
			"10000", result.Class, "aggregation_failure")
	}
	if result.Evidence == "" {
		t.Error("ClassifyFailure: Evidence must not be empty")
	}
}

// TestClassifyFailure_FiveDigitNumericMissingRecall verifies that a five-digit
// numeric gold answer with no gold sessions in the retrieved set is classified
// as "missing_recall" (data not present), not "generation_failure". (#815)
func TestClassifyFailure_FiveDigitNumericMissingRecall(t *testing.T) {
	result := longmemeval.ClassifyFailure(longmemeval.TaxonomyInput{
		GoldAnswer:        "10000",
		GoldSessionIDs:    []string{"sess-gold"},
		RetrievedSessions: []string{"sess-other"},
	})
	if result.Class != "missing_recall" {
		t.Errorf("ClassifyFailure(gold=%q, no sessions retrieved): got %q, want missing_recall (#815)",
			"10000", result.Class)
	}
}
