package longmemeval_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

func TestParseScoreLabel_Valid(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"CORRECT\nBecause X.", "CORRECT"},
		{"PARTIALLY_CORRECT\nSome details missing.", "PARTIALLY_CORRECT"},
		{"INCORRECT\nWrong answer.", "INCORRECT"},
		{"  correct  \nexplanation here", "CORRECT"},
	}
	for _, c := range cases {
		got, _ := longmemeval.ParseScoreLabel(c.input)
		if got != c.want {
			t.Errorf("ParseScoreLabel(%q) label = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestParseScoreLabel_Invalid(t *testing.T) {
	label, _ := longmemeval.ParseScoreLabel("I'm not sure about this one.")
	if label != "PARTIALLY_CORRECT" {
		t.Errorf("unrecognised label default = %q, want PARTIALLY_CORRECT", label)
	}
}

func TestParseScoreLabel_Explanation(t *testing.T) {
	_, explanation := longmemeval.ParseScoreLabel("CORRECT\nThe answer matches the reference exactly.")
	if !strings.Contains(explanation, "matches") {
		t.Errorf("explanation = %q, want it to contain 'matches'", explanation)
	}
}

func TestParseScoreLabel_NoExplanation(t *testing.T) {
	label, explanation := longmemeval.ParseScoreLabel("CORRECT")
	if label != "CORRECT" {
		t.Errorf("label = %q, want CORRECT", label)
	}
	if explanation != "" {
		t.Errorf("explanation = %q, want empty", explanation)
	}
}

func TestScoringPrompt_ContainsFields(t *testing.T) {
	prompt := longmemeval.ScoringPrompt("What year?", "2023", "It was 2023.")
	if !strings.Contains(prompt, "What year?") {
		t.Error("prompt missing question")
	}
	if !strings.Contains(prompt, "2023") {
		t.Error("prompt missing reference answer")
	}
	if !strings.Contains(prompt, "It was 2023.") {
		t.Error("prompt missing hypothesis")
	}
	if !strings.Contains(prompt, "CORRECT") {
		t.Error("prompt missing CORRECT label instruction")
	}
}

func TestGenerationPrompt_ContainsFields(t *testing.T) {
	ctx := []string{"Memory block one.", "Memory block two."}
	prompt := longmemeval.GenerationPrompt("Who was there?", "2024-01-15", ctx)
	if !strings.Contains(prompt, "Who was there?") {
		t.Error("prompt missing question")
	}
	if !strings.Contains(prompt, "2024-01-15") {
		t.Error("prompt missing question date")
	}
	if !strings.Contains(prompt, "Memory block one.") {
		t.Error("prompt missing first context block")
	}
	if !strings.Contains(prompt, "Memory block two.") {
		t.Error("prompt missing second context block")
	}
}

func TestGenerationPrompt_EmptyContext(t *testing.T) {
	prompt := longmemeval.GenerationPrompt("Q?", "2024-01-01", nil)
	if !strings.Contains(prompt, "Q?") {
		t.Error("prompt missing question")
	}
}

func TestGenerateOAI_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected Content-Type: %s", r.Header.Get("Content-Type"))
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"hello world"}}]}`)
	}))
	defer srv.Close()

	ctx := context.Background()
	out, err := longmemeval.GenerateOAI(ctx, "say hello", srv.URL, "test-model", 0)
	if err != nil {
		t.Fatalf("GenerateOAI: %v", err)
	}
	if out != "hello world" {
		t.Errorf("output = %q, want %q", out, "hello world")
	}
}

func TestGenerateOAI_EmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[]}`)
	}))
	defer srv.Close()

	ctx := context.Background()
	_, err := longmemeval.GenerateOAI(ctx, "prompt", srv.URL, "test-model", 0)
	if err == nil {
		t.Error("expected error for empty choices, got nil")
	}
}

func TestGenerateOAI_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx := context.Background()
	_, err := longmemeval.GenerateOAI(ctx, "prompt", srv.URL, "test-model", 0)
	if err == nil {
		t.Error("expected error for HTTP 500, got nil")
	}
}

func TestGenerateOAI_TrimsWhitespace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"  trimmed\n"}}]}`)
	}))
	defer srv.Close()

	ctx := context.Background()
	out, err := longmemeval.GenerateOAI(ctx, "prompt", srv.URL, "model", 0)
	if err != nil {
		t.Fatalf("GenerateOAI: %v", err)
	}
	if out != "trimmed" {
		t.Errorf("output = %q, want %q", out, "trimmed")
	}
}

func TestGenerateOAI_StripThinkBlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"<think>reasoning here</think>\nfinal answer"}}]}`)
	}))
	defer srv.Close()

	ctx := context.Background()
	out, err := longmemeval.GenerateOAI(ctx, "prompt", srv.URL, "model", 0)
	if err != nil {
		t.Fatalf("GenerateOAI: %v", err)
	}
	if strings.Contains(out, "<think>") {
		t.Errorf("output still contains <think>: %q", out)
	}
	if out != "final answer" {
		t.Errorf("output = %q, want %q", out, "final answer")
	}
}

func TestGenerateOAI_EmptyContentFallsBackToReasoning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"","reasoning":"reasoning only"}}]}`)
	}))
	defer srv.Close()

	ctx := context.Background()
	out, err := longmemeval.GenerateOAI(ctx, "prompt", srv.URL, "model", 0)
	if err != nil {
		t.Fatalf("GenerateOAI: %v", err)
	}
	if out != "reasoning only" {
		t.Errorf("output = %q, want reasoning only", out)
	}
}

func TestGenerateOAI_BothEmpty_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"","reasoning":""}}]}`)
	}))
	defer srv.Close()

	ctx := context.Background()
	_, err := longmemeval.GenerateOAI(ctx, "prompt", srv.URL, "model", 0)
	if err == nil {
		t.Error("expected error when both content and reasoning are empty")
	}
}

func TestScoreOAI_ReturnsCorrect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"CORRECT\nMatches reference."}}]}`)
	}))
	defer srv.Close()

	ctx := context.Background()
	result, err := longmemeval.ScoreOAI(ctx, "What is X?", "X is 5", "X is 5", srv.URL, "test-model", 0)
	if err != nil {
		t.Fatalf("ScoreOAI: %v", err)
	}
	if result.Label != "CORRECT" {
		t.Errorf("label = %q, want CORRECT", result.Label)
	}
	if !strings.Contains(result.Explanation, "Matches") {
		t.Errorf("explanation = %q, want it to contain 'Matches'", result.Explanation)
	}
}

func TestScoreOAI_HTTPError_ReturnsPartiallyCorrect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctx := context.Background()
	result, err := longmemeval.ScoreOAI(ctx, "Q", "A", "B", srv.URL, "model", 0)
	if err == nil {
		t.Error("expected error for HTTP 503, got nil")
	}
	if result.Label != "PARTIALLY_CORRECT" {
		t.Errorf("label = %q, want PARTIALLY_CORRECT as default on error", result.Label)
	}
}

// TestGenerate_RequiresClaude is skipped in short mode.
func TestGenerate_RequiresClaude(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	ctx := context.Background()
	out, err := longmemeval.Generate(ctx, "Reply with only the word: HELLO", 1)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if out == "" {
		t.Error("Generate returned empty output")
	}
}
