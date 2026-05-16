package mcp

// Tests for the /health endpoint (F1, closes #274) and request correlation
// helpers (F6, closes #320). All tests use httptest stubs — no real database
// or Ollama instance required.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// ---------------------------------------------------------------------------
// F1: /health endpoint tests (#274)
// ---------------------------------------------------------------------------

// healthResponse is the shape returned by handleHealth.
type healthResponse struct {
	Status   string `json:"status"`
	Postgres string `json:"postgres"`
	Ollama   string `json:"ollama"`
}

// newHealthServer builds a Server configured with a fake Ollama URL pointing
// to srv. PgPool is nil (no real DB), so the Postgres probe is skipped (ok).
func newHealthServer(ollamaURL string) *Server {
	return &Server{
		cfg: Config{
			LiteLLMURL: ollamaURL,
		},
		embedDegraded: &atomic.Bool{},
	}
}

// TestHealth_BothDepsUp verifies that when both probes succeed the handler
// returns 200 with {"status":"ok","postgres":"ok","ollama":"ok"}.
// Postgres: PgPool is nil — the handler skips the probe and reports ok.
// Ollama: a test HTTP server returns 200 HEAD /api/tags.
func TestHealth_BothDepsUp(t *testing.T) {
	// Fake Infinity server: responds to GET /v1/models with healthy queue stats.
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"data": []map[string]any{{
					"id": "BAAI/bge-m3",
					"stats": map[string]any{
						"queue_fraction":  0.10,
						"queue_absolute":  6,
						"results_pending": 2,
						"batch_size":      64,
					},
				}},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ollamaServer.Close()

	s := newHealthServer(ollamaServer.URL)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	s.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp healthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("status: expected %q, got %q", "ok", resp.Status)
	}
	if resp.Postgres != "ok" {
		t.Errorf("postgres: expected %q, got %q", "ok", resp.Postgres)
	}
	if resp.Ollama != "ok" {
		t.Errorf("ollama: expected %q, got %q", "ok", resp.Ollama)
	}
}

// TestHealth_OllamaDown verifies that when Ollama is unreachable the handler
// returns 503 with {"status":"degraded","postgres":"ok","ollama":"error"}.
func TestHealth_OllamaDown(t *testing.T) {
	// Point at a URL that will immediately refuse connections.
	s := newHealthServer("http://127.0.0.1:1") // port 1 will be refused

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	s.handleHealth(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp healthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "degraded" {
		t.Errorf("status: expected %q, got %q", "degraded", resp.Status)
	}
	if resp.Ollama != "error" {
		t.Errorf("ollama: expected %q, got %q", "error", resp.Ollama)
	}
	if resp.Postgres != "ok" {
		t.Errorf("postgres: expected %q, got %q", "ok", resp.Postgres)
	}
}

// TestHealth_OllamaReturns500 verifies that a 5xx response from Ollama
// is treated as a failure (degraded).
func TestHealth_OllamaReturns500(t *testing.T) {
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ollamaServer.Close()

	s := newHealthServer(ollamaServer.URL)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	s.handleHealth(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	var resp healthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Ollama != "error" {
		t.Errorf("ollama should be error on 500, got %q", resp.Ollama)
	}
}

// TestHealth_NoOllamaURLConfigured verifies that when no Ollama URL is set
// the handler does not panic and reports both deps ok.
func TestHealth_NoOllamaURLConfigured(t *testing.T) {
	s := newHealthServer("")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	s.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// F6: request correlation context key helper tests (#320)
// ---------------------------------------------------------------------------

// TestRequestIDFromContext_Present verifies the helper extracts a stored ID.
func TestRequestIDFromContext_Present(t *testing.T) {
	ctx := context.WithValue(context.Background(), requestIDKey, "abc123")
	if id := requestIDFromContext(ctx); id != "abc123" {
		t.Errorf("expected %q, got %q", "abc123", id)
	}
}

// TestRequestIDFromContext_Absent verifies the helper returns empty string
// when no ID has been stored.
func TestRequestIDFromContext_Absent(t *testing.T) {
	if id := requestIDFromContext(context.Background()); id != "" {
		t.Errorf("expected empty string, got %q", id)
	}
}

// TestApplyMiddleware_SetsRequestID verifies that applyMiddleware injects a
// non-empty request ID into the request context for authenticated requests.
func TestApplyMiddleware_SetsRequestID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Server{}
	rl := newRateLimiter(ctx)
	apiKey := "test-key"

	var capturedID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = requestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := s.applyMiddleware(inner, apiKey, rl)
	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if capturedID == "" {
		t.Error("expected a non-empty request_id in context, got empty string")
	}
	if len(capturedID) < 8 {
		t.Errorf("request_id seems too short: %q", capturedID)
	}
}

// ---------------------------------------------------------------------------
// Infinity hung-state detection tests (#649)
// ---------------------------------------------------------------------------

// TestHealth_InfinityHung verifies that when /v1/models reports the GPU thread
// deadlock signature, handleHealth returns 503 with ollama="error".
func TestHealth_InfinityHung(t *testing.T) {
	infinityServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"data": []map[string]any{{
					"id": "BAAI/bge-m3",
					"stats": map[string]any{
						"queue_fraction":  1.00059375,
						"queue_absolute":  32019,
						"results_pending": 0,
						"batch_size":      64,
					},
				}},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer infinityServer.Close()

	s := newHealthServer(infinityServer.URL)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	s.handleHealth(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for hung Infinity, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp healthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Ollama != "error" {
		t.Errorf("expected ollama=error for hung GPU thread, got %q", resp.Ollama)
	}
}

// TestHealth_InfinityHealthyStats verifies that a healthy Infinity queue
// (queue_fraction < 1.0, results_pending > 0) returns 200 with ollama="ok".
func TestHealth_InfinityHealthyStats(t *testing.T) {
	infinityServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"data": []map[string]any{{
					"id": "BAAI/bge-m3",
					"stats": map[string]any{
						"queue_fraction":  0.42,
						"queue_absolute":  27,
						"results_pending": 5,
						"batch_size":      64,
					},
				}},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer infinityServer.Close()

	s := newHealthServer(infinityServer.URL)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	s.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for healthy Infinity, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp healthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Ollama != "ok" {
		t.Errorf("expected ollama=ok for healthy queue, got %q", resp.Ollama)
	}
}
