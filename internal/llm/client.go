// Package llm provides a minimal OpenAI-compatible generation client used by
// the summarize and consolidate packages. It targets LiteLLM's
// /v1/chat/completions endpoint, which is also compatible with direct OpenAI
// and any other provider LiteLLM proxies.
package llm

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

var httpClient = &http.Client{
	Timeout: 120 * time.Second,
	Transport: &http.Transport{
		IdleConnTimeout:     60 * time.Second,
		MaxIdleConnsPerHost: 4,
	},
}

// Complete sends a single-turn user prompt to an OpenAI-compatible
// /v1/chat/completions endpoint and returns the assistant's text response.
// apiKey may be empty for unauthenticated local deployments.
func Complete(ctx context.Context, baseURL, apiKey, model, prompt string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"stream": false,
	})
	if err != nil {
		return "", fmt.Errorf("llm complete: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(baseURL, "/")+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("llm complete: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm complete: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		rb, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("llm complete: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(rb)))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("llm complete: decode: %w", err)
	}
	if len(result.Choices) == 0 || result.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("llm complete: empty response")
	}
	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}
