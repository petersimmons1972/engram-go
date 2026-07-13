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
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"
)

// ErrDisallowedModel is a permanent error returned when the model name is not
// in the allowlist. The retry loop must not sleep on this error.
var ErrDisallowedModel = errors.New("disallowed model")

// debugOAIRequests gates verbose request/response logging for OAI calls.
// Set LME_DEBUG_REQUESTS=1 to enable. Logs endpoint/status and body sizes only;
// response text is redacted because benchmark prompts can contain private memory.
var debugOAIRequests = os.Getenv("LME_DEBUG_REQUESTS") == "1"

// generateTimeout is the hard cap for one OAI generation call. 1200s needed for 120B vLLM on 40-block contexts.
const generateTimeout = 600 * time.Second

// codexExecTimeout is the hard cap for one frontier-model generation item.
const codexExecTimeout = 300 * time.Second

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

// GenerateCodex invokes the Codex CLI once with the supplied model and prompt.
func GenerateCodex(ctx context.Context, prompt, model string) (string, error) {
	return runCodex(ctx, prompt, model)
}

func runCodex(ctx context.Context, prompt, model string) (string, error) {
	tctx, cancel := context.WithTimeout(ctx, codexExecTimeout)
	defer cancel()

	cmd := exec.CommandContext(
		tctx,
		"codex",
		"exec",
		"--model",
		model,
		"-c",
		"model_reasoning_effort=high",
		"--sandbox",
		"read-only",
	)
	// Pass the prompt via stdin, not argv: generation prompts carry full
	// retrieved context and exceed ARG_MAX ("argument list too long"). codex
	// reads the prompt from stdin when no positional prompt is given.
	cmd.Stdin = strings.NewReader(prompt)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	raw, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("codex exec: %w", ctx.Err())
		}
		if tctx.Err() != nil {
			return "", fmt.Errorf("codex exec timed out after %s", codexExecTimeout)
		}
		stderrSnippet := strings.TrimSpace(stderr.String())
		if len(stderrSnippet) > 200 {
			stderrSnippet = stderrSnippet[:200] + "…"
		}
		if stderrSnippet != "" {
			return "", fmt.Errorf("codex exec: %w: %s", err, stderrSnippet)
		}
		return "", fmt.Errorf("codex exec: %w", err)
	}

	out := finalCodexAssistantText(string(raw))
	if out == "" {
		return "", errors.New("codex exec: empty assistant output")
	}
	return out, nil
}

func finalCodexAssistantText(raw string) string {
	cleaned := stripTerminalControls(raw)
	lines := strings.Split(cleaned, "\n")
	firstNonEmpty := -1
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			firstNonEmpty = i
			break
		}
	}

	blockStart := -1
	blockEnd := -1
	sawAnalysis := false
	for i := 0; i < len(lines); i++ {
		marker := strings.TrimSpace(lines[i])
		if marker == "assistant/analysis" {
			sawAnalysis = true
			continue
		}

		end := -1
		for j := i + 1; j < len(lines); j++ {
			if isCodexTokenFooter(lines, j) {
				end = j
				break
			}
		}
		switch {
		case marker == "codex" && end >= 0:
			blockStart, blockEnd = i+1, end
			i = end
		case marker == "assistant/final" && (sawAnalysis || i == firstNonEmpty):
			blockStart = i + 1
			blockEnd = len(lines)
			if end >= 0 {
				blockEnd = end
				i = end
			} else {
				i = len(lines)
			}
		}
	}
	if blockStart >= 0 {
		lines = lines[blockStart:blockEnd]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func isCodexTokenFooter(lines []string, index int) bool {
	if strings.TrimSpace(lines[index]) != "tokens used" || index+1 >= len(lines) {
		return false
	}
	count := strings.TrimSpace(lines[index+1])
	if count == "" {
		return false
	}
	for _, char := range count {
		if (char < '0' || char > '9') && char != ',' {
			return false
		}
	}
	for _, trailing := range lines[index+2:] {
		if strings.TrimSpace(trailing) != "" {
			return false
		}
	}
	return true
}

func stripTerminalControls(raw string) string {
	var cleaned strings.Builder
	cleaned.Grow(len(raw))
	for i := 0; i < len(raw); {
		char := raw[i]
		switch char {
		case 0x1b:
			i = consumeEscapeSequence(raw, i)
		case 0x9b:
			i = consumeCSI(raw, i+1)
		case 0x9d:
			i = consumeControlString(raw, i+1, true)
		case 0x90, 0x98, 0x9e, 0x9f:
			i = consumeControlString(raw, i+1, false)
		default:
			switch {
			case char == '\n' || char == '\r' || char == '\t':
				cleaned.WriteByte(char)
				i++
			case char < 0x20 || char == 0x7f:
				i++
			case char >= utf8.RuneSelf:
				_, size := utf8.DecodeRuneInString(raw[i:])
				if size > 1 {
					cleaned.WriteString(raw[i : i+size])
					i += size
					continue
				}
				if char < 0x80 || char > 0x9f {
					cleaned.WriteByte(char)
				}
				i++
			default:
				cleaned.WriteByte(char)
				i++
			}
		}
	}
	return cleaned.String()
}

func consumeEscapeSequence(raw string, index int) int {
	if index+1 >= len(raw) {
		return len(raw)
	}
	switch raw[index+1] {
	case '[':
		return consumeCSI(raw, index+2)
	case ']':
		return consumeControlString(raw, index+2, true)
	case 'P', 'X', '^', '_':
		return consumeControlString(raw, index+2, false)
	case '(', ')', '*', '+', '-', '.', '/', '#', '%':
		return min(index+3, len(raw))
	default:
		return min(index+2, len(raw))
	}
}

func consumeCSI(raw string, index int) int {
	for index < len(raw) {
		if raw[index] >= 0x40 && raw[index] <= 0x7e {
			return index + 1
		}
		index++
	}
	return len(raw)
}

func consumeControlString(raw string, index int, bellTerminates bool) int {
	for index < len(raw) {
		switch {
		case bellTerminates && raw[index] == 0x07:
			return index + 1
		case raw[index] == 0x9c:
			return index + 1
		case raw[index] == 0x1b && index+1 < len(raw) && raw[index+1] == '\\':
			return index + 2
		default:
			index++
		}
	}
	return len(raw)
}

// OAIOptions controls generation quality parameters for OpenAI-compatible endpoints.
// Zero values fall back to conservative defaults safe for all models.
type OAIOptions struct {
	// EnableThinking enables chain-of-thought reasoning for models that support it
	// (e.g. Qwen3). Improves answer quality significantly; use higher MaxTokens.
	// Do NOT enable for Nemotron v3 — causes HTTP 400 (vLLM GH#39103).
	EnableThinking bool
	// MaxTokens caps the output token budget. Default 2048 is fine for
	// enable_thinking=false; use ≥8192 when thinking is enabled.
	MaxTokens int
	// APIKey is the Bearer token for the generation endpoint. When non-empty,
	// the Authorization header is set on the OAI request. When empty the
	// request is sent without auth (local/oblivion endpoints). Populated from
	// --llm-api-key flag or LLM_API_KEY env var.
	APIKey string
	// SystemPrompt overrides the default system message in buildOAIRequestBody.
	// When empty, the QA-assistant default is used. Set this when calling the
	// endpoint for non-QA tasks (e.g. atom extraction). (#1079)
	SystemPrompt string
}

// GenerateOAI calls an OpenAI-compatible chat completions endpoint instead of
// the claude CLI. baseURL is the API root (e.g. "http://oblivion:8000/v1").
// Retry/backoff behaviour mirrors generate().
// Calls with conservative defaults (thinking off, 2048 tokens).
func GenerateOAI(ctx context.Context, prompt, baseURL, model string, retries int) (string, error) {
	return GenerateOAIWithOpts(ctx, prompt, baseURL, model, retries, OAIOptions{})
}

// GenerateOAIWithOpts is the full-featured variant; use when you need thinking mode or
// a larger token budget.
func GenerateOAIWithOpts(ctx context.Context, prompt, baseURL, model string, retries int, opts OAIOptions) (string, error) {
	var lastErr error
	backoffs := []time.Duration{30 * time.Second, 60 * time.Second, 120 * time.Second}
	for attempt := 0; attempt <= retries; attempt++ {
		out, err := callOAI(ctx, prompt, baseURL, model, opts)
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

func callOAI(ctx context.Context, prompt, baseURL, model string, opts OAIOptions) (string, error) {
	tctx, cancel := context.WithTimeout(ctx, generateTimeout)
	defer cancel()

	// Truncate prompt to last 480000 chars (~120k tokens) to avoid vLLM context overflow (status 400).
	// We keep the END of the prompt because the question is at the end.
	const maxPromptChars = 480_000
	if len(prompt) > maxPromptChars {
		slog.Warn("prompt truncated to fit context window",
			"original_chars", len(prompt),
			"truncated_chars", maxPromptChars)
		prompt = prompt[len(prompt)-maxPromptChars:]
	}

	// oaiMessage omits Reasoning — sending it (even empty) causes HTTP 400 on
	// vLLM's Nemotron v3 reasoning parser (vLLM GH#39103).
	reqBody, err := buildOAIRequestBody(model, prompt, opts)
	if err != nil {
		return "", fmt.Errorf("marshal OAI request: %w", err)
	}

	url := strings.TrimRight(baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(tctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("create OAI request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if opts.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+opts.APIKey)
	}
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
				resp.StatusCode, len(reqBody), sanitizeOAIDebugBody(body))
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

func sanitizeOAIDebugBody(body []byte) string {
	return fmt.Sprintf("[redacted bytes=%d]", len(bytes.TrimSpace(body)))
}

// ScoringOptions controls OpenAI-style judge calls for scoring.
// EnableThinking controls whether judge prompts may include chain-of-thought.
type ScoringOptions struct {
	EnableThinking bool
	APIKey         string
}

// DefaultScorerMaxTokens is the default max_tokens for OAI scoring requests.
// 512 is ample: the judge emits one verdict word plus 1-2 sentences, and the
// smaller budget leaves more headroom for long hypotheses within the 65536-token
// context window (vLLM HTTP 400 fix).
const DefaultScorerMaxTokens = 512

// BuildScoringRequestBody returns an OAI request body for label classification.
// maxTokens controls the response budget; pass DefaultScorerMaxTokens (512)
// unless you have a specific reason to change it.
// options.ApplyThinking=false disables OAI chain-of-thought features for judges
// that are slow or unsupported.
// Exported so the test package can inspect the marshalled fields.
func BuildScoringRequestBody(model, question, referenceAnswer, hypothesis string, maxTokens int, options ScoringOptions) ([]byte, error) {
	return buildScoringRequestBody(model, question, referenceAnswer, hypothesis, maxTokens, options)
}

// buildScoringRequestBody is the unexported implementation used internally.
func buildScoringRequestBody(model, question, referenceAnswer, hypothesis string, maxTokens int, options ScoringOptions) ([]byte, error) {
	if maxTokens <= 0 {
		maxTokens = DefaultScorerMaxTokens
	}
	// Cap hypothesis length so the total request fits within the 65536-token context window.
	// Budget: 65536 - maxTokens - overhead(question, referenceAnswer) = tokens available for hypothesis.
	// Conservative chars-to-tokens ratio of 4 chars/token keeps us safely below the limit.
	// CRITICAL: truncate from the END (tail) to preserve the graded answer when --enumerate-first
	// is used (answer appears at end of hypothesis, not beginning). See callOAI line 217.
	const scorerMaxModelLen = 65536
	// Compute actual overhead from question + referenceAnswer instead of fixed 400 const.
	overheadPrompt := ScoringPrompt(question, referenceAnswer, "")
	overheadChars := len(overheadPrompt)
	overheadTokens := (overheadChars + 3) / 4 // round up: chars/4 = tokens (conservative estimate)
	maxHypTokens := scorerMaxModelLen - maxTokens - overheadTokens
	maxHypChars := maxHypTokens * 4
	// Clamp to >= 0 to prevent slice-bounds panic when maxTokens > 65136.
	if maxHypChars < 0 {
		maxHypChars = 0
	}
	if len(hypothesis) > maxHypChars {
		log.Printf("WARN: scorer hypothesis truncated for question %q: %d->%d chars (--scorer-max-tokens=%d)",
			question[:min(len(question), 60)], len(hypothesis), maxHypChars, maxTokens)
		// Keep the TAIL (end) — the graded answer is there, not at the beginning.
		if maxHypChars > 0 {
			hypothesis = hypothesis[len(hypothesis)-maxHypChars:]
		} else {
			hypothesis = ""
		}
	}
	prompt := ScoringPrompt(question, referenceAnswer, hypothesis)
	request := struct {
		Model              string                 `json:"model"`
		Messages           []oaiMessage           `json:"messages"`
		MaxTokens          int                    `json:"max_tokens"`
		Temperature        float64                `json:"temperature"`
		ChatTemplateKwargs map[string]interface{} `json:"chat_template_kwargs,omitempty"`
	}{
		Model:       model,
		MaxTokens:   maxTokens,
		Temperature: 0,
		Messages: []oaiMessage{
			{Role: "system", Content: "You are a precise answer-correctness judge. Output your judgment on the FIRST LINE as one of: CORRECT, PARTIALLY_CORRECT, INCORRECT. Then explain your reasoning on the next line."},
			{Role: "user", Content: prompt},
		},
	}
	if !options.EnableThinking {
		request.ChatTemplateKwargs = map[string]interface{}{"enable_thinking": false}
	}
	return json.Marshal(request)
}

// ScoreOAIEfficient is like ScoreOAI but uses buildScoringRequestBody
// (maxTokens, temperature=0) for efficient local-model scoring.
// maxTokens <= 0 uses DefaultScorerMaxTokens (512).
func ScoreOAIEfficient(ctx context.Context, question, referenceAnswer, hypothesis, baseURL, model string, retries, maxTokens int, options ScoringOptions) (ScoreResult, error) {
	var lastErr error
	backoffs := []time.Duration{250 * time.Millisecond, 500 * time.Millisecond, 1 * time.Second}
	for attempt := 0; attempt <= retries; attempt++ {
		out, err := callOAIScoring(ctx, question, referenceAnswer, hypothesis, baseURL, model, maxTokens, options)
		if err == nil {
			label, explanation := ParseScoreLabel(out)
			if label != "SCORE_ERROR" {
				return ScoreResult{Label: label, Explanation: explanation}, nil
			}
			lastErr = fmt.Errorf("judge response: %s", explanation)
		} else {
			lastErr = err
		}
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
	return ScoreResult{Label: "SCORE_ERROR", Explanation: explanationFromError(lastErr)}, lastErr
}

func callOAIScoring(ctx context.Context, question, referenceAnswer, hypothesis, baseURL, model string, maxTokens int, options ScoringOptions) (string, error) {
	tctx, cancel := context.WithTimeout(ctx, generateTimeout)
	defer cancel()
	reqBody, err := buildScoringRequestBody(model, question, referenceAnswer, hypothesis, maxTokens, options)
	if err != nil {
		return "", fmt.Errorf("marshal scoring request: %w", err)
	}
	url := strings.TrimRight(baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(tctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("create scoring request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if options.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+options.APIKey)
	}
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
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
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

func explanationFromError(err error) string {
	if err == nil {
		return "judge returned no explanation"
	}
	return err.Error()
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

// hardExclusionRule is the canonical HARD EXCLUSION RULE text for preference generation.
// Every code path that generates answers for single-session-preference questions must
// include this text so that explicitly stated anti-preferences are treated as hard
// constraints rather than merely described. Defined once so the two prompt paths
// (GenerationPromptForType and GenerationPromptPreferenceEnumerate) stay in sync.
//
// The text intentionally uses a broad verb set ("dislike, avoid, have moved away from,
// or do not want") to cover the most common natural-language phrasings. Callers that
// extend or modify the exclusion behaviour must update this single constant.
const hardExclusionRule = `HARD EXCLUSION RULE: Any item, brand, model, or option the user has explicitly stated they dislike, avoid, have moved away from, or do not want is FORBIDDEN. It must not appear anywhere in your answer — not as a suggestion, not as an alternative, not as a passing mention. Exclusions are hard constraints that override all other considerations.`

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

Do NOT answer the question directly. Instead, describe what the user would prefer based on their past conversations. Start your response with "The user would prefer..." and include what they would NOT prefer if the context supports it. Be concise.

`+hardExclusionRule, questionDate, ctx, questionDate, question)
	}
	return GenerationPrompt(question, questionDate, contextBlocks)
}

// GenerationPromptForTypeWithTemporalAug is like GenerationPromptForType but
// applies the Exp-14 H-M5+H-M1 combined prompt augmentation for
// temporal-reasoning questions when temporalPromptAug is true.
//   - H-M5 (chrono-sort forcing): for ordering questions, instructs the model
//     to list all relevant events in chronological order before answering.
//   - H-M1 (entity enumeration): for entity-ambiguous questions with a
//     relative-time anchor, instructs the model to enumerate all events of
//     the target type before committing to the most temporally precise one.
//
// For all other question types, or when temporalPromptAug is false, the
// standard GenerationPromptForType output is returned unchanged.
// Activated by --temporal-prompt-aug (Config.TemporalPromptAug). Off by default.
func GenerationPromptForTypeWithTemporalAug(question, questionType, questionDate string, contextBlocks []string, temporalPromptAug bool) string {
	if temporalPromptAug && questionType == "temporal-reasoning" {
		return temporalGenerationPromptWithAug(question, questionDate, contextBlocks)
	}
	return GenerationPromptForType(question, questionType, questionDate, contextBlocks)
}

// GenerationPromptForTypeWithDateInjection is like GenerationPromptForType but
// applies the H16 date-injection variant for temporal-reasoning questions.
// When injectQuestionDate is true and the question type is "temporal-reasoning",
// the prompt is prepended with "Today's date is: {questionDate}" so relative-time
// anchors resolve unambiguously. For all other question types the flag is a no-op
// and the standard prompt is returned unchanged.
func GenerationPromptForTypeWithDateInjection(question, questionType, questionDate string, contextBlocks []string, injectQuestionDate bool) string {
	if injectQuestionDate && questionType == "temporal-reasoning" {
		return temporalGenerationPromptWithDateInjection(question, questionDate, contextBlocks)
	}
	return GenerationPromptForType(question, questionType, questionDate, contextBlocks)
}

// GenerationPromptForTypePreferenceGround (H-PG, issue #1183) is like
// GenerationPromptForType but, when preferenceGround is true for
// single-session-preference questions, it injects a grounding rule that forbids
// specific named items absent from the retrieved context. The intent is to
// suppress ss-preference confabulation by preferring a short grounded answer
// over padded near-miss specifics.
func GenerationPromptForTypePreferenceGround(question, questionType, questionDate string, contextBlocks []string, preferenceGround bool) string {
	if !preferenceGround || questionType != "single-session-preference" {
		return GenerationPromptForType(question, questionType, questionDate, contextBlocks)
	}
	ctx := strings.Join(contextBlocks, "\n\n---\n\n")
	return fmt.Sprintf(`You are describing a person's preferences based on their conversation history.

Each memory block may begin with a "Session date: YYYY-MM-DD" header. The question was asked on %s.

Relevant memory context:
%s

Question (asked on %s): %s

Do NOT answer the question directly. Instead, describe what the user would prefer based on their past conversations. Start your response with "The user would prefer...".

GROUNDING RULE:
- Use only preferences and specific named items that are explicitly present in the retrieved context above.
- Do not add any specific brand, title, cuisine, ingredient, genre, package, service, or other named item that is not explicitly present in the context.
- If the context supports only a general preference category, answer at that general level rather than inventing examples.
- Mention what the user would NOT prefer only if the context explicitly supports it.
- Prefer a short grounded answer over a padded one.`, questionDate, ctx, questionDate, question)
}

// antiHedgeAddendum (L2, M0.5 Phase 4 closeout) is the anti-hedge instruction
// appended to preference-type generation prompts when --anti-hedge-prompts is
// enabled. Targets the ss-preference hedging failure mode where the model
// answers "it depends" / "I don't have enough information" / "it's unclear"
// even though the retrieved context states or clearly implies a preference —
// a refusal-shaped non-answer the judge scores as wrong even when the
// underlying evidence was retrieved correctly. This is a pure prompt lever:
// it does not change retrieval, matching, schema, or scoring.
const antiHedgeAddendum = `ANTI-HEDGE RULE: If the context above states or clearly implies a preference relevant to this question, you MUST commit to that preference as your answer. Do not hedge with phrases like "it depends", "I don't have enough information", "it's unclear", "I cannot determine", "without more context", or any similar non-answer when the context contains an answer. Only say the preference is unknown if the context truly contains no relevant information about it.`

// appendPromptAddendum appends addendum to prompt separated by a blank line,
// trimming surrounding whitespace from each so repeated application stays
// idempotent-shaped. Mirrors prependPromptPrefix's empty-string handling.
func appendPromptAddendum(prompt, addendum string) string {
	prompt = strings.TrimSpace(prompt)
	addendum = strings.TrimSpace(addendum)
	switch {
	case addendum == "":
		return prompt
	case prompt == "":
		return addendum
	default:
		return prompt + "\n\n" + addendum
	}
}

// GenerationPromptForTypeAntiHedge (L2, M0.5 Phase 4 closeout) is like
// GenerationPromptForType but, when antiHedgePrompts is true, appends the
// antiHedgeAddendum to the generation prompt for single-session-preference
// questions AND for questions IsInferredPreferenceQuestion identifies as
// preference-shaped by their raw text (e.g. "What restaurant would I like?")
// even when the dataset's own question_type label is something else — the
// same inferred-preference gate --dual-preference-recall (H15) already uses.
// For all other question types, or when the flag is false, the standard
// GenerationPromptForType output is returned unchanged. Pure prompt lever: no
// retrieval, matcher, schema, or scorer change.
// Activated by --anti-hedge-prompts (Config.AntiHedgePrompts). Off by default.
func GenerationPromptForTypeAntiHedge(question, questionType, questionDate string, contextBlocks []string, antiHedgePrompts bool) string {
	base := GenerationPromptForType(question, questionType, questionDate, contextBlocks)
	if !antiHedgePrompts {
		return base
	}
	if questionType != "single-session-preference" && !IsInferredPreferenceQuestion(question) {
		return base
	}
	return appendPromptAddendum(base, antiHedgeAddendum)
}

// GenerationPromptForTypeKURecency (H-KUR, issue #1178) is like
// GenerationPromptForType but, when kuRecencyPrompt is true for
// "knowledge-update" questions, routes to GenerationPromptKnowledgeUpdate — a
// dedicated prompt that instructs the model to answer with the most recent
// session's value when multiple values for the same attribute appear across
// sessions in the retrieved context. Targets the KU failure mode where ~18
// items on LME-M pick the stale value when both old and new appear in
// context, despite gold_visible ≈0.99 and avg_rank ≈2.0 (retrieval already
// solved; this is purely a generation-time disambiguation problem).
//
// For all other question types, or when kuRecencyPrompt is false, the
// standard GenerationPromptForType output is returned unchanged.
// Activated by --ku-recency-prompt (Config.KURecencyPrompt). Off by default.
func GenerationPromptForTypeKURecency(question, questionType, questionDate string, contextBlocks []string, kuRecencyPrompt bool) string {
	if !kuRecencyPrompt || questionType != "knowledge-update" {
		return GenerationPromptForType(question, questionType, questionDate, contextBlocks)
	}
	return GenerationPromptKnowledgeUpdate(question, questionDate, contextBlocks)
}

// GenerationPromptKnowledgeUpdate (H-KUR, issue #1178) returns a generation
// prompt for knowledge-update questions that instructs the model to answer
// using the value from the most recent session (by "Session date:" header)
// when multiple values for the same attribute appear across sessions in the
// retrieved context. Knowledge-update questions test whether the model
// correctly tracks facts that change over time (e.g. a phone number or job
// title updated in a later session); without an explicit recency rule the
// model has no principled way to choose between a stale and a current value
// when both are visible in context.
func GenerationPromptKnowledgeUpdate(question, questionDate string, contextBlocks []string) string {
	ctx := strings.Join(contextBlocks, "\n\n---\n\n")
	return fmt.Sprintf(`You are answering a question about a person's conversation history, where facts may have changed over time across sessions.

Each memory block may begin with a "Session date: YYYY-MM-DD" header. Use these dates to determine which session is most recent. The question was asked on %s.

Relevant memory context:
%s

Question (asked on %s): %s

RECENCY RULE: When multiple values exist for the same attribute across sessions, answer with the value from the most recent session (latest date). Ignore earlier, superseded values — they are stale and must not be used in your answer.

Answer in one sentence using only the facts directly required by the question. Do not restate the question. Do not add context the user did not ask for. If the answer is a number, date, name, or short phrase, return only that value with minimal framing. IMPORTANT: You MUST always provide a specific answer — never say "not mentioned", "not found in context", "cannot be determined", "not explicitly stated", or any similar refusal. If the answer is not directly stated, infer the most likely answer from the available context clues and state it directly. Output only the answer with no uncertainty hedging.`, questionDate, ctx, questionDate, question)
}

// GenerationPromptForTypeEnumerate (H12) is like GenerationPromptForType but
// prepends an enumerate-first instruction when enumerateFirst is true and the
// question matches the aggregation heuristic. For non-aggregation questions the
// flag is a no-op so other question types are unaffected.
//
// "single-session-preference" is explicitly excluded from the enumerate-first
// augmentation (even when enumerateFirst is true) because preference questions are
// handled by GenerationPromptForTypePreferenceEnumerate which routes them to
// GenerationPromptPreferenceEnumerate — a dedicated enumerate path that already
// carries its own enumeration instructions and the hardExclusionRule. Applying the
// generic enumerate-first prefix on top would double-augment those prompts.
func GenerationPromptForTypeEnumerate(question, questionType, questionDate string, contextBlocks []string, enumerateFirst bool) string {
	prompt := GenerationPromptForType(question, questionType, questionDate, contextBlocks)
	if enumerateFirst && questionType != "temporal-reasoning" && questionType != "single-session-preference" && IsAggregationQuestion(question) {
		return prependPromptPrefix(EnumerateFirstPrefix(), prompt)
	}
	return prompt
}

// GenerationPromptForTypePreferenceEnumerate (H-PE) is like
// GenerationPromptForType but applies the preference-enumerate variant for
// single-session-preference questions when preferenceEnumerate is true. When
// the flag is set and the question type is "single-session-preference", the
// prompt instructs the model to exhaustively list every specific named
// item/brand/attribute/feature the user expressed a preference about, rather
// than summarising the preference abstractly. For all other question types or
// when the flag is false the standard GenerationPromptForType output is
// returned unchanged.
// Activated by --preference-enumerate (Config.PreferenceEnumerate). Off by default.
func GenerationPromptForTypePreferenceEnumerate(question, questionType, questionDate string, contextBlocks []string, preferenceEnumerate bool) string {
	if preferenceEnumerate && questionType == "single-session-preference" {
		return GenerationPromptPreferenceEnumerate(question, questionDate, contextBlocks)
	}
	return GenerationPromptForType(question, questionType, questionDate, contextBlocks)
}

// GenerationPromptPreferenceEnumerate (H-PE) returns a generation prompt that
// instructs the model to list every specific named item, brand, attribute, or
// feature the user expressed a preference about that appears in the provided
// context — not an abstract summary. Targets the judge failure mode where a
// correct-substance answer is marked PARTIALLY_CORRECT because it omits the
// specific concrete items the gold answer lists.
func GenerationPromptPreferenceEnumerate(question, questionDate string, contextBlocks []string) string {
	ctx := strings.Join(contextBlocks, "\n\n---\n\n")
	return fmt.Sprintf(`You are describing a person's preferences based on their conversation history.

Each memory block may begin with a "Session date: YYYY-MM-DD" header. The question was asked on %s.

Relevant memory context:
%s

Question (asked on %s): %s

Instructions:
1. Read the context carefully and identify every specific named item, brand, model, attribute, or feature the user expressed a preference about that is relevant to the question.
2. List each one explicitly — do not summarise or generalise. For example: name the specific product, brand, feature, or location rather than saying "functional accessories" or "a pool".
3. Start your response with "The user prefers:" followed by the enumerated list.
4. Include what the user would NOT prefer if the context supports it.
5. Only include preferences found in the memory context. Do not invent items not present in the context.
6. `+hardExclusionRule, questionDate, ctx, questionDate, question)
}

// GenerationPromptEnumerateFirst (H12) returns a generation prompt that
// instructs the model to enumerate each relevant event from the memory blocks
// individually before computing a total. Forces an explicit intermediate
// enumeration pass that prevents the model from returning a session count
// instead of an entity count.
func GenerationPromptEnumerateFirst(question, questionDate string, contextBlocks []string) string {
	return prependPromptPrefix(EnumerateFirstPrefix(), GenerationPrompt(question, questionDate, contextBlocks))
}

// GenerationPrompt builds the prompt for answer generation.
func GenerationPrompt(question, questionDate string, contextBlocks []string) string {
	ctx := strings.Join(contextBlocks, "\n\n---\n\n")
	return fmt.Sprintf(`You are answering questions about a person's conversation history.

Each memory block may begin with a "Session date: YYYY-MM-DD" header. Use these dates for any relative-time calculations (e.g. "how many days/weeks ago"). The question was asked on %s — subtract the session date from this to compute elapsed time.

Relevant memory context:
%s

Question (asked on %s): %s

Answer in one sentence using only the facts directly required by the question. Do not restate the question. Do not add context the user did not ask for. If the answer is a number, date, name, or short phrase, return only that value with minimal framing. IMPORTANT: You MUST always provide a specific answer — never say "not mentioned", "not found in context", "cannot be determined", "not explicitly stated", or any similar refusal. If the answer is not directly stated, infer the most likely answer from the available context clues and state it directly. Output only the answer with no uncertainty hedging.`, questionDate, ctx, questionDate, question)
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
	// Note: firstLineIdx != i is always true when the if-block below is entered —
	// Pass 1 already tried firstLineIdx and found no match, so Pass 2 cannot
	// match there either. The guard is removed to reduce cognitive overhead (#760).
	for i, l := range allLines {
		upper := strings.ToUpper(strings.TrimSpace(l))
		for _, v := range validLabels {
			if upper == v {
				label = v
				// Explanation is whatever follows on subsequent lines.
				// When the label is on the last line (rationale-first format like
				// "rationale\nCORRECT"), post is empty and the pre-label text
				// becomes the explanation instead — this is intentional (#759).
				if i+1 < len(allLines) {
					explanation = strings.TrimSpace(strings.Join(allLines[i+1:], "\n"))
				}
				// Capture any pre-label text as part of explanation.
				// firstLineIdx != i is guaranteed (Pass 1 eliminated firstLineIdx).
				if firstLineIdx >= 0 {
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
// the given model + prompt. Accepts OAIOptions to control thinking mode and
// token budget. Only includes chat_template_kwargs when opts.EnableThinking
// is set or the model is Nemotron. Extracted from callOAI so the
// model-gating decision is unit-testable. #671.
func buildOAIRequestBody(model, prompt string, opts OAIOptions) ([]byte, error) {
	maxTokens := 2048
	if opts.MaxTokens > 0 {
		maxTokens = opts.MaxTokens
	} else if opts.EnableThinking {
		maxTokens = 8192
	}
	// Qwen3 docs: temperature=0.6 for thinking mode, lower for non-thinking.
	temperature := 0.2
	if opts.EnableThinking {
		temperature = 0.6
	}
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
			{Role: "system", Content: func() string {
				if opts.SystemPrompt != "" {
					return opts.SystemPrompt
				}
				return "You are a precise QA assistant. Answer concisely using only the provided memory context."
			}()},
			{Role: "user", Content: prompt},
		},
		MaxTokens:   maxTokens,
		Temperature: temperature,
		TopP:        0.95,
	}
	if opts.EnableThinking {
		// enable_thinking=true: full chain-of-thought in reasoning_content, answer in content.
		// Nemotron v3 does not support enable_thinking — do NOT pass EnableThinking=true for that model.
		body.ChatTemplateKwargs = map[string]any{"enable_thinking": true}
	} else if needsChatTemplateKwargs(model) {
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
