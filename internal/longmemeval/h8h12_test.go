package longmemeval_test

import (
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

func TestIsAggregationQuestion_H8H12(t *testing.T) {
	positives := []string{
		"How many times did I go to the doctor?",
		"List every place I said I wanted to revisit.",
		"What were all occasions I mentioned biking to work?",
	}
	for _, q := range positives {
		if !longmemeval.IsAggregationQuestion(q) {
			t.Errorf("IsAggregationQuestion(%q) = false, want true", q)
		}
	}

	if longmemeval.IsAggregationQuestion("When did I last go to the doctor?") {
		t.Fatal("IsAggregationQuestion matched a non-aggregation question")
	}
}

// TestIsAggregationQuestion_MonetaryTotals covers the two full-coverage-but-wrong
// multi-session failures (2026-06-21 diagnostic) whose monetary "how much" phrasing
// escaped the original detector, so the enumerate-first lever never fired on them:
//   - 09ba9854: "How much will I save by taking the bus instead of a taxi?"
//   - d851d5ba: "How much money did I raise for charity in total?"
func TestIsAggregationQuestion_MonetaryTotals(t *testing.T) {
	positives := []string{
		"How much will I save by taking the bus from the airport to my hotel instead of a taxi?",
		"How much money did I raise for charity in total?",
		"What was the page count of the two novels I finished in January and March?",
	}
	for _, q := range positives {
		if !longmemeval.IsAggregationQuestion(q) {
			t.Errorf("IsAggregationQuestion(%q) = false, want true", q)
		}
	}
	// Guard: a plain single-fact "how much does X cost" lookup must NOT trip the
	// aggregation gate (no total/scope/list semantics).
	if longmemeval.IsAggregationQuestion("How much does the new phone cost?") {
		t.Fatal("IsAggregationQuestion matched a single-fact price lookup")
	}
}

// TestEnumerateFirstPrefix_EnforcesAggregationSteps pins the four operations the
// model botched on full-coverage multi-session aggregation items (2026-06-21):
// scope/temporal filtering, type deduplication, abstention on absent values, and
// an explicit final total. Wording may evolve; these are the load-bearing clauses.
func TestEnumerateFirstPrefix_EnforcesAggregationSteps(t *testing.T) {
	p := strings.ToLower(longmemeval.EnumerateFirstPrefix())
	// Load-bearing clauses (wording may evolve): enumerate, source provenance,
	// explicit exclude decision, distinct-once dedup, and a structured recompute.
	for _, must := range []string{"enumerate", "source", "exclude", "once", "sum"} {
		if !strings.Contains(p, must) {
			t.Errorf("EnumerateFirstPrefix() missing enforcement clause %q; got:\n%s",
				must, longmemeval.EnumerateFirstPrefix())
		}
	}
}

func TestRunOptsExhaustiveAggregation_DisabledIsBaseline(t *testing.T) {
	ops := longmemeval.RunOpts{}
	if got := ops.EffectiveRecallTopK("How many times did I call my sister?", 100); got != 100 {
		t.Fatalf("EffectiveRecallTopK() = %d, want 100", got)
	}
}

func TestRunOptsExhaustiveAggregation_GateSkipsNonAgg(t *testing.T) {
	ops := longmemeval.RunOpts{ExhaustiveAggregation: true}
	if got := ops.EffectiveRecallTopK("When did I last call my sister?", 100); got != 100 {
		t.Fatalf("EffectiveRecallTopK() = %d, want 100", got)
	}
}

func TestRunOptsExhaustiveAggregation_SetsTopK500(t *testing.T) {
	ops := longmemeval.RunOpts{ExhaustiveAggregation: true}
	if got := ops.EffectiveRecallTopK("How many times did I call my sister?", 100); got != 500 {
		t.Fatalf("EffectiveRecallTopK() = %d, want 500", got)
	}
}

func TestEnumerateFirst_DisabledIsBaseline(t *testing.T) {
	base := longmemeval.GenerationPromptForType(
		"How many times did I call my sister?",
		"multi-session",
		"2024-06-01",
		[]string{"Session date: 2024-05-10\nCalled my sister."},
	)
	ops := longmemeval.RunOpts{}
	got := ops.ApplyEnumerateFirst(
		base,
		"How many times did I call my sister?",
		"multi-session",
	)
	if got != base {
		t.Fatal("ApplyEnumerateFirst changed the baseline prompt when disabled")
	}
}

func TestEnumerateFirst_PrefixPresent(t *testing.T) {
	base := longmemeval.GenerationPromptForType(
		"How many times did I call my sister?",
		"multi-session",
		"2024-06-01",
		[]string{"Session date: 2024-05-10\nCalled my sister."},
	)
	ops := longmemeval.RunOpts{EnumerateFirst: true}
	got := ops.ApplyEnumerateFirst(
		base,
		"How many times did I call my sister?",
		"multi-session",
	)
	if !strings.Contains(got, longmemeval.EnumerateFirstPrefix()) {
		t.Fatalf("ApplyEnumerateFirst() missing prefix %q", longmemeval.EnumerateFirstPrefix())
	}
}

func TestH8H12_CombinedFlags(t *testing.T) {
	base := longmemeval.GenerationPromptForType(
		"How many times did I call my sister?",
		"multi-session",
		"2024-06-01",
		[]string{"Session date: 2024-05-10\nCalled my sister."},
	)
	ops := longmemeval.RunOpts{
		ExhaustiveAggregation: true,
		EnumerateFirst:        true,
	}
	if got := ops.EffectiveRecallTopK("How many times did I call my sister?", 100); got != 500 {
		t.Fatalf("EffectiveRecallTopK() = %d, want 500", got)
	}
	prompt := ops.ApplyEnumerateFirst(base, "How many times did I call my sister?", "multi-session")
	if !strings.Contains(prompt, longmemeval.EnumerateFirstPrefix()) {
		t.Fatal("combined flags prompt missing enumerate-first prefix")
	}
}
