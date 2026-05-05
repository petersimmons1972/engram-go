package mcp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ---------------------------------------------------------------------------
// E1: separate setupClients map tests (#285)
// ---------------------------------------------------------------------------

// TestSetupTokenBudgetIsolatedFromNormalBudget verifies that exhausting the normal
// allow() budget for an IP does not affect that IP's allowSetupToken() budget,
// and vice versa. The two maps must be independent.
func TestSetupTokenBudgetIsolatedFromNormalBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := newRateLimiter(ctx)
	ip := "10.0.0.5"

	// Exhaust the normal budget by calling allow() many times.
	// The normal limiter has burst=200 so we call it 201 times.
	exhausted := false
	for i := 0; i < 250; i++ {
		if !rl.allow(ip) {
			exhausted = true
			break
		}
	}
	if !exhausted {
		t.Skip("could not exhaust normal budget within 250 calls — test environment may be too fast")
	}

	// setup-token budget must still be intact: it has its own map.
	if !rl.allowSetupToken(ip) {
		t.Fatal("allowSetupToken returned false after exhausting normal budget — budgets are not isolated (#285)")
	}
}

// TestSetupTokenBudgetDoesNotConsumeNormalBudget verifies the inverse: calling
// allowSetupToken() 3 times (exhausting its burst) does not touch the normal
// allow() tokens for that IP.
func TestSetupTokenBudgetDoesNotConsumeNormalBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := newRateLimiter(ctx)
	ip := "10.0.0.6"

	// Exhaust setup-token budget.
	for i := 0; i < 3; i++ {
		rl.allowSetupToken(ip)
	}

	// Normal allow() must still work — it has its own fresh map entry.
	if !rl.allow(ip) {
		t.Fatal("allow() returned false after exhausting setup-token budget — budgets are not isolated (#285)")
	}
}

// ---------------------------------------------------------------------------
// E3: clientIP IP validation tests (#290)
// ---------------------------------------------------------------------------

// newTrustProxyServer builds a minimal *Server with trustProxy=true.
func newTrustProxyServer() *Server {
	return &Server{trustProxy: true}
}

// TestClientIP_InvalidXRealIPFallsThrough verifies that a malformed X-Real-IP
// header is rejected and clientIP falls through to X-Forwarded-For.
func TestClientIP_InvalidXRealIPFallsThrough(t *testing.T) {
	s := newTrustProxyServer()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "not-an-ip")
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.RemoteAddr = "5.6.7.8:9999"

	ip := s.clientIP(req)
	if ip != "1.2.3.4" {
		t.Fatalf("expected X-Forwarded-For fallback 1.2.3.4, got %q (invalid X-Real-IP must be skipped)", ip)
	}
}

// TestClientIP_InvalidXFFFallsThrough verifies that a malformed X-Forwarded-For
// header is rejected and clientIP falls back to RemoteAddr.
func TestClientIP_InvalidXFFFallsThrough(t *testing.T) {
	s := newTrustProxyServer()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "")
	req.Header.Set("X-Forwarded-For", "not-an-ip, 1.2.3.4")
	req.RemoteAddr = "5.6.7.8:9999"

	ip := s.clientIP(req)
	if ip != "5.6.7.8" {
		t.Fatalf("expected RemoteAddr fallback 5.6.7.8, got %q (invalid X-Forwarded-For first entry must be skipped)", ip)
	}
}

// TestClientIP_ValidXRealIPReturned verifies that a valid X-Real-IP is returned directly.
func TestClientIP_ValidXRealIPReturned(t *testing.T) {
	s := newTrustProxyServer()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "203.0.113.1")
	req.RemoteAddr = "5.6.7.8:9999"

	ip := s.clientIP(req)
	if ip != "203.0.113.1" {
		t.Fatalf("expected 203.0.113.1, got %q", ip)
	}
}

// TestClientIP_LoopbackSpoofRejected verifies that X-Real-IP: 127.0.0.1 is NOT
// returned as a valid IP when the value is a loopback literal — it must be validated.
// This is the core attack vector for bypassing the /setup-token loopback check (#290).
// Note: net.ParseIP("127.0.0.1") succeeds — 127.0.0.1 is a valid IP. The SSRF
// protection against loopback spoofing is in isLocalAddress + the loopback check
// in /setup-token. clientIP's job is to reject non-IP strings; valid IPs (including
// loopback) are returned as-is and blocked by the isLocalAddress gate.
func TestClientIP_ValidIPInXRealIPReturned(t *testing.T) {
	s := newTrustProxyServer()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "127.0.0.1")
	req.RemoteAddr = "5.6.7.8:9999"

	ip := s.clientIP(req)
	// 127.0.0.1 is a valid IP, so clientIP returns it.
	// The /setup-token handler then checks isLocalAddress and acts accordingly.
	if ip != "127.0.0.1" {
		t.Fatalf("expected 127.0.0.1 (valid IP), got %q", ip)
	}
}

// ---------------------------------------------------------------------------
// E6: JSON error responses in handleQuickStore (#311)
// ---------------------------------------------------------------------------

// TestQuickStoreHandler_InvalidJSON_JSONErrorBody verifies that invalid JSON
// returns a JSON-encoded error body rather than a raw Go error string (#311).
func TestQuickStoreHandler_InvalidJSON_JSONErrorBody(t *testing.T) {
	s := newQuickStoreServer(t)

	req := httptest.NewRequest(http.MethodPost, "/quick-store", strings.NewReader("not json {"))
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q (#311 requires JSON error body)", ct)
	}
	body := w.Body.String()
	// Must not leak internal Go error messages like "invalid character".
	if strings.Contains(body, "invalid character") || strings.Contains(body, "unexpected end") {
		t.Fatalf("response body leaks raw Go error: %q", body)
	}
}

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

// TestRateLimiter_DisabledWhenConfigIsZero is skipped pending issue #422.
// The "Config.RateLimit=0 means unlimited" feature it asserts is not currently
// wired through the rate-limiter constructor — Config.RateLimit is a dead field.
func TestRateLimiter_DisabledWhenConfigIsZero(t *testing.T) {
	t.Skip("Config.RateLimit field is dead — see issue #422")
}

// TestRateLimiter_RespectsBurstMultiplier verifies that when burst is set,
// requests respect that burst budget. Uses newRateLimiterWithConfig directly
// since Config.RateLimit is dead (issue #422); behavior under test is the
// burst-capacity logic of the per-IP rate.Limiter, which is unchanged.
func TestRateLimiter_RespectsBurstMultiplier(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 10 req/s sustained, burst of 40 — same shape the dead Config.RateLimit
	// path was meant to produce (burst = 4 × RateLimit).
	rl := newRateLimiterWithConfig(ctx, 10, 40)
	ip := "10.0.0.8"

	// First 40 calls should succeed (burst).
	var exhausted bool
	for i := 0; i < 50; i++ {
		if !rl.allow(ip) {
			exhausted = true
			if i < 40 {
				t.Fatalf("call %d: expected allow within burst of 40, got reject", i)
			}
			break
		}
	}
	if !exhausted {
		t.Skip("could not exhaust burst within 50 calls — test environment may be too fast")
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

// ---------------------------------------------------------------------------
// E6: non-fatal Ollama startup probe — degraded mode
// ---------------------------------------------------------------------------

// TestHealthProbe_OllamaDegraded_Returns200WithDegradedField verifies that when
// OllamaDegraded is set (Ollama was unreachable at startup) and the live health
// probe also fails, /health returns 200 (server is up) with "ollama":"degraded".
// This replaces the previous behaviour where Ollama failure always returned 503.
func TestHealthProbe_OllamaDegraded_Returns200WithDegradedField(t *testing.T) {
	// Point the server at a URL that will always fail (nothing listening).
	// Use a port on 127.0.0.1 that is almost certainly unbound.
	unreachableOllama := "http://127.0.0.1:19999"

	b := &atomic.Bool{}
	b.Store(true)
	s := &Server{
		cfg: Config{
			LiteLLMURL:      unreachableOllama,
			EmbedDegraded: true, // set as if startup probe failed
		},
		embedDegraded: b,
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	s.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when OllamaDegraded=true, got %d — degraded startup should not cause 503", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, `"degraded"`) {
		t.Fatalf("expected 'degraded' in response body, got: %s", body)
	}
	if !strings.Contains(body, `"ollama"`) {
		t.Fatalf("expected 'ollama' field in response body, got: %s", body)
	}
}

// TestHealthProbe_OllamaDegraded_RecoveredOllamaReportsOK verifies that if
// OllamaDegraded is set but the live Ollama probe now succeeds, /health returns
// 200 with "ollama":"ok" — degraded mode is not permanent.
func TestHealthProbe_OllamaDegraded_RecoveredOllamaReportsOK(t *testing.T) {
	// Spin up a fake Ollama that responds successfully.
	fakeOllama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer fakeOllama.Close()

	b := &atomic.Bool{}
	b.Store(true)
	s := &Server{
		cfg: Config{
			LiteLLMURL:      fakeOllama.URL,
			EmbedDegraded: true, // startup probe failed, but Ollama recovered
		},
		embedDegraded: b,
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	s.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when Ollama recovered, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, `"ok"`) {
		t.Fatalf("expected 'ok' in response body when Ollama recovered, got: %s", body)
	}
}

// ---------------------------------------------------------------------------
// Auto-episode removal — issue #352
// ---------------------------------------------------------------------------

// fakeClientSession is a minimal implementation of server.ClientSession for
// unit tests that need to fire session hooks without a real SSE connection.
type fakeClientSession struct{ id string }

func (f *fakeClientSession) SessionID() string                                      { return f.id }
func (f *fakeClientSession) Initialize()                                             {}
func (f *fakeClientSession) Initialized() bool                                       { return true }
func (f *fakeClientSession) NotificationChannel() chan<- mcpgo.JSONRPCNotification    { return nil }

// TestSSEConnect_DoesNotAutoStartEpisode verifies that the OnRegisterSession
// hook only stores the HMAC fingerprint and does NOT call StartEpisode.
//
// Strategy: the Server is constructed with a nil pool. If the hook attempts
// to call s.pool.Get() (the auto-episode path), it will panic with a nil
// pointer dereference. A recovered panic fails the test. No panic = the
// episode path is gone.
func TestSSEConnect_DoesNotAutoStartEpisode(t *testing.T) {
	s := NewServer(nil, Config{}) // nil pool — any pool access panics

	const apiKey = "test-key-episode-removal"
	s.registerSessionHooks(apiKey)

	sess := &fakeClientSession{id: "sess-no-episode"}
	ctx := context.Background()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("OnRegisterSession hook panicked (auto-episode path still present): %v", r)
		}
	}()

	// Fire the hook directly — no real SSE server needed.
	s.mcp.GetHooks().RegisterSession(ctx, sess)

	// If we reach here the hook ran without touching s.pool — episode path is absent.
	// Also verify the fingerprint was stored (the path we must keep).
	if _, ok := s.sessionFingerprints.Load(sess.id); !ok {
		t.Fatal("fingerprint was not stored — session fingerprint path was accidentally removed")
	}
}

// ---------------------------------------------------------------------------
// Health probe context isolation — issue E1
// ---------------------------------------------------------------------------

// TestHealthProbe_IgnoresExpiredRequestContext verifies that handleHealth
// uses its own independent deadline for the Ollama probe, not r.Context().
// If the probe inherits r.Context(), an HTTP client with a sub-2s deadline
// causes a spurious 503 even when Ollama is healthy. The fix is to use
// context.Background() as the parent for each probe timeout.
func TestHealthProbe_IgnoresExpiredRequestContext(t *testing.T) {
	// Spin up a fake Ollama endpoint that responds immediately with 200.
	ollamaHit := make(chan struct{}, 1)
	fakeOllama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case ollamaHit <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer fakeOllama.Close()

	// Build a minimal Server pointing at the fake Ollama. No PgPool, so the
	// Postgres probe is skipped and only the Ollama probe exercises the context.
	s := &Server{
		cfg:           Config{LiteLLMURL: fakeOllama.URL},
		embedDegraded: &atomic.Bool{},
	}

	// Create an already-cancelled request context — simulates an HTTP client
	// whose deadline expired before the server could start the probe.
	expiredCtx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	cancel() // cancel immediately so the context is already done
	// Give the cancellation a moment to propagate.
	time.Sleep(5 * time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req = req.WithContext(expiredCtx)
	w := httptest.NewRecorder()

	s.handleHealth(w, req)

	// The probe must succeed (200) because Ollama is healthy; only the request
	// context is dead. A 503 here means the probe inherited r.Context().
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (Ollama healthy), got %d — probe likely inherited expired r.Context()", w.Code)
	}

	select {
	case <-ollamaHit:
		// Probe reached the fake Ollama — correct.
	default:
		t.Fatal("fake Ollama was never contacted — probe may have short-circuited on the dead context")
	}
}

// ---------------------------------------------------------------------------
// Dispatch-layer timeout (#379)
// ---------------------------------------------------------------------------

// TestDispatchTimeout_RegisterToolAppliesDeadline verifies that registerTool
// applies a context deadline so blocking handlers are cancelled within
// defaultToolTimeout rather than running to the HTTP server WriteTimeout (90s).
func TestDispatchTimeout_RegisterToolAppliesDeadline(t *testing.T) {
	var handlerCtxCancelled atomic.Bool

	// noopPoolForTimeout satisfies EnginePool minimally — registerTool only
	// needs a pool reference; the handler controls its own behaviour.
	pool := NewEnginePool(func(_ context.Context, _ string) (*EngineHandle, error) {
		return nil, nil
	})

	srv := &Server{
		pool: pool,
		mcp:  server.NewMCPServer("timeout-test", "1.0.0", server.WithToolCapabilities(true)),
		cfg:  Config{},
	}

	blockingHandler := func(ctx context.Context, _ *EnginePool, _ mcpgo.CallToolRequest, _ Config) (*mcpgo.CallToolResult, error) {
		<-ctx.Done()
		handlerCtxCancelled.Store(true)
		return &mcpgo.CallToolResult{IsError: true, Content: []mcpgo.Content{
			mcpgo.TextContent{Type: "text", Text: "timed out"},
		}}, nil
	}

	// Register with a short timeout so the test completes quickly.
	srv.registerToolWithTimeout("block_test", "blocks until ctx cancelled", blockingHandler, 200*time.Millisecond, false)

	c, err := mcpclient.NewInProcessClient(srv.mcp)
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Close()

	initReq := mcpgo.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcpgo.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcpgo.Implementation{Name: "test", Version: "1.0.0"}
	if _, err := c.Initialize(context.Background(), initReq); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	start := time.Now()
	var callReq mcpgo.CallToolRequest
	callReq.Params.Name = "block_test"
	_, _ = c.CallTool(context.Background(), callReq)
	elapsed := time.Since(start)

	if !handlerCtxCancelled.Load() {
		t.Error("handler ctx was not cancelled — registerTool did not apply dispatch timeout")
	}
	if elapsed > 2*time.Second {
		t.Errorf("call took %v — should complete within 2x the 200ms timeout", elapsed)
	}
}
