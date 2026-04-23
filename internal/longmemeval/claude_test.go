package longmemeval_test

import (
	"context"
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
