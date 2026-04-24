// Package claude provides an HTTP client for the Anthropic Messages API
// with the advisor tool declared on every request.
package claude

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

// Client sends requests to the Anthropic Messages API.
type Client struct {
	apiKey     string
	BaseURL    string // default: "https://api.anthropic.com"; exported for test overrides
	httpClient *http.Client
}

// New returns a Client or an error if apiKey is empty.
func New(apiKey string) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("claude: apiKey must not be empty")
	}

	// DNS-safe transport: short idle timeout ensures DNS changes propagate within 30s.
	transport := &http.Transport{
		IdleConnTimeout:     30 * time.Second,
		MaxIdleConnsPerHost: 2,
	}

	return &Client{
		apiKey:     apiKey,
		BaseURL:    "https://api.anthropic.com",
		httpClient: &http.Client{Transport: transport},
	}, nil
}

// Complete sends a messages request with the advisor tool declared and returns
// the text from the first content block.
//
// executorModel is the model that runs the request (e.g. "claude-sonnet-4-6").
// advisorModel is the model to escalate to (e.g. "claude-opus-4-6").
// advisorMaxUses is the max_uses value in the advisor tool declaration.
// maxTokens is the max_tokens field in the request.
// claudeAPITimeout is the maximum time a single Claude API call may take.
// This guards against hung connections when the caller's context has no deadline.
const claudeAPITimeout = 90 * time.Second

// Complete calls CompleteWithUsage and discards the token-usage metadata.
func (c *Client) Complete(ctx context.Context, system, prompt, executorModel, advisorModel string, advisorMaxUses, maxTokens int) (string, error) {
	text, _, err := c.CompleteWithUsage(ctx, system, prompt, executorModel, advisorModel, advisorMaxUses, maxTokens)
	return text, err
}

// TokenUsage reports token accounting for a single completion call.
type TokenUsage struct {
	InputTokens  int
	OutputTokens int
}

// Total returns the sum of input + output tokens.
func (u TokenUsage) Total() int { return u.InputTokens + u.OutputTokens }

// CompleteWithUsage is like Complete but also returns token usage from the response.
func (c *Client) CompleteWithUsage(ctx context.Context, system, prompt, executorModel, advisorModel string, advisorMaxUses, maxTokens int) (string, TokenUsage, error) {
	// Apply a per-request deadline if the caller's context has none.
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, claudeAPITimeout)
		defer cancel()
	}

	reqBody := messagesRequest{
		Model:     executorModel,
		MaxTokens: maxTokens,
		System:    system,
		Messages: []message{
			{Role: "user", Content: prompt},
		},
		Tools: []advisorTool{
			{
				Type:    "advisor_20260301",
				Name:    "advisor",
				Model:   advisorModel,
				MaxUses: advisorMaxUses,
			},
		},
	}

	encoded, err := json.Marshal(reqBody)
	if err != nil {
		return "", TokenUsage{}, fmt.Errorf("claude: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/messages", bytes.NewReader(encoded))
	if err != nil {
		return "", TokenUsage{}, fmt.Errorf("claude: build request: %w", err)
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", TokenUsage{}, fmt.Errorf("claude: HTTP request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", TokenUsage{}, fmt.Errorf("claude: HTTP %d: %s", resp.StatusCode, body)
	}

	var result messagesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", TokenUsage{}, fmt.Errorf("claude: decode response: %w", err)
	}

	if len(result.Content) == 0 {
		return "", TokenUsage{}, fmt.Errorf("claude: empty content in response")
	}

	usage := TokenUsage{
		InputTokens:  result.Usage.InputTokens,
		OutputTokens: result.Usage.OutputTokens,
	}
	return extractJSON(result.Content[0].Text), usage, nil
}

// extractJSON strips leading/trailing ```json / ``` markdown fences if present.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}
	return s
}

// -- request / response types -------------------------------------------------

type messagesRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	System    string        `json:"system,omitempty"`
	Messages  []message     `json:"messages"`
	Tools     []advisorTool `json:"tools"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type advisorTool struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Model   string `json:"model"`
	MaxUses int    `json:"max_uses"`
}

type messagesResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}
