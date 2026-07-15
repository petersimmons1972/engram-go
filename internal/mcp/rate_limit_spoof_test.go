package mcp

// Tests for issue #1190 (SEC-07): loopback rate-limit exemption must check
// the physical network address (r.RemoteAddr), not client-supplied proxy headers.
//
// A remote attacker sending "X-Real-IP: 127.0.0.1" with
// ENGRAM_TRUST_PROXY_HEADERS=1 must NOT bypass rate limiting.
// A genuine loopback connection (RemoteAddr is 127.0.0.1) remains exempt.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestLoopbackSpoofNotExempted verifies that a remote TCP connection sending
// "X-Real-IP: 127.0.0.1" is NOT exempt from rate limiting when trustProxy=true.
//
// The physical network address (RemoteAddr = "10.0.0.1:12345") is non-loopback,
// so the rate limiter applies regardless of the header. The RL bucket key is the
// proxy-header IP (127.0.0.1 from X-Real-IP), so we pre-exhaust that bucket to
// prove the request goes through the limiter and gets rejected.
func TestLoopbackSpoofNotExempted(t *testing.T) {
	s := newTrustProxyServer()

	const apiKey = "test-key-spoof"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// With trustProxy=true, clientIP() returns the X-Real-IP value ("127.0.0.1")
	// as the RL bucket key. Pre-exhaust that bucket so the next request is rejected.
	rl := newRateLimiterWithConfig(ctx, 1, 1)
	rl.allow("127.0.0.1") // consumes the bucket the spoofed header maps to

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := s.applyMiddlewareWithRL(inner, apiKey, rl)

	// Attacker is physically at 10.0.0.1 but sends spoofed X-Real-IP.
	// physicalHost = "10.0.0.1" → not loopback → RL applies → bucket "127.0.0.1" is exhausted → 429.
	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("X-Real-IP", "127.0.0.1") // spoofed loopback header
	req.RemoteAddr = "10.0.0.1:12345"        // physical non-loopback address
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// The physical address is non-loopback → limiter is applied → bucket exhausted → 429.
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 (rate limited) for spoofed loopback header, got %d — SEC-07 regression", w.Code)
	}
}

// TestLoopbackSpoofXFFNotExempted verifies the same protection applies for
// X-Forwarded-For: 127.0.0.1. clientIP() returns the XFF value ("127.0.0.1")
// as the RL bucket key; we pre-exhaust it, then confirm 429 is returned.
func TestLoopbackSpoofXFFNotExempted(t *testing.T) {
	s := newTrustProxyServer()

	const apiKey = "test-key-spoof-xff"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// clientIP() will return "127.0.0.1" (from X-Forwarded-For) as the bucket key.
	rl := newRateLimiterWithConfig(ctx, 1, 1)
	rl.allow("127.0.0.1") // pre-exhaust the bucket that the spoof maps to

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := s.applyMiddlewareWithRL(inner, apiKey, rl)

	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("X-Forwarded-For", "127.0.0.1") // spoofed via XFF
	req.RemoteAddr = "10.0.0.2:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for XFF-spoofed loopback, got %d — SEC-07 regression", w.Code)
	}
}

// TestPhysicalLoopbackExempt verifies that a genuine loopback connection
// (RemoteAddr is 127.0.0.1) remains exempt from rate limiting regardless of
// proxy header configuration or limiter state.
func TestPhysicalLoopbackExempt(t *testing.T) {
	s := newTrustProxyServer() // trustProxy=true should not affect loopback exemption

	const apiKey = "test-key-phys-loopback"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Exhaust the limiter for 127.0.0.1 so it would reject if not exempt.
	rl := newRateLimiterWithConfig(ctx, 1, 1)
	rl.allow("127.0.0.1") // consume burst

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := s.applyMiddlewareWithRL(inner, apiKey, rl)

	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.RemoteAddr = "127.0.0.1:54321" // genuine loopback
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code == http.StatusTooManyRequests {
		t.Fatal("physical loopback connection (RemoteAddr=127.0.0.1) was rate-limited — must remain exempt (#387, #1190)")
	}
}

// TestPhysicalLoopbackExemptTrustProxyOff verifies that loopback exemption
// also works when trustProxy=false (the no-proxy default).
func TestPhysicalLoopbackExemptTrustProxyOff(t *testing.T) {
	s := &Server{cfg: Config{}, trustProxy: false}

	const apiKey = "test-key-phys-loopback-noproxy"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := newRateLimiterWithConfig(ctx, 1, 1)
	rl.allow("127.0.0.1")

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
		t.Fatal("physical loopback with trustProxy=false was rate-limited — must remain exempt")
	}
}
