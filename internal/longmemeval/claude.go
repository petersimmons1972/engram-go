package longmemeval

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// claudePrintTimeout is the hard cap for one claude --print call.

const generateTimeout = 90 * time.Second

// Generate calls `claude --print prompt` and returns trimmed stdout.
// retries is the number of additional attempts on failure (0 = try once).
func Generate(ctx context.Context, prompt string, retries int) (string, error) {
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		out, err := runClaude(ctx, prompt)
		if err == nil {
			return out, nil
		}
		lastErr = err
	}
	return "", lastErr
}

func runClaude(ctx context.Context, prompt string) (string, error) {
	tctx, cancel := context.WithTimeout(ctx, generateTimeout)
	defer cancel()
	// Pass the prompt via stdin rather than argv so we don't blow past
	// the OS argv limit (E2BIG / "argument list too long") on large
	// retrieved contexts (~10 sessions × ~7 KB = ~70 KB prompts).
	cmd := exec.CommandContext(tctx, "claude", "--print")
	cmd.Stdin = strings.NewReader(prompt)
	raw, err := cmd.Output()
	if err != nil {
		if tctx.Err() != nil {
			return "", fmt.Errorf("claude --print timed out after %s", generateTimeout)
		}
		return "", fmt.Errorf("claude --print: %w", err)
	}
	return strings.TrimSpace(string(raw)), nil
}

// GenerationPrompt builds the prompt for answer generation.
func GenerationPrompt(question, questionDate string, contextBlocks []string) string {
	ctx := strings.Join(contextBlocks, "\n\n---\n\n")
	return fmt.Sprintf(`You are answering questions about a person's conversation history.

Relevant memory context:
%s

Question (asked on %s): %s

Answer the question based only on the provided context. If the answer cannot be determined from the context, respond with exactly: I don't know.`, ctx, questionDate, question)
}

// ScoringPrompt builds the judge prompt for answer scoring.
func ScoringPrompt(question, referenceAnswer, hypothesis string) string {
	return fmt.Sprintf(`You are judging whether a generated answer correctly answers a question about conversation history.

Question: %s

Reference answer: %s

Generated answer: %s

Is the generated answer correct? Reply with exactly one of these labels on the first line:
CORRECT
PARTIALLY_CORRECT
INCORRECT

Then on the second line, briefly explain why (one sentence).`, question, referenceAnswer, hypothesis)
}

// ScoreResult holds the parsed output of the judge prompt.
type ScoreResult struct {
	Label       string
	Explanation string
}

// Score calls claude --print with the judge prompt and parses the result.
func Score(ctx context.Context, question, referenceAnswer, hypothesis string, retries int) (ScoreResult, error) {
	prompt := ScoringPrompt(question, referenceAnswer, hypothesis)
	out, err := Generate(ctx, prompt, retries)
	if err != nil {
		return ScoreResult{Label: "PARTIALLY_CORRECT"}, err
	}
	label, explanation := ParseScoreLabel(out)
	return ScoreResult{Label: label, Explanation: explanation}, nil
}

// ParseScoreLabel extracts the label and explanation from raw judge output.
// Returns PARTIALLY_CORRECT as default if the label is unrecognised.
func ParseScoreLabel(raw string) (label, explanation string) {
	lines := strings.SplitN(strings.TrimSpace(raw), "\n", 2)
	first := strings.ToUpper(strings.TrimSpace(lines[0]))
	switch first {
	case "CORRECT", "PARTIALLY_CORRECT", "INCORRECT":
		label = first
	default:
		label = "PARTIALLY_CORRECT"
	}
	if len(lines) > 1 {
		explanation = strings.TrimSpace(lines[1])
	}
	return label, explanation
}
