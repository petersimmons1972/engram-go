//go:build ignore
// Remove the ignore tag when Pillar 3D (TOFU / setupTokenTOFUHandler) is implemented.

package mcp

// Tests for TOFU (Trust On First Use) behavior on /setup-token — Pillar 3D.
//
// Currently /setup-token requires Bearer authentication (post-#540). The TOFU
// feature allows exactly ONE unauthenticated request from localhost, so that
// engram-session-end.sh and fresh installations can self-configure without a
// chicken-and-egg key problem.
//
// These tests reference Server.tofuGranted (atomic.Bool) and
// Server.setupTokenTOFUHandler / Server.setupTokenTOFUHandlerWithLimiter
// which do NOT exist yet. Tests will FAIL TO COMPILE until the implementation
// is added. That is the expected red-phase state.

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// newTOFUTestServer returns a minimal Server with tofuGranted initialized to false.
// Will FAIL TO COMPILE until Server gains the tofuGranted field.
func newTOFUTestServer(t *testing.T, apiKey string) *Server {
	t.Helper()
	pool := newTestNoopPool(t)
	cfg := testConfig()
	s := NewServer(pool, cfg)
	s.tofuGranted.Store(false) // field doesn't exist yet — COMPILE FAIL (red phase)
	return s
}

// buildSetupTokenTOFUHandler returns the TOFU-aware handler for /setup-token.
// Will FAIL TO COMPILE until Server.setupTokenTOFUHandler is implemented.
func buildSetupTokenTOFUHandler(s *Server, apiKey string) http.Handler {
	return s.setupTokenTOFUHandler(apiKey) // method doesn't exist yet
}

// TestSetupToken_TOFUFirstLocalhostSucceeds verifies that the very first
// unauthenticated GET from 127.0.0.1 succeeds (TOFU grant) and returns the token.
func TestSetupToken_TOFUFirstLocalhostSucceeds(t *testing.T) {
	const apiKey = "test-api-key-tofu"
	s := newTOFUTestServer(t, apiKey)
	handler := buildSetupTokenTOFUHandler(s, apiKey)

	require.False(t, s.tofuGranted.Load(), "tofuGranted must start as false")

	req := httptest.NewRequest(http.MethodGet, "/setup-token", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code,
		"first unauthenticated localhost request must return 200")
	require.Contains(t, w.Body.String(), apiKey,
		"response body must contain the API key")
	require.True(t, s.tofuGranted.Load(),
		"tofuGranted must be true after first successful TOFU grant")
}

// TestSetupToken_TOFUSecondLocalhostFails verifies that after the first TOFU
// grant, a second unauthenticated request returns 401.
func TestSetupToken_TOFUSecondLocalhostFails(t *testing.T) {
	const apiKey = "test-api-key-tofu2"
	s := newTOFUTestServer(t, apiKey)
	handler := buildSetupTokenTOFUHandler(s, apiKey)

	// First request — TOFU grant.
	req1 := httptest.NewRequest(http.MethodGet, "/setup-token", nil)
	req1.RemoteAddr = "127.0.0.1:11111"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	require.Equal(t, http.StatusOK, w1.Code, "first TOFU request must succeed")

	// Second unauthenticated request — must be rejected.
	req2 := httptest.NewRequest(http.MethodGet, "/setup-token", nil)
	req2.RemoteAddr = "127.0.0.1:22222"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusUnauthorized, w2.Code,
		"second unauthenticated request must return 401 (TOFU already consumed)")
}

// TestSetupToken_TOFURemoteIPFails verifies that an unauthenticated request from
// a non-loopback address is always rejected — TOFU is localhost-only.
func TestSetupToken_TOFURemoteIPFails(t *testing.T) {
	const apiKey = "test-api-key-remote"
	s := newTOFUTestServer(t, apiKey)
	handler := buildSetupTokenTOFUHandler(s, apiKey)

	req := httptest.NewRequest(http.MethodGet, "/setup-token", nil)
	req.RemoteAddr = "10.0.0.42:9999" // non-loopback
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code,
		"unauthenticated non-localhost request must return 401")
	require.False(t, s.tofuGranted.Load(),
		"tofuGranted must remain false after remote IP rejection")
}

// TestSetupToken_AuthedRequestAlwaysSucceeds verifies that an authenticated
// Bearer request succeeds even after the TOFU budget is exhausted.
func TestSetupToken_AuthedRequestAlwaysSucceeds(t *testing.T) {
	const apiKey = "test-api-key-authed"
	s := newTOFUTestServer(t, apiKey)
	// Mark TOFU as already consumed.
	s.tofuGranted.Store(true)

	handler := buildSetupTokenTOFUHandler(s, apiKey)

	req := httptest.NewRequest(http.MethodGet, "/setup-token", nil)
	req.RemoteAddr = "127.0.0.1:33333"
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code,
		"authenticated Bearer request must always return 200, even after TOFU exhausted")
}

// TestSetupToken_TOFUConcurrentRequests verifies that under concurrent unauthenticated
// requests, exactly one succeeds (TOFU atomicity via CompareAndSwap).
func TestSetupToken_TOFUConcurrentRequests(t *testing.T) {
	const apiKey = "test-api-key-concurrent"
	s := newTOFUTestServer(t, apiKey)
	handler := buildSetupTokenTOFUHandler(s, apiKey)

	const goroutines = 20
	results := make([]int, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/setup-token", nil)
			req.RemoteAddr = "127.0.0.1:40000"
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			results[i] = w.Code
		}()
	}
	wg.Wait()

	successes := 0
	for _, code := range results {
		if code == http.StatusOK {
			successes++
		}
	}
	require.Equal(t, 1, successes,
		"exactly one concurrent unauthenticated TOFU request must succeed; got %d", successes)
}

// TestSetupToken_TOFURateLimitRunsFirst verifies that rate-limit enforcement (429)
// takes priority over TOFU grant when the setupLimiter budget is exhausted.
func TestSetupToken_TOFURateLimitRunsFirst(t *testing.T) {
	const apiKey = "test-api-key-ratelimit"
	s := newTOFUTestServer(t, apiKey)

	// Build a rate limiter and exhaust the setup-token budget.
	rl := newRateLimiter(t.Context())
	const ip = "127.0.0.1"
	for rl.allowSetupToken(ip) {
		// Consume all tokens.
	}

	// Build the handler using the test-only variant that accepts an external limiter.
	// setupTokenTOFUHandlerWithLimiter doesn't exist yet — COMPILE FAIL (red phase).
	handler := s.setupTokenTOFUHandlerWithLimiter(apiKey, rl)

	req := httptest.NewRequest(http.MethodGet, "/setup-token", nil)
	req.RemoteAddr = ip + ":55555"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusTooManyRequests, w.Code,
		"exhausted rate limiter must return 429 before TOFU logic runs")
}
