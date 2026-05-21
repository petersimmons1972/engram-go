package longmemeval_test

import (
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// ---------------------------------------------------------------------------
// H8 — Exhaustive aggregation recall helpers
// ---------------------------------------------------------------------------

// TestIsAggregationQuestion verifies that count-shaped questions are detected
// and other question shapes are not.
func TestIsAggregationQuestion(t *testing.T) {
	agg := []string{
		"How many times did I visit the doctor?",
		"How many books have I read this year?",
		"How often do I exercise?",
		"How much total did I spend on coffee?",
		"What is the total number of flights I took?",
		"What is the sum of my expenses?",
		"Give me a count of my gym sessions.",
		"Count of vacation days used?",
	}
	nonAgg := []string{
		"When did I last see Alice?",
		"What restaurant did I recommend?",
		"Where did I go on my last trip?",
		"What do I prefer for breakfast?",
	}
	for _, q := range agg {
		if !longmemeval.IsAggregationQuestion(q) {
			t.Errorf("IsAggregationQuestion(%q) = false, want true", q)
		}
	}
	for _, q := range nonAgg {
		if longmemeval.IsAggregationQuestion(q) {
			t.Errorf("IsAggregationQuestion(%q) = true, want false", q)
		}
	}
}

// TestExtractAggregationAnchor verifies that the object noun-phrase is extracted
// from a counting question for use as the exhaustive sweep query.
func TestExtractAggregationAnchor(t *testing.T) {
	cases := []struct {
		question   string
		wantTokens []string
	}{
		{
			question:   "How many times did I visit the doctor?",
			wantTokens: []string{"doctor"},
		},
		{
			question:   "How many books have I read this year?",
			wantTokens: []string{"books"},
		},
		{
			question:   "How often do I exercise?",
			wantTokens: []string{"exercise"},
		},
	}
	for _, c := range cases {
		anchor := longmemeval.ExtractAggregationAnchor(c.question)
		if anchor == "" {
			t.Errorf("ExtractAggregationAnchor(%q) = empty, want non-empty", c.question)
			continue
		}
		for _, tok := range c.wantTokens {
			if !strings.Contains(anchor, tok) {
				t.Errorf("ExtractAggregationAnchor(%q) = %q, want it to contain %q",
					c.question, anchor, tok)
			}
		}
	}
}

// TestExhaustiveAggregationUnion is the key falsifiability test for H8:
// when the primary top-100 misses some relevant items, the union with a
// topK=500 sweep query recovers them.
//
// We simulate this by constructing two disjoint ID lists: primary (items the
// default recall found) and sweep (additional items found by the sweep). The
// union must contain all items from both sets.
func TestExhaustiveAggregationUnion(t *testing.T) {
	// Simulate: default recall at top-100 surfaced 3 relevant doctor-visit IDs.
	primary := []string{"visit-1", "visit-2", "visit-3", "unrelated-7"}
	// Sweep at top-500 surfaced 5 more that were outside the primary top-100.
	sweep := []string{"visit-4", "visit-5", "visit-6", "visit-3", "visit-1"}

	got := longmemeval.UnionMemoryIDs(primary, sweep)

	// Expected: all unique IDs, primary order preserved, sweep additions appended.
	wantSet := map[string]bool{
		"visit-1": true, "visit-2": true, "visit-3": true,
		"visit-4": true, "visit-5": true, "visit-6": true,
		"unrelated-7": true,
	}
	if len(got) != len(wantSet) {
		t.Fatalf("union len = %d, want %d; got %v", len(got), len(wantSet), got)
	}
	for _, id := range got {
		if !wantSet[id] {
			t.Errorf("unexpected ID in union: %q", id)
		}
	}

	// Primary items must appear first (primary-order guarantee).
	if got[0] != "visit-1" {
		t.Errorf("first item = %q, want %q (primary first)", got[0], "visit-1")
	}
}
