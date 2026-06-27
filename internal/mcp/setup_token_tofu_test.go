package mcp

// Tests for TOFU (Trust On First Use) behavior on /setup-token — Pillar 3D.
//
// /setup-token allows exactly ONE unauthenticated request from localhost, so
// that engram-session-end.sh and fresh installations can self-configure without
// a chicken-and-egg key problem (#613). All subsequent requests require Bearer
// authentication (unchanged from #540).
//
// RFC1918 extension (#1206): when ENGRAM_SETUP_TOKEN_ALLOW_RFC1918=1, the
// one-time TOFU grant is also issued to Docker bridge addresses (172.x RFC1918).
// Without this flag, RFC1918 addresses are rejected identically to any other
// non-loopback address.

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
	return s.setupTokenTOFUHandler(context.Background(), apiKey)
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
	rl := newRateLimiter(context.Background())
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

// ── RFC1918 extension tests (#1206) ────────────────────────────────────────

// newTOFUTestServerWithRFC1918 returns a Server with AllowRFC1918SetupToken set.
func newTOFUTestServerWithRFC1918(t *testing.T, apiKey string, allow bool) *Server {
	t.Helper()
	pool := newTestNoopPool(t)
	cfg := testConfig()
	cfg.AllowRFC1918SetupToken = allow
	s := NewServer(pool, cfg)
	s.tofuGranted.Store(false)
	return s
}

// TestSetupToken_RFC1918FlagOffRejectsDockerBridgeIP verifies that an
// unauthenticated request from a Docker bridge IP (172.x, RFC1918) is rejected
// when ENGRAM_SETUP_TOKEN_ALLOW_RFC1918 is false (the default).
// This is the regression guard: before #1206 the flag existed but was never
// read, so even "flag OFF" was silently granted.
func TestSetupToken_RFC1918FlagOffRejectsDockerBridgeIP(t *testing.T) {
	const apiKey = "test-api-key-rfc1918-off"
	s := newTOFUTestServerWithRFC1918(t, apiKey, false /* flag OFF */)
	handler := s.setupTokenTOFUHandlerWithLimiter(apiKey, newRateLimiter(context.Background()))

	req := httptest.NewRequest(http.MethodGet, "/setup-token", nil)
	req.RemoteAddr = "172.23.0.1:48000" // typical Docker bridge IP
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code,
		"RFC1918 address must be rejected when AllowRFC1918SetupToken=false")
	require.False(t, s.tofuGranted.Load(),
		"tofuGranted must remain false after RFC1918 rejection with flag OFF")
}

// TestSetupToken_RFC1918FlagOnGrantsDockerBridgeIP verifies that an
// unauthenticated request from a Docker bridge IP is granted the one-time TOFU
// token when ENGRAM_SETUP_TOKEN_ALLOW_RFC1918=1. This is the main fix for #1206:
// Docker TOFU bootstrap was silently broken because 172.x is RFC1918, not loopback.
func TestSetupToken_RFC1918FlagOnGrantsDockerBridgeIP(t *testing.T) {
	const apiKey = "test-api-key-rfc1918-on"
	s := newTOFUTestServerWithRFC1918(t, apiKey, true /* flag ON */)
	handler := s.setupTokenTOFUHandlerWithLimiter(apiKey, newRateLimiter(context.Background()))

	require.False(t, s.tofuGranted.Load(), "tofuGranted must start as false")

	req := httptest.NewRequest(http.MethodGet, "/setup-token", nil)
	req.RemoteAddr = "172.23.0.1:48000" // typical Docker bridge IP
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code,
		"RFC1918 address must receive the one-time TOFU grant when AllowRFC1918SetupToken=true")
	require.Contains(t, w.Body.String(), apiKey,
		"response body must contain the API key on RFC1918 TOFU grant")
	require.Contains(t, w.Body.String(), "/mcp",
		"response body must advertise /mcp endpoint (not /sse)")
	require.True(t, s.tofuGranted.Load(),
		"tofuGranted must be true after RFC1918 TOFU grant")
}

// TestSetupToken_RFC1918FlagOnSecondRequestFails verifies that the RFC1918 TOFU
// grant is still one-time-only: a second unauthenticated RFC1918 request after
// the grant is consumed must return 401.
func TestSetupToken_RFC1918FlagOnSecondRequestFails(t *testing.T) {
	const apiKey = "test-api-key-rfc1918-second"
	s := newTOFUTestServerWithRFC1918(t, apiKey, true)
	handler := s.setupTokenTOFUHandlerWithLimiter(apiKey, newRateLimiter(context.Background()))

	// First RFC1918 request — TOFU grant.
	req1 := httptest.NewRequest(http.MethodGet, "/setup-token", nil)
	req1.RemoteAddr = "172.23.0.1:48001"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	require.Equal(t, http.StatusOK, w1.Code, "first RFC1918 TOFU request must succeed")

	// Second unauthenticated RFC1918 request — must be rejected.
	req2 := httptest.NewRequest(http.MethodGet, "/setup-token", nil)
	req2.RemoteAddr = "172.23.0.2:48002"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusUnauthorized, w2.Code,
		"second unauthenticated RFC1918 request must return 401 (TOFU already consumed)")
}

// TestSetupToken_LoopbackResponseContainsMCPEndpoint verifies that the loopback
// TOFU grant response advertises /mcp (not /sse) in the endpoint field (#1206).
func TestSetupToken_LoopbackResponseContainsMCPEndpoint(t *testing.T) {
	const apiKey = "test-api-key-endpoint-check"
	s := newTOFUTestServer(t, apiKey)
	handler := s.setupTokenTOFUHandlerWithLimiter(apiKey, newRateLimiter(context.Background()))

	req := httptest.NewRequest(http.MethodGet, "/setup-token", nil)
	req.RemoteAddr = "127.0.0.1:60001"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "loopback TOFU must return 200")
	body := w.Body.String()
	require.Contains(t, body, "/mcp",
		"setup-token response must contain /mcp in endpoint field")
	require.NotContains(t, body, `"endpoint":"/sse"`,
		"setup-token response must not advertise /sse as the endpoint")
}

// TestSetupToken_RFC1918AllSubnets verifies isRFC1918IP across all three
// private address ranges and confirms a public IP is not matched.
func TestSetupToken_RFC1918AllSubnets(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
		desc string
	}{
		{"10.0.0.1", true, "10.x class-A private"},
		{"10.255.255.255", true, "10.x upper bound"},
		{"172.16.0.1", true, "172.16.x lower bound"},
		{"172.23.0.1", true, "172.23.x Docker bridge typical"},
		{"172.31.255.255", true, "172.31.x upper bound"},
		{"172.15.0.1", false, "172.15.x just below 172.16/12"},
		{"172.32.0.1", false, "172.32.x just above 172.31/12"},
		{"192.168.0.1", true, "192.168.x private"},
		{"192.168.255.255", true, "192.168.x upper bound"},
		{"8.8.8.8", false, "public IP"},
		{"127.0.0.1", false, "loopback is not RFC1918"},
		{"::1", false, "IPv6 loopback is not RFC1918"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			got := isRFC1918IP(tc.ip)
			require.Equal(t, tc.want, got, "isRFC1918IP(%q): %s", tc.ip, tc.desc)
		})
	}
}
