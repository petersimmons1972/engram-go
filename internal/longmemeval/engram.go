package longmemeval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// Client wraps the MCP SSE client with retry logic for eval use.
type Client struct {
	mcp     *client.Client
	retries int
}

// Connect creates an authenticated MCP SSE client connected to serverURL.
func Connect(ctx context.Context, serverURL, apiKey string) (*Client, error) {
	sseURL := strings.TrimRight(serverURL, "/") + "/sse"
	headers := map[string]string{}
	if apiKey != "" {
		headers["Authorization"] = "Bearer " + apiKey
	}
	c, err := client.NewSSEMCPClient(sseURL, transport.WithHeaders(headers))
	if err != nil {
		return nil, err
	}
	if err := c.Start(ctx); err != nil {
		return nil, err
	}
	_, err = c.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "longmemeval", Version: "1.0.0"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("initialize MCP: %w", err)
	}
	return &Client{mcp: c, retries: 1}, nil
}

// Store stores one session as a memory and returns the memory ID.
func (c *Client) Store(ctx context.Context, project, content string, tags []string) (string, error) {
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		id, err := c.store(ctx, project, content, tags)
		if err == nil {
			return id, nil
		}
		lastErr = err
		if attempt < c.retries {
			time.Sleep(5 * time.Second)
		}
	}
	return "", lastErr
}

func (c *Client) store(ctx context.Context, project, content string, tags []string) (string, error) {
	result, err := c.mcp.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "memory_store",
			Arguments: map[string]any{
				"content": content,
				"project": project,
				"tags":    tags,
			},
		},
	})
	if err != nil {
		return "", err
	}
	if result.IsError {
		return "", fmt.Errorf("memory_store tool error")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		return "", fmt.Errorf("unexpected content type from memory_store")
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(tc.Text), &resp); err != nil {
		return "", fmt.Errorf("parse store response: %w", err)
	}
	return resp.ID, nil
}

// Recall calls memory_recall and returns ranked memory IDs.
// The server returns {"results":[{"memory":{"id":"..."},"score":...},...]}
func (c *Client) Recall(ctx context.Context, project, query string, topK int) ([]string, error) {
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		ids, err := c.recall(ctx, project, query, topK)
		if err == nil {
			return ids, nil
		}
		lastErr = err
		if attempt < c.retries {
			time.Sleep(5 * time.Second)
		}
	}
	return nil, lastErr
}

func (c *Client) recall(ctx context.Context, project, query string, topK int) ([]string, error) {
	result, err := c.mcp.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "memory_recall",
			Arguments: map[string]any{
				"query":   query,
				"project": project,
				"top_k":   topK,
				"detail":  "summary",
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if result.IsError {
		if len(result.Content) > 0 {
			if tc, ok := result.Content[0].(mcp.TextContent); ok {
				return nil, fmt.Errorf("memory_recall tool error: %s", tc.Text)
			}
		}
		return nil, fmt.Errorf("memory_recall tool error")
	}
	if len(result.Content) == 0 {
		return nil, fmt.Errorf("memory_recall returned no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		return nil, fmt.Errorf("unexpected content type from memory_recall: %T", result.Content[0])
	}
	// Server returns {"results":[{"memory":{"id":"..."},"score":...},...]}
	var resp struct {
		Results []struct {
			Memory struct {
				ID string `json:"id"`
			} `json:"memory"`
			Score float64 `json:"score"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(tc.Text), &resp); err != nil {
		return nil, fmt.Errorf("parse recall response: %w", err)
	}
	ids := make([]string, 0, len(resp.Results))
	for _, r := range resp.Results {
		ids = append(ids, r.Memory.ID)
	}
	return ids, nil
}

// FetchContent fetches the full content of a memory by ID.
func (c *Client) FetchContent(ctx context.Context, id string) (string, error) {
	result, err := c.mcp.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "memory_fetch",
			Arguments: map[string]any{
				"id":     id,
				"detail": "full",
			},
		},
	})
	if err != nil {
		return "", err
	}
	if result.IsError {
		return "", fmt.Errorf("memory_fetch tool error")
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("memory_fetch returned no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		return "", fmt.Errorf("unexpected content type from memory_fetch: %T", result.Content[0])
	}
	var resp struct {
		Memory struct {
			Content string `json:"content"`
		} `json:"memory"`
	}
	if err := json.Unmarshal([]byte(tc.Text), &resp); err != nil {
		return "", fmt.Errorf("parse fetch response: %w", err)
	}
	return resp.Memory.Content, nil
}

// DeleteProject calls memory_delete_project to clean up an isolation project.
func (c *Client) DeleteProject(ctx context.Context, project string) error {
	result, err := c.mcp.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "memory_delete_project",
			Arguments: map[string]any{"project": project},
		},
	})
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("memory_delete_project tool error for %q", project)
	}
	return nil
}

// SessionContent concatenates the user-role turns of a session into a single string.
func SessionContent(turns []Turn) string {
	var sb strings.Builder
	for _, t := range turns {
		if t.Role == "user" {
			if sb.Len() > 0 {
				sb.WriteByte(' ')
			}
			sb.WriteString(t.Content)
		}
	}
	return sb.String()
}
