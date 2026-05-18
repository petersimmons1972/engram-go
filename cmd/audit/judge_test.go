package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// mockLLMClient is a test spy implementing llm.LLMClient.
// It records calls and returns a fixed response or error.
type mockLLMClient struct {
	response    string
	err         error
	capturedSys string
	capturedUser string
}

func (m *mockLLMClient) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	m.capturedSys = systemPrompt
	m.capturedUser = userPrompt
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return m.response, m.err
}

// wellFormedVerdict produces a complete audit response for a given verdict.
func wellFormedVerdict(verdict string) string {
	return "IS_VALID: yes\nIS_ACTIONABLE: yes\nIS_SPECIFIC: yes\nFALSE_POSITIVE: no\nVERDICT: " + verdict + "\nREASON: Test reason sentence."
}

func TestJudgeHappyPath(t *testing.T) {
	client := &mockLLMClient{response: wellFormedVerdict("KEEP")}
	m := engramMemory{
		ID:         "test-id",
		Content:    "always retries failed edits",
		Tags:       []string{"instinct", "correction", "git", "sig-retry"},
		Importance: 0.8,
	}

	res := Judge(context.Background(), client, 60*time.Second, m)

	if res.ID != "test-id" {
		t.Errorf("ID: want test-id, got %s", res.ID)
	}
	if res.Verdict != "KEEP" {
		t.Errorf("Verdict: want KEEP, got %s", res.Verdict)
	}
	if res.IsValid != "yes" {
		t.Errorf("IsValid: want yes, got %s", res.IsValid)
	}
	if res.IsActionable != "yes" {
		t.Errorf("IsActionable: want yes, got %s", res.IsActionable)
	}
	if res.IsSpecific != "yes" {
		t.Errorf("IsSpecific: want yes, got %s", res.IsSpecific)
	}
	if res.FalsePositive != "no" {
		t.Errorf("FalsePositive: want no, got %s", res.FalsePositive)
	}
	if res.Reason != "Test reason sentence." {
		t.Errorf("Reason: want %q, got %q", "Test reason sentence.", res.Reason)
	}
	if res.Confidence != 0.8 {
		t.Errorf("Confidence: want 0.8, got %f", res.Confidence)
	}
}

func TestJudgeAllVerdictTypes(t *testing.T) {
	tests := []struct {
		verdict string
	}{
		{"KEEP"},
		{"TUNE"},
		{"REJECT"},
	}
	for _, tt := range tests {
		t.Run(tt.verdict, func(t *testing.T) {
			client := &mockLLMClient{response: wellFormedVerdict(tt.verdict)}
			m := engramMemory{ID: "x", Tags: []string{"instinct", "sig-x"}}
			res := Judge(context.Background(), client, 60*time.Second, m)
			if res.Verdict != tt.verdict {
				t.Errorf("Verdict: want %s, got %s", tt.verdict, res.Verdict)
			}
		})
	}
}

func TestJudgeErrorPath(t *testing.T) {
	client := &mockLLMClient{err: errors.New("llm unavailable")}
	m := engramMemory{ID: "err-id", Tags: []string{"instinct", "sig-e"}}

	res := Judge(context.Background(), client, 60*time.Second, m)

	if res.Verdict != "ERROR" {
		t.Errorf("Verdict: want ERROR, got %s", res.Verdict)
	}
	if res.Reason == "" {
		t.Error("Reason: must be set when error occurs")
	}
	if !strings.Contains(res.Reason, "llm unavailable") {
		t.Errorf("Reason: want it to contain error message, got %q", res.Reason)
	}
}

func TestJudgeMalformedResponse(t *testing.T) {
	tests := []struct {
		name     string
		response string
	}{
		{"empty", ""},
		{"garbage", "not a valid response at all"},
		{"partial lines", "IS_VALID: yes\nVERDICT: KEEP"},
		{"no colon", "ISVALID yes\nVERDICT KEEP"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockLLMClient{response: tt.response}
			m := engramMemory{ID: "m-id", Tags: []string{"instinct", "sig-m"}}
			// Must not panic.
			res := Judge(context.Background(), client, 60*time.Second, m)
			_ = res // partial parse is acceptable; no panic is the requirement
		})
	}
}

func TestJudgeContextTimeout(t *testing.T) {
	// Use a very short timeout so it fires before the mock can respond.
	client := &mockLLMClient{
		response: wellFormedVerdict("KEEP"),
		// We'll cancel the context before the call to simulate timeout.
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	// Yield so the timeout fires.
	time.Sleep(1 * time.Millisecond)

	m := engramMemory{ID: "timeout-id", Tags: []string{"instinct", "sig-t"}}
	// Judge should handle context cancellation without panic.
	res := Judge(ctx, client, 60*time.Second, m)
	// Either an error verdict or the context error shows up in Reason.
	if res.Verdict != "ERROR" && res.Verdict != "KEEP" {
		t.Errorf("unexpected verdict on cancelled ctx: %s", res.Verdict)
	}
}

func TestJudgePromptIncludesPatternFields(t *testing.T) {
	client := &mockLLMClient{response: wellFormedVerdict("KEEP")}
	m := engramMemory{
		ID:      "spy-id",
		Content: "test content description",
		Tags:    []string{"instinct", "correction", "git", "sig-spy"},
	}
	_ = Judge(context.Background(), client, 60*time.Second, m)

	// The user prompt must contain the pattern's ptype, content, domain, and sig tag.
	userPrompt := client.capturedUser
	if userPrompt == "" {
		t.Fatal("no user prompt was captured")
	}
	for _, substr := range []string{"correction", "test content description", "git", "sig-spy"} {
		if !strings.Contains(userPrompt, substr) {
			t.Errorf("user prompt missing %q\nfull prompt:\n%s", substr, userPrompt)
		}
	}
}
