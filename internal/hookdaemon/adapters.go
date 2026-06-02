package hookdaemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// httpEngramClient is the production EngramClient. It keeps a persistent
// http.Client so connections are pooled across events (issue #396).
type httpEngramClient struct {
	base string
	hc   *http.Client
}

// NewHTTPEngramClient returns an EngramClient targeting baseURL (e.g.
// "http://127.0.0.1:8788"). The client reuses connections across calls.
func NewHTTPEngramClient(baseURL string) EngramClient {
	return &httpEngramClient{
		base: strings.TrimRight(baseURL, "/"),
		hc: &http.Client{
			Timeout: 8 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        4,
				MaxIdleConnsPerHost: 4,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

func (c *httpEngramClient) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health: status %d", resp.StatusCode)
	}
	return nil
}

func (c *httpEngramClient) CheckAuth(ctx context.Context, token string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	body := []byte(`{"query":"auth-check","project":"global","limit":1}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/quick-recall", bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		// 000-equivalent: unreachable → not OK.
		return false, err
	}
	defer resp.Body.Close()
	// 401 = bad token; anything else (incl. 500) = token accepted. Matches the
	// shell scripts' auth semantics exactly.
	return resp.StatusCode != http.StatusUnauthorized, nil
}

func (c *httpEngramClient) Recall(ctx context.Context, token, query, project string, limit int) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	body := jsonMarshal(map[string]any{"query": query, "project": project, "limit": limit})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/quick-recall", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("recall: status %d", resp.StatusCode)
	}
	return readAllLimited(resp.Body, 4<<20)
}

func (c *httpEngramClient) QuickStore(ctx context.Context, token string, body []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/quick-store", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("quick-store: status %d", resp.StatusCode)
	}
	return nil
}

func readAllLimited(r interface{ Read([]byte) (int, error) }, max int64) ([]byte, error) {
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	var total int64
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			total += int64(n)
			if total > max {
				return nil, fmt.Errorf("response too large")
			}
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			if err.Error() == "EOF" {
				return buf, nil
			}
			return buf, nil // tolerate other read terminations; we have what we have
		}
	}
}

// mcpTokenStore reads/writes the Engram bearer token in mcp_servers.json.
type mcpTokenStore struct {
	path string
}

// NewMCPTokenStore returns a TokenStore backed by the given mcp_servers.json
// path (typically ~/.claude/mcp_servers.json).
func NewMCPTokenStore(path string) TokenStore { return &mcpTokenStore{path: path} }

func (s *mcpTokenStore) Load() (string, error) {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return "", err
	}
	var cfg struct {
		MCPServers map[string]struct {
			Headers struct {
				Authorization string `json:"Authorization"`
			} `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return "", err
	}
	auth := cfg.MCPServers["engram"].Headers.Authorization
	return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")), nil
}

// Store writes the token back into mcp_servers.json atomically, preserving all
// other fields. It only touches mcpServers.engram.headers.Authorization.
func (s *mcpTokenStore) Store(token string) error {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		return err
	}
	servers, _ := cfg["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
		cfg["mcpServers"] = servers
	}
	engram, _ := servers["engram"].(map[string]any)
	if engram == nil {
		engram = map[string]any{}
		servers["engram"] = engram
	}
	engram["headers"] = map[string]any{"Authorization": "Bearer " + token}

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return atomicWrite(s.path, out)
}

// fileMemoryWriter writes the recall section into MEMORY.md atomically.
type fileMemoryWriter struct {
	path string
}

// NewFileMemoryWriter returns a MemoryWriter backed by the given MEMORY.md path.
func NewFileMemoryWriter(path string) MemoryWriter { return &fileMemoryWriter{path: path} }

const recallHeading = "## Engram Session Recall"

func (w *fileMemoryWriter) WriteRecallSection(section string) error {
	content := ""
	if b, err := os.ReadFile(w.path); err == nil {
		content = string(b)
	}
	if idx := strings.Index(content, recallHeading); idx >= 0 {
		content = strings.TrimRight(content[:idx], "\n \t")
	}
	content = strings.TrimRight(content, "\n") + "\n" + section + "\n"
	return atomicWrite(w.path, []byte(content))
}

// fileFallbackStore appends entries to fallback.md atomically (read-modify-write
// from a single owning goroutine — no flock needed, issue #396).
type fileFallbackStore struct {
	path string
}

// NewFileFallbackStore returns a FallbackStore backed by the given fallback.md
// path.
func NewFileFallbackStore(path string) FallbackStore { return &fileFallbackStore{path: path} }

func (s *fileFallbackStore) Append(entries []string) error {
	if len(entries) == 0 {
		return nil
	}
	existing := ""
	if b, err := os.ReadFile(s.path); err == nil {
		existing = string(b)
	}
	var sb strings.Builder
	sb.WriteString(strings.TrimRight(existing, "\n"))
	if existing != "" {
		sb.WriteString("\n")
	}
	for _, e := range entries {
		sb.WriteString(e)
		sb.WriteString("\n")
	}
	return atomicWrite(s.path, []byte(sb.String()))
}

// atomicWrite writes data to path via a temp file + rename, so readers never
// observe a partially written file.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".engram_hook_tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}
