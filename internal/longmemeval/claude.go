package longmemeval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// claudePrintTimeout is the hard cap for one claude --print call.

const generateTimeout = 180 * time.Second

// GenerateForType generates an answer using Sonnet for all question types.
func GenerateForType(ctx context.Context, prompt, questionType string, retries int) (string, error) {
	return generate(ctx, prompt, "sonnet", retries)
}

// Generate calls `claude --print prompt` using Opus and returns trimmed stdout.
// retries is the number of additional attempts on failure (0 = try once).
// On failure a backoff sleep (30s, 60s, 120s) is inserted between attempts
// so transient API rate limits have a chance to clear before retrying.
func Generate(ctx context.Context, prompt string, retries int) (string, error) {
	return generate(ctx, prompt, "opus", retries)
}

// GenerateSonnet is like Generate but uses Sonnet.
func GenerateSonnet(ctx context.Context, prompt string, retries int) (string, error) {
	return generate(ctx, prompt, "sonnet", retries)
}

// GenerateHaiku is like Generate but uses Haiku — suitable for simple
// classification tasks like scoring where reasoning depth doesn't matter.
func GenerateHaiku(ctx context.Context, prompt string, retries int) (string, error) {
	return generate(ctx, prompt, "haiku", retries)
}

func generate(ctx context.Context, prompt, model string, retries int) (string, error) {
	var lastErr error
	backoffs := []time.Duration{30 * time.Second, 60 * time.Second, 120 * time.Second}
	for attempt := 0; attempt <= retries; attempt++ {
		out, err := runClaude(ctx, prompt, model)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if attempt >= retries {
			break
		}
		wait := backoffs[attempt%len(backoffs)]
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(wait):
		}
	}
	return "", lastErr
}

func runClaude(ctx context.Context, prompt, model string) (string, error) {
	tctx, cancel := context.WithTimeout(ctx, generateTimeout)
	defer cancel()
	// Pass the prompt via stdin rather than argv so we don't blow past
	// the OS argv limit (E2BIG / "argument list too long") on large
	// retrieved contexts (~10 sessions × ~7 KB = ~70 KB prompts).
	cmd := exec.CommandContext(tctx, "claude", "--print", "--model", model)
	cmd.Stdin = strings.NewReader(prompt)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	raw, err := cmd.Output()
	if err != nil {
		if tctx.Err() != nil {
			return "", fmt.Errorf("claude --print timed out after %s", generateTimeout)
		}
		stderrSnippet := strings.TrimSpace(stderr.String())
		if len(stderrSnippet) > 200 {
			stderrSnippet = stderrSnippet[:200] + "…"
		}
		if stderrSnippet != "" {
			return "", fmt.Errorf("claude --print: %w: %s", err, stderrSnippet)
		}
		return "", fmt.Errorf("claude --print: %w", err)
	}
	// Sometimes claude prints usage/rate-limit messages to stdout instead of
	// stderr and still exits 0. Treat those as failures so the retry loop
	// kicks in instead of returning a useless "API Error" string as the
	// hypothesis.
	out := strings.TrimSpace(string(raw))
	if strings.HasPrefix(out, "API Error") {
		return "", fmt.Errorf("claude --print: %s", out)
	}
	return out, nil
}

// GenerateOAI calls an OpenAI-compatible chat completions endpoint instead of
// the claude CLI. baseURL is the API root (e.g. "http://oblivion:8000/v1").
// Retry/backoff behaviour mirrors generate().
func GenerateOAI(ctx context.Context, prompt, baseURL, model string, retries int) (string, error) {
	var lastErr error
	backoffs := []time.Duration{30 * time.Second, 60 * time.Second, 120 * time.Second}
	for attempt := 0; attempt <= retries; attempt++ {
		out, err := callOAI(ctx, prompt, baseURL, model)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if attempt >= retries {
			break
		}
		wait := backoffs[attempt%len(backoffs)]
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(wait):
		}
	}
	return "", lastErr
}

func callOAI(ctx context.Context, prompt, baseURL, model string) (string, error) {
	tctx, cancel := context.WithTimeout(ctx, generateTimeout)
	defer cancel()

	type oaiMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	reqBody, err := json.Marshal(struct {
		Model    string       `json:"model"`
		Messages []oaiMessage `json:"messages"`
	}{
		Model:    model,
		Messages: []oaiMessage{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return "", fmt.Errorf("marshal OAI request: %w", err)
	}

	url := strings.TrimRight(baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(tctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("create OAI request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if tctx.Err() != nil {
			return "", fmt.Errorf("OAI request timed out after %s", generateTimeout)
		}
		return "", fmt.Errorf("OAI request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OAI request: status %d", resp.StatusCode)
	}

	var oaiResp struct {
		Choices []struct {
			Message oaiMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&oaiResp); err != nil {
		return "", fmt.Errorf("decode OAI response: %w", err)
	}
	if len(oaiResp.Choices) == 0 {
		return "", fmt.Errorf("OAI response: no choices returned")
	}
	return strings.TrimSpace(oaiResp.Choices[0].Message.Content), nil
}

// ScoreOAI is like Score but uses the OpenAI-compatible endpoint.
func ScoreOAI(ctx context.Context, question, referenceAnswer, hypothesis, baseURL, model string, retries int) (ScoreResult, error) {
	prompt := ScoringPrompt(question, referenceAnswer, hypothesis)
	out, err := GenerateOAI(ctx, prompt, baseURL, model, retries)
	if err != nil {
		return ScoreResult{Label: "PARTIALLY_CORRECT"}, err
	}
	label, explanation := ParseScoreLabel(out)
	return ScoreResult{Label: label, Explanation: explanation}, nil
}

// GenerationPrompt builds the prompt for answer generation.
func GenerationPrompt(question, questionDate string, contextBlocks []string) string {
	ctx := strings.Join(contextBlocks, "\n\n---\n\n")
	return fmt.Sprintf(`You are answering questions about a person's conversation history.

Each memory block may begin with a "Session date: YYYY-MM-DD" header. Use these dates for any relative-time calculations (e.g. "how many days/weeks ago"). The question was asked on %s — subtract the session date from this to compute elapsed time.

Relevant memory context:
%s

Question (asked on %s): %s

Answer in one sentence using only the facts directly required by the question. Do not restate the question. Do not add context the user did not ask for. If the answer is a number, date, name, or short phrase, return only that value with minimal framing. If the answer cannot be determined from the provided context, respond with exactly: I don't know.`, questionDate, ctx, questionDate, question)
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
// Uses Haiku — classifying CORRECT/INCORRECT is a simple comparison task.
func Score(ctx context.Context, question, referenceAnswer, hypothesis string, retries int) (ScoreResult, error) {
	prompt := ScoringPrompt(question, referenceAnswer, hypothesis)
	out, err := GenerateHaiku(ctx, prompt, retries)
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
