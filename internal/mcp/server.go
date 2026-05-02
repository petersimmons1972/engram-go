// Package mcp registers MCP tools and owns the SSE server lifecycle.
package mcp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/time/rate"

	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/petersimmons1972/engram/internal/metrics"
)

// contextKey is a typed key for values stored in request contexts.
// Using a named type prevents collisions with context values from other packages.
type contextKey string

// requestIDKey is the context key for the per-request correlation ID.
const requestIDKey contextKey = "request_id"

// requestIDFromContext retrieves the correlation ID stored by the middleware,
// or returns empty string when no ID is present.
func requestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// rateLimiter holds per-IP token-bucket state for HTTP rate limiting (#140).
type rateLimiter struct {
	mu           sync.Mutex
	rps          int // sustained req/s per IP (set at construction; never mutated)
	burst        int // token-bucket burst size per IP (set at construction; never mutated)
	clients      map[string]*rateLimiterEntry
	setupClients map[string]*rateLimiterEntry // separate budget for /setup-token (#285)
}

type rateLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// newRateLimiter creates a rate limiter with default parameters (50 req/s,
// burst 200) that evicts idle IPs every 5 minutes.
// The eviction goroutine stops when ctx is cancelled.
func newRateLimiter(ctx context.Context) *rateLimiter {
	return newRateLimiterWithConfig(ctx, 50, 200)
}

// newRateLimiterWithConfig creates a rate limiter with the given RPS and burst.
// The eviction goroutine stops when ctx is cancelled.
func newRateLimiterWithConfig(ctx context.Context, rps int, burst int) *rateLimiter {
	rl := &rateLimiter{
		rps:          rps,
		burst:        burst,
		clients:      make(map[string]*rateLimiterEntry),
		setupClients: make(map[string]*rateLimiterEntry),
	}
	go rl.evictLoop(ctx)
	return rl
}

// allow returns true if the request from ip should be allowed.
// The configured RPS and burst are used per-IP; new entries are created on
// first access and evicted after 5 minutes of inactivity.
func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	e, ok := rl.clients[ip]
	if !ok {
		e = &rateLimiterEntry{
			limiter: rate.NewLimiter(rate.Limit(rl.rps), rl.burst),
		}
		rl.clients[ip] = e
	}
	e.lastSeen = time.Now()
	ok = e.limiter.Allow()
	rl.mu.Unlock()
	return ok
}

// evictLoop removes idle IP entries every 5 minutes until ctx is cancelled.
func (rl *rateLimiter) evictLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rl.evict()
		}
	}
}

// evict removes entries not seen in the last 5 minutes from both maps.
func (rl *rateLimiter) evict() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for ip, e := range rl.clients {
		if time.Since(e.lastSeen) > 5*time.Minute {
			delete(rl.clients, ip)
		}
	}
	for ip, e := range rl.setupClients {
		if time.Since(e.lastSeen) > 5*time.Minute {
			delete(rl.setupClients, ip)
		}
	}
}

// allowSetupToken is like allow but uses a tighter budget for the /setup-token
// endpoint: 3 requests per 5-minute window (burst 3, one token every ~100s) (#243).
// Uses a separate setupClients map so IPs already present in the normal clients
// map cannot consume their setup-token budget via authenticated requests (#285).
func (rl *rateLimiter) allowSetupToken(ip string) bool {
	rl.mu.Lock()
	e, ok := rl.setupClients[ip]
	if !ok {
		e = &rateLimiterEntry{
			limiter: rate.NewLimiter(rate.Every(setupTokenWindow), 3),
		}
		rl.setupClients[ip] = e
	}
	e.lastSeen = time.Now()
	ok = e.limiter.Allow()
	rl.mu.Unlock()
	return ok
}

// Server wraps the MCP SSE server and owns the EnginePool.
type Server struct {
	pool                *EnginePool
	cfg                 Config
	mcp                 *server.MCPServer
	uploadMu            sync.Mutex
	uploads             map[string]*uploadSession
	sessionFingerprints sync.Map // sessionID -> []byte HMAC fingerprint (#245)
	sessionEpisodes     sync.Map // sessionID -> episodeID string for auto-episode sessions (#356)
	trustProxy          bool     // honour X-Forwarded-For / X-Real-IP (#255)
}

// NewServer constructs a Server with all MCP tools registered.
func NewServer(pool *EnginePool, cfg Config) *Server {
	trustProxy := false
	if v := os.Getenv("ENGRAM_TRUST_PROXY_HEADERS"); v == "1" || strings.EqualFold(v, "true") {
		trustProxy = true
		slog.Warn("ENGRAM_TRUST_PROXY_HEADERS is enabled — ensure a trusted reverse proxy terminates all inbound connections; direct clients can spoof X-Forwarded-For to bypass rate limiting")
	}
	s := &Server{pool: pool, cfg: cfg, uploads: make(map[string]*uploadSession), trustProxy: trustProxy}
	mcpServer := server.NewMCPServer("engram", "1.0.0",
		server.WithToolCapabilities(true),
		server.WithHooks(&server.Hooks{}),
	)
	s.mcp = mcpServer
	s.registerTools()
	return s
}

// SetClaudeClient sets the Claude client used for advisor operations (e.g. consolidation).
// Must be called before Start.
func (s *Server) SetClaudeClient(client *claude.Client) {
	s.cfg.claudeClient = client
	s.cfg.ClaudeEnabled = (client != nil)
}

// setupTokenWindow is the rate-limit window for /setup-token: 3 calls per 5 minutes.
const setupTokenWindow = 5 * time.Minute / 3 // one token every 100 seconds

// hashAPIKey returns the SHA-256 hex digest of the API key. Stored in the
// sessions table instead of the plaintext key so the DB does not become a
// credential store.
func hashAPIKey(apiKey string) string {
	sum := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(sum[:])
}

// rehydratedSession is a minimal ClientSession that satisfies the mcp-go
// interface for the purpose of re-registering sessions after a server restart.
// Notifications are sent to a buffered channel that silently drops overflow —
// the real SSE connection is gone, but tool calls (POST /message) still work.
type rehydratedSession struct {
	id string
	ch chan mcpgo.JSONRPCNotification
}

func newRehydratedSession(id string) *rehydratedSession {
	// Buffer 256 notifications — the real SSE consumer is gone, but the mcp-go
	// transport may attempt sends. A buffer large enough to absorb any burst
	// prevents the sender goroutine from blocking on a dead channel.
	return &rehydratedSession{id: id, ch: make(chan mcpgo.JSONRPCNotification, 256)}
}

func (r *rehydratedSession) Initialize()                                           {}
func (r *rehydratedSession) Initialized() bool                                     { return true }
func (r *rehydratedSession) NotificationChannel() chan<- mcpgo.JSONRPCNotification { return r.ch }
func (r *rehydratedSession) SessionID() string                                     { return r.id }

// RehydrateSessions loads active sessions from the database and re-registers
// them in the mcp-go transport so POST /message calls with pre-restart session
// IDs succeed immediately after a restart (#362). Must be called before Start.
func (s *Server) RehydrateSessions(ctx context.Context, apiKey string) error {
	if s.cfg.SessionDB == nil {
		return nil
	}
	ids, err := s.cfg.SessionDB.ListActiveSessions(ctx, 2*time.Hour)
	if err != nil {
		return fmt.Errorf("list active sessions for rehydration: %w", err)
	}
	for _, id := range ids {
		sess := newRehydratedSession(id)
		if err := s.mcp.RegisterSession(ctx, sess); err != nil {
			slog.Warn("rehydrate sessions: failed to register", "session_id", id, "err", err)
			continue
		}
		// Restore the HMAC fingerprint so withSessionFingerprint passes (#245).
		mac := hmac.New(sha256.New, []byte(apiKey))
		mac.Write([]byte(id))
		s.sessionFingerprints.Store(id, mac.Sum(nil))
	}
	slog.Info("session rehydration complete", "count", len(ids))
	return nil
}

// registerSessionHooks wires the OnRegisterSession and OnUnregisterSession
// hooks on the MCP server. Extracted from Start so tests can call it directly
// without binding a port.
//
// OnRegisterSession: stores a session→bearer HMAC fingerprint so POST /message
// can verify the session was opened by the same bearer that is now posting (#245).
// Also auto-starts an episode when the session context carries the auto-episode
// flag (injected by applyMiddleware when ?auto_episode=1 is in the SSE URL) (#356).
//
// OnUnregisterSession: removes the fingerprint when the session disconnects (#245).
// Also closes any auto-started episode using context.Background() — the session
// context is already cancelled at this point (#356).
func (s *Server) registerSessionHooks(apiKey string) {
	s.mcp.GetHooks().AddOnRegisterSession(func(ctx context.Context, sess server.ClientSession) {
		sessionID := sess.SessionID()

		// Store the HMAC fingerprint for POST /message verification (#245).
		mac := hmac.New(sha256.New, []byte(apiKey))
		mac.Write([]byte(sessionID))
		s.sessionFingerprints.Store(sessionID, mac.Sum(nil))

		// Persist to DB so the session survives a server restart (#362).
		if s.cfg.SessionDB != nil {
			go func() {
				dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := s.cfg.SessionDB.RegisterSession(dbCtx, sessionID, hashAPIKey(apiKey)); err != nil {
					slog.Warn("session persist: RegisterSession failed", "session_id", sessionID, "err", err)
				}
			}()
		}

		// Auto-episode opt-in: start an episode only when ?auto_episode=1 was
		// present in the SSE URL. The flag is injected into the context by
		// applyMiddleware before the SSE handler calls RegisterSession (#356).
		if !autoEpisodeFlagFromContext(ctx) {
			return
		}
		h, err := s.pool.Get(ctx, "global")
		if err != nil {
			slog.Warn("auto-episode: pool.Get failed", "session_id", sessionID, "err", err)
			return
		}
		ep, err := h.Engine.Backend().StartEpisode(ctx, "global", "auto: "+time.Now().Format(time.RFC3339))
		if err != nil {
			slog.Warn("auto-episode: StartEpisode failed", "session_id", sessionID, "err", err)
			return
		}
		s.sessionEpisodes.Store(sessionID, ep.ID)
		metrics.EpisodesStartedTotal.Inc()
		slog.Info("auto-episode: started", "session_id", sessionID, "episode_id", ep.ID)
	})

	s.mcp.GetHooks().AddOnUnregisterSession(func(_ context.Context, sess server.ClientSession) {
		sessionID := sess.SessionID()
		s.sessionFingerprints.Delete(sessionID)

		// Remove from DB on clean disconnect (#362).
		if s.cfg.SessionDB != nil {
			go func() {
				dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := s.cfg.SessionDB.UnregisterSession(dbCtx, sessionID); err != nil {
					slog.Warn("session persist: UnregisterSession failed", "session_id", sessionID, "err", err)
				}
			}()
		}

		// Close any auto-started episode. Use context.Background() because the
		// session context is already cancelled when this hook fires (#356).
		epIDAny, ok := s.sessionEpisodes.LoadAndDelete(sessionID)
		if !ok {
			return
		}
		epID, _ := epIDAny.(string)
		if epID == "" {
			return
		}
		bg, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		h, err := s.pool.Get(bg, "global")
		if err != nil {
			slog.Warn("auto-episode: pool.Get failed on disconnect", "session_id", sessionID, "episode_id", epID, "err", err)
			return
		}
		if err := h.Engine.Backend().EndEpisode(bg, epID, "auto-closed: session ended"); err != nil {
			slog.Warn("auto-episode: EndEpisode failed", "session_id", sessionID, "episode_id", epID, "err", err)
			return
		}
		metrics.EpisodesEndedCleanTotal.Inc()
		slog.Info("auto-episode: closed", "session_id", sessionID, "episode_id", epID)
	})
}

// sweepStaleEpisodes runs a ticker every hour and calls runEpisodeSweep.
// It exits when ctx is cancelled or when EpisodeTTL == 0 (disabled).
func (s *Server) sweepStaleEpisodes(ctx context.Context) {
	if s.cfg.EpisodeTTL == 0 {
		return
	}
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runEpisodeSweep(ctx)
		}
	}
}

// runEpisodeSweep performs one sweep: closes all open episodes older than
// cfg.EpisodeTTL. Errors are logged but do not propagate — the sweep is
// best-effort. Uses a fresh background context with a 30 s timeout so that
// a cancelled server context does not abort the final reap.
func (s *Server) runEpisodeSweep(ctx context.Context) {
	sweepCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	h, err := s.pool.Get(sweepCtx, "global")
	if err != nil {
		slog.Warn("episode-sweep: pool.Get failed", "err", err)
		return
	}
	n, err := h.Engine.Backend().CloseStaleEpisodes(sweepCtx, s.cfg.EpisodeTTL)
	if err != nil {
		slog.Warn("episode-sweep: CloseStaleEpisodes failed", "err", err)
		return
	}
	if n > 0 {
		metrics.EpisodesEndedByReaperTotal.Add(float64(n))
		slog.Info("episode-sweep: closed stale episodes", "count", n, "ttl", s.cfg.EpisodeTTL)
	}
}

// Start begins serving SSE on host:port. Blocks until ctx is cancelled.
// baseURL overrides the URL advertised in SSE endpoint events; when empty,
// it defaults to http://<host>:<port>. Set this to the externally-reachable
// address (e.g. http://127.0.0.1:8788) when the bind address is 0.0.0.0,
// so MCP clients forward auth headers to the correct host.
func (s *Server) Start(ctx context.Context, host string, port int, apiKey string, baseURL string) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	slog.Info("engram MCP server starting", "addr", addr)

	advertised := baseURL
	if advertised == "" {
		advertised = fmt.Sprintf("http://%s", addr)
	}
	slog.Info("SSE base URL", "url", advertised)
	sse := server.NewSSEServer(s.mcp, server.WithBaseURL(advertised))

	s.registerSessionHooks(apiKey)

	// setupTokenLimiter enforces 3 calls per 5-minute window per IP (#243).
	// Uses a fixed budget regardless of RateLimitDisable — /setup-token is security-sensitive.
	setupLimiter := newRateLimiter(ctx) // eviction goroutine shares ctx lifetime

	// Top-level mux routes unauthenticated utility endpoints before auth middleware.
	mux := http.NewServeMux()

	// GET /health — unauthenticated; returns dependency status for diagnostics and readiness checks.
	// Probes PostgreSQL (SELECT 1) and Ollama (/api/tags) with 2-second deadlines each.
	// Returns 200 {"status":"ok",...} when all probes pass; 200 {"status":"degraded","ollama":"degraded",...}
	// when Ollama was unavailable at startup (EmbedDegraded=true); 503 {"status":"degraded",...} when
	// a previously-healthy dependency is now unreachable.
	mux.HandleFunc("/health", s.handleHealth)

	// GET /metrics — unauthenticated Prometheus metrics endpoint.
	mux.Handle("/metrics", promhttp.Handler())

	// GET /setup-token — local-network only; returns the current bearer token so MCP
	// clients can self-configure without manual copy-paste.
	//
	// Security rationale: the Docker port mapping `127.0.0.1:8788->8788/tcp` already
	// restricts external access at the host-network level. Inside the container, requests
	// arriving from the host appear as Docker gateway IPs (172.x.x.x, 10.x.x.x) rather
	// than 127.0.0.1 due to NAT. We accept RFC1918 addresses because they can only reach
	// this port via the loopback-bound Docker port mapping — not from the network.
	// The token is equivalent in sensitivity to ~/.claude.json which is already on disk.
	//
	// Rate limit: 3 calls per 5-minute window per IP (#243).
	mux.HandleFunc("/setup-token", func(w http.ResponseWriter, r *http.Request) {
		ip := s.clientIP(r)
		if !isLocalAddress(ip, s.cfg.AllowRFC1918SetupToken) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if !setupLimiter.allowSetupToken(ip) {
			slog.Warn("setup-token rate limited", "remote_ip", ip)
			w.Header().Set("Retry-After", "100")
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		slog.Warn("setup-token accessed", "remote_ip", ip)
		writeJSON(w, http.StatusOK, map[string]string{
			"token":    apiKey,
			"endpoint": advertised + "/sse",
			"name":     "engram",
		})
	})

	// GET /.well-known/oauth-authorization-server and /.well-known/oauth-protected-resource —
	// Return 404 to tell Claude Code this server does not use MCP OAuth (spec 2025-03-26).
	// Without these handlers, the catch-all auth middleware returns 401, which Claude Code
	// misinterprets as "OAuth required" and shows a "needs authentication" dialog every session.
	notFound := func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}
	mux.HandleFunc("/.well-known/oauth-authorization-server", notFound)
	mux.HandleFunc("/.well-known/oauth-protected-resource", notFound)

	// All other routes require Bearer authentication and per-IP rate limiting (#140).
	// The SSE and message endpoints are mounted separately so that the /message
	// handler can be wrapped with session-fingerprint verification (#245).
	// Use the configured RPS/burst from the server config (#387).
	rl := newRateLimiterWithConfig(ctx, s.cfg.rateLimitRPS(), s.cfg.rateLimitBurst())
	if s.cfg.RateLimitDisable {
		slog.Info("HTTP rate limiter disabled via ENGRAM_RATE_LIMIT_DISABLE (#387)")
	} else {
		slog.Info("HTTP rate limiter active", "rps", s.cfg.rateLimitRPS(), "burst", s.cfg.rateLimitBurst())
	}

	// /message — wrap with session-fingerprint check before the auth middleware.
	msgHandler := s.withSessionFingerprint(sse.MessageHandler(), apiKey)
	mux.Handle("/message", s.applyMiddleware(msgHandler, apiKey, rl))

	// /quick-store — sessionless REST endpoint for hook scripts and CLI callers
	// that cannot establish an SSE session (e.g. PreCompact hooks).
	mux.Handle("/quick-store", s.applyMiddleware(http.HandlerFunc(s.handleQuickStore), apiKey, rl))

	// /quick-recall — sessionless REST endpoint for reading memories without an
	// active SSE session (e.g. Python subprocesses in the Clearwatch pipeline).
	mux.Handle("/quick-recall", s.applyMiddleware(http.HandlerFunc(s.handleQuickRecall), apiKey, rl))

	// All other authenticated routes (including /sse) go through the standard middleware.
	mux.Handle("/", s.applyMiddleware(sse, apiKey, rl))

	// Background sweeper closes crash-orphaned open episodes on an hourly interval.
	go s.sweepStaleEpisodes(ctx)

	// Background sweeper clears stale upload sessions on a 5-minute interval.
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				s.uploadMu.Lock()
				s.evictExpiredUploadsLocked(now)
				s.uploadMu.Unlock()
			}
		}
	}()

	const maxRequestBodyBytes = 2 * 1024 * 1024 // 2 MiB (#6)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           http.MaxBytesHandler(mux, maxRequestBodyBytes),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	slog.Info("engram ready — to configure MCP client run: make setup  (from host machine, inside ~/projects/engram-go; or: go run ./cmd/engram-setup)")

	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.ListenAndServe() }()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}

// applyMiddleware chains per-IP rate limiting (#140) and Bearer auth onto next.
// Rate limiting is skipped when cfg.RateLimitDisable is true, or when the
// remote IP is a loopback address (127.0.0.1 / ::1) — a local machine is not
// a DDoS threat and should never receive 429s from its own process (#387).
func (s *Server) applyMiddleware(next http.Handler, apiKey string, rl *rateLimiter) http.Handler {
	return s.applyMiddlewareWithRL(next, apiKey, rl)
}

// applyMiddlewareWithRL is the testable core of applyMiddleware. It accepts
// an explicit *rateLimiter so unit tests can inject a pre-configured limiter
// without starting a real HTTP server.
func (s *Server) applyMiddlewareWithRL(next http.Handler, apiKey string, rl *rateLimiter) http.Handler {
	// apiKey is always non-empty — enforced by main.go startup check.
	// This guard is a defence-in-depth backstop; it must never be the primary gate.
	if apiKey == "" {
		panic("engram: auth middleware called with empty apiKey — programming error")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Attach a short request correlation ID to the context for log threading (#320).
		requestID := fmt.Sprintf("%x", time.Now().UnixNano())[:12]
		ctx := context.WithValue(r.Context(), requestIDKey, requestID)

		// Inject the auto-episode flag when ?auto_episode=1 is present in the URL.
		// This is read by the OnRegisterSession hook to decide whether to start an
		// episode automatically for the connecting client session (#356).
		if r.URL.Query().Get("auto_episode") == "1" {
			ctx = withAutoEpisodeFlag(ctx)
		}
		r = r.WithContext(ctx)

		// Rate limit before auth check to prevent timing-based enumeration.
		// Exempt loopback IPs (127.0.0.1, ::1) — a local process is not a threat.
		// Also exempt entirely when RateLimitDisable=true (#387).
		remoteHost := s.clientIP(r)
		if !s.cfg.RateLimitDisable && !isLoopbackIP(remoteHost) {
			if !rl.allow(remoteHost) {
				w.Header().Set("Retry-After", "1")
				writeJSON(w, http.StatusTooManyRequests, map[string]string{
					"error": "rate_limited",
					"hint":  "too many requests — back off and retry",
				})
				return
			}
		}
		// ConstantTimeCompare leaks length when len(got) != len(want).
		// Use ConstantTimeEq on the HMAC of each side so the comparison is
		// always the same length regardless of input length (#129).
		got := hmac.New(sha256.New, []byte(apiKey))
		got.Write([]byte(r.Header.Get("Authorization")))
		want := hmac.New(sha256.New, []byte(apiKey))
		want.Write([]byte("Bearer " + apiKey))
		if subtle.ConstantTimeCompare(got.Sum(nil), want.Sum(nil)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "unauthorized",
				"hint":  "Bearer token mismatch — on the host machine run: cd ~/projects/engram-go && make setup  (or: go run ./cmd/engram-setup). Then run /mcp in Claude Code to reconnect.",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isLoopbackIP returns true when ip is the IPv4 loopback address (127.0.0.1)
// or the IPv6 loopback address (::1). These are auto-exempt from rate limiting
// because they can only originate from the same host process (#387).
func isLoopbackIP(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil && parsed.IsLoopback()
}

// withSessionFingerprint wraps next with a check that the sessionId query
// parameter was established by the same bearer token that is presenting the
// request (#245). The fingerprint is HMAC(apiKey, sessionID) stored at SSE
// connection time. Any mismatch returns 403.
func (s *Server) withSessionFingerprint(next http.Handler, apiKey string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.URL.Query().Get("sessionId")
		if sessionID == "" {
			// Let the underlying handler produce the canonical error.
			next.ServeHTTP(w, r)
			return
		}
		stored, ok := s.sessionFingerprints.Load(sessionID)
		if !ok {
			// Session not found — let the underlying handler produce the error.
			next.ServeHTTP(w, r)
			return
		}

		// Update last_seen_at so the session remains within the rehydration window
		// even for long-lived connections that exceed the registration timestamp (#362).
		if s.cfg.SessionDB != nil {
			go func() {
				dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := s.cfg.SessionDB.TouchSession(dbCtx, sessionID); err != nil {
					slog.Debug("session touch failed", "session_id", sessionID, "err", err)
				}
			}()
		}

		storedFP, _ := stored.([]byte)

		// Compute the expected fingerprint for the current bearer.
		mac := hmac.New(sha256.New, []byte(apiKey))
		mac.Write([]byte(sessionID))
		expected := mac.Sum(nil)

		if subtle.ConstantTimeCompare(storedFP, expected) != 1 {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "forbidden",
				"hint":  "session bearer mismatch",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isLocalAddress reports whether ipStr is a loopback address.
// When allowRFC1918 is true, private RFC1918 ranges are also accepted — required
// for Docker setups where the host appears as a bridge IP (172.x, 10.x, 192.168.x)
// rather than 127.0.0.1 due to NAT. Enable via ENGRAM_SETUP_TOKEN_ALLOW_RFC1918=1.
func isLocalAddress(ipStr string, allowRFC1918 bool) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	local := []string{
		"127.0.0.0/8", // loopback
		"::1/128",     // IPv6 loopback
	}
	if allowRFC1918 {
		local = append(local,
			"10.0.0.0/8",     // RFC1918 — Docker bridge networks
			"172.16.0.0/12",  // RFC1918 — Docker default bridge
			"192.168.0.0/16", // RFC1918 — Docker custom networks
		)
	}
	for _, cidr := range local {
		_, n, _ := net.ParseCIDR(cidr)
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// clientIP extracts the real client IP from the request.
// When s.trustProxy is true, it checks X-Real-IP and the first entry in
// X-Forwarded-For before falling back to RemoteAddr. This handles the case
// where engram runs behind a reverse proxy (#255).
// ENGRAM_TRUST_PROXY_HEADERS=1 (or =true) enables proxy header trust.
func (s *Server) clientIP(r *http.Request) string {
	if s.trustProxy {
		// Validate X-Real-IP with net.ParseIP to prevent header spoofing (#290).
		// An attacker who submits X-Real-IP: 127.0.0.1 could otherwise bypass the
		// /setup-token loopback check when ENGRAM_TRUST_PROXY_HEADERS=1.
		if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
			if net.ParseIP(realIP) != nil {
				return realIP
			}
			// Invalid IP in X-Real-IP — fall through to X-Forwarded-For.
		}
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			// X-Forwarded-For may be a comma-separated list; leftmost is the client.
			parts := strings.SplitN(forwarded, ",", 2)
			if ip := strings.TrimSpace(parts[0]); ip != "" && net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

// toolHandler is the unified handler signature for all registered MCP tools.
// Handlers that don't use cfg accept it with a blank identifier.
type toolHandler func(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error)

// noConfig adapts a pool-only handler (no cfg parameter) to toolHandler.
func noConfig(h func(context.Context, *EnginePool, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)) toolHandler {
	return func(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, _ Config) (*mcpgo.CallToolResult, error) {
		return h(ctx, pool, req)
	}
}

// withEntityEnqueue wraps handleMemoryStore to async-enqueue entity extraction on success.
func withEntityEnqueue(h func(context.Context, *EnginePool, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)) toolHandler {
	return func(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, _ Config) (*mcpgo.CallToolResult, error) {
		result, err := h(ctx, pool, req)
		if err != nil {
			slog.Warn("memory_store: store failed", "err", err, "request_id", requestIDFromContext(ctx))
			return result, err
		}
		// Enqueue entity extraction asynchronously. Runs in a detached goroutine
		// with its own context so it never blocks memory_store.
		// Non-fatal: if the enqueue fails the store has already succeeded.
		args := req.GetArguments()
		project, err := getProject(args, "default")
		if err != nil {
			return nil, err
		}
		if memID, ok := extractResultID(result); ok {
			slog.Debug("memory_store: stored, enqueuing extraction",
				"id", memID, "project", project, "request_id", requestIDFromContext(ctx))
			go enqueueExtractionAsync(pool, memID, project)
		}
		return result, nil
	}
}

// withWarnLog wraps a handler to emit slog.Warn when it returns an error.
func withWarnLog(name string, h toolHandler) toolHandler {
	return func(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
		result, err := h(ctx, pool, req, cfg)
		if err != nil {
			slog.Warn(name+": failed", "err", err, "request_id", requestIDFromContext(ctx))
		}
		return result, err
	}
}

// defaultToolTimeout is the per-call deadline applied at the MCP dispatch layer.
// Prevents silent hangs up to the HTTP server WriteTimeout (90s) when a handler
// blocks on Ollama, a slow DB query, or an unguarded third-party call. (#379)
//
// All synchronous handlers return well under 10s in normal operation; 15s gives
// headroom for transient slowness without ever approaching the 90s HTTP timeout.
// Tools that trigger background work (migrate_embedder, consolidate) also return
// quickly — the heavy lifting happens in background goroutines, not the handler.
const defaultToolTimeout = 15 * time.Second

// registerToolWithTimeout adds a tool to the MCP server with a per-call deadline
// and Prometheus instrumentation. timeout=0 uses defaultToolTimeout.
func (s *Server) registerToolWithTimeout(name, desc string, h toolHandler, timeout time.Duration) {
	if timeout == 0 {
		timeout = defaultToolTimeout
	}
	pool, cfg := s.pool, s.cfg
	toolName := name
	toolTimeout := timeout
	s.mcp.AddTool(mcpgo.NewTool(name, mcpgo.WithDescription(desc)),
		func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
			ctx, cancel := context.WithTimeout(ctx, toolTimeout)
			defer cancel()

			timer := prometheus.NewTimer(metrics.ToolDuration.WithLabelValues(toolName))
			defer timer.ObserveDuration()

			result, err := h(ctx, pool, req, cfg)
			if err != nil && ctx.Err() == context.DeadlineExceeded {
				slog.Warn("mcp tool timed out", "tool", toolName, "timeout", toolTimeout)
				metrics.ToolRequests.WithLabelValues(toolName, "timeout").Inc()
				return &mcpgo.CallToolResult{
					IsError: true,
					Content: []mcpgo.Content{
						mcpgo.TextContent{
							Type: "text",
							Text: toolName + " timed out after " + toolTimeout.String() + " — Ollama may be slow or unavailable",
						},
					},
				}, nil
			}
			status := "ok"
			if err != nil || (result != nil && result.IsError) {
				status = "error"
			}
			metrics.ToolRequests.WithLabelValues(toolName, status).Inc()
			return result, err
		})
}

// registerTool adds a tool with the default dispatch timeout.
func (s *Server) registerTool(name, desc string, h toolHandler) {
	s.registerToolWithTimeout(name, desc, h, 0)
}

func (s *Server) registerTools() {
	type toolDef struct {
		name    string
		desc    string
		handler toolHandler
	}
	tools := []toolDef{
		// Core store operations
		{"memory_store", "Store a focused memory (<=10k chars)",
			withEntityEnqueue(handleMemoryStore)},
		{"memory_store_document", "Store a large document (auto-tiered up to 50 MB via synopsis + raw blob storage)",
			handleMemoryStoreDocument},
		{"memory_ingest_document_stream", "Ingest a very large document via server-local path or chunked base64 upload (auto-tiered, up to 50 MB)",
			func(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
				return handleMemoryIngestDocumentStream(ctx, s, pool, req, cfg)
			}},
		{"memory_store_batch", "Store multiple memories in one call",
			noConfig(handleMemoryStoreBatch)},
		// Recall and retrieval
		{"memory_recall", "Recall memories by semantic + full-text query",
			withWarnLog("memory_recall", handleMemoryRecall)},
		{"memory_fetch", "Fetch a single memory by ID; detail=summary|chunk|full",
			handleMemoryFetch},
		{"memory_list", "List memories with optional filters",
			noConfig(handleMemoryList)},
		{"memory_history", "Return the full version chain for a memory",
			noConfig(handleMemoryHistory)},
		{"memory_timeline", "Recall memories that were active at a given point in time (as_of param, RFC3339)",
			noConfig(handleMemoryTimeline)},
		// Graph operations
		{"memory_connect", "Create a directed relationship between two memories. relation_type values: caused_by, relates_to, depends_on, supersedes, used_in, resolved_by, contradicts, supports, derived_from, part_of, follows",
			noConfig(handleMemoryConnect)},
		{"memory_expand", "Explore the relationship graph neighbourhood of a known memory.",
			noConfig(handleMemoryExpand)},
		// Mutations
		{"memory_correct", "Update content, tags, or importance on an existing memory",
			noConfig(handleMemoryCorrect)},
		{"memory_forget", "Soft-delete a memory (sets valid_to, preserves history, respects immutability)",
			noConfig(handleMemoryForget)},
		// Maintenance
		{"memory_summarize", "Immediately summarize a memory",
			handleMemorySummarize},
		{"memory_resummarize", "Clear all summaries for a project — they regenerate automatically within 60s",
			noConfig(handleMemoryResummarize)},
		{"memory_status", "Return project statistics",
			noConfig(handleMemoryStatus)},
		{"memory_verify", "Integrity check -- hash coverage and corrupt count",
			noConfig(handleMemoryVerify)},
		// Feedback and aggregation
		{"memory_feedback", "Record retrieval feedback. failure_class values (for misses): vocabulary_mismatch, aggregation_failure, stale_ranking, missing_content, scope_mismatch, other",
			noConfig(handleMemoryFeedback)},
		{"memory_aggregate", "Group and count memories. by=tag|type|failure_class. filter: optional ILIKE substring — tag mode only, error for failure_class.",
			noConfig(handleMemoryAggregate)},
		// Consolidation
		{"memory_consolidate", "Prune stale memories, decay edges, merge near-duplicates",
			handleMemoryConsolidate},
		{"memory_sleep", "Run full sleep-consolidation cycle: infer relationships between semantically related memories",
			handleMemorySleep},
		{"memory_delete_project", "Delete all memories and project data for an eval isolation project. Not for normal use.",
			noConfig(handleMemoryDeleteProject)},
		// Episodes
		{"memory_episode_start", "Start a named episode to group memories from this session",
			withWarnLog("memory_episode_start", noConfig(handleMemoryEpisodeStart))},
		{"memory_episode_end", "End an episode with an optional summary",
			noConfig(handleMemoryEpisodeEnd)},
		{"memory_episode_list", "List recent episodes for a project",
			noConfig(handleMemoryEpisodeList)},
		{"memory_episode_recall", "Return all memories from a specific episode in chronological order",
			noConfig(handleMemoryEpisodeRecall)},
		// Embedder management
		{"memory_migrate_embedder", "Switch embedding model; triggers background re-embedding. Also resets any learned adaptive weights for the project to compile-time defaults.",
			handleMemoryMigrateEmbedder},
		{"memory_models", "List installed and suggested Ollama embedding models. Shows which suggested models are installed, which is current, and flags the recommended upgrade.",
			handleMemoryModels},
		{"memory_embedding_eval", "Compare two Ollama embedding models using probe sentences. model_a defaults to the configured embedding model; model_b defaults to the recommended registry entry. Use this to validate a 1024-dim compatible replacement before migrating. Auto-pulls missing models. Read-only — does not migrate stored embeddings.",
			handleMemoryEmbeddingEval},
		// Import / export
		{"memory_export_all", "Export all memories to markdown files",
			handleMemoryExportAll},
		{"memory_import_claudemd", "Import a CLAUDE.md file as structured memories",
			handleMemoryImportClaudeMD},
		{"memory_ingest", "Ingest a file or directory as document memories",
			handleMemoryIngest},
		{"memory_ingest_export",
			"Ingest a server-local AI conversation export file (Slack workspace .zip, Claude.ai conversations.json, or ChatGPT conversations.json). Parses the file, auto-detects format, and stores one memory per conversation or channel.",
			handleMemoryIngestExport},
		{"memory_ingest_status",
			"Check the status of an async ingestion job queued by memory_ingest_export, memory_ingest, or memory_ingest_document_stream.",
			func(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
				args := req.GetArguments()
				jobID := getString(args, "job_id", "")
				if jobID == "" {
					return mcpgo.NewToolResultError("job_id is required"), nil
				}
				if cfg.IngestQueue == nil {
					return toolResult(map[string]any{"status": "unavailable", "message": "async queue not enabled"})
				}
				r := cfg.IngestQueue.Status(jobID)
				if r == nil {
					return toolResult(map[string]any{"status": "unknown", "job_id": jobID})
				}
				out := map[string]any{"job_id": r.JobID, "status": string(r.Status)}
				if r.Error != "" {
					out["error"] = r.Error
				}
				if !r.StartedAt.IsZero() {
					out["started_at"] = r.StartedAt.Format(time.RFC3339)
				}
				if !r.DoneAt.IsZero() {
					out["done_at"] = r.DoneAt.Format(time.RFC3339)
					out["duration_ms"] = r.DoneAt.Sub(r.StartedAt).Milliseconds()
				}
				return toolResult(out)
			}},
		// Cross-project federation
		{"memory_projects", "List all projects with memory counts",
			noConfig(handleMemoryProjects)},
		{"memory_adopt", "Create a cross-project reference relationship",
			noConfig(handleMemoryAdopt)},
		// Simplified front-door tools
		{"memory_quick_store", "Store a memory and automatically extract entities. Simplified front door for memory_store.",
			withWarnLog("memory_quick_store", noConfig(handleMemoryQuickStore))},
		{"memory_query", "Simplified front door for memory_recall. Accepts a 'limit' param instead of top_k; sensible defaults applied.",
			handleMemoryQuery},
		// Safety constraint verification
		{"get_constraints", "List constraint and policy memories relevant to an optional query",
			noConfig(handleGetConstraints)},
		{"check_constraints", "Classify a proposed action and return matching constraints with a verification decision",
			noConfig(handleCheckConstraints)},
		{"verify_before_acting", "Run the full constraint verification pipeline and return a proceed/warn/require_approval/block decision",
			noConfig(handleVerifyBeforeActing)},
		// Audit and weight tuning
		{"memory_audit_add_query", "Register a canonical query for retrieval drift monitoring",
			handleMemoryAuditAddQuery},
		{"memory_audit_list_queries", "List canonical queries registered for drift monitoring in a project",
			handleMemoryAuditListQueries},
		{"memory_audit_deactivate_query", "Deactivate a canonical query (stops future drift snapshots)",
			handleMemoryAuditDeactivateQuery},
		{"memory_audit_run", "Run a decay audit pass for a project immediately and return snapshot summaries",
			handleMemoryAuditRun},
		{"memory_audit_compare", "Compare retrieval snapshots for a canonical query to detect ranking drift",
			handleMemoryAuditCompare},
		{"memory_weight_history", "Return current retrieval weights and tuning history for a project",
			handleMemoryWeightHistory},
		// Diagnose (always available — no Claude required)
		{"memory_diagnose", "Return evidence map for recalled memories: conflicts, confidence, invalidated sources — no synthesis",
			noConfig(handleMemoryDiagnose)},
	}
	for _, t := range tools {
		s.registerTool(t.name, t.desc, t.handler)
	}

	// Claude-required tools: registered only when a client is available.
	if s.cfg.ClaudeEnabled {
		s.registerTool("memory_reason",
			"Recall memories and synthesize a grounded answer using Claude",
			handleMemoryReason)
		s.registerTool("memory_explore",
			"Iterative recall+score+synthesis loop — returns a single grounded answer (A3)",
			handleMemoryExplore)
		// memory_query_document (A5): query a single large document by regex/substring
		// or semantic recall and synthesize an answer with Claude.
		s.registerTool("memory_query_document",
			"Query a large document stored in memory using regex/substring matching or semantic search. Returns relevant spans and an AI-synthesized answer.",
			handleMemoryQueryDocument)
		// memory_ask (P2): retrieval-augmented question answering with numbered citations.
		s.registerTool("memory_ask",
			"Answer a question using stored memories as context. Returns answer + numbered citations.",
			handleMemoryAsk)
	}
}

// handleHealth checks PostgreSQL and Ollama reachability and returns a structured
// JSON response. Both probes use a 2-second deadline.
//
// Status semantics:
//   - 200 {"status":"ok",...}      — all probes pass.
//   - 200 {"status":"degraded",...} — Ollama failed at startup (EmbedDegraded=true)
//     and the live probe also fails. The server is operational; embeddings are
//     unavailable until Ollama recovers.
//   - 503 {"status":"degraded",...} — a probe that was healthy at startup is now
//     unreachable (Postgres failure, or Ollama failure when EmbedDegraded=false).
//
// Unauthenticated — suitable as a K8s readiness probe.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	type result struct {
		Status   string `json:"status"`
		Postgres string `json:"postgres"`
		Ollama   string `json:"ollama"`
	}

	pgStatus := "ok"
	ollamaStatus := "ok"

	// Probe PostgreSQL. Use context.Background() so a short-deadline HTTP client
	// cannot cause a false 503 — the probe needs its own independent 2s window.
	if s.cfg.PgPool != nil {
		pgCtx, pgCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer pgCancel()
		if err := s.cfg.PgPool.Ping(pgCtx); err != nil {
			pgStatus = "error"
			slog.Warn("health: postgres probe failed", "err", err)
		}
	} else {
		// No shared pool configured — skip the probe but do not report degraded.
		// This happens in test environments that construct Server without a PgPool.
		pgStatus = "ok"
	}

	// Probe LiteLLM via GET /v1/models (OpenAI-compatible, available on all deployments).
	embedLiveOK := false
	if s.cfg.LiteLLMURL != "" {
		embedCtx, embedCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer embedCancel()
		modelsURL := strings.TrimRight(s.cfg.LiteLLMURL, "/") + "/v1/models"
		req, err := http.NewRequestWithContext(embedCtx, http.MethodGet, modelsURL, nil)
		if err == nil {
			resp, herr := http.DefaultClient.Do(req)
			if herr != nil {
				ollamaStatus = "error"
				slog.Warn("health: litellm probe failed", "err", herr)
			} else {
				resp.Body.Close()
				if resp.StatusCode >= 500 {
					ollamaStatus = "error"
					slog.Warn("health: litellm returned server error", "status", resp.StatusCode)
				} else {
					embedLiveOK = true
				}
			}
		} else {
			ollamaStatus = "error"
			slog.Warn("health: could not build litellm probe request", "err", err)
		}
	}

	// When EmbedDegraded is set, LiteLLM was unavailable at startup but the server
	// started anyway. If LiteLLM has since recovered, promote to "ok". If still
	// unreachable, report "degraded" — the server is operational, BM25+recency active.
	if s.cfg.EmbedDegraded {
		if embedLiveOK {
			ollamaStatus = "ok"
		} else {
			ollamaStatus = "degraded"
		}
	}

	res := result{
		Status:   "ok",
		Postgres: pgStatus,
		Ollama:   ollamaStatus,
	}
	statusCode := http.StatusOK
	// Return 503 only for hard failures: Postgres down, or LiteLLM down when it was
	// healthy at startup (not in degraded mode). A degraded LiteLLM (startup miss)
	// keeps 200 so K8s readiness probes do not kill a running server.
	if pgStatus != "ok" || (ollamaStatus == "error") {
		res.Status = "degraded"
		statusCode = http.StatusServiceUnavailable
	} else if ollamaStatus == "degraded" {
		res.Status = "degraded"
		// statusCode stays 200 — server is operational
	}

	writeJSON(w, statusCode, res)
}

// extractionSem caps the number of concurrent entity-extraction goroutines.
// At 50 req/s burst each spawning a goroutine, an unbounded pool exhausts the
// per-project pgxpool (MaxConns=10) within the 5-second async timeout window.
// Non-blocking select: when the semaphore is full the goroutine exits immediately
// rather than queuing, keeping goroutine count bounded.
var extractionSem = make(chan struct{}, 20)

// enqueueExtractionAsync submits memID to the entity extraction queue via pool.
// Intended to run in a detached goroutine; all failures are logged, never surfaced.
// Bounded by extractionSem: if more than 20 extraction goroutines are already
// running, this call is dropped and a warning is logged.
func enqueueExtractionAsync(pool *EnginePool, memID, project string) {
	select {
	case extractionSem <- struct{}{}:
		// acquired — proceed
	default:
		// semaphore full; skip this extraction rather than queuing unboundedly
		slog.Warn("memory_store: entity extraction skipped, semaphore full",
			"id", memID, "project", project)
		return
	}
	defer func() { <-extractionSem }()

	// No request context available here — this runs in a detached goroutine
	// after the store has already returned. Use background context.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h, herr := pool.Get(ctx, project)
	if herr != nil {
		slog.Warn("memory_store: enqueue pool.Get failed",
			"id", memID, "project", project, "err", herr)
		return
	}
	if eerr := h.Engine.Backend().EnqueueExtractionJob(ctx, memID, project); eerr != nil {
		slog.Warn("memory_store: enqueue extraction job failed",
			"id", memID, "project", project, "err", eerr)
	}
}

// handleQuickStore is a sessionless REST endpoint that stores a single memory
// without requiring an active SSE session. Used by hook scripts (e.g. PreCompact)
// and CLI callers that cannot perform the SSE handshake.
//
// POST /quick-store
// Authorization: Bearer <token>
// {"content":"...","project":"...","tags":[...],"importance":N}.
func (s *Server) handleQuickStore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Content    string   `json:"content"`
		Project    string   `json:"project"`
		Tags       []string `json:"tags"`
		Importance int      `json:"importance"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(body.Content) == "" {
		http.Error(w, "content is required", http.StatusBadRequest)
		return
	}

	args := map[string]any{"content": body.Content}
	if body.Project != "" {
		args["project"] = body.Project
	}
	if len(body.Tags) > 0 {
		tags := make([]any, len(body.Tags))
		for i, tag := range body.Tags {
			tags[i] = tag
		}
		args["tags"] = tags
	}
	if body.Importance != 0 {
		args["importance"] = float64(body.Importance)
	}

	var req mcpgo.CallToolRequest
	req.Params.Arguments = args

	result, err := handleMemoryQuickStore(r.Context(), s.pool, req)
	if err != nil {
		slog.Error("quick-store failed", "err", err, "request_id", requestIDFromContext(r.Context()))
		writeJSONError(w, http.StatusInternalServerError, "store failed")
		return
	}
	if result.IsError {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false})
		return
	}

	var id string
	for _, c := range result.Content {
		if tc, ok := c.(mcpgo.TextContent); ok {
			var m map[string]any
			if json.Unmarshal([]byte(tc.Text), &m) == nil {
				if v, ok := m["id"].(string); ok {
					id = v
				}
			}
			break
		}
	}
	if id == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"ok": false, "error": "id extraction failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

// handleQuickRecall is a sessionless REST endpoint that recalls memories by
// semantic + full-text query without requiring an active SSE session. Used by
// Python subprocesses and other callers (e.g. the Clearwatch pipeline) that
// cannot perform the SSE handshake.
//
// POST /quick-recall
// Authorization: Bearer <token>
// {"query":"...","project":"...","tags":[...],"limit":N}
//
// Returns {"results":[{"id":"...","summary":"...","content":"...","tags":[...],"score":N},...]}
// On no results returns {"results":[]}, never an error.
func (s *Server) handleQuickRecall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Query   string   `json:"query"`
		Project string   `json:"project"`
		Tags    []string `json:"tags"`
		Limit   int      `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(body.Project) == "" {
		writeJSONError(w, http.StatusBadRequest, "project is required")
		return
	}
	if strings.TrimSpace(body.Query) == "" {
		writeJSONError(w, http.StatusBadRequest, "query is required")
		return
	}

	// Clamp limit: default 5, max 20.
	limit := body.Limit
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}

	args := map[string]any{
		"project": body.Project,
		"query":   body.Query,
		"top_k":   float64(limit),
		// Force full-results mode so the response contains a "results" key with
		// memory objects rather than opaque handles (the server default is "handle").
		"mode": "summary",
	}
	if len(body.Tags) > 0 {
		tags := make([]any, len(body.Tags))
		for i, tag := range body.Tags {
			tags[i] = tag
		}
		args["tags"] = tags
	}

	var req mcpgo.CallToolRequest
	req.Params.Arguments = args

	result, err := handleMemoryRecall(r.Context(), s.pool, req, s.cfg)
	if err != nil {
		slog.Error("quick-recall failed", "err", err, "request_id", requestIDFromContext(r.Context()))
		writeJSONError(w, http.StatusInternalServerError, "recall failed")
		return
	}

	// Extract the JSON payload from the tool result text.
	var raw map[string]any
	for _, c := range result.Content {
		if tc, ok := c.(mcpgo.TextContent); ok {
			if jsonErr := json.Unmarshal([]byte(tc.Text), &raw); jsonErr == nil {
				break
			}
		}
	}

	// Map full SearchResult slice to the slim wire format the caller expects.
	// Graceful degradation: if we can't parse the results, return empty array.
	type wireResult struct {
		ID      string   `json:"id"`
		Summary string   `json:"summary"`
		Content string   `json:"content"`
		Tags    []string `json:"tags"`
		Score   float64  `json:"score"`
	}

	out := make([]wireResult, 0)
	if rawResults, ok := raw["results"]; ok {
		if arr, ok := rawResults.([]any); ok {
			for _, item := range arr {
				obj, ok := item.(map[string]any)
				if !ok {
					continue
				}
				mem, _ := obj["memory"].(map[string]any)
				score, _ := obj["score"].(float64)

				var id, summary, content string
				var tags []string
				if mem != nil {
					id, _ = mem["id"].(string)
					summary, _ = mem["summary"].(string)
					content, _ = mem["content"].(string)
					if rawTags, ok := mem["tags"].([]any); ok {
						for _, t := range rawTags {
							if s, ok := t.(string); ok {
								tags = append(tags, s)
							}
						}
					}
				}
				if tags == nil {
					tags = []string{}
				}
				out = append(out, wireResult{
					ID:      id,
					Summary: summary,
					Content: content,
					Tags:    tags,
					Score:   score,
				})
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"results": out})
}
