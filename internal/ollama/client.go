package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultBase = "http://localhost:11434"

type Client struct {
	base string
	http *http.Client
}

// NewClient returns a client for the Ollama HTTP API.
// Callers are responsible for context timeouts; no client-level deadline is set so Pull can run long.
func NewClient(base string) *Client {
	if base == "" {
		base = defaultBase
	}
	return &Client{base: base, http: &http.Client{Timeout: 60 * time.Second}}
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: %d %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: %d %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *Client) Version(ctx context.Context) (string, error) {
	var v VersionResponse
	if err := c.get(ctx, "/api/version", &v); err != nil {
		return "", err
	}
	return v.Version, nil
}

func (c *Client) IsAvailable(ctx context.Context, model string) (bool, string, error) {
	var tags TagsResponse
	if err := c.get(ctx, "/api/tags", &tags); err != nil {
		return false, "", err
	}
	for _, m := range tags.Models {
		if m.Name == model {
			return true, m.Digest, nil
		}
	}
	return false, "", nil
}

// Pull streams pull progress to w. Returns (digest, error).
// Uses 1MB scanner buffer — Ollama progress lines can exceed default 64KB.
func (c *Client) Pull(ctx context.Context, model string, w io.Writer) (string, error) {
	body := map[string]string{"name": model}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/pull", bytes.NewReader(b))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("pull request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("pull %s: %d %s", model, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	var lastStatus string
	for sc.Scan() {
		line := sc.Text()
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			continue
		}
		if errMsg, ok := m["error"].(string); ok {
			return "", fmt.Errorf("pull error: %s", errMsg)
		}
		if status, ok := m["status"].(string); ok {
			lastStatus = status
			fmt.Fprintf(w, "\r  pulling %s: %s        ", model, status)
		}
	}
	fmt.Fprintln(w)
	if sc.Err() != nil {
		return "", sc.Err()
	}
	if strings.Contains(lastStatus, "error") {
		return "", fmt.Errorf("pull failed: %s", lastStatus)
	}
	// Fetch digest after pull
	_, digest, err := c.IsAvailable(ctx, model)
	return digest, err
}

// Chat sends a single non-streaming request. Caller must supply a bounded ctx.
func (c *Client) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	req.Stream = false
	var resp ChatResponse
	if err := c.post(ctx, "/api/chat", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Evict unloads the model from GPU memory.
func (c *Client) Evict(ctx context.Context, model string) error {
	ka := 0
	body := ChatRequest{
		Model:     model,
		Messages:  []Message{},
		Stream:    false,
		KeepAlive: &ka,
	}
	var resp map[string]any
	return c.post(ctx, "/api/chat", body, &resp)
}
