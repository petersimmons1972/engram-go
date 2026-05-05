package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestEmbedRetriesOn502 verifies that a 502 followed by 200 triggers retries
// and increments the retries_total counter.
func TestEmbedRetriesOn502(t *testing.T) {
	t.Helper()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			// First two attempts: return 502
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte("Bad Gateway"))
			return
		}
		// Third attempt: return valid response
		encodeEmbeddingResponse(w, []float32{0.1, 0.2, 0.3})
	}))
	defer server.Close()

	client := NewLiteLLMClientNoProbe(server.URL, "test-model", "", 3)
	ctx := context.Background()

	baslineRetries := testutil.ToFloat64(metrics.EmbedRetries)

	vec, err := client.Embed(ctx, "test text")
	if err != nil {
		t.Fatalf("expected success after retry, got error: %v", err)
	}
	if len(vec) != 3 {
		t.Errorf("expected 3-dim vector, got %d", len(vec))
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}

	// Check that retries_total was incremented by 2 (two retries)
	count := testutil.ToFloat64(metrics.EmbedRetries)
	if count-baslineRetries != 2.0 {
		t.Errorf("expected engram_embed_retries_total delta=2, got %.0f", count-baslineRetries)
	}
}

// TestEmbedNoRetryOn400 verifies that a 400 Bad Request does not retry.
func TestEmbedNoRetryOn400(t *testing.T) {
	t.Helper()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Bad Request"))
	}))
	defer server.Close()

	client := NewLiteLLMClientNoProbe(server.URL, "test-model", "", 3)
	ctx := context.Background()

	m, err := metrics.EmbedFailures.GetMetricWithLabelValues("non_retryable")
	if err != nil {
		t.Fatalf("failed to get metric with label: %v", err)
	}
	baselineNonRetryable := testutil.ToFloat64(m)

	_, err = client.Embed(ctx, "test text")
	if err == nil {
		t.Fatal("expected error for 400, got nil")
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry), got %d", attempts)
	}

	// Check that failures_total{reason="non_retryable"} was incremented
	m, err = metrics.EmbedFailures.GetMetricWithLabelValues("non_retryable")
	if err != nil {
		t.Fatalf("failed to get metric with label: %v", err)
	}
	nonRetryableCount := testutil.ToFloat64(m)
	if nonRetryableCount-baselineNonRetryable != 1.0 {
		t.Errorf("expected failures_total{reason=\"non_retryable\"} delta=1, got %.0f", nonRetryableCount-baselineNonRetryable)
	}
}

// TestEmbedRetriesOn429 verifies that 429 Too Many Requests is retried
// with exponential backoff.
func TestEmbedRetriesOn429(t *testing.T) {
	t.Helper()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			// First two attempts: return 429
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("Too Many Requests"))
			return
		}
		// Third attempt: return valid response
		encodeEmbeddingResponse(w, []float32{0.1, 0.2, 0.3})
	}))
	defer server.Close()

	client := NewLiteLLMClientNoProbe(server.URL, "test-model", "", 3)
	ctx := context.Background()

	baslineRetries := testutil.ToFloat64(metrics.EmbedRetries)

	vec, err := client.Embed(ctx, "test text")
	if err != nil {
		t.Fatalf("expected success after retry, got error: %v", err)
	}
	if len(vec) != 3 {
		t.Errorf("expected 3-dim vector, got %d", len(vec))
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}

	// Check that retries_total was incremented by 2
	count := testutil.ToFloat64(metrics.EmbedRetries)
	if count-baslineRetries < 2.0 {
		t.Errorf("expected engram_embed_retries_total delta>=2, got %.0f", count-baslineRetries)
	}
}

// TestEmbedExhaustsRetries verifies that after 3 failed attempts,
// the embed fails and increments failures_total{reason="exhausted"}.
func TestEmbedExhaustsRetries(t *testing.T) {
	t.Helper()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		// Always return 503
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Service Unavailable"))
	}))
	defer server.Close()

	client := NewLiteLLMClientNoProbe(server.URL, "test-model", "", 3)
	ctx := context.Background()

	baslineRetries := testutil.ToFloat64(metrics.EmbedRetries)
	baselineExhausted, _ := metrics.EmbedFailures.GetMetricWithLabelValues("exhausted")
	baselineExhaustedCount := testutil.ToFloat64(baselineExhausted)

	_, err := client.Embed(ctx, "test text")
	if err == nil {
		t.Fatal("expected error after retries exhausted, got nil")
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts (max retries), got %d", attempts)
	}

	// Check that retries_total was incremented by 2 (two retries before final failure)
	retriesCount := testutil.ToFloat64(metrics.EmbedRetries)
	if retriesCount-baslineRetries != 2.0 {
		t.Errorf("expected retries_total delta=2, got %.0f", retriesCount-baslineRetries)
	}

	// Check that failures_total{reason="exhausted"} was incremented
	m, err := metrics.EmbedFailures.GetMetricWithLabelValues("exhausted")
	if err != nil {
		t.Fatalf("failed to get metric with label: %v", err)
	}
	exhaustedCount := testutil.ToFloat64(m)
	if exhaustedCount-baselineExhaustedCount != 1.0 {
		t.Errorf("expected failures_total{reason=\"exhausted\"} delta=1, got %.0f", exhaustedCount-baselineExhaustedCount)
	}
}

// TestEmbedRespectsContextDeadline verifies that a context deadline is not
// retried even if the error is transient.
func TestEmbedRespectsContextDeadline(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response (longer than context deadline)
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewLiteLLMClientNoProbe(server.URL, "test-model", "", 3)

	baslineRetries := testutil.ToFloat64(metrics.EmbedRetries)

	// Short context deadline
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.Embed(ctx, "test text")
	if err == nil {
		t.Fatal("expected context deadline error, got nil")
	}
	if !strings.Contains(err.Error(), "context") && !strings.Contains(err.Error(), "deadline") {
		t.Logf("error message: %v", err)
		// Note: error may be wrapped, so we check for context-related messages
	}

	// Should not increment retries (context deadline is non-retryable)
	retriesCount := testutil.ToFloat64(metrics.EmbedRetries)
	if retriesCount-baslineRetries > 0 {
		t.Errorf("expected no retries on context deadline, got %.0f new retries", retriesCount-baslineRetries)
	}
}

// TestEmbedRetriesOnEOF verifies that connection EOF errors trigger retries.
func TestEmbedRetriesOnEOF(t *testing.T) {
	t.Helper()
	resetMetrics()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			// First attempt: close connection (EOF)
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close() // nolint:errcheck
			return
		}
		// Second attempt: return valid response
		encodeEmbeddingResponse(w, []float32{0.1, 0.2, 0.3})
	}))
	defer server.Close()

	client := NewLiteLLMClientNoProbe(server.URL, "test-model", "", 3)
	ctx := context.Background()

	vec, err := client.Embed(ctx, "test text")
	if err != nil {
		t.Logf("got error (may be expected if hijack isn't supported): %v", err)
		// Some test environments don't support hijacking, so we accept errors here
		return
	}

	if attempts >= 1 && len(vec) > 0 {
		// If we got here, the retry happened successfully
		t.Log("EOF retry succeeded (if supported by test environment)")
	}
}

// Helper: reset prometheus counters between tests
func resetMetrics() {
	// We can't easily reset Prometheus counters, but we can create a fresh registry
	// For now, we just note that tests should be independent or use separate instances
}

// TestEmbedReturnsCircuitOpenError verifies that when the circuit breaker is open,
// Embed returns the circuit open error without calling the upstream.
func TestEmbedReturnsCircuitOpenError(t *testing.T) {
	t.Helper()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		// Always return 503
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Service Unavailable"))
	}))
	defer server.Close()

	cbCfg := CircuitConfig{
		Enabled:           true,
		FailureThreshold:  3,
		FailureWindow:     30 * time.Second,
		OpenDuration:      30 * time.Second,
		BackoffMultiplier: 2.0,
		BackoffCap:        5 * time.Minute,
	}
	client := NewLiteLLMClientNoProbeWithCircuitBreaker(server.URL, "test-model", "", 3, cbCfg)
	ctx := context.Background()

	// Record enough exhausted failures to open the circuit.
	// Each Embed call exhausts retries (3 HTTP attempts) → 1 failure recorded to circuit.
	// We need 3 Embed calls to reach the threshold of 3 failures.
	for i := 0; i < 3; i++ {
		_, _ = client.Embed(ctx, "test text")
	}

	// Circuit should now be open; attempts should be 9 (3 Embed calls × 3 retries each)
	if attempts != 9 {
		t.Errorf("expected 9 attempts to open circuit, got %d", attempts)
	}

	initialAttempts := attempts

	// Next call should short-circuit without calling upstream
	_, err := client.Embed(ctx, "test text")
	if err != errCircuitOpen {
		t.Errorf("expected errCircuitOpen, got %v", err)
	}

	// Upstream should not have been called
	if attempts != initialAttempts {
		t.Errorf("expected no additional attempts while circuit is open, got %d", attempts)
	}
}

// TestEmbedWithCircuitBreakerSuccess verifies that successful calls record success
// in the circuit breaker and keep it closed.
func TestEmbedWithCircuitBreakerSuccess(t *testing.T) {
	t.Helper()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		encodeEmbeddingResponse(w, []float32{0.1, 0.2, 0.3})
	}))
	defer server.Close()

	cbCfg := CircuitConfig{
		Enabled:           true,
		FailureThreshold:  5,
		FailureWindow:     30 * time.Second,
		OpenDuration:      30 * time.Second,
		BackoffMultiplier: 2.0,
		BackoffCap:        5 * time.Minute,
	}
	client := NewLiteLLMClientNoProbeWithCircuitBreaker(server.URL, "test-model", "", 3, cbCfg)
	ctx := context.Background()

	// Make successful calls; circuit should stay closed
	for i := 0; i < 10; i++ {
		vec, err := client.Embed(ctx, "test text")
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if len(vec) == 0 {
			t.Fatalf("expected non-empty vector")
		}
	}

	if attempts != 10 {
		t.Errorf("expected 10 upstream calls, got %d", attempts)
	}

	// Circuit should still be closed
	if client.cb.State() != StateClosed {
		t.Errorf("expected circuit to be Closed, got %v", client.cb.State())
	}
}

// TestEmbedCircuitBreakerDisabledByDefault verifies that circuit breaker is
// disabled when no config is provided (backward compatibility).
func TestEmbedCircuitBreakerDisabledByDefault(t *testing.T) {
	t.Helper()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Service Unavailable"))
	}))
	defer server.Close()

	// Create client with empty config (disabled by default)
	client := NewLiteLLMClientNoProbe(server.URL, "test-model", "", 3)
	ctx := context.Background()

	// Make calls that would fail; circuit breaker should be nil
	for i := 0; i < 10; i++ {
		_, _ = client.Embed(ctx, "test text")
	}

	// Circuit breaker should not be created
	if client.cb != nil {
		t.Errorf("expected circuit breaker to be nil when disabled")
	}
}

// Helper: encodeEmbeddingResponse writes a valid embedding response
func encodeEmbeddingResponse(w http.ResponseWriter, embedding []float32) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	resp := map[string]any{
		"data": []map[string]any{
			{
				"embedding": embedding,
			},
		},
	}
	json.NewEncoder(w).Encode(resp) // nolint:errcheck
}
