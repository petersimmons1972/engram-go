package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// LiteLLMClient implements embed.Client against an OpenAI-compatible
// /v1/embeddings endpoint (LiteLLM, OpenAI, etc.).
type LiteLLMClient struct {
	baseURL    string
	model      string
	apiKey     string
	dims       int
	targetDims int
	http       *http.Client
}

func newLiteLLMHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     60 * time.Second,
			// 500ms connect timeout: fail fast when the embed endpoint is
			// unreachable so BM25 fallback fires before the MCP client's
			// context expires. GPU inference timeout is governed by the
			// per-call context (4s in RecallWithOpts), not this dialer.
			DialContext: (&net.Dialer{
				Timeout: 500 * time.Millisecond,
			}).DialContext,
		},
	}
}

// NewLiteLLMClient constructs a LiteLLMClient and validates connectivity with
// a startup probe. apiKey may be empty for unauthenticated local deployments.
// targetDims > 0 requests MRL truncation from the server (model-dependent).
func NewLiteLLMClient(ctx context.Context, baseURL, model, apiKey string, targetDims int) (*LiteLLMClient, error) {
	c := &LiteLLMClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		model:      model,
		apiKey:     apiKey,
		targetDims: targetDims,
		http:       newLiteLLMHTTPClient(),
	}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	vec, err := c.Embed(probeCtx, "probe")
	if err != nil {
		return nil, fmt.Errorf("litellm startup probe: %w", err)
	}
	c.dims = len(vec)
	return c, nil
}

// NewLiteLLMClientNoProbe constructs a LiteLLMClient without a connectivity
// probe. Use when the server must start even if LiteLLM is unavailable;
// Embed() will return errors until LiteLLM recovers.
func NewLiteLLMClientNoProbe(baseURL, model, apiKey string, targetDims int) *LiteLLMClient {
	return &LiteLLMClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		model:      model,
		apiKey:     apiKey,
		targetDims: targetDims,
		http:       newLiteLLMHTTPClient(),
	}
}

func (c *LiteLLMClient) Name() string { return c.model }

// Dimensions returns the known vector size. Before the first successful Embed
// call, falls back to targetDims so the dimension guard in checkEmbedderMeta
// does not falsely report a mismatch on startup.
func (c *LiteLLMClient) Dimensions() int {
	if c.dims > 0 {
		return c.dims
	}
	return c.targetDims
}

// Embed encodes text using the LiteLLM /v1/embeddings endpoint.
// MRL truncation is done client-side (truncate + L2-normalize) rather than
// via the server-side "dimensions" parameter, which is not universally
// supported (llama.cpp returns 400 for unknown params despite drop_params).
func (c *LiteLLMClient) Embed(ctx context.Context, text string) ([]float32, error) {
	text = TruncateToModelWindow(text, ModelMaxTokens(c.model))

	reqBody := map[string]any{
		"model": c.model,
		"input": text,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("litellm embed: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("litellm embed: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("litellm embed request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		rb, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("litellm embed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(rb)))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("litellm embed decode: %w", err)
	}
	if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("litellm embed: empty response")
	}

	vec := result.Data[0].Embedding
	if c.targetDims > 0 && len(vec) > c.targetDims {
		// MRL client-side truncation: take first N dims, then L2-normalize
		// so cosine similarity remains meaningful at the reduced dimension.
		vec = L2Normalize(vec[:c.targetDims])
	}
	if c.dims == 0 {
		c.dims = len(vec)
	}
	return vec, nil
}
