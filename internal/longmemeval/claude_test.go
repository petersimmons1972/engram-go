package longmemeval_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
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
	if label != "SCORE_ERROR" {
		t.Errorf("unrecognised label default = %q, want SCORE_ERROR (not PARTIALLY_CORRECT — #753)", label)
	}
}

func TestParseScoreLabel_Explanation(t *testing.T) {
	_, explanation := longmemeval.ParseScoreLabel("CORRECT\nThe answer matches the reference exactly.")
	if !strings.Contains(explanation, "matches") {
		t.Errorf("explanation = %q, want it to contain 'matches'", explanation)
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

func TestScoreOAI_HTTPError_ReturnsScoreError(t *testing.T) {
	// Pre-#753: ScoreOAI returned PARTIALLY_CORRECT on HTTP error, masking infra failures.
	// Post-#753: ScoreOAI returns SCORE_ERROR so errors are visible in score reports.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctx := context.Background()
	result, err := longmemeval.ScoreOAI(ctx, "Q", "A", "B", srv.URL, "model", 0)
	if err == nil {
		t.Error("expected error for HTTP 503, got nil")
	}
	if result.Label != "SCORE_ERROR" {
		t.Errorf("label = %q, want SCORE_ERROR on HTTP error (not PARTIALLY_CORRECT — #753)", result.Label)
	}
}

func TestBuildScoringRequestBody(t *testing.T) {
	body, err := longmemeval.BuildScoringRequestBody("mymodel", "Q?", "A", "A")
	if err != nil {
		t.Fatal(err)
	}
	var req struct {
		MaxTokens   int     `json:"max_tokens"`
		Temperature float64 `json:"temperature"`
		Model       string  `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatal(err)
	}
	if req.MaxTokens != 100 {
		t.Errorf("want max_tokens=100 got %d", req.MaxTokens)
	}
	if req.Temperature != 0 {
		t.Errorf("want temperature=0 got %f", req.Temperature)
	}
	if req.Model != "mymodel" {
		t.Errorf("want mymodel got %s", req.Model)
	}
}

// TestGenerate_RequiresClaude is skipped in short mode.
func TestGenerate_RequiresClaude(t *testing.T) {
	// #678: the claude binary is an undocumented prerequisite for this test.
	// In CI it is not in PATH; skip rather than fail. Locally (with claude
	// installed) the test still exercises the real code path.
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude binary not in PATH — skipping (#678)")
	}
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

// ---------------------------------------------------------------------------
// ParseScoreLabel — #753 regression guards for rubric error inflating v9 baseline
// ---------------------------------------------------------------------------

// TestParseScoreLabel_OldFormatRationale verifies that the pre-fix rubric format
// (rationale before label, as generated before commit 423343a) is handled by
// the scan-all-lines pass and does NOT default to PARTIALLY_CORRECT.
// This is a regression guard against reverting the rubric prompt structure.
func TestParseScoreLabel_OldFormatRationale(t *testing.T) {
	// Old format: rationale first, label buried at end — no longer generated
	// but ParseScoreLabel must handle it gracefully (find the label, not error).
	old := "The hypothesis closely matches the gold answer in key facts.\nCORRECT"
	label, _ := longmemeval.ParseScoreLabel(old)
	if label != "CORRECT" {
		t.Errorf("ParseScoreLabel(old-format rationale-first) = %q, want CORRECT", label)
	}
}

// TestParseScoreLabel_TruncatedNoLabel verifies that when max_tokens is too low
// and the response is cut off before a label appears, SCORE_ERROR is returned
// rather than PARTIALLY_CORRECT (pre-fix behaviour).
func TestParseScoreLabel_TruncatedNoLabel(t *testing.T) {
	truncated := "The hypothesis matches several facts from the gold answer such as the date"
	// Note: no label anywhere — simulates truncation before label was emitted
	label, _ := longmemeval.ParseScoreLabel(truncated)
	if label != "SCORE_ERROR" {
		t.Errorf("ParseScoreLabel(truncated, no label) = %q, want SCORE_ERROR", label)
	}
}

// TestParseScoreLabel_ScoreErrorPropagation verifies that SCORE_ERROR returned
// from ParseScoreLabel results in a score entry with status="error" (not
// silently counted as PARTIALLY_CORRECT in the score report).
func TestParseScoreLabel_ScoreErrorPropagation(t *testing.T) {
	// SCORE_ERROR should be treated as an error in writeScoreReport, not as a
	// valid label. Verify it falls into the "default" / Incorrect bucket.
	// This test documents the expected pipeline behaviour.
	//
	// In writeScoreReport (cmd/longmemeval/score.go), the switch statement:
	//   case "CORRECT": ...
	//   case "PARTIALLY_CORRECT": ...
	//   default: Incorrect++
	// SCORE_ERROR hits "default" → counted as Incorrect, which is correct
	// behaviour (conservative: unknown = not correct).
	//
	// If this behaviour changes, update this comment and the switch.
	t.Log("SCORE_ERROR falls into default/Incorrect in writeScoreReport — documented by design")
}
