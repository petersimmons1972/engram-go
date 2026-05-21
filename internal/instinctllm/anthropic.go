package instinctllm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	defaultAnthropicEndpoint = "https://api.anthropic.com/v1/messages"
	defaultAnthropicModel    = "claude-haiku-4-5-20251001"
	defaultAnthropicTimeout  = 30 * time.Second
)

// anthropicClient implements LLMClient using the Anthropic Messages API.
// It preserves the exact request shape, headers, and error handling that
// existed in cmd/instinct/main.go's callHaiku function, reshaped into the
// generic Complete interface.
type anthropicClient struct {
	endpoint string
	apiKey   string
	model    string
	timeout  time.Duration
}

// NewAnthropicClient constructs an Anthropic LLMClient from cfg.
// Endpoint defaults to the Anthropic production URL when empty.
// Model defaults to claude-haiku-4-5-20251001 when empty.
// Timeout defaults to 30s when zero.
func NewAnthropicClient(cfg Config) (LLMClient, error) {
	c := &anthropicClient{
		endpoint: cfg.Endpoint,
		apiKey:   cfg.APIKey,
		model:    cfg.Model,
		timeout:  cfg.Timeout,
	}
	if c.endpoint == "" {
		c.endpoint = defaultAnthropicEndpoint
	}
	if c.model == "" {
		c.model = defaultAnthropicModel
	}
	if c.timeout == 0 {
		c.timeout = defaultAnthropicTimeout
	}
	return c, nil
}

// Complete sends systemPrompt and userPrompt to Anthropic and returns the
// model's raw response text.  Prompt caching is enabled via the
// anthropic-beta header; the system prompt is marked ephemeral so it
// participates in the cache.
//
// On any HTTP or parse error the method returns a non-nil error.  The caller
// (consolidator.Detect) decides whether to log-and-skip or propagate.
func (c *anthropicClient) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model":      c.model,
		"max_tokens": 1024,
		"system": []map[string]any{{
			"type":          "text",
			"text":          systemPrompt,
			"cache_control": map[string]string{"type": "ephemeral"},
		}},
		"messages": []map[string]any{{
			"role":    "user",
			"content": userPrompt,
		}},
	})
	if err != nil {
		return "", fmt.Errorf("llm/anthropic: marshal request: %w", err)
	}

	// Per-call timeout: cap the Anthropic call so a hung API does not block
	// indefinitely after the buffer has been rotated.
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("llm/anthropic: build request: %w", err)
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm/anthropic: HTTP: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm/anthropic: non-200 status: %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("llm/anthropic: read body: %w", err)
	}

	var apiResp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		slog.Error("llm/anthropic: parse response", "err", err)
		return "", fmt.Errorf("llm/anthropic: parse response: %w", err)
	}
	if len(apiResp.Content) == 0 {
		// Model returned nothing (e.g. content-filter triggered, zero output
		// tokens) — this is a model-behaviour condition, not transport
		// unavailability.  Return a plain error without wrapping the sentinel
		// so callers can distinguish "backend down" from "model said nothing".
		slog.Warn("llm/anthropic: empty content array")
		return "", fmt.Errorf("llm/anthropic: empty content array")
	}

	text := apiResp.Content[0].Text
	// Strip accidental markdown fences — some models ignore "no fences"
	// instructions; strip defensively here so callers see clean text.
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	return text, nil
}
