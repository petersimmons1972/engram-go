package consolidate

// LLM-powered contradiction detection — second pass for classifyContradictionLLM.
// This file contains everything specific to the Ollama second-pass:
//   - classifyContradictionLLM: calls /api/generate and parses YES/NO
//   - the HTTP client used for those calls
//
// Design decisions:
//   - Exported as ClassifyContradictionLLM so tests in the _test package can
//     call it directly without a detour through the Runner.
//   - A dedicated HTTP client (llmHTTPClient) keeps the contradiction-detection
//     connection pool separate from the summarize worker's pool, so a slow
//     Ollama response during sleep consolidation does not starve summarization.
//   - The 30-second timeout lives on the per-call context, not the HTTP client
//     Transport, so callers can further tighten the deadline by passing a
//     shorter context.
//   - Temperature 0.1 and num_predict 8 minimise hallucinated filler around
//     the YES/NO token; we only need the first word.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// llmHTTPClient is shared across classifyContradictionLLM calls so that the
// underlying connection pool is reused rather than rebuilt on every request.
var llmHTTPClient = &http.Client{
	Timeout: 35 * time.Second, // outer guard; callers also pass a context timeout
	Transport: &http.Transport{
		IdleConnTimeout:     30 * time.Second,
		MaxIdleConnsPerHost: 2,
	},
}

const (
	// llmCallTimeout is the default per-call deadline when the caller's context
	// does not already have a shorter deadline.
	llmCallTimeout = 30 * time.Second

	// llmContradictionPromptTemplate is the prompt sent to Ollama.
	// Kept short so it fits within num_predict=8 budgets on tiny models.
	llmContradictionPromptTemplate = "Do these two statements contradict each other? Answer only YES or NO.\n\nStatement A: %s\n\nStatement B: %s"
)

// ClassifyContradictionLLM asks an Ollama model whether contentA and contentB
// contradict each other. Returns true when the model responds with a string
// that starts with "yes" (case-insensitive). The function is best-effort:
// network errors, non-200 responses, and timeouts all return (false, err).
//
// The function name is exported so package-level tests can call it directly.
func ClassifyContradictionLLM(ctx context.Context, contentA, contentB, ollamaURL, model string) (bool, error) {
	// Enforce a 30-second per-call deadline if the caller's context is longer.
	callCtx, cancel := context.WithTimeout(ctx, llmCallTimeout)
	defer cancel()

	prompt := fmt.Sprintf(llmContradictionPromptTemplate, contentA, contentB)
	body, err := json.Marshal(map[string]any{
		"model":  model,
		"prompt": prompt,
		"stream": false,
		"options": map[string]any{
			"num_predict": 8,
			"temperature": 0.1,
		},
	})
	if err != nil {
		return false, fmt.Errorf("classifyContradictionLLM: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(callCtx, http.MethodPost,
		strings.TrimRight(ollamaURL, "/")+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("classifyContradictionLLM: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := llmHTTPClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("classifyContradictionLLM: HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("classifyContradictionLLM: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var result struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("classifyContradictionLLM: decode response: %w", err)
	}

	// Parse: if the trimmed lowercase response starts with "yes" → contradiction.
	trimmed := strings.ToLower(strings.TrimSpace(result.Response))
	return strings.HasPrefix(trimmed, "yes"), nil
}
