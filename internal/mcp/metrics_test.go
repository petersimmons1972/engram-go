package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestMetricsRequiresBearerAuth verifies that /metrics is protected by bearer
// authentication and is not accessible without valid credentials (#552).
func TestMetricsRequiresBearerAuth(t *testing.T) {
	s := &Server{
		trustProxy: false,
	}

	apiKey := "test-secret-key-123"
	setupLimiter := newRateLimiter(context.Background())

	mux := http.NewServeMux()

	// Register /metrics with auth middleware (FIX for #552).
	mux.Handle("/metrics", s.applyMiddlewareWithRL(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; version=0.0.4")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("# HELP test_metric Test metric\n"))
		}),
		apiKey,
		setupLimiter,
	))

	tests := []struct {
		name           string
		authorization  string
		expectedStatus int
		description    string
	}{
		{
			name:           "no auth",
			authorization:  "",
			expectedStatus: http.StatusUnauthorized,
			description:    "/metrics without bearer token must be rejected (#552)",
		},
		{
			name:           "invalid bearer",
			authorization:  "Bearer wrong-key",
			expectedStatus: http.StatusUnauthorized,
			description:    "invalid bearer tokens must be rejected",
		},
		{
			name:           "valid bearer",
			authorization:  "Bearer " + apiKey,
			expectedStatus: http.StatusOK,
			description:    "valid bearer tokens must be accepted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("%s: expected status %d, got %d", tt.description, tt.expectedStatus, w.Code)
			}
		})
	}
}
