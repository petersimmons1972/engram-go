package longmemeval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
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

// IsStaleSessionError reports whether err indicates that the MCP server no
// longer recognises the current SSE session — typically because the server was
// restarted between items. The check is case-insensitive and matches the
// canonical "Invalid session ID" text that both the Engram server and the
// mcp-go library produce (JSON-RPC code -32602).
func IsStaleSessionError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "invalid session id")
}

func toolErrorMsg(result *mcp.CallToolResult, toolName string) error {
	if len(result.Content) > 0 {
		if tc, ok := result.Content[0].(mcp.TextContent); ok {
			return fmt.Errorf("%s tool error: %s", toolName, tc.Text)
		}
	}
	return fmt.Errorf("%s tool error", toolName)
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
			select {
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
				return "", ctx.Err()
			}
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
		return "", toolErrorMsg(result, "memory_store")
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("memory_store returned no content")
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

// BatchItem is one entry for StoreBatch.
type BatchItem struct {
	Content string
	Tags    []string
}

// StoreBatch stores up to 100 sessions in a single MCP call and returns their
// IDs in the same order as items.
func (c *Client) StoreBatch(ctx context.Context, project string, items []BatchItem) ([]string, error) {
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		ids, err := c.storeBatch(ctx, project, items)
		if err == nil {
			return ids, nil
		}
		lastErr = err
		if attempt < c.retries {
			select {
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	return nil, lastErr
}

func (c *Client) storeBatch(ctx context.Context, project string, items []BatchItem) ([]string, error) {
	memories := make([]any, len(items))
	for i, item := range items {
		memories[i] = map[string]any{
			"content": item.Content,
			"tags":    item.Tags,
		}
	}
	result, err := c.mcp.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "memory_store_batch",
			Arguments: map[string]any{
				"project":  project,
				"memories": memories,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return nil, toolErrorMsg(result, "memory_store_batch")
	}
	if len(result.Content) == 0 {
		return nil, fmt.Errorf("memory_store_batch returned no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		return nil, fmt.Errorf("unexpected content type from memory_store_batch: %T", result.Content[0])
	}
	var resp struct {
		IDs    []string `json:"ids"`
		Count  int      `json:"count"`
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal([]byte(tc.Text), &resp); err != nil {
		return nil, fmt.Errorf("parse store_batch response: %w", err)
	}
	if len(resp.Errors) > 0 {
		sample := resp.Errors
		if len(sample) > 3 {
			sample = sample[:3]
		}
		return nil, fmt.Errorf("memory_store_batch rejected %d items: %s", len(resp.Errors), strings.Join(sample, "; "))
	}
	if len(resp.IDs) != len(items) {
		return nil, fmt.Errorf("memory_store_batch returned %d ids for %d items", len(resp.IDs), len(items))
	}
	return resp.IDs, nil
}

// Recall calls memory_recall and returns ranked memory IDs.
func (c *Client) Recall(ctx context.Context, project, query string, topK int) ([]string, error) {
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		ids, err := c.recall(ctx, project, query, topK)
		if err == nil {
			return ids, nil
		}
		lastErr = err
		if attempt < c.retries {
			select {
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
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
		return nil, toolErrorMsg(result, "memory_recall")
	}
	if len(result.Content) == 0 {
		return nil, fmt.Errorf("memory_recall returned no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		return nil, fmt.Errorf("unexpected content type from memory_recall: %T", result.Content[0])
	}
	if os.Getenv("LME_DEBUG_RECALL") != "" {
		preview := tc.Text
		if len(preview) > 800 {
			preview = preview[:800] + "…"
		}
		fmt.Fprintf(os.Stderr, "DEBUG recall raw: %s\n", preview)
	}
	// Server may respond in either of two shapes depending on configured default
	// mode (ENGRAM_RECALL_DEFAULT_MODE):
	//   full:   {"results":[{"memory":{"id":"..."},"score":...}, ...]}
	//   handle: {"handles":[{"id":"...","score":...}, ...]}
	// Parse both and prefer whichever is populated.
	var resp struct {
		Results []struct {
			Memory struct {
				ID string `json:"id"`
			} `json:"memory"`
			Score float64 `json:"score"`
		} `json:"results"`
		Handles []struct {
			ID    string  `json:"id"`
			Score float64 `json:"score"`
		} `json:"handles"`
	}
	if err := json.Unmarshal([]byte(tc.Text), &resp); err != nil {
		return nil, fmt.Errorf("parse recall response: %w", err)
	}
	ids := make([]string, 0, len(resp.Results)+len(resp.Handles))
	for _, r := range resp.Results {
		if r.Memory.ID != "" {
			ids = append(ids, r.Memory.ID)
		}
	}
	for _, h := range resp.Handles {
		if h.ID != "" {
			ids = append(ids, h.ID)
		}
	}
	return ids, nil
}

// FetchContent fetches the full content of a memory by ID within a project.
func (c *Client) FetchContent(ctx context.Context, project, id string) (string, error) {
	result, err := c.mcp.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "memory_fetch",
			Arguments: map[string]any{
				"project": project,
				"id":      id,
				"detail":  "full",
			},
		},
	})
	if err != nil {
		return "", err
	}
	if result.IsError {
		return "", toolErrorMsg(result, "memory_fetch")
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("memory_fetch returned no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		return "", fmt.Errorf("unexpected content type from memory_fetch: %T", result.Content[0])
	}
	if os.Getenv("LME_DEBUG_FETCH") != "" {
		preview := tc.Text
		if len(preview) > 600 {
			preview = preview[:600] + "…"
		}
		fmt.Fprintf(os.Stderr, "DEBUG fetch raw: %s\n", preview)
	}
	var resp struct {
		Memory struct {
			Content string `json:"content"`
		} `json:"memory"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(tc.Text), &resp); err != nil {
		return "", fmt.Errorf("parse fetch response: %w", err)
	}
	if resp.Memory.Content != "" {
		return resp.Memory.Content, nil
	}
	return resp.Content, nil
}

// DeleteProject calls memory_delete_project to clean up an isolation project.
//
// Bug #642 fix: when Engram is restarted mid-run the SSE session ID held by
// this client is no longer valid.  Every subsequent cleanup call then returns
// {"code":-32602,"message":"Invalid session ID"}.  Rather than logging a WARN
// per item (which pollutes multi-hundred-item runs), we detect the stale-
// session condition, emit a single DEBUG-level line, and return nil so the
// caller's WARN path is bypassed.  The project data is effectively gone
// anyway: the restart wiped the in-memory session table.
func (c *Client) DeleteProject(ctx context.Context, project string) error {
	result, err := c.mcp.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "memory_delete_project",
			Arguments: map[string]any{"project": project},
		},
	})
	if err != nil {
		if IsStaleSessionError(err) {
			log.Printf("DEBUG cleanup [%s] stale session (server restarted?) — skipping", project)
			return nil
		}
		return err
	}
	if result.IsError {
		return toolErrorMsg(result, "memory_delete_project")
	}
	return nil
}

// Close shuts down the underlying MCP SSE connection.
func (c *Client) Close() error {
	return c.mcp.Close()
}

// RestClient calls engram's sessionless REST endpoints (/quick-store,
// /quick-recall) directly over plain HTTP — no MCP SSE session, no 60s
// transport timeout. Used by the ingest stage for large haystack items.
type RestClient struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewRestClient constructs a RestClient pointed at baseURL with Bearer auth.
func NewRestClient(baseURL, token string) *RestClient {
	return &RestClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// QuickStore stores a single memory via POST /quick-store and returns its ID.
// Retries with exponential backoff on 429 and 5xx (server-side pool init can
// transiently time out when many projects are created concurrently).
func (r *RestClient) QuickStore(ctx context.Context, project, content string, tags []string) (string, error) {
	body := map[string]any{
		"content": content,
		"project": project,
		"tags":    tags,
	}
	data, _ := json.Marshal(body)

	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s, 8s… capped at 16s.
			backoff := time.Duration(1<<min(attempt-1, 4)) * time.Second
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/quick-store", bytes.NewReader(data))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+r.token)

		resp, err := r.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		var result struct {
			OK    bool   `json:"ok"`
			ID    string `json:"id"`
			Error string `json:"error"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&result)
		_ = resp.Body.Close()
		if decodeErr != nil {
			lastErr = fmt.Errorf("quick-store decode: %w", decodeErr)
			continue
		}
		if resp.StatusCode == 429 {
			lastErr = fmt.Errorf("quick-store rate limited (status 429)")
			continue
		}
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("quick-store server error (status %d): %s", resp.StatusCode, result.Error)
			continue
		}
		if !result.OK || result.ID == "" {
			return "", fmt.Errorf("quick-store failed: %s (status %d)", result.Error, resp.StatusCode)
		}
		return result.ID, nil
	}
	return "", lastErr
}

// QuickRecall calls POST /quick-recall and returns memory IDs ranked by score.
func (r *RestClient) QuickRecall(ctx context.Context, project, query string, limit int) ([]string, error) {
	body := map[string]any{
		"query":   query,
		"project": project,
		"limit":   limit,
	}
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/quick-recall", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.token)

	resp, err := r.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("quick-recall decode: %w", err)
	}
	return result.IDs, nil
}

// SessionContent concatenates all turns of a session into a single string,
// preserving role labels so the retriever and generator can distinguish
// user questions from assistant responses.
//
// C0/C1 control characters (except \t, \n, \r) are stripped — the server's
// validateContent rejects them and, because memory_store_batch is all-or-
// nothing, one offender tanks an entire 100-item batch.
func SessionContent(turns []Turn) string {
	var sb strings.Builder
	for _, t := range turns {
		if t.Content == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteByte('\n')
		}
		role := t.Role
		if role == "" {
			role = "user"
		}
		sb.WriteString(role)
		sb.WriteString(": ")
		sb.WriteString(sanitizeControlChars(t.Content))
	}
	return sb.String()
}

// sanitizeControlChars removes C0 (0x00-0x1F except \t \n \r) and C1
// (0x7F-0x9F) control characters from s.
func sanitizeControlChars(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	for _, r := range s {
		if r == '\t' || r == '\n' || r == '\r' {
			sb.WriteRune(r)
			continue
		}
		if r < 0x20 || (r >= 0x7F && r <= 0x9F) {
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

