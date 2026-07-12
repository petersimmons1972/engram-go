package main

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// TestSelectGenerationPrompt_AllFlagBranches drives every branch of the shared
// selectGenerationPrompt switch and asserts each matches the corresponding
// per-type prompt variant. With RunOpts{} (EnumerateFirst=false) ApplyEnumerateFirst
// is a no-op, so the helper output must equal the variant call directly. This
// exercises the branches (temporal-prompt-aug, preference-enumerate,
// preference-ground, anti-hedge, ku-recency) that the oracle-path table test
// does not, keeping the new file above the coverage floor.
func TestSelectGenerationPrompt_AllFlagBranches(t *testing.T) {
	const (
		q   = "How many concerts did I attend?"
		qd  = "2024-06-01"
		qt  = "temporal-reasoning"
		qtp = "single-session-preference"
		qtk = "knowledge-update"
	)
	ctx := []string{"Session date: 2024-05-10\nuser: went to a concert."}
	ro := longmemeval.RunOpts{}

	cases := []struct {
		name  string
		cfg   *Config
		qtype string
		want  string
	}{
		{"temporal-prompt-aug", &Config{TemporalPromptAug: true}, qt,
			longmemeval.GenerationPromptForTypeWithTemporalAug(q, qt, qd, ctx, true)},
		{"inject-question-date", &Config{InjectQuestionDate: true}, qt,
			longmemeval.GenerationPromptForTypeWithDateInjection(q, qt, qd, ctx, true)},
		{"preference-enumerate", &Config{PreferenceEnumerate: true}, qtp,
			longmemeval.GenerationPromptForTypePreferenceEnumerate(q, qtp, qd, ctx, true)},
		{"preference-ground", &Config{PreferenceGround: true}, qtp,
			longmemeval.GenerationPromptForTypePreferenceGround(q, qtp, qd, ctx, true)},
		{"anti-hedge", &Config{AntiHedgePrompts: true}, qtp,
			longmemeval.GenerationPromptForTypeAntiHedge(q, qtp, qd, ctx, true)},
		{"ku-recency", &Config{KURecencyPrompt: true}, qtk,
			longmemeval.GenerationPromptForTypeKURecency(q, qtk, qd, ctx, true)},
		{"default", &Config{}, qt,
			longmemeval.GenerationPromptForType(q, qt, qd, ctx)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			item := longmemeval.Item{Question: q, QuestionType: c.qtype, QuestionDate: qd}
			got := selectGenerationPrompt(c.cfg, ro, item, ctx)
			if got != c.want {
				t.Errorf("%s: selectGenerationPrompt did not match the expected variant.\n got: %q\nwant: %q",
					c.name, got, c.want)
			}
		})
	}
}

// TestSelectGenerationPrompt_Precedence verifies temporal-prompt-aug wins over
// inject-question-date when both are set (the documented precedence).
func TestSelectGenerationPrompt_Precedence(t *testing.T) {
	q, qd, qt := "How many days ago?", "2024-06-01", "temporal-reasoning"
	ctx := []string{"Session date: 2024-05-10\nuser: hi"}
	item := longmemeval.Item{Question: q, QuestionType: qt, QuestionDate: qd}
	got := selectGenerationPrompt(&Config{TemporalPromptAug: true, InjectQuestionDate: true}, longmemeval.RunOpts{}, item, ctx)
	want := longmemeval.GenerationPromptForTypeWithTemporalAug(q, qt, qd, ctx, true)
	if got != want {
		t.Errorf("precedence: temporal-prompt-aug must win over inject-question-date")
	}
}
