// Package embed provides the embedding client for Engram.
// Only Ollama is supported — no remote/cloud providers.
package embed

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
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

// OllamaClient calls the local Ollama /api/embed endpoint.
type OllamaClient struct {
	baseURL string
	model   string
	dims    int
	http    *http.Client
}

// NewOllamaClient constructs an OllamaClient and validates connectivity.
// If the model is absent from Ollama, it triggers a pull and waits (max 5 min).
func NewOllamaClient(ctx context.Context, baseURL, model string) (*OllamaClient, error) {
	baseURL = strings.TrimRight(baseURL, "/")

	// DNS-safe transport: short idle timeout ensures DNS changes propagate within 30s.
	transport := &http.Transport{
		IdleConnTimeout:     30 * time.Second,
		MaxIdleConnsPerHost: 2,
	}
	hc := &http.Client{Transport: transport, Timeout: 60 * time.Second}

	c := &OllamaClient{baseURL: baseURL, model: model, http: hc}

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

// Embed calls POST /api/embed and returns the first embedding vector.
func (c *OllamaClient) Embed(ctx context.Context, text string) ([]float32, error) {
	body, _ := json.Marshal(map[string]string{"model": c.model, "input": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama embed: HTTP %d", resp.StatusCode)
	}

	var result struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
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
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}
	for _, m := range result.Models {
		if strings.HasPrefix(m.Name, c.model) {
			return true, nil
		}
	}
	return false, nil
}

func (c *OllamaClient) pullModel(ctx context.Context) error {
	pullCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	body, _ := json.Marshal(map[string]string{"name": c.model})
	req, err := http.NewRequestWithContext(pullCtx, http.MethodPost, c.baseURL+"/api/pull", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("ollama pull: %w", err)
	}
	defer resp.Body.Close()

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
	return scanner.Err()
}
