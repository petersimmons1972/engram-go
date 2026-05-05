package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/metrics"
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
	cb         *CircuitBreaker
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
	return NewLiteLLMClientNoProbeWithCircuitBreaker(baseURL, model, apiKey, targetDims, CircuitConfig{})
}

// NewLiteLLMClientNoProbeWithCircuitBreaker constructs a LiteLLMClient without a connectivity
// probe, with optional circuit breaker configuration.
func NewLiteLLMClientNoProbeWithCircuitBreaker(baseURL, model, apiKey string, targetDims int, cbCfg CircuitConfig) *LiteLLMClient {
	c := &LiteLLMClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		model:      model,
		apiKey:     apiKey,
		targetDims: targetDims,
		http:       newLiteLLMHTTPClient(),
	}

	if cbCfg.Enabled || cbCfg.FailureThreshold > 0 {
		c.cb = NewCircuitBreaker(cbCfg)
		// Register callback to emit metrics on state transitions
		c.cb.onStateChange = func(from, to CircuitState) {
			fromStr := from.String()
			toStr := to.String()
			metrics.EmbedCircuitTransitions.WithLabelValues(fromStr, toStr).Inc()
			// Emit state gauge (convert state to numeric value for clarity)
			stateValue := 0.0
			switch to {
			case StateClosed:
				stateValue = 1.0
			case StateOpen:
				stateValue = 2.0
			case StateHalfOpen:
				stateValue = 3.0
			}
			metrics.EmbedCircuitState.WithLabelValues(toStr).Set(stateValue)
		}
	}

	return c
}

// isRetryableError checks if an error is transient and should trigger a retry.
// Retryable errors: connection refused, EOF, 502, 503, 504, 429, 408.
// Non-retryable: context canceled, context deadline, 4xx other than 408/429.
func isRetryableError(err error, statusCode int) bool {
	// Context cancellation is never retryable
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Check HTTP status codes
	if statusCode > 0 {
		switch statusCode {
		case http.StatusBadGateway, // 502
			http.StatusServiceUnavailable, // 503
			http.StatusGatewayTimeout,     // 504
			http.StatusTooManyRequests,    // 429
			http.StatusRequestTimeout:     // 408
			return true
		}
		// Other 4xx are not retryable
		if statusCode >= 400 && statusCode < 500 {
			return false
		}
	}

	// Check for transient network errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		// Connection refused, timeout, etc. are retryable
		return true
	}

	// EOF is retryable
	if errors.Is(err, io.EOF) {
		return true
	}

	// Unexpected EOF is also retryable
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	return false
}

// computeBackoff returns the backoff duration for the given attempt (0-indexed).
// Formula: base * 2^attempt + jitter, where base=100ms.
// Attempt 0: 100ms → 200ms ± 25% = 75–125ms (no jitter needed, it's the first retry)
// Attempt 1: 200ms → 400ms ± 25% = 300–500ms
// Attempt 2: 400ms → 1600ms ± 25% = 1.2–2.0s.
func computeBackoff(attempt int) time.Duration {
	base := 100 * time.Millisecond
	// base * 2^attempt gives us the exponential part
	exponential := time.Duration(int64(base) * int64(math.Pow(2, float64(attempt))))

	// Add jitter: ±25%
	jitterFraction := 0.25
	jitterRange := time.Duration(int64(float64(exponential) * jitterFraction))
	// Random jitter: -25% to +25%
	jitter := time.Duration(rand.Int63n(int64(2*jitterRange)) - int64(jitterRange))

	return exponential + jitter
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
//
// Implements exponential backoff retry on transient errors (502, 503, 504, 429, 408,
// connection refused, EOF) with up to 3 attempts (2 retries), 100ms→400ms→1.6s
// backoff with ±25% jitter. Increments engram_embed_retries_total per retry
// and engram_embed_failures_total{reason=exhausted|non_retryable} on final failure.
//
// If a circuit breaker is configured, checks circuit state first and short-circuits
// with errCircuitOpen if the upstream is consistently failing.
func (c *LiteLLMClient) Embed(ctx context.Context, text string) ([]float32, error) {
	// Check circuit breaker before attempting request
	if c.cb != nil {
		if err := c.cb.Allow(); err != nil {
			metrics.EmbedFailures.WithLabelValues("circuit_open").Inc()
			return nil, err
		}
	}

	text = TruncateToModelWindow(text, ModelMaxTokens(c.model))

	reqBody := map[string]any{
		"model": c.model,
		"input": text,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("litellm embed: marshal: %w", err)
	}

	const maxAttempts = 3
	var lastErr error
	var lastStatusCode int

	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Apply backoff before retry (not on first attempt)
		if attempt > 0 {
			backoff := computeBackoff(attempt - 1)
			select {
			case <-time.After(backoff):
				// Continue after backoff
			case <-ctx.Done():
				// Context expired during backoff
				metrics.EmbedFailures.WithLabelValues("non_retryable").Inc()
				return nil, fmt.Errorf("litellm embed: context canceled during backoff: %w", ctx.Err())
			}
		}

		// Check if context is still valid before making the request
		if ctx.Err() != nil {
			metrics.EmbedFailures.WithLabelValues("non_retryable").Inc()
			return nil, fmt.Errorf("litellm embed: context deadline exceeded before attempt: %w", ctx.Err())
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
		lastErr = err
		lastStatusCode = 0

		if err != nil {
			// Network error: check if retryable
			if !isRetryableError(err, 0) {
				metrics.EmbedFailures.WithLabelValues("non_retryable").Inc()
				if c.cb != nil {
					c.cb.RecordFailure()
				}
				return nil, fmt.Errorf("litellm embed request: %w", err)
			}
			// Retryable error; log and retry
			if attempt < maxAttempts-1 {
				metrics.EmbedRetries.Inc()
				continue
			}
			// Last attempt failed
			metrics.EmbedFailures.WithLabelValues("exhausted").Inc()
			if c.cb != nil {
				c.cb.RecordFailure()
			}
			return nil, fmt.Errorf("litellm embed request (exhausted retries): %w", err)
		}

		defer resp.Body.Close() //nolint:errcheck
		lastStatusCode = resp.StatusCode

		if resp.StatusCode != http.StatusOK {
			rb, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			statusErr := fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(rb)))

			if !isRetryableError(statusErr, resp.StatusCode) {
				metrics.EmbedFailures.WithLabelValues("non_retryable").Inc()
				if c.cb != nil {
					c.cb.RecordFailure()
				}
				return nil, fmt.Errorf("litellm embed: %w", statusErr)
			}
			// Retryable HTTP error; log and retry
			if attempt < maxAttempts-1 {
				metrics.EmbedRetries.Inc()
				continue
			}
			// Last attempt failed
			metrics.EmbedFailures.WithLabelValues("exhausted").Inc()
			if c.cb != nil {
				c.cb.RecordFailure()
			}
			return nil, fmt.Errorf("litellm embed: %w (exhausted retries)", statusErr)
		}

		// Success: decode response
		var result struct {
			Data []struct {
				Embedding []float32 `json:"embedding"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			// Decode error is non-retryable
			metrics.EmbedFailures.WithLabelValues("non_retryable").Inc()
			if c.cb != nil {
				c.cb.RecordFailure()
			}
			return nil, fmt.Errorf("litellm embed decode: %w", err)
		}
		if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
			// Empty response is non-retryable
			metrics.EmbedFailures.WithLabelValues("non_retryable").Inc()
			if c.cb != nil {
				c.cb.RecordFailure()
			}
			return nil, fmt.Errorf("litellm embed: empty response")
		}

		// Success!
		vec := result.Data[0].Embedding
		if c.targetDims > 0 && len(vec) > c.targetDims {
			// MRL client-side truncation: take first N dims, then L2-normalize
			// so cosine similarity remains meaningful at the reduced dimension.
			vec = L2Normalize(vec[:c.targetDims])
		}
		if c.dims == 0 {
			c.dims = len(vec)
		}

		// Record success in circuit breaker and return
		if c.cb != nil {
			c.cb.RecordSuccess()
		}

		return vec, nil
	}

	// Should not reach here, but as a fallback
	metrics.EmbedFailures.WithLabelValues("exhausted").Inc()
	if c.cb != nil {
		c.cb.RecordFailure()
	}
	return nil, fmt.Errorf("litellm embed: exhausted %d attempts, last error: %w (status %d)", maxAttempts, lastErr, lastStatusCode)
}

// Probe checks if the LiteLLM endpoint is healthy and reachable. It makes a
// single /v1/embeddings request with a minimal input and returns (true, "") on
// success or (false, reason) on failure. The caller is responsible for context
// timeout/cancellation.
func (c *LiteLLMClient) Probe(ctx context.Context) (ok bool, reason string) {
	_, err := c.Embed(ctx, "probe")
	if err == nil {
		return true, ""
	}
	return false, fmt.Sprintf("embed_probe: %v", err)
}
