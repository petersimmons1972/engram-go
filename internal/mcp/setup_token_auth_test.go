package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSetupTokenRequiresBearerAuth_B540 verifies that /setup-token requires
// a valid bearer token and rejects unauthenticated requests (#540 B1).
// This test expects the FIX to be in place: /setup-token protected by bearer auth.
func TestSetupTokenRequiresBearerAuth_B540(t *testing.T) {
	s := &Server{
		trustProxy: false,
	}

	setupLimiter := newRateLimiter(context.Background())
	apiKey := "test-secret-key-123"

	mux := http.NewServeMux()

	// Register /setup-token with auth middleware (FIX for #540).
	// The fixed version applies bearer auth to /setup-token.
	mux.Handle("/setup-token", s.applyMiddlewareWithRL(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !setupLimiter.allowSetupToken(s.clientIP(r)) {
				w.Header().Set("Retry-After", "100")
				http.Error(w, "rate limited", http.StatusTooManyRequests)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{
				"token":    apiKey,
				"endpoint": "http://localhost:8788/sse",
				"name":     "engram",
			})
		}),
		apiKey,
		setupLimiter,
	))

	tests := []struct {
		name           string
		remoteAddr     string
		authorization  string
		expectedStatus int
		description    string
	}{
		{
			name:           "loopback IP no auth",
			remoteAddr:     "127.0.0.1:12345",
			authorization:  "",
			expectedStatus: http.StatusUnauthorized,
			description:    "loopback IPs without bearer token must now be rejected (#540)",
		},
		{
			name:           "loopback IP invalid bearer",
			remoteAddr:     "127.0.0.1:12345",
			authorization:  "Bearer wrong-key",
			expectedStatus: http.StatusUnauthorized,
			description:    "invalid bearer tokens must be rejected",
		},
		{
			name:           "loopback IP valid bearer",
			remoteAddr:     "127.0.0.1:12345",
			authorization:  "Bearer " + apiKey,
			expectedStatus: http.StatusOK,
			description:    "valid bearer tokens must be accepted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/setup-token", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("%s: expected status %d, got %d", tt.description, tt.expectedStatus, w.Code)
			}

			// For 200 responses, verify the response body is valid JSON with token.
			if w.Code == http.StatusOK {
				var resp map[string]string
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Errorf("response body is not valid JSON: %v", err)
				}
				if resp["token"] != apiKey {
					t.Errorf("expected token=%q, got %q", apiKey, resp["token"])
				}
			}
		})
	}
}

// TestSetupTokenRejectsX_Real_IPSpoofingWithoutAuth_S551 verifies that spoofing
// X-Real-IP without valid bearer auth is rejected (#551 regression test).
func TestSetupTokenRejectsX_Real_IPSpoofingWithoutAuth_S551(t *testing.T) {
	s := &Server{
		trustProxy: true, // Trust proxy headers
	}

	apiKey := "test-secret-key-123"
	setupLimiter := newRateLimiter(context.Background())

	mux := http.NewServeMux()
	mux.Handle("/setup-token", s.applyMiddlewareWithRL(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !setupLimiter.allowSetupToken(s.clientIP(r)) {
				w.Header().Set("Retry-After", "100")
				http.Error(w, "rate limited", http.StatusTooManyRequests)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{
				"token":    apiKey,
				"endpoint": "http://localhost:8788/sse",
				"name":     "engram",
			})
		}),
		apiKey,
		setupLimiter,
	))

	// Even with spoofed X-Real-IP claiming loopback, requests without
	// valid bearer auth must be rejected.
	req := httptest.NewRequest(http.MethodGet, "/setup-token", nil)
	req.Header.Set("X-Real-IP", "127.0.0.1")
	// Deliberately no Authorization header.
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d (spoofed X-Real-IP without bearer auth), got %d",
			http.StatusUnauthorized, w.Code)
	}
}
