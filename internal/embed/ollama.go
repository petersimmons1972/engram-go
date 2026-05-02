// Package embed provides the embedding client for Engram.
// Only Ollama is supported — no remote/cloud providers.
package embed

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/netutil"
)

// Client is the embedding provider interface.
type Client interface {
	// Embed returns a float32 vector for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)
	// Name returns the model identifier (e.g. "nomic-embed-text").
	Name() string
	// Dimensions returns the vector size, or 0 before the first successful embed.
	Dimensions() int
}

const maxEmbedResponseBytes = 10 * 1024 * 1024 // 10 MiB (#16)

// OllamaClient calls the local Ollama /api/embed endpoint.
type OllamaClient struct {
	baseURL    string
	model      string
	dims       int
	targetDims int          // MRL truncation target; 0 = use model native output
	http       *http.Client // short-timeout client for embed/list requests
	pullHTTP   *http.Client // no client-level timeout for long model pulls
}

// NewOllamaClient constructs an OllamaClient and validates connectivity.
// If the model is absent from Ollama, it triggers a pull and waits (max 5 min).
func NewOllamaClient(ctx context.Context, baseURL, model string) (*OllamaClient, error) {
	return newOllamaClient(ctx, baseURL, model, 0, nil)
}

// NewOllamaClientWithDims constructs an OllamaClient that requests Ollama to
// truncate embeddings to targetDims using Matryoshka Representation Learning.
// Pass 0 for targetDims to use the model's native output dimension.
func NewOllamaClientWithDims(ctx context.Context, baseURL, model string, targetDims int) (*OllamaClient, error) {
	return newOllamaClient(ctx, baseURL, model, targetDims, nil)
}

// NewOllamaClientWithTransport constructs an OllamaClient using a supplied
// HTTP transport. Tests use this to avoid binding loopback listeners.
func NewOllamaClientWithTransport(ctx context.Context, baseURL, model string, transport http.RoundTripper) (*OllamaClient, error) {
	return newOllamaClient(ctx, baseURL, model, 0, transport)
}

// newOllamaClient is the internal constructor. When customTransport is non-nil
// it is used as-is (test seam only — bypasses SSRF guard). When nil, the
// production DNS-rebinding-safe transport is constructed.
func newOllamaClient(ctx context.Context, baseURL, model string, targetDims int, customTransport http.RoundTripper) (*OllamaClient, error) {
	baseURL = strings.TrimRight(baseURL, "/")

	var transport http.RoundTripper
	if customTransport != nil {
		transport = customTransport
	} else {
		// DNS-rebinding-safe transport (#242).
		// Re-resolves the hostname on every dial and rejects private IPs,
		// preventing an attacker from using a short-TTL DNS record to bypass
		// the startup IP check by switching the resolution to an internal address.
		//
		// The configured baseURL host is allow-listed: the operator explicitly
		// chose it (e.g. Docker service name "ollama"), so its private-IP resolution
		// is intentional and must not be blocked.
		var configuredHost string
		if u, err := url.Parse(baseURL); err == nil {
			configuredHost = u.Hostname()
		}
		baseDialer := &net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		transport = &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				// Re-resolve on every dial — prevents DNS rebinding SSRF.
				addrs, err := net.DefaultResolver.LookupHost(ctx, host)
				if err != nil {
					return nil, fmt.Errorf("DNS resolution failed for %q: %w", host, err)
				}
				// Skip private-IP check for the operator-configured host.
				if host != configuredHost {
					for _, resolved := range addrs {
						if netutil.IsPrivateIP(resolved) {
							return nil, fmt.Errorf("ollama URL resolved to private IP %q (SSRF protection, closes #242)", resolved)
						}
					}
				}
				return baseDialer.DialContext(ctx, network, net.JoinHostPort(addrs[0], port))
			},
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			IdleConnTimeout:       30 * time.Second,
			MaxIdleConnsPerHost:   2,
		}
	}

	// Client-level Timeout provides an outer bound for non-pull requests.
	// Embed calls include text up to a few KB; 60s is generous.
	hc := &http.Client{Transport: transport, Timeout: 60 * time.Second}

	// pullHTTP omits the client-level timeout — model pulls stream for minutes
	// and are bounded only by the per-request context.
	pullHTTP := &http.Client{Transport: transport}

	c := &OllamaClient{baseURL: baseURL, model: model, targetDims: targetDims, http: hc, pullHTTP: pullHTTP}

	if err := c.ensureModel(ctx); err != nil {
		return nil, fmt.Errorf("ollama startup check failed: %w", err)
	}

	// Detect dimensions from a probe embed.
	vec, err := c.Embed(ctx, "probe")
	if err != nil {
		return nil, fmt.Errorf("probe embed failed: %w", err)
	}
	c.dims = len(vec)

	return c, nil
}

func (c *OllamaClient) Name() string    { return c.model }
func (c *OllamaClient) Dimensions() int { return c.dims }

// MaxTokens returns the context window limit for the configured embedding model.
// Used by the search engine to size chunks correctly and as a safety-net in Embed.
func (c *OllamaClient) MaxTokens() int { return ModelMaxTokens(c.model) }

// Embed calls POST /api/embed and returns the first embedding vector.
// When targetDims > 0, passes "dimensions" to Ollama for MRL truncation.
// Text is truncated to the model's context window before the call so Ollama
// never receives text that would exceed the model's token limit (#361).
func (c *OllamaClient) Embed(ctx context.Context, text string) ([]float32, error) {
	text = TruncateToModelWindow(text, c.MaxTokens())
	reqBody := map[string]any{"model": c.model, "input": text}
	if c.targetDims > 0 {
		reqBody["dimensions"] = c.targetDims
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("ollama embed: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama embed: HTTP %d", resp.StatusCode)
	}

	var result struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxEmbedResponseBytes)).Decode(&result); err != nil {
		return nil, fmt.Errorf("ollama embed decode: %w", err)
	}
	if len(result.Embeddings) == 0 || len(result.Embeddings[0]) == 0 {
		return nil, fmt.Errorf("ollama embed: empty response")
	}
	return result.Embeddings[0], nil
}

func (c *OllamaClient) ensureModel(ctx context.Context) error {
	present, err := c.modelPresent(ctx)
	if err != nil {
		return err
	}
	if present {
		return nil
	}
	return c.pullModel(ctx)
}

func (c *OllamaClient) modelPresent(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return false, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("ollama tags: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}
	for _, m := range result.Models {
		if m.Name == c.model || m.Name == c.model+":latest" {
			return true, nil
		}
	}
	return false, nil
}

func (c *OllamaClient) pullModel(ctx context.Context) error {
	pullCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	body, err := json.Marshal(map[string]string{"name": c.model})
	if err != nil {
		return fmt.Errorf("ollama pull: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(pullCtx, http.MethodPost, c.baseURL+"/api/pull", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.pullHTTP.Do(req)
	if err != nil {
		return fmt.Errorf("ollama pull: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama pull: HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Drain streaming NDJSON lines until "success" or end of body.
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var line struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(scanner.Bytes(), &line) == nil && line.Status == "success" {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("ollama pull: reading response: %w", err)
	}
	return fmt.Errorf("ollama pull: stream ended without success status")
}
