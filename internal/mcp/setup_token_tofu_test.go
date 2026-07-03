package mcp

// Tests for TOFU (Trust On First Use) behavior on /setup-token — Pillar 3D.
//
// /setup-token allows exactly ONE unauthenticated request from localhost, so
// that engram-session-end.sh and fresh installations can self-configure without
// a chicken-and-egg key problem (#613). All subsequent requests require Bearer
// authentication (unchanged from #540).

import (
	"context"
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
func buildSetupTokenTOFUHandler(s *Server, apiKey string) http.Handler {
	// Tests that use this helper don't check the endpoint URL, so advertised is "".
	return s.setupTokenTOFUHandler(context.Background(), apiKey, "")
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

// TestSetupToken_XFFBypassBlocked verifies that a remote caller cannot bypass the
// /setup-token rate limiter by rotating the X-Forwarded-For header.  Pre-fix,
// s.clientIP(r) keyed the rate-limit bucket, so each new XFF value minted a fresh
// bucket; post-fix the bucket key is the physical r.RemoteAddr. (#1209)
func TestSetupToken_XFFBypassBlocked(t *testing.T) {
	const apiKey = "test-api-key-xff-bypass"
	s := newTOFUTestServer(t, apiKey)
	s.trustProxy = true // honour X-Forwarded-For so clientIP() diverges from RemoteAddr

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Build a limiter and exhaust the setup-token budget for the physical peer "10.0.0.1".
	// allowSetupToken burst is 3 (see server.go — rate.Every(setupTokenWindow), burst 3).
	rl := newRateLimiter(ctx)
	rl.allowSetupToken("10.0.0.1") // #1
	rl.allowSetupToken("10.0.0.1") // #2
	rl.allowSetupToken("10.0.0.1") // #3 — budget exhausted
	if rl.allowSetupToken("10.0.0.1") {
		t.Fatal("test setup: expected budget to be exhausted after 3 calls but still has tokens")
	}

	handler := s.setupTokenTOFUHandlerWithLimiter(apiKey, "", rl)

	// Attacker is physically at 10.0.0.1 but rotates X-Forwarded-For to "5.5.5.5".
	// Pre-fix: clientIP(r)="5.5.5.5" → new bucket → not rate-limited → TOFU/auth logic runs.
	// Post-fix: physical peer="10.0.0.1" → exhausted bucket → 429.
	req := httptest.NewRequest(http.MethodGet, "/setup-token", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "5.5.5.5") // spoofed IP — must not mint a new RL bucket
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusTooManyRequests, w.Code,
		"XFF rotation must not bypass /setup-token rate limit: physical peer 10.0.0.1 is exhausted (#1209)")
}

// TestSetupToken_TOFURateLimitRunsFirst verifies that rate-limit enforcement (429)
// takes priority over TOFU grant when the setupLimiter budget is exhausted.
func TestSetupToken_TOFURateLimitRunsFirst(t *testing.T) {
	const apiKey = "test-api-key-ratelimit"
	s := newTOFUTestServer(t, apiKey)

	// Build a rate limiter and exhaust the setup-token budget.
	rl := newRateLimiter(context.Background())
	const ip = "127.0.0.1"
	for rl.allowSetupToken(ip) {
		// Consume all tokens.
	}

	// Build the handler using the test-only variant that accepts an external limiter.
	handler := s.setupTokenTOFUHandlerWithLimiter(apiKey, "", rl)

	req := httptest.NewRequest(http.MethodGet, "/setup-token", nil)
	req.RemoteAddr = ip + ":55555"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusTooManyRequests, w.Code,
		"exhausted rate limiter must return 429 before TOFU logic runs")
}

// TestSetupToken_EndpointURLContainsAdvertisedBase verifies that /setup-token
// returns the full advertised URL in the "endpoint" field, not a bare "/mcp"
// path. This is the regression test for #1214: the production handler and the
// test helper now share a single implementation that accepts advertised.
func TestSetupToken_EndpointURLContainsAdvertisedBase(t *testing.T) {
	const apiKey = "test-api-key-endpoint-url"
	const advertised = "https://engram.example.com"
	s := newTOFUTestServer(t, apiKey)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rl := newRateLimiter(ctx)
	handler := s.setupTokenTOFUHandlerWithLimiter(apiKey, advertised, rl)

	t.Run("tofu_returns_full_endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/setup-token", nil)
		req.RemoteAddr = "127.0.0.1:54399"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "TOFU grant must return 200")
		body := w.Body.String()
		require.Contains(t, body, advertised+"/mcp",
			"endpoint must be advertised+/mcp, not a bare /mcp path (#1214)")
	})

	t.Run("authed_returns_full_endpoint", func(t *testing.T) {
		// Use a fresh server (tofuGranted was consumed above).
		s2 := newTOFUTestServer(t, apiKey)
		s2.tofuGranted.Store(true) // TOFU already consumed
		ctx2, cancel2 := context.WithCancel(context.Background())
		defer cancel2()
		h2 := s2.setupTokenTOFUHandlerWithLimiter(apiKey, advertised, newRateLimiter(ctx2))

		req := httptest.NewRequest(http.MethodGet, "/setup-token", nil)
		req.RemoteAddr = "127.0.0.1:54400"
		req.Header.Set("Authorization", "Bearer "+apiKey)
		w := httptest.NewRecorder()
		h2.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "authenticated request must return 200")
		body := w.Body.String()
		require.Contains(t, body, advertised+"/mcp",
			"authenticated endpoint must be advertised+/mcp (#1214)")
	})
}
