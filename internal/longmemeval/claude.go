package longmemeval

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ErrDisallowedModel is a permanent error returned when the model name is not
// in the allowlist. The retry loop must not sleep on this error.
var ErrDisallowedModel = errors.New("disallowed model")

// debugOAIRequests gates verbose request/response logging for OAI calls.
// Set LME_DEBUG_REQUESTS=1 to enable. Logs request body size and full response
// body on non-200, so 400 errors include the vLLM error detail.
var debugOAIRequests = os.Getenv("LME_DEBUG_REQUESTS") == "1"


// claudePrintTimeout is the hard cap for one claude --print call.

// generateTimeout is the hard cap for one OAI generation call. 1200s needed for 120B vLLM on 40-block contexts.
const generateTimeout = 600 * time.Second

// GenerateForType generates an answer using Sonnet for all question types.
func GenerateForType(ctx context.Context, prompt, questionType string, retries int) (string, error) {
	return generate(ctx, prompt, "sonnet", retries)
}

// Generate calls `claude --print prompt` using Sonnet and returns trimmed stdout.
// retries is the number of additional attempts on failure (0 = try once).
// On failure a backoff sleep (30s, 60s, 120s) is inserted between attempts
// so transient API rate limits have a chance to clear before retrying.
// 2026-05-18: batch re-run of 102 errored items — Sonnet (rate-limit-safe).
func Generate(ctx context.Context, prompt string, retries int) (string, error) {
	return generate(ctx, prompt, "sonnet", retries)
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

// GenerateOpus is like Generate but uses Opus — highest-capability model,
// suitable for complex multi-session and temporal-reasoning questions.
func GenerateOpus(ctx context.Context, prompt string, retries int) (string, error) {
	return generate(ctx, prompt, "opus", retries)
}

// GenerateForModel calls generate with the given model alias. model must be
// one of the values in validClaudeModels ("opus", "sonnet", "haiku"); an
// unknown value causes generate → runClaude to return an error immediately.
func GenerateForModel(ctx context.Context, prompt, model string, retries int) (string, error) {
	return generate(ctx, prompt, model, retries)
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
		// Permanent validation errors must not be retried.
		if errors.Is(err, ErrDisallowedModel) {
			break
		}
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

// GenerateForModel calls generate with the given model alias. model must be
// one of the values in validClaudeModels ("opus", "sonnet", "haiku"); an
// unknown value causes generate → runClaude to return ErrDisallowedModel
// immediately without retrying.
func GenerateForModel(ctx context.Context, prompt, model string, retries int) (string, error) {
	return generate(ctx, prompt, model, retries)
}

// validClaudeModels is the allowlist for the --model flag passed to
// `claude --print`. Restricting to known values prevents argv injection
// (#678) — an LLM-hallucinated or env-controlled value containing "--" or
// shell metacharacters cannot reach the claude binary.
var validClaudeModels = map[string]bool{
	"opus":   true,
	"sonnet": true,
	"haiku":  true,
}

// isValidClaudeModel reports whether the supplied model name is in the
// strict allowlist (case-sensitive). #678.
func isValidClaudeModel(model string) bool {
	return validClaudeModels[model]
}

func runClaude(ctx context.Context, prompt, model string) (string, error) {
	if !isValidClaudeModel(model) {
		return "", fmt.Errorf("%w: %q (allowed: opus, sonnet, haiku) (#678)", ErrDisallowedModel, model)
	}
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
	reqBody, err := buildOAIRequestBody(model, prompt)
	if err != nil {
		return "", fmt.Errorf("marshal OAI request: %w", err)
	}

	url := strings.TrimRight(baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(tctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("create OAI request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if debugOAIRequests {
		log.Printf("DEBUG callOAI: url=%s request_body_bytes=%d", url, len(reqBody))
	}

	resp, err := oaiHTTPClient.Do(req)
	if err != nil {
		if tctx.Err() != nil {
			return "", fmt.Errorf("OAI request timed out after %s", generateTimeout)
		}
		return "", fmt.Errorf("OAI request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		if debugOAIRequests {
			body, _ := io.ReadAll(resp.Body)
			log.Printf("DEBUG callOAI: status=%d request_body_bytes=%d response_body=%s",
				resp.StatusCode, len(reqBody), strings.TrimSpace(string(body)))
		}
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

// DefaultScorerMaxTokens is the default max_tokens for OAI scoring requests.
// 2048 gives the model room to produce its label and a full explanation even
// after reasoning tokens, preventing truncation from stripping the label.
const DefaultScorerMaxTokens = 2048

// BuildScoringRequestBody returns an OAI request body for label classification.
// maxTokens controls the response budget; pass DefaultScorerMaxTokens (2048)
// unless you have a specific reason to reduce it.
// Exported so the test package can inspect the marshalled fields.
func BuildScoringRequestBody(model, question, referenceAnswer, hypothesis string, maxTokens int) ([]byte, error) {
	return buildScoringRequestBody(model, question, referenceAnswer, hypothesis, maxTokens)
}

// buildScoringRequestBody is the unexported implementation used internally.
func buildScoringRequestBody(model, question, referenceAnswer, hypothesis string, maxTokens int) ([]byte, error) {
	if maxTokens <= 0 {
		maxTokens = DefaultScorerMaxTokens
	}
	prompt := ScoringPrompt(question, referenceAnswer, hypothesis)
	body := struct {
		Model       string       `json:"model"`
		Messages    []oaiMessage `json:"messages"`
		MaxTokens   int          `json:"max_tokens"`
		Temperature float64      `json:"temperature"`
	}{
		Model: model,
		Messages: []oaiMessage{
			{Role: "system", Content: "You are a precise answer-correctness judge. Output your judgment on the FIRST LINE as one of: CORRECT, PARTIALLY_CORRECT, INCORRECT. Then explain your reasoning on the next line."},
			{Role: "user", Content: prompt},
		},
		MaxTokens:   maxTokens,
		Temperature: 0,
	}
	return json.Marshal(body)
}

// ScoreOAIEfficient is like ScoreOAI but uses buildScoringRequestBody
// (maxTokens, temperature=0) for efficient local-model scoring.
// maxTokens <= 0 uses DefaultScorerMaxTokens (2048).
func ScoreOAIEfficient(ctx context.Context, question, referenceAnswer, hypothesis, baseURL, model string, retries, maxTokens int) (ScoreResult, error) {
	var lastErr error
	backoffs := []time.Duration{30 * time.Second, 60 * time.Second, 120 * time.Second}
	for attempt := 0; attempt <= retries; attempt++ {
		out, err := callOAIScoring(ctx, question, referenceAnswer, hypothesis, baseURL, model, maxTokens)
		if err == nil {
			label, explanation := ParseScoreLabel(out)
			return ScoreResult{Label: label, Explanation: explanation}, nil
		}
		lastErr = err
		if attempt >= retries {
			break
		}
		wait := backoffs[attempt%len(backoffs)]
		select {
		case <-ctx.Done():
			return ScoreResult{Label: "SCORE_ERROR"}, ctx.Err()
		case <-time.After(wait):
		}
	}
	return ScoreResult{Label: "SCORE_ERROR"}, lastErr
}

func callOAIScoring(ctx context.Context, question, referenceAnswer, hypothesis, baseURL, model string, maxTokens int) (string, error) {
	tctx, cancel := context.WithTimeout(ctx, generateTimeout)
	defer cancel()
	reqBody, err := buildScoringRequestBody(model, question, referenceAnswer, hypothesis, maxTokens)
	if err != nil {
		return "", fmt.Errorf("marshal scoring request: %w", err)
	}
	url := strings.TrimRight(baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(tctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("create scoring request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := oaiHTTPClient.Do(req)
	if err != nil {
		if tctx.Err() != nil {
			return "", fmt.Errorf("scoring request timed out")
		}
		return "", fmt.Errorf("scoring request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("scoring request: HTTP %d", resp.StatusCode)
	}
	var oaiResp struct {
		Choices []struct {
			Message struct{ Content string `json:"content"` } `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&oaiResp); err != nil {
		return "", fmt.Errorf("decode scoring response: %w", err)
	}
	if len(oaiResp.Choices) == 0 {
		return "", fmt.Errorf("scoring response: no choices")
	}
	content := strings.TrimSpace(oaiResp.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("scoring response: empty content")
	}
	return content, nil
}

// ScoreOAI is like Score but uses the OpenAI-compatible endpoint.
func ScoreOAI(ctx context.Context, question, referenceAnswer, hypothesis, baseURL, model string, retries int) (ScoreResult, error) {
	prompt := ScoringPrompt(question, referenceAnswer, hypothesis)
	out, err := GenerateOAI(ctx, prompt, baseURL, model, retries)
	if err != nil {
		return ScoreResult{Label: "SCORE_ERROR"}, err
	}
	label, explanation := ParseScoreLabel(out)
	return ScoreResult{Label: label, Explanation: explanation}, nil
}


// GenerationPromptForType builds a generation prompt tailored to the question type.
// For single-session-preference questions the model is instructed to describe the
// user's preferences rather than answer the question directly — answering directly
// was the root cause of 0/30 on that category in v9 (engram-go#741 follow-up).
func GenerationPromptForType(question, questionType, questionDate string, contextBlocks []string) string {
	if questionType == "temporal-reasoning" {
		return temporalGenerationPrompt(question, questionDate, contextBlocks)
	}
	if questionType == "single-session-preference" {
		ctx := strings.Join(contextBlocks, "\n\n---\n\n")
		return fmt.Sprintf(`You are describing a person's preferences based on their conversation history.

Each memory block may begin with a "Session date: YYYY-MM-DD" header. The question was asked on %s.

Relevant memory context:
%s

Question (asked on %s): %s

Do NOT answer the question directly. Instead, describe what the user would prefer based on their past conversations. Start your response with "The user would prefer..." and include what they would NOT prefer if the context supports it. Be concise.`, questionDate, ctx, questionDate, question)
	}
	return GenerationPrompt(question, questionDate, contextBlocks)
}

// GenerationPrompt builds the prompt for answer generation.
func GenerationPrompt(question, questionDate string, contextBlocks []string) string {
	ctx := strings.Join(contextBlocks, "\n\n---\n\n")
	return fmt.Sprintf(`You are answering questions about a person's conversation history.

Each memory block may begin with a "Session date: YYYY-MM-DD" header. Use these dates for any relative-time calculations (e.g. "how many days/weeks ago"). The question was asked on %s — subtract the session date from this to compute elapsed time.

Relevant memory context:
%s

Question (asked on %s): %s

When the question asks "how many", "how often", "how much total", or similar quantitative aggregation, enumerate each relevant event from the context before stating the total. If no relevant events appear in the context, answer 0. For all other questions: answer in one sentence using only the facts directly required by the question. Do not restate the question. Do not add context the user did not ask for. If the answer is a number, date, name, or short phrase, return only that value with minimal framing.`, questionDate, ctx, questionDate, question)
}

// ScoringPrompt builds the judge prompt for answer scoring.
// Label is requested on the FIRST LINE so that truncation cannot strip it.
func ScoringPrompt(question, referenceAnswer, hypothesis string) string {
	return fmt.Sprintf(`You are grading a hypothesis against a gold answer. Output your judgment on the FIRST LINE as one of: CORRECT, PARTIALLY_CORRECT, INCORRECT. Then on the NEXT LINE explain your reasoning in 1-3 sentences.

Definitions:
- CORRECT: hypothesis contains all key facts from the gold answer with no contradictions. Extra correct context is fine.
- PARTIALLY_CORRECT: some key facts present, others missing or hedged; partial overlap with gold.
- INCORRECT: key facts wrong, contradicted, or completely absent (even if topically related).

Question: %s

Gold answer: %s

Hypothesis: %s

Judgment (one word on first line):`, question, referenceAnswer, hypothesis)
}

// ScoreResult holds the parsed output of the judge prompt.
type ScoreResult struct {
	Label       string
	Explanation string
}

// Score calls claude --print with the judge prompt and parses the result.
// Uses Sonnet for scoring (Haiku too strict on long-context QA — LME v9 2026-05-18).
func Score(ctx context.Context, question, referenceAnswer, hypothesis string, retries int) (ScoreResult, error) {
	prompt := ScoringPrompt(question, referenceAnswer, hypothesis)
	out, err := GenerateSonnet(ctx, prompt, retries)
	if err != nil {
		return ScoreResult{Label: "SCORE_ERROR"}, err
	}
	label, explanation := ParseScoreLabel(out)
	return ScoreResult{Label: label, Explanation: explanation}, nil
}

// validLabels is the ordered set of recognised score labels.
var validLabels = []string{"CORRECT", "PARTIALLY_CORRECT", "INCORRECT"}

// ParseScoreLabel extracts the label and explanation from raw judge output.
//
// Strategy:
//  1. Read the first non-empty line; match case-insensitively against the
//     three valid labels.
//  2. If no match on the first line, scan every line for the first occurrence
//     of any valid label (handles preamble / COT output).
//  3. If still no match, return SCORE_ERROR — never default to PARTIALLY_CORRECT,
//     which was masking truncation failures.
func ParseScoreLabel(raw string) (label, explanation string) {
	allLines := strings.Split(strings.TrimSpace(raw), "\n")

	// Pass 1: first non-empty line.
	firstLineIdx := -1
	for i, l := range allLines {
		if strings.TrimSpace(l) != "" {
			firstLineIdx = i
			first := strings.ToUpper(strings.TrimSpace(l))
			for _, v := range validLabels {
				if first == v {
					label = v
					if i+1 < len(allLines) {
						explanation = strings.TrimSpace(strings.Join(allLines[i+1:], "\n"))
					}
					return label, explanation
				}
			}
			break
		}
	}

	// Pass 2: scan all lines for first label occurrence.
	for i, l := range allLines {
		upper := strings.ToUpper(strings.TrimSpace(l))
		for _, v := range validLabels {
			if upper == v {
				label = v
				// Explanation is whatever follows on subsequent lines.
				if i+1 < len(allLines) {
					explanation = strings.TrimSpace(strings.Join(allLines[i+1:], "\n"))
				}
				// Also capture any pre-label text as part of explanation if first line had content.
				if firstLineIdx >= 0 && firstLineIdx != i {
					pre := strings.TrimSpace(strings.Join(allLines[firstLineIdx:i], "\n"))
					if pre != "" && explanation != "" {
						explanation = pre + "\n" + explanation
					} else if pre != "" {
						explanation = pre
					}
				}
				return label, explanation
			}
		}
	}

	// Pass 3: no label found — explicit error, not a silent PARTIALLY_CORRECT.
	return "SCORE_ERROR", strings.TrimSpace(raw)
}


// oaiMessage is one entry in an OpenAI-compatible chat completion request.
type oaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// needsChatTemplateKwargs reports whether the given model accepts the
// `chat_template_kwargs.enable_thinking` extension. Currently only vLLM
// Nemotron models do. Other OAI endpoints (gpt-*, llama-*, qwen-*, etc.)
// may return HTTP 400 if an unknown field is sent (#671).
func needsChatTemplateKwargs(model string) bool {
	return strings.Contains(strings.ToLower(model), "nemotron")
}

// buildOAIRequestBody marshals the OpenAI chat completion request body for
// the given model + prompt. Only includes chat_template_kwargs for models
// that understand it (currently vLLM Nemotron). Extracted from callOAI so
// the model-gating decision is unit-testable. #671.
func buildOAIRequestBody(model, prompt string) ([]byte, error) {
	body := struct {
		Model              string         `json:"model"`
		Messages           []oaiMessage   `json:"messages"`
		MaxTokens          int            `json:"max_tokens"`
		Temperature        float64        `json:"temperature"`
		TopP               float64        `json:"top_p"`
		ChatTemplateKwargs map[string]any `json:"chat_template_kwargs,omitempty"`
	}{
		Model: model,
		Messages: []oaiMessage{
			{Role: "system", Content: "You are a precise QA assistant. Answer concisely using only the provided memory context."},
			{Role: "user", Content: prompt},
		},
		MaxTokens:   2048,
		Temperature: 0.2,
		TopP:        0.95,
	}
	if needsChatTemplateKwargs(model) {
		body.ChatTemplateKwargs = map[string]any{"enable_thinking": false}
	}
	return json.Marshal(body)
}

// oaiHTTPClient is a private *http.Client for OAI-compatible LLM endpoints.
// Per-call context deadlines guard hangs; the explicit Timeout below is a
// second layer of defense and ensures we never share http.DefaultClient
// (whose Transport can be mutated by any imported package's init() — #687).
var oaiHTTPClient = &http.Client{
	Timeout: 15 * time.Minute, // generous: LLM generation can be slow; context deadline tightens further per call
}

// anthropicBatchClient is a dedicated *http.Client for the Anthropic Message
// Batches API. Kept separate from oaiHTTPClient so the generous 20-minute
// timeout does not interfere with OAI scoring calls, and to make test
// injection straightforward. The poll loop for batch completion can take up
// to ~15 minutes on a 500-item batch.
var anthropicBatchClient = &http.Client{
	Timeout: 20 * time.Minute,
}

// anthropicBaseURL is the Anthropic API root. Overridable in tests via
// SetAnthropicBaseURL.
var anthropicBaseURL = "https://api.anthropic.com"

// SetAnthropicBaseURL overrides the Anthropic API base URL. Intended for use
// in tests that stand up an httptest.Server. Call with the real URL to restore
// production behaviour after each test.
func SetAnthropicBaseURL(url string) {
	anthropicBaseURL = url
}

// BatchScoringItem is one item to score in a batch request.
type BatchScoringItem struct {
	QuestionID      string
	Question        string
	ReferenceAnswer string
	Hypothesis      string
}

// ScoreBatch scores all items in a single Anthropic Message Batches API call.
// apiKey must be non-empty. model is the Anthropic model ID (e.g.
// "claude-haiku-4-5"). On batch creation failure the error is returned and
// the caller may fall back to per-item scoring. Items with errored results
// receive SCORE_ERROR — never PARTIALLY_CORRECT, which would mask infrastructure failures.
//
// Poll loop: starts at 2 s backoff, doubles each iteration, caps at 30 s.
// Terminal state is "ended" only — "canceling" is NOT terminal.
//
// NDJSON result streaming uses bufio.Scanner (line-buffered); the full body
// is never held in memory.
func ScoreBatch(ctx context.Context, items []BatchScoringItem, apiKey, model string) (map[string]ScoreResult, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("ScoreBatch: apiKey is required")
	}
	if len(items) == 0 {
		return map[string]ScoreResult{}, nil
	}

	// --- Step 1: build batch create request ---
	type anthropicContent struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type anthropicMessage struct {
		Role    string             `json:"role"`
		Content []anthropicContent `json:"content"`
	}
	type anthropicParams struct {
		Model     string             `json:"model"`
		MaxTokens int                `json:"max_tokens"`
		System    string             `json:"system"`
		Messages  []anthropicMessage `json:"messages"`
	}
	type batchRequest struct {
		CustomID string          `json:"custom_id"`
		Params   anthropicParams `json:"params"`
	}

	batchReqs := make([]batchRequest, len(items))
	for i, item := range items {
		prompt := ScoringPrompt(item.Question, item.ReferenceAnswer, item.Hypothesis)
		batchReqs[i] = batchRequest{
			CustomID: item.QuestionID,
			Params: anthropicParams{
				Model:     model,
				MaxTokens: DefaultScorerMaxTokens,
				System:    "You are a precise answer-correctness judge. Output your judgment on the FIRST LINE as one of: CORRECT, PARTIALLY_CORRECT, INCORRECT. Then explain your reasoning on the next line.",
				Messages: []anthropicMessage{
					{Role: "user", Content: []anthropicContent{{Type: "text", Text: prompt}}},
				},
			},
		}
	}

	createBody := struct {
		Requests []batchRequest `json:"requests"`
	}{Requests: batchReqs}

	createJSON, err := json.Marshal(createBody)
	if err != nil {
		return nil, fmt.Errorf("ScoreBatch: marshal create request: %w", err)
	}

	createURL := anthropicBaseURL + "/v1/messages/batches"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, createURL, bytes.NewReader(createJSON))
	if err != nil {
		return nil, fmt.Errorf("ScoreBatch: create request: %w", err)
	}
	setAnthropicHeaders(req, apiKey)

	resp, err := anthropicBatchClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ScoreBatch: POST batches: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("ScoreBatch: POST batches: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var createResp struct {
		ID               string `json:"id"`
		ProcessingStatus string `json:"processing_status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		return nil, fmt.Errorf("ScoreBatch: decode create response: %w", err)
	}
	batchID := createResp.ID

	// --- Step 2: poll until processing_status == "ended" ---
	pollURL := anthropicBaseURL + "/v1/messages/batches/" + batchID
	backoff := 2 * time.Second
	const maxBackoff = 30 * time.Second
	for {
		// Check context cancellation before sleeping.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}

		pollReq, err := http.NewRequestWithContext(ctx, http.MethodGet, pollURL, nil)
		if err != nil {
			return nil, fmt.Errorf("ScoreBatch: poll request: %w", err)
		}
		setAnthropicHeaders(pollReq, apiKey)

		pollResp, err := anthropicBatchClient.Do(pollReq)
		if err != nil {
			return nil, fmt.Errorf("ScoreBatch: poll GET: %w", err)
		}

		var status struct {
			ProcessingStatus string `json:"processing_status"`
		}
		decErr := json.NewDecoder(pollResp.Body).Decode(&status)
		_ = pollResp.Body.Close()
		if decErr != nil {
			return nil, fmt.Errorf("ScoreBatch: decode poll response: %w", decErr)
		}

		if status.ProcessingStatus == "ended" {
			break
		}
		// "in_progress" and "canceling" are non-terminal; keep polling.
		// Double the backoff, cap at maxBackoff.
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	// --- Step 3: stream NDJSON results ---
	resultsURL := anthropicBaseURL + "/v1/messages/batches/" + batchID + "/results"
	resultsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, resultsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("ScoreBatch: results request: %w", err)
	}
	setAnthropicHeaders(resultsReq, apiKey)

	resultsResp, err := anthropicBatchClient.Do(resultsReq)
	if err != nil {
		return nil, fmt.Errorf("ScoreBatch: GET results: %w", err)
	}
	defer func() { _ = resultsResp.Body.Close() }()

	if resultsResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resultsResp.Body, 1024))
		return nil, fmt.Errorf("ScoreBatch: GET results: HTTP %d: %s", resultsResp.StatusCode, strings.TrimSpace(string(body)))
	}

	// Parse NDJSON line-by-line using bufio.Scanner.
	type resultContent struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type resultMessage struct {
		Content []resultContent `json:"content"`
	}
	type resultSuccess struct {
		Type    string        `json:"type"`
		Message resultMessage `json:"message"`
	}
	type resultErrorDetail struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	type resultLine struct {
		CustomID string `json:"custom_id"`
		Result   struct {
			Type    string            `json:"type"`
			Message resultMessage     `json:"message"`
			Error   resultErrorDetail `json:"error"`
		} `json:"result"`
	}

	out := make(map[string]ScoreResult, len(items))
	scanner := bufio.NewScanner(resultsResp.Body)
	// Each line is one NDJSON object; size up the buffer for long explanations.
	scanner.Buffer(make([]byte, 256*1024), 256*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rl resultLine
		if err := json.Unmarshal(line, &rl); err != nil {
			// Skip malformed lines — log but don't abort.
			continue
		}
		if rl.Result.Type != "succeeded" {
			// errored or expired items are explicitly flagged — never silently
			// default to PARTIALLY_CORRECT, which would mask infrastructure failures.
			out[rl.CustomID] = ScoreResult{Label: "SCORE_ERROR", Explanation: rl.Result.Error.Message}
			continue
		}
		if len(rl.Result.Message.Content) == 0 {
			out[rl.CustomID] = ScoreResult{Label: "SCORE_ERROR", Explanation: "empty content"}
			continue
		}
		text := rl.Result.Message.Content[0].Text
		label, explanation := ParseScoreLabel(text)
		out[rl.CustomID] = ScoreResult{Label: label, Explanation: explanation}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("ScoreBatch: scan NDJSON results: %w", err)
	}

	return out, nil
}

// setAnthropicHeaders attaches the required headers for all Anthropic API calls,
// including the message-batches beta header.
func setAnthropicHeaders(req *http.Request, apiKey string) {
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "message-batches-2024-09-24")
	req.Header.Set("Content-Type", "application/json")
}
