package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestRequestIDFromHeader verifies that incoming X-Request-Id headers are
// extracted and echoed in the response (#555).
func TestRequestIDFromHeader(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := &Server{
		sessionTouchTimes: make(map[string]time.Time),
		embedDegraded:     &atomic.Bool{},
	}

	incomingID := "custom-request-id-123"
	handler := server.applyMiddlewareWithRL(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify the request ID is in the context
			reqID := requestIDFromContext(r.Context())
			if reqID != incomingID {
				http.Error(w, "request id mismatch in context", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		}),
		"test-api-key",
		newRateLimiterWithConfig(ctx, 50, 200),
	)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer test-api-key")
	req.Header.Set("X-Request-Id", incomingID)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Verify the request ID is echoed in the response
	responseID := w.Header().Get("X-Request-Id")
	if responseID != incomingID {
		t.Errorf("expected X-Request-Id %q in response, got %q", incomingID, responseID)
	}
}

// TestRequestIDFromTraceparent verifies that traceparent headers are parsed
// and a trace_id is extracted as the correlation ID (#555).
func TestRequestIDFromTraceparent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := &Server{
		sessionTouchTimes: make(map[string]time.Time),
		embedDegraded:     &atomic.Bool{},
	}

	// traceparent format: version-trace_id-parent_id-trace_flags
	traceID := "0af7651916cd43dd8448eb211c80319c"
	traceparent := "00-" + traceID + "-b7ad6b7169203331-01"
	expectedID := traceID[:12] // "0af7651916cd"

	handler := server.applyMiddlewareWithRL(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := requestIDFromContext(r.Context())
			if reqID != expectedID {
				http.Error(w, "expected trace_id from traceparent", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		}),
		"test-api-key",
		newRateLimiterWithConfig(ctx, 50, 200),
	)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer test-api-key")
	req.Header.Set("traceparent", traceparent)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	responseID := w.Header().Get("X-Request-Id")
	if responseID != expectedID {
		t.Errorf("expected X-Request-Id %q from traceparent, got %q", expectedID, responseID)
	}
}

// TestRequestIDGenerated verifies that when no X-Request-Id or traceparent
// header is provided, a request ID is generated and echoed in the response.
func TestRequestIDGenerated(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := &Server{
		sessionTouchTimes: make(map[string]time.Time),
		embedDegraded:     &atomic.Bool{},
	}

	handler := server.applyMiddlewareWithRL(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := requestIDFromContext(r.Context())
			if reqID == "" {
				http.Error(w, "no request id in context", http.StatusInternalServerError)
				return
			}
			// Should be a 12-char hex string
			if len(reqID) != 12 {
				http.Error(w, "request id wrong length", http.StatusInternalServerError)
				return
			}
			for _, c := range reqID {
				if !strings.ContainsRune("0123456789abcdef", c) {
					http.Error(w, "request id not hex", http.StatusInternalServerError)
					return
				}
			}
			w.WriteHeader(http.StatusOK)
		}),
		"test-api-key",
		newRateLimiterWithConfig(ctx, 50, 200),
	)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer test-api-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	responseID := w.Header().Get("X-Request-Id")
	if responseID == "" {
		t.Errorf("expected X-Request-Id in response")
	}
}
