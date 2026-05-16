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

// generateTimeout is the hard cap for one OAI generation call. 1200s needed for 120B vLLM on 40-block contexts.
const generateTimeout = 600 * time.Second

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

	// oaiMessage omits Reasoning — sending it (even empty) causes HTTP 400 on
	// vLLM's Nemotron v3 reasoning parser (vLLM GH#39103).
	type oaiMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	reqBody, err := json.Marshal(struct {
		Model              string         `json:"model"`
		Messages           []oaiMessage   `json:"messages"`
		MaxTokens          int            `json:"max_tokens"`
		Temperature        float64        `json:"temperature"`
		TopP               float64        `json:"top_p"`
		ChatTemplateKwargs map[string]any `json:"chat_template_kwargs,omitempty"`
	}{
		Model: model,
		Messages: []oaiMessage{
			{
				Role:    "system",
				Content: "You are a precise QA assistant. Answer concisely using only the provided memory context.",
			},
			{Role: "user", Content: prompt},
		},
		MaxTokens:   2048, // with enable_thinking=false, answers are short QA; 20480 overflowed 131k context limit
		Temperature: 0.2,   // instruct/deterministic mode per NVIDIA model card
		TopP:        0.95,
		ChatTemplateKwargs: map[string]any{
			"enable_thinking": false, // disable reasoning chain; answer goes to content not reasoning_content
		},
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

	// Response struct reads reasoning_content (vLLM GH#39103: Nemotron v3 puts answer
	// there when reasoning parser is active), falling back to content then reasoning.
	var oaiResp struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				Reasoning        string `json:"reasoning"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&oaiResp); err != nil {
		return "", fmt.Errorf("decode OAI response: %w", err)
	}
	if len(oaiResp.Choices) == 0 {
		return "", fmt.Errorf("OAI response: no choices returned")
	}
	msg := oaiResp.Choices[0].Message
	content := strings.TrimSpace(msg.ReasoningContent)
	if content == "" {
		content = strings.TrimSpace(msg.Content)
	}
	if content == "" {
		content = strings.TrimSpace(msg.Reasoning)
	}
	if content == "" {
		return "", fmt.Errorf("OAI response: content, reasoning_content, and reasoning are all empty")
	}
	// Strip <think>...</think> reasoning block if present; keep only the final answer.
	if idx := strings.LastIndex(content, "</think>"); idx != -1 {
		content = strings.TrimSpace(content[idx+len("</think>"):])
	}
	return content, nil
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

Answer in one sentence using only the facts directly required by the question. Do not restate the question. Do not add context the user did not ask for. If the answer is a number, date, name, or short phrase, return only that value with minimal framing. If the answer is not explicitly present, provide your best inference from the strongest matching context.`, questionDate, ctx, questionDate, question)
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
