package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/petersimmons1972/engram/internal/metrics"
	"github.com/petersimmons1972/engram/internal/netutil"
)

// LiteLLMClient implements embed.Client against an OpenAI-compatible
// /v1/embeddings endpoint (LiteLLM, OpenAI, etc.).
type LiteLLMClient struct {
	baseURL    string
	model      string
	apiKey     string
	dims       atomic.Int32
	targetDims int
	http       *http.Client
	cb         *CircuitBreaker
	// probeFunc performs the recovery health check. Defaults to c.Probe (a
	// real GET /v1/models). Overridable in tests so the background-probe loop
	// can be driven deterministically without a network or wall-clock sleeps.
	probeFunc func(ctx context.Context) (ok bool, reason string)
}

// newLiteLLMHTTPClient builds a *http.Client for the LiteLLM/olla endpoint.
//
// #688: DNS-rebind / SSRF guard parity with internal/embed/ollama.go. The
// configured baseURL host is allow-listed (operator explicitly chose it).
// On every dial we re-resolve the hostname and reject responses that point
// to a private IP unless they resolve to the allow-listed host.
//
// baseURL may be empty (the function then degrades to the basic transport
// without rebind protection — used by tests + the no-probe constructor's
// short-lived calls).
func newLiteLLMHTTPClient(baseURL string) *http.Client {
	var configuredHost string
	if baseURL != "" {
		if u, err := url.Parse(baseURL); err == nil {
			configuredHost = u.Hostname()
		}
	}
	baseDialer := &net.Dialer{
		Timeout:   500 * time.Millisecond,
		KeepAlive: 30 * time.Second,
	}
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     60 * time.Second,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, _, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				// Skip the rebind guard when no baseURL is configured (test paths).
				if configuredHost == "" {
					return baseDialer.DialContext(ctx, network, addr)
				}
				// Re-resolve on every dial to prevent short-TTL rebinding from
				// bypassing the startup IP check.
				addrs, err := net.DefaultResolver.LookupHost(ctx, host)
				if err != nil {
					return nil, fmt.Errorf("DNS resolution failed for %q: %w", host, err)
				}
				// Skip private-IP check for the operator-configured host.
				if host != configuredHost {
					for _, resolved := range addrs {
						if netutil.IsPrivateIP(resolved) {
							return nil, fmt.Errorf("litellm URL resolved to private IP %q (SSRF protection, closes #688)", resolved)
						}
					}
				}
				return baseDialer.DialContext(ctx, network, addr)
			},
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
		http:       newLiteLLMHTTPClient(baseURL),
	}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	vec, err := c.Embed(probeCtx, "probe")
	if err != nil {
		return nil, fmt.Errorf("litellm startup probe: %w", err)
	}
	c.dims.Store(int32(len(vec)))
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
		http:       newLiteLLMHTTPClient(baseURL),
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

// CircuitState returns the current circuit breaker state for this client.
// Returns StateClosed when circuit breaking is disabled (c.cb == nil) so
// callers can safely call String() on the result without a nil check (#926).
func (c *LiteLLMClient) CircuitState() CircuitState {
	if c.cb == nil {
		return StateClosed
	}
	return c.cb.State()
}

// Dimensions returns the known vector size. Before the first successful Embed
// call, falls back to targetDims so the dimension guard in checkEmbedderMeta
// does not falsely report a mismatch on startup.
func (c *LiteLLMClient) Dimensions() int {
	if d := c.dims.Load(); d > 0 {
		return int(d)
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
	vec, _, err := c.EmbedWithModel(ctx, text)
	return vec, err
}

func (c *LiteLLMClient) EmbedWithModel(ctx context.Context, text string) ([]float32, string, error) {
	// Check circuit breaker before attempting request.
	//
	// #1003: record-once guard — guarantees the probe slot granted by Allow()
	// is released on EVERY return path (ctx-cancel, panic, normal exit).
	// Mirrors the guarantee in runProbe.
	//
	// AbandonProbe (neutral release) is used for unresolved slots instead of
	// RecordFailure: a cancelled client request is not evidence the backend is
	// unhealthy. RecordFailure on cancel would spuriously inflate
	// consecutiveOpens and extend backoff during load-shedding / drain events,
	// potentially keeping the breaker stuck Open on a healthy upstream. (#1003)
	cbSlotRecorded := false
	recordCBOutcome := func(success bool) {
		if c.cb == nil || cbSlotRecorded {
			return
		}
		cbSlotRecorded = true
		if success {
			c.cb.RecordSuccess()
		} else {
			c.cb.RecordFailure()
		}
	}
	if c.cb != nil {
		if err := c.cb.Allow(); err != nil {
			metrics.EmbedFailures.WithLabelValues("circuit_open").Inc()
			return nil, "", err
		}
		// Slot granted. Deferred fallback: if neither RecordSuccess nor
		// RecordFailure ran by the time the function returns (e.g. ctx was
		// already cancelled), abandon the slot without penalty. (#1003)
		defer func() {
			if !cbSlotRecorded {
				c.cb.AbandonProbe()
			}
		}()
	}

	text = TruncateToModelWindow(text, ModelMaxTokens(c.model))

	reqBody := map[string]any{
		"model": c.model,
		"input": text,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, "", fmt.Errorf("litellm embed: marshal: %w", err)
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
				return nil, "", fmt.Errorf("litellm embed: context canceled during backoff: %w", ctx.Err())
			}
		}

		// Check if context is still valid before making the request
		if ctx.Err() != nil {
			metrics.EmbedFailures.WithLabelValues("non_retryable").Inc()
			return nil, "", fmt.Errorf("litellm embed: context deadline exceeded before attempt: %w", ctx.Err())
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/embeddings", bytes.NewReader(body))
		if err != nil {
			return nil, "", fmt.Errorf("litellm embed: build request: %w", err)
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
				recordCBOutcome(false)
				return nil, "", fmt.Errorf("litellm embed request: %w", err)
			}
			// Retryable error; log and retry
			if attempt < maxAttempts-1 {
				metrics.EmbedRetries.Inc()
				continue
			}
			// Last attempt failed
			metrics.EmbedFailures.WithLabelValues("exhausted").Inc()
			recordCBOutcome(false)
			return nil, "", fmt.Errorf("litellm embed request (exhausted retries): %w", err)
		}

		lastStatusCode = resp.StatusCode

		if resp.StatusCode != http.StatusOK {
			rb, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			_ = resp.Body.Close() //nolint:errcheck
			statusErr := fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(rb)))

			if !isRetryableError(statusErr, resp.StatusCode) {
				metrics.EmbedFailures.WithLabelValues("non_retryable").Inc()
				recordCBOutcome(false)
				return nil, "", fmt.Errorf("litellm embed: %w", statusErr)
			}
			// Retryable HTTP error; log and retry
			if attempt < maxAttempts-1 {
				metrics.EmbedRetries.Inc()
				continue
			}
			// Last attempt failed
			metrics.EmbedFailures.WithLabelValues("exhausted").Inc()
			recordCBOutcome(false)
			return nil, "", fmt.Errorf("litellm embed: %w (exhausted retries)", statusErr)
		}

		// Success: decode response
		var result struct {
			Model string `json:"model"`
			Data  []struct {
				Embedding []float32 `json:"embedding"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			_ = resp.Body.Close() //nolint:errcheck
			// Decode error is non-retryable
			metrics.EmbedFailures.WithLabelValues("non_retryable").Inc()
			recordCBOutcome(false)
			return nil, "", fmt.Errorf("litellm embed decode: %w", err)
		}
		if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close() //nolint:errcheck
			// Empty response is non-retryable
			metrics.EmbedFailures.WithLabelValues("non_retryable").Inc()
			recordCBOutcome(false)
			return nil, "", fmt.Errorf("litellm embed: empty response")
		}

		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close() //nolint:errcheck
		// Success!
		vec := result.Data[0].Embedding
		if c.targetDims > 0 && len(vec) > c.targetDims {
			// MRL client-side truncation: take first N dims, then L2-normalize
			// so cosine similarity remains meaningful at the reduced dimension.
			vec = L2Normalize(vec[:c.targetDims])
		}
		// CompareAndSwap is more explicit than the check-then-Store idiom: it
		// atomically sets dims to the observed vector size only when dims is still
		// 0, preventing a redundant store on subsequent calls. Functionally
		// equivalent but makes the intent clear.
		c.dims.CompareAndSwap(0, int32(len(vec)))

		// Record success in circuit breaker and return
		recordCBOutcome(true)

		modelID := result.Model
		if modelID == "" {
			modelID = c.model
		}
		return vec, modelID, nil
	}

	// Should not reach here, but as a fallback
	metrics.EmbedFailures.WithLabelValues("exhausted").Inc()
	recordCBOutcome(false)
	return nil, "", fmt.Errorf("litellm embed: exhausted %d attempts, last error: %w (status %d)", maxAttempts, lastErr, lastStatusCode)
}

// Probe checks if the LiteLLM endpoint is healthy and advertising the configured
// embedding model. It uses GET /v1/models instead of a real embeddings request
// so post-store degraded checks do not false-negative on cold but healthy GPU
// backends. The caller is responsible for context timeout/cancellation.
func (c *LiteLLMClient) Probe(ctx context.Context) (ok bool, reason string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", nil)
	if err != nil {
		return false, fmt.Sprintf("embed_probe: build request: %v", err)
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Sprintf("embed_probe: request: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Sprintf("embed_probe: HTTP %d", resp.StatusCode)
	}

	var modelsResp infinityModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return false, fmt.Sprintf("embed_probe: decode /v1/models: %v", err)
	}

	for _, m := range modelsResp.Data {
		if m.ID != c.model {
			continue
		}
		s := m.Stats
		if s.QueueFraction > 1.0 && s.ResultsPending == 0 && s.QueueAbsolute > 0 {
			return false, fmt.Sprintf(
				"embed_probe: GPU thread hung (model=%s queue_fraction=%.4f results_pending=%d queue_absolute=%d)",
				m.ID, s.QueueFraction, s.ResultsPending, s.QueueAbsolute,
			)
		}
		return true, ""
	}

	return false, fmt.Sprintf("embed_probe: model %q not advertised by /v1/models", c.model)
}

// StartBackgroundProbe launches a goroutine that periodically probes the
// LiteLLM endpoint when the circuit breaker is Open and past its nextProbeAt
// cooldown.  This decouples recovery from query load — the probe uses a
// dedicated 5 s timeout instead of the 500 ms recall budget, so a warming
// MI50 GPU can answer the lightweight GET /v1/models check even when it cannot
// serve embeddings within the recall deadline.
//
// The goroutine exits when ctx is cancelled (wire to the server root context
// so it stops on shutdown).  interval controls the ticker period; callers
// should use a value on the order of the OpenDuration config (e.g., 10 s).
//
// No-op when the client has no circuit breaker configured.
func (c *LiteLLMClient) StartBackgroundProbe(ctx context.Context, interval time.Duration) {
	if c.cb == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		const probeTimeout = 5 * time.Second
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Guard shutdown BEFORE claiming a probe slot: if ctx is already
				// cancelled, deriving a probe context from it would fail
				// immediately and record a spurious failure that inflates
				// backoff on the way down. Checking first means we neither claim
				// the slot nor record anything — we just exit. Mirrors the
				// EmbedWithModel ctx guard. #1000 review.
				if ctx.Err() != nil {
					return
				}

				// AllowProbe() makes the whole decision atomically under one
				// lock: it returns nil only when the circuit is degraded (Open
				// past cooldown, or HalfOpen with a free slot) and claims the
				// probe slot in the same critical section. This closes the
				// TOCTOU window a State()+Allow() two-step would open, where a
				// demand-path Allow could interleave and launch a second probe.
				if err := c.cb.AllowProbe(); err != nil {
					// Not degraded, within backoff, or a probe is already in
					// flight. Nothing to do this tick.
					continue
				}

				// runProbe isolates the probe call so a panic can never leave
				// the breaker wedged in HalfOpen with probeInFlight=true: the
				// deferred recover guarantees exactly one of
				// RecordSuccess/RecordFailure runs. #1000 review.
				c.runProbe(ctx, probeTimeout)
			}
		}
	}()
}

// runProbe executes a single background recovery probe and records the result
// on the circuit breaker. It is panic-safe: if c.Probe panics, the deferred
// recover records a failure so the probe slot (probeInFlight) is always
// released and the breaker cannot wedge in HalfOpen.
func (c *LiteLLMClient) runProbe(ctx context.Context, probeTimeout time.Duration) {
	recorded := false
	record := func(ok bool) {
		if recorded {
			return
		}
		recorded = true
		if ok {
			c.cb.RecordSuccess()
		} else {
			c.cb.RecordFailure()
		}
	}
	defer func() {
		if r := recover(); r != nil {
			slog.Error("embed background probe panicked — recording failure", "panic", r)
			record(false)
		}
	}()

	// Run the cheap GET /v1/models probe with a dedicated timeout that is
	// independent of the recall budget. probeFunc defaults to c.Probe; tests
	// override it to drive this real loop deterministically.
	probe := c.probeFunc
	if probe == nil {
		probe = c.Probe
	}
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	// Deferred so the timeout context is released even if record() panics; the
	// recover() defer above still runs (defers are LIFO, both fire). #1000 review.
	defer cancel()
	ok, _ := probe(probeCtx)
	record(ok)
}

// InfinityModelStats holds per-model GPU queue statistics from the Infinity
// /v1/models response. These fields are Infinity-specific and absent from
// standard OpenAI-compatible servers (zero value is safe: QueueAbsolute == 0
// means the hung-state guard never fires on non-Infinity responses).
type InfinityModelStats struct {
	QueueFraction  float64 `json:"queue_fraction"`
	QueueAbsolute  int     `json:"queue_absolute"`
	ResultsPending int     `json:"results_pending"`
	BatchSize      int     `json:"batch_size"`
}

// infinityModelsResponse is the JSON shape of GET /v1/models from Infinity.
type infinityModelsResponse struct {
	Data []struct {
		ID    string             `json:"id"`
		Stats InfinityModelStats `json:"stats"`
	} `json:"data"`
}

// InfinityQueueCheck probes GET /v1/models at baseURL and detects the Infinity
// GPU thread deadlock signature:
//
//	queue_fraction > 1.0  — queue is over capacity
//	results_pending == 0  — GPU inference thread has stopped processing
//	queue_absolute > 0    — work is enqueued (guards against zero-value structs)
//
// Returns (false, reason) when the hung state is detected.
// Returns (true, "") for healthy queues and for non-Infinity servers that return
// a standard OpenAI /v1/models payload without the Infinity "stats" field.
func InfinityQueueCheck(ctx context.Context, httpClient *http.Client, baseURL string) (ok bool, reason string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/v1/models", nil)
	if err != nil {
		return false, fmt.Sprintf("infinity_queue_check: build request: %v", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, fmt.Sprintf("infinity_queue_check: request: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Sprintf("infinity_queue_check: HTTP %d", resp.StatusCode)
	}

	var modelsResp infinityModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		// Non-Infinity server or unexpected body — treat as healthy.
		return true, ""
	}

	for _, m := range modelsResp.Data {
		s := m.Stats
		// GPU hang: ResultsPending == 0 means the GPU has stopped processing entirely
		// (queue drained with no output). High-load cases where ResultsPending > 0
		// are intentionally NOT flagged — backpressure without stall is normal.
		// See TestInfinityQueueCheck_HighLoadNotHung (litellm_test.go).
		if s.QueueFraction > 1.0 && s.ResultsPending == 0 && s.QueueAbsolute > 0 {
			return false, fmt.Sprintf(
				"infinity_queue_check: GPU thread hung (model=%s queue_fraction=%.4f results_pending=%d queue_absolute=%d)",
				m.ID, s.QueueFraction, s.ResultsPending, s.QueueAbsolute,
			)
		}
	}

	return true, ""
}
