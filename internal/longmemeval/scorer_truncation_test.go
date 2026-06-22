// scorer_truncation_test.go — TDD tests for hypothesis truncation and max_tokens budget.
// These tests must FAIL before the fix is applied and PASS after.
// Context: Nemotron-as-judge (65536-token context) returns HTTP 400 when the
// scoring request exceeds the model context window. Root cause: BuildScoringRequestBody
// inserts the hypothesis with no length guard, and DefaultScorerMaxTokens was 2048.
// Fix A: DefaultScorerMaxTokens = 512.
// Fix B: cap hypothesis length before building prompt so total tokens fit in 65536.
package longmemeval

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestDefaultScorerMaxTokens_Is512 verifies that the default max_tokens budget
// for the scoring judge is 512, not 2048. The judge emits one verdict word plus
// 1-2 sentences — 512 is ample and keeps us well inside the 65536-token context.
func TestDefaultScorerMaxTokens_Is512(t *testing.T) {
	if DefaultScorerMaxTokens != 512 {
		t.Errorf("DefaultScorerMaxTokens = %d, want 512 (Fix A: reduce from 2048 to leave room for long hypotheses)", DefaultScorerMaxTokens)
	}
}

// TestBuildScoringRequestBody_MaxTokensIs512 verifies that a scoring request
// built with the default max_tokens (<=0 sentinel -> DefaultScorerMaxTokens)
// encodes max_tokens=512 in the JSON body.
func TestBuildScoringRequestBody_MaxTokensIs512(t *testing.T) {
	body, err := buildScoringRequestBody(
		"nvidia/Nemotron-H-8B-Reasoning-HF",
		"What is the user's favourite food?",
		"Pizza",
		"The user likes pizza.",
		0, // sentinel -> DefaultScorerMaxTokens
		ScoringOptions{},
	)
	if err != nil {
		t.Fatalf("buildScoringRequestBody: %v", err)
	}
	var req struct {
		MaxTokens int `json:"max_tokens"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if req.MaxTokens != 512 {
		t.Errorf("max_tokens = %d, want 512", req.MaxTokens)
	}
}

// TestBuildScoringRequestBody_ShortHypothesis_Unchanged verifies that a
// hypothesis well below the cap threshold passes through verbatim.
func TestBuildScoringRequestBody_ShortHypothesis_Unchanged(t *testing.T) {
	const hypothesis = "The user prefers Italian food."
	body, err := buildScoringRequestBody(
		"nvidia/Nemotron-H-8B-Reasoning-HF",
		"What food does the user prefer?",
		"Italian food",
		hypothesis,
		0,
		ScoringOptions{},
	)
	if err != nil {
		t.Fatalf("buildScoringRequestBody: %v", err)
	}
	if !strings.Contains(string(body), hypothesis) {
		t.Errorf("short hypothesis should be present verbatim in the request body; body=%s", body)
	}
}

// TestBuildScoringRequestBody_LongHypothesis_Capped verifies that a
// hypothesis exceeding the per-call character budget is silently capped
// so the total request stays within the 65536-token context window.
//
// Budget: 65536 - DefaultScorerMaxTokens(512) - 400 overhead = 64624 tokens
// Conservative chars-to-tokens ratio: x4 -> 258496 chars max.
// We send a hypothesis of 300000 chars — it must be capped.
func TestBuildScoringRequestBody_LongHypothesis_Capped(t *testing.T) {
	const overLongLen = 300_000
	// Build a hypothesis that is clearly over the budget.
	longHyp := strings.Repeat("a", overLongLen)

	body, err := buildScoringRequestBody(
		"nvidia/Nemotron-H-8B-Reasoning-HF",
		"Summarise the user's preferences.",
		"Short gold answer.",
		longHyp,
		0,
		ScoringOptions{},
	)
	if err != nil {
		t.Fatalf("buildScoringRequestBody with long hypothesis: %v", err)
	}

	// The body must NOT contain the full over-long hypothesis.
	// If hypothesis was not capped, the body would be >= overLongLen bytes.
	if len(body) >= overLongLen {
		t.Errorf("body length = %d (>= %d): hypothesis was NOT capped; expected body to be smaller after capping",
			len(body), overLongLen)
	}
}
