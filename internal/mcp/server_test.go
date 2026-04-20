package mcp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// Rate limiter tests — issue #243
// ---------------------------------------------------------------------------

// TestRateLimiter_AllowSetupToken verifies that the setup-token limiter allows
// exactly 3 rapid calls per IP and rejects the 4th (burst=3).
func TestRateLimiter_AllowSetupToken(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := newRateLimiter(ctx)
	ip := "192.168.1.1"

	// First 3 calls must succeed — burst of 3 tokens.
	for i := 1; i <= 3; i++ {
		if !rl.allowSetupToken(ip) {
			t.Fatalf("call %d: expected allow, got reject", i)
		}
	}

	// 4th call must be rejected — tokens exhausted.
	if rl.allowSetupToken(ip) {
		t.Fatal("4th call: expected reject, got allow")
	}
}

// TestRateLimiter_SetupTokenIndependentPerIP verifies that exhausting the
// setup-token budget for one IP does not affect a different IP's budget.
func TestRateLimiter_SetupTokenIndependentPerIP(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := newRateLimiter(ctx)

	// Exhaust setup-token budget for ip1.
	for i := 0; i < 3; i++ {
		rl.allowSetupToken("10.0.0.1")
	}

	// ip2 must still have its own fresh budget.
	if !rl.allowSetupToken("10.0.0.2") {
		t.Fatal("ip2 should have an independent budget from ip1")
	}
}

// ---------------------------------------------------------------------------
// Session fingerprint middleware tests — issue #245
// ---------------------------------------------------------------------------

// newFingerprintServer builds the minimal Server required to call withSessionFingerprint.
// No pool or config is needed because the method only reads sessionFingerprints.
func newFingerprintServer() *Server {
	return &Server{}
}

// storeFingerprint seeds s.sessionFingerprints the same way the SSE
// OnRegisterSession hook does: HMAC(apiKey, sessionID).
func storeFingerprint(s *Server, apiKey, sessionID string) {
	mac := hmac.New(sha256.New, []byte(apiKey))
	mac.Write([]byte(sessionID))
	s.sessionFingerprints.Store(sessionID, mac.Sum(nil))
}

// TestWithSessionFingerprint_RejectsWrongToken verifies that a request carrying
// a sessionId whose stored fingerprint was created with a different key than
// the one the middleware holds is rejected with 403.
func TestWithSessionFingerprint_RejectsWrongToken(t *testing.T) {
	s := newFingerprintServer()

	const sessionID = "test-session-123"
	const storedKey = "original-api-key"  // key used when the SSE session was opened
	const middlewareKey = "wrong-api-key" // key the middleware is holding now

	// Seed the fingerprint as if the session was opened with storedKey.
	storeFingerprint(s, storedKey, sessionID)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// The middleware uses middlewareKey — mismatch with the stored fingerprint.
	handler := s.withSessionFingerprint(inner, middlewareKey)

	req := httptest.NewRequest(http.MethodPost, "/message?sessionId="+sessionID, nil)
	req.Header.Set("Authorization", "Bearer "+middlewareKey)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", w.Code)
	}
}

// TestWithSessionFingerprint_AllowsCorrectToken verifies that a request whose
// stored fingerprint matches the middleware's apiKey passes through to the inner
// handler and returns 200.
func TestWithSessionFingerprint_AllowsCorrectToken(t *testing.T) {
	s := newFingerprintServer()

	const sessionID = "test-session-456"
	const apiKey = "correct-api-key"

	// Seed the fingerprint with the same key the middleware will use.
	storeFingerprint(s, apiKey, sessionID)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := s.withSessionFingerprint(inner, apiKey)

	req := httptest.NewRequest(http.MethodPost, "/message?sessionId="+sessionID, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
}

// TestWithSessionFingerprint_PassesThroughMissingSession verifies that a
// request with a sessionId that has no stored fingerprint is forwarded to the
// inner handler rather than rejected (the underlying handler owns that error).
func TestWithSessionFingerprint_PassesThroughMissingSession(t *testing.T) {
	s := newFingerprintServer()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := s.withSessionFingerprint(inner, "any-key")

	req := httptest.NewRequest(http.MethodPost, "/message?sessionId=no-such-session", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// No stored fingerprint → inner handler runs → 200.
	if w.Code != http.StatusOK {
		t.Fatalf("expected inner handler 200, got %d", w.Code)
	}
}

// TestWithSessionFingerprint_PassesThroughNoSessionID verifies that a request
// with no sessionId query param at all is forwarded without fingerprint check.
func TestWithSessionFingerprint_PassesThroughNoSessionID(t *testing.T) {
	s := newFingerprintServer()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := s.withSessionFingerprint(inner, "any-key")

	req := httptest.NewRequest(http.MethodPost, "/message", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected inner handler 200, got %d", w.Code)
	}
}
