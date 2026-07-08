package main

import (
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// TestAntiHedgePrompts_SSPreferenceHappyPath verifies that with
// --anti-hedge-prompts on, a single-session-preference question's generation
// prompt (as actually sent to the LLM through the runOne pipeline) contains
// the anti-hedge addendum. Uses the h8h12Capture fake MCP/LLM server pattern
// (runOneWithCapture, cmd/longmemeval/h8h12_test.go) so this exercises real
// wiring through run.go, not just the pure prompt builder.
func TestAntiHedgePrompts_SSPreferenceHappyPath(t *testing.T) {
	item := longmemeval.Item{
		QuestionID:   "q-anti-hedge-happy",
		Question:     "What kind of restaurant would I like?",
		QuestionType: "single-session-preference",
		QuestionDate: "2024-06-01",
	}
	capture := runOneWithCapture(t, &Config{
		RecallTopK:       100,
		AntiHedgePrompts: true,
	}, item)
	got := capture.lastPrompt(t)
	if !strings.Contains(strings.ToLower(got), "anti-hedge rule") {
		t.Fatalf("prompt sent to LLM missing anti-hedge addendum for ss-preference question:\n%s", got)
	}
}

// TestAntiHedgePrompts_FlagOffIsBaseline verifies that with the flag off, the
// prompt actually sent to the LLM for a single-session-preference question is
// byte-identical to the standard baseline prompt (no addendum leaks in).
func TestAntiHedgePrompts_FlagOffIsBaseline(t *testing.T) {
	item := longmemeval.Item{
		QuestionID:   "q-anti-hedge-off",
		Question:     "What kind of restaurant would I like?",
		QuestionType: "single-session-preference",
		QuestionDate: "2024-06-01",
	}
	capture := runOneWithCapture(t, &Config{RecallTopK: 100}, item)
	got := capture.lastPrompt(t)
	want := longmemeval.GenerationPromptForType(item.Question, item.QuestionType, item.QuestionDate, []string{
		"Session date: 2024-05-10\nCalled my sister.",
	})
	if got != want {
		t.Fatalf("anti-hedge flag off should preserve the baseline ss-preference prompt\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

// TestAntiHedgePrompts_NonPreferenceTypeUnaffected verifies the flag is a
// no-op end-to-end for a question type that is neither
// single-session-preference nor inferred-preference-shaped, even when on.
func TestAntiHedgePrompts_NonPreferenceTypeUnaffected(t *testing.T) {
	item := longmemeval.Item{
		QuestionID:   "q-anti-hedge-nonpref",
		Question:     "How many times did I call my sister?",
		QuestionType: "multi-session",
		QuestionDate: "2024-06-01",
	}
	capture := runOneWithCapture(t, &Config{
		RecallTopK:       100,
		AntiHedgePrompts: true,
	}, item)
	got := capture.lastPrompt(t)
	if strings.Contains(strings.ToLower(got), "anti-hedge rule") {
		t.Fatalf("anti-hedge addendum leaked into a non-preference question type prompt:\n%s", got)
	}
	want := longmemeval.GenerationPromptForType(item.Question, item.QuestionType, item.QuestionDate, []string{
		"Session date: 2024-05-10\nCalled my sister.",
	})
	if got != want {
		t.Fatalf("non-preference type prompt should be unaffected by --anti-hedge-prompts\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

// TestAntiHedgePrompts_FeatureFlagPersisted verifies buildFeatureFlags records
// anti_hedge_prompts=true in run provenance when the flag is set, so registry
// rows can attribute results to this lever (mirrors sibling flags like
// dual_preference_recall/topic_anchor_boost in cmd/longmemeval/provenance.go).
func TestAntiHedgePrompts_FeatureFlagPersisted(t *testing.T) {
	flags := buildFeatureFlags(&Config{AntiHedgePrompts: true})
	v, ok := flags["anti_hedge_prompts"]
	if !ok || v != true {
		t.Fatalf("buildFeatureFlags missing anti_hedge_prompts=true, got: %#v", flags)
	}
}

// TestAntiHedgePrompts_FeatureFlagAbsentWhenOff verifies the flag key is
// omitted entirely (not just false) when --anti-hedge-prompts is not set —
// matching the sparse feature_flags convention every sibling flag follows.
func TestAntiHedgePrompts_FeatureFlagAbsentWhenOff(t *testing.T) {
	flags := buildFeatureFlags(&Config{})
	if _, ok := flags["anti_hedge_prompts"]; ok {
		t.Fatalf("buildFeatureFlags should omit anti_hedge_prompts when off, got: %#v", flags)
	}
}

// TestAntiHedgePrompts_InferredPreferenceBoundary verifies the addendum fires
// end-to-end for a "recommend"-phrased question even when the dataset's own
// question_type label is not single-session-preference — the boundary case
// covered by IsInferredPreferenceQuestion (the same gate --dual-preference-recall
// already uses).
func TestAntiHedgePrompts_InferredPreferenceBoundary(t *testing.T) {
	item := longmemeval.Item{
		QuestionID:   "q-anti-hedge-inferred",
		Question:     "Can you recommend a hotel for my trip to Miami?",
		QuestionType: "single-session-user",
		QuestionDate: "2024-06-01",
	}
	if !longmemeval.IsInferredPreferenceQuestion(item.Question) {
		t.Fatalf("test setup: question must be recognised as inferred-preference")
	}
	capture := runOneWithCapture(t, &Config{
		RecallTopK:       100,
		AntiHedgePrompts: true,
	}, item)
	got := capture.lastPrompt(t)
	if !strings.Contains(strings.ToLower(got), "anti-hedge rule") {
		t.Fatalf("inferred-preference question missing anti-hedge addendum end-to-end:\n%s", got)
	}
}
