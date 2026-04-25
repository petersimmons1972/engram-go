package mcp

// Tests for the /health endpoint (F1, closes #274) and request correlation
// helpers (F6, closes #320). All tests use httptest stubs — no real database
// or Ollama instance required.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
			OllamaURL: ollamaURL,
		},
	}
}

// TestHealth_BothDepsUp verifies that when both probes succeed the handler
// returns 200 with {"status":"ok","postgres":"ok","ollama":"ok"}.
// Postgres: PgPool is nil — the handler skips the probe and reports ok.
// Ollama: a test HTTP server returns 200 HEAD /api/tags.
func TestHealth_BothDepsUp(t *testing.T) {
	// Fake Ollama that accepts HEAD /api/tags.
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && strings.HasSuffix(r.URL.Path, "/api/tags") {
			w.WriteHeader(http.StatusOK)
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
