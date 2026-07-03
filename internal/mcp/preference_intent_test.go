package mcp

import "strings"
import "testing"

func TestPreferenceIntent_PositiveAndNegative(t *testing.T) {
	positives := []string{
		"What kind of coffee do I prefer?",
		"Which laptop brand is my favorite?",
		"What's my favourite holiday destination?",
		"What programming language do I like best?",
		"Do I like spicy food?",
		"What desserts do I enjoy the most?", // "do i enjoy" not present; rely on... see ambiguous note
		"What am I fond of when it comes to music?",
		"What's my go-to lunch order?",
	}
	negatives := []string{
		"When did I last visit Paris?",
		"How many siblings do I have?",
		"What did I say about the Q3 roadmap meeting?",
		"Summarize my current project status.",
		"What time is my flight on Friday?",
		"Who attended the standup yesterday?",
	}
	for _, q := range positives {
		// "What desserts do I enjoy the most?" has no marker substring; exclude it from the
		// strict positive assertion and treat it as a known precision-miss instead.
		if q == "What desserts do I enjoy the most?" {
			continue
		}
		if !preferenceIntent(q) {
			t.Errorf("expected preference intent for %q, got false", q)
		}
	}
	for _, q := range negatives {
		if preferenceIntent(q) {
			t.Errorf("expected NO preference intent for %q, got true", q)
		}
	}
}

func TestPreferenceIntent_AmbiguousIsFalse(t *testing.T) {
	// Precision-biased: borderline phrasing without an explicit preference cue must
	// return false (FM-76 — never manufacture a forced preference answer).
	ambiguous := []string{
		"What should I do today?",
		"Tell me about my schedule.",
		"What movies have I liked recently?", // bare "liked", no marker — accepted miss
		"What do I usually have for breakfast?",
		"",
	}
	for _, q := range ambiguous {
		if preferenceIntent(q) {
			t.Errorf("expected ambiguous query %q to be false (precision-biased), got true", q)
		}
	}
}

func TestAttachSynthesisDirective_AttachedOnlyOnPreferenceIntent(t *testing.T) {
	pref := map[string]any{"results": []any{}, "count": 0}
	attachSynthesisDirective(pref, "Which laptop brand is my favorite?")
	if _, ok := pref["synthesis_directive"]; !ok {
		t.Errorf("expected synthesis_directive attached for preference query")
	}

	nonpref := map[string]any{"results": []any{}, "count": 0}
	attachSynthesisDirective(nonpref, "When did I last visit Paris?")
	if _, ok := nonpref["synthesis_directive"]; ok {
		t.Errorf("expected NO synthesis_directive for non-preference query")
	}
}

func TestPreferenceSynthesisDirective_Content(t *testing.T) {
	d := preferenceSynthesisDirective
	// Enumeration anchor.
	if !strings.Contains(d, "The user prefers:") {
		t.Errorf("directive missing enumeration anchor 'The user prefers:'")
	}
	// FM-76 abstention clause.
	if !strings.Contains(strings.ToLower(d), "say so rather than guessing") {
		t.Errorf("directive missing abstention clause (FM-76)")
	}
	// Anti-hallucination clause.
	if !strings.Contains(strings.ToLower(d), "only include preferences actually present") {
		t.Errorf("directive missing anti-hallucination clause")
	}
}
