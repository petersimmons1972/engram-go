package mcp

import (
	"strings"
	"testing"
)

func TestAggregationIntent_PositiveAndNegative(t *testing.T) {
	positives := []string{
		"How many pre-1920 American coins do I have in my collection?",
		"How much money did I raise in total through all the charity events?",
		"What is the total number of trips I took last year?",
		"Give me a count of the books I finished.",
		"How many model kits have I worked on or bought?",
	}
	for _, q := range positives {
		if !aggregationIntent(q) {
			t.Errorf("aggregationIntent(%q) = false, want true", q)
		}
	}
	negatives := []string{
		"How much does the new phone cost?",      // single-fact price lookup
		"How much can I raise my bicycle seat?",  // bare "raise" must NOT fire (FM-76)
		"How much does the total cost come to?",  // bare "total" must NOT fire (FM-76)
		"Which laptop brand is my favorite?",     // preference, not aggregation
		"When did I last visit Paris?",           // factual recall
		"What did I say about the project?",      // open recall
	}
	for _, q := range negatives {
		if aggregationIntent(q) {
			t.Errorf("aggregationIntent(%q) = true, want false", q)
		}
	}
}

// Relative-time arithmetic ("how many days/weeks/hours ago") is temporal reasoning,
// not aggregation — it must NOT trip the aggregation directive.
func TestAggregationIntent_TemporalExcluded(t *testing.T) {
	for _, q := range []string{
		"How many weeks ago did I recover from the flu?",
		"How many days ago did I attend the baking class?",
		"How many hours before the flight did I leave?",
	} {
		if aggregationIntent(q) {
			t.Errorf("aggregationIntent(%q) = true, want false (temporal)", q)
		}
	}
}

func TestAttachSynthesisDirective_AggregationAndPrecedence(t *testing.T) {
	// aggregation query → aggregation directive attached
	agg := map[string]any{}
	attachSynthesisDirective(agg, "How many cocktails have I made in total?")
	d, ok := agg["synthesis_directive"].(string)
	if !ok {
		t.Fatal("expected synthesis_directive for aggregation query")
	}
	if d != aggregationSynthesisDirective {
		t.Error("aggregation query attached the wrong directive")
	}
	// preference takes precedence when a query reads as both
	pref := map[string]any{}
	attachSynthesisDirective(pref, "What is my favorite drink and how many do I prefer?")
	if pref["synthesis_directive"] != preferenceSynthesisDirective {
		t.Error("preference intent must take precedence over aggregation")
	}
	// neither → nothing attached
	plain := map[string]any{}
	attachSynthesisDirective(plain, "When did I last visit Paris?")
	if _, ok := plain["synthesis_directive"]; ok {
		t.Error("expected NO synthesis_directive for a plain factual query")
	}
}

func TestAggregationSynthesisDirective_Content(t *testing.T) {
	d := strings.ToLower(aggregationSynthesisDirective)
	// breadth (context-depth lesson), enumerate, dedup, explicit total, abstention (FM-76)
	for _, must := range []string{"all", "every", "total", "insufficient", "once"} {
		if !strings.Contains(d, must) {
			t.Errorf("aggregationSynthesisDirective missing %q clause:\n%s", must, aggregationSynthesisDirective)
		}
	}
}
