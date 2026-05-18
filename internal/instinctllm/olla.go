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
	defaultOllaHost    = "https://olla.petersimmons.com"
	ollaModelsPath     = "/olla/models"
	ollaCompletePath   = "/v1/chat/completions"
	defaultOllaTimeout = 60 * time.Second
)

// skipFamilies lists Olla model families whose thinking-mode token leakage
// breaks structured JSON output (benchmark 2026-04-24).
// See: ~/projects/instinct/consolidator/instinct/haiku_client.py:30
var skipFamilies = map[string]struct{}{
	"qwen3":    {},
	"qwen3moe": {},
}

// ollaClient implements LLMClient using Olla's OpenAI-compatible endpoint.
// Model selection is dynamic: on each Complete call (or lazily on first use)
// the client queries /olla/models, picks the first text-generation-capable
// available model that is not in the skip list, and uses that model for the
// completion call.
//
// Fail-quiet semantics: if the model discovery endpoint is unreachable, returns
// no suitable model, or the completion fails, Complete returns ("", nil) —
// matching the Python HaikuClient's behaviour (pattern detection silently
// disabled rather than crashing the consolidator).
type ollaClient struct {
	host    string
	timeout time.Duration
}

// NewOllaClient constructs an Olla LLMClient from cfg.
// cfg.Endpoint is the base host (e.g. "https://olla.petersimmons.com").
// Defaults to defaultOllaHost when empty.
// cfg.APIKey and cfg.Model are ignored; Olla resolves the model dynamically.
func NewOllaClient(cfg Config) (LLMClient, error) {
	host := cfg.Endpoint
	if host == "" {
		host = defaultOllaHost
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultOllaTimeout
	}
	return &ollaClient{host: host, timeout: timeout}, nil
}

// pickModel queries /olla/models and returns the first model id suitable for
// text generation.  Returns "" if none is found or if the endpoint is
// unreachable.
func (c *ollaClient) pickModel(ctx context.Context) string {
	url := c.host + ollaModelsPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		slog.Warn("llm/olla: build model-discovery request", "err", err)
		return ""
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Warn("llm/olla: cannot reach Olla", "host", c.host, "err", err)
		return ""
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("llm/olla: model discovery non-200", "status", resp.StatusCode)
		return ""
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Warn("llm/olla: read model-discovery body", "err", err)
		return ""
	}

	var payload struct {
		Data []struct {
			ID   string `json:"id"`
			Olla struct {
				Family       string   `json:"family"`
				Capabilities []string `json:"capabilities"`
				Availability []struct {
					State string `json:"state"`
				} `json:"availability"`
			} `json:"olla"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		slog.Warn("llm/olla: parse model-discovery response", "err", err)
		return ""
	}

	for _, model := range payload.Data {
		olla := model.Olla

		// Must support text generation.
		hasTextGen := false
		for _, cap := range olla.Capabilities {
			if cap == "text-generation" {
				hasTextGen = true
				break
			}
		}
		if !hasTextGen {
			continue
		}

		// At least one availability slot must be "available".
		available := false
		for _, a := range olla.Availability {
			if a.State == "available" {
				available = true
				break
			}
		}
		if !available {
			continue
		}

		// Skip qwen3 family — thinking-mode token leakage breaks JSON output.
		if _, skip := skipFamilies[olla.Family]; skip {
			continue
		}
		if strings.HasPrefix(strings.ToLower(model.ID), "qwen3") {
			continue
		}

		return model.ID
	}

	slog.Warn("llm/olla: no suitable model found", "host", c.host)
	return ""
}

// Complete sends systemPrompt + userPrompt to Olla using the OpenAI-compatible
// chat completions API.  If no suitable model is available, returns ("", nil)
// (fail-quiet — matches Python HaikuClient.detect behaviour).
func (c *ollaClient) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	// Per-call context with timeout so a slow Olla does not block forever.
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	model := c.pickModel(callCtx)
	if model == "" {
		// No usable model — silently return empty (pattern detection disabled).
		return "", nil
	}

	reqBody, err := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]any{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 0,
	})
	if err != nil {
		return "", fmt.Errorf("llm/olla: marshal completion request: %w", err)
	}

	url := c.host + ollaCompletePath
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("llm/olla: build completion request: %w", err)
	}
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Warn("llm/olla: completion HTTP error", "err", err)
		// Fail-quiet: Olla may be transiently unavailable.
		return "", nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("llm/olla: completion non-200", "status", resp.StatusCode)
		return "", nil
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Warn("llm/olla: read completion body", "err", err)
		return "", nil
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &apiResp); err != nil || len(apiResp.Choices) == 0 {
		slog.Warn("llm/olla: parse completion response", "err", err)
		return "", nil
	}

	text := apiResp.Choices[0].Message.Content
	// Strip accidental markdown fences — matches Python haiku_client.py:116-117.
	if strings.HasPrefix(text, "```") {
		text = strings.SplitN(text, "\n", 2)[1]
		if idx := strings.LastIndex(text, "```"); idx >= 0 {
			text = text[:idx]
		}
	}
	text = strings.TrimSpace(text)

	return text, nil
}
