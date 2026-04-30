package mcp

// Tests for issue #387: configurable rate limiter.
//
// These tests are written first (TDD) and must FAIL before the implementation
// is added. They cover:
//   - RateLimiterConfig defaults
//   - newRateLimiterWithConfig respects custom RPS/burst
//   - ENGRAM_RATE_LIMIT_DISABLE skips the rate limiter in applyMiddleware
//   - Loopback IPs are never rate-limited regardless of config
//   - Config struct carries the new fields

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// RateLimiterConfig field defaults
// ---------------------------------------------------------------------------

// TestRateLimiterConfigDefaults verifies that a zero-value Config carries the
// expected default rate-limit values (RPS=50, Burst=200, Disable=false).
func TestRateLimiterConfigDefaults(t *testing.T) {
	cfg := Config{}
	// zero values map to defaults — the accessors enforce this
	rps := cfg.rateLimitRPS()
	burst := cfg.rateLimitBurst()

	if rps != 50 {
		t.Errorf("default RPS: want 50, got %d", rps)
	}
	if burst != 200 {
		t.Errorf("default burst: want 200, got %d", burst)
	}
	if cfg.RateLimitDisable {
		t.Error("default RateLimitDisable must be false")
	}
}

// TestRateLimiterConfigCustomValues verifies that non-zero Config values are
// returned verbatim from the accessor helpers.
func TestRateLimiterConfigCustomValues(t *testing.T) {
	cfg := Config{
		RateLimitRPS:   10,
		RateLimitBurst: 20,
	}

	if got := cfg.rateLimitRPS(); got != 10 {
		t.Errorf("want RPS 10, got %d", got)
	}
	if got := cfg.rateLimitBurst(); got != 20 {
		t.Errorf("want burst 20, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// newRateLimiterWithConfig — custom RPS/burst
// ---------------------------------------------------------------------------

// TestNewRateLimiterWithConfig_CustomBurst verifies that a limiter created with
// burst=3 allows exactly 3 immediate requests and rejects the 4th.
func TestNewRateLimiterWithConfig_CustomBurst(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := newRateLimiterWithConfig(ctx, 100, 3) // 100 RPS, burst 3
	ip := "1.2.3.4"

	for i := 1; i <= 3; i++ {
		if !rl.allow(ip) {
			t.Fatalf("call %d: expected allow (burst=3), got reject", i)
		}
	}
	if rl.allow(ip) {
		t.Fatal("4th call: expected reject (burst exhausted), got allow")
	}
}

// ---------------------------------------------------------------------------
// ENGRAM_RATE_LIMIT_DISABLE — applyMiddleware bypasses rate check
// ---------------------------------------------------------------------------

// newDisabledRateLimitServer builds a minimal *Server with RateLimitDisable=true.
func newDisabledRateLimitServer() *Server {
	return &Server{
		cfg: Config{RateLimitDisable: true},
	}
}

// TestApplyMiddleware_DisabledRateLimit_NeverRejects verifies that with
// RateLimitDisable=true, no request is ever rejected with 429 — even after
// hundreds of rapid calls from the same IP.
func TestApplyMiddleware_DisabledRateLimit_NeverRejects(t *testing.T) {
	s := newDisabledRateLimitServer()

	const apiKey = "test-key-disable"

	// Create a rate limiter that would reject immediately (burst=1, then exhausted).
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rl := newRateLimiterWithConfig(ctx, 1, 1)

	// Pre-exhaust the limiter for our test IP.
	rl.allow("10.0.0.1") // consume the 1-token burst

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := s.applyMiddlewareWithRL(inner, apiKey, rl)

	// With RateLimitDisable=true the pre-exhausted limiter must be bypassed.
	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code == http.StatusTooManyRequests {
		t.Fatal("got 429 with RateLimitDisable=true — rate limiter was not bypassed")
	}
}

// ---------------------------------------------------------------------------
// Loopback auto-disable
// ---------------------------------------------------------------------------

// TestApplyMiddleware_LoopbackNeverRateLimited verifies that requests from
// 127.0.0.1 are never rejected with 429 regardless of limiter state.
func TestApplyMiddleware_LoopbackNeverRateLimited(t *testing.T) {
	s := &Server{cfg: Config{}} // rate limiting enabled, defaults

	const apiKey = "test-key-loopback"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Burst=0 would reject every request normally — but loopback must be exempt.
	// We use burst=1 and pre-exhaust it instead (burst=0 is not valid for x/time/rate).
	rl := newRateLimiterWithConfig(ctx, 1, 1)
	rl.allow("127.0.0.1") // exhaust the 1-token burst

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := s.applyMiddlewareWithRL(inner, apiKey, rl)

	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.RemoteAddr = "127.0.0.1:54321"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code == http.StatusTooManyRequests {
		t.Fatal("got 429 for 127.0.0.1 — loopback IPs must never be rate-limited (#387)")
	}
}

// TestApplyMiddleware_IPv6LoopbackNeverRateLimited verifies ::1 is also exempt.
func TestApplyMiddleware_IPv6LoopbackNeverRateLimited(t *testing.T) {
	s := &Server{cfg: Config{}}

	const apiKey = "test-key-ipv6-loopback"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := newRateLimiterWithConfig(ctx, 1, 1)
	rl.allow("::1") // exhaust

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := s.applyMiddlewareWithRL(inner, apiKey, rl)

	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.RemoteAddr = "[::1]:54321"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code == http.StatusTooManyRequests {
		t.Fatal("got 429 for ::1 — IPv6 loopback must never be rate-limited (#387)")
	}
}

// TestApplyMiddleware_NonLoopbackIsRateLimited verifies that non-loopback IPs
// are still rate-limited normally after the loopback exemption is added.
func TestApplyMiddleware_NonLoopbackIsRateLimited(t *testing.T) {
	s := &Server{cfg: Config{}}

	const apiKey = "test-key-non-loopback"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := newRateLimiterWithConfig(ctx, 1, 1)
	rl.allow("10.20.30.40") // exhaust

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := s.applyMiddlewareWithRL(inner, apiKey, rl)

	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.RemoteAddr = "10.20.30.40:5555"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for non-loopback with exhausted budget, got %d", w.Code)
	}
}
