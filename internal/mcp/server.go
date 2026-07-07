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
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/time/rate"

	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/metrics"
)

// serverPhase constants track the startup lifecycle of the Server.
// The phase advances from starting → warming → warm as pool pre-warming completes.
// /ready returns 503 until phaseWarm is reached.
const (
	phaseStarting int32 = 0
	phaseWarming  int32 = 1
	phaseWarm     int32 = 2
)

const restEndpointRootHint = "REST endpoints (/quick-store,/atoms,/quick-recall) are at the server root, not under /mcp"

// Exported aliases for use by main.go and other external callers that need to
// advance the server phase without accessing unexported fields directly.
const (
	PhaseStarting = phaseStarting
	PhaseWarming  = phaseWarming
	PhaseWarm     = phaseWarm
)

// SetPhase advances the server startup phase. Called by main.go during startup
// to mark progression from starting → warming → warm. Thread-safe.
func (s *Server) SetPhase(phase int32) {
	s.serverPhase.Store(phase)
}

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

// RuntimeConfig holds feature flags and settings that can be reloaded at runtime via SIGHUP (#557).
// All fields are atomic to support zero-copy reads from tool handlers without locks.
type RuntimeConfig struct {
	ClaudeSummarize   atomic.Bool
	ClaudeConsolidate atomic.Bool
	ClaudeRerank      atomic.Bool
	LogLevel          atomic.Int32 // 0=debug, 1=info, 2=warn, 3=error
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

	// embedderHealth probes the configured LiteLLM embedder and caches the result
	// for 5 seconds. Used by memory store/recall tools to surface a degraded field
	// on tool responses when the embedder is unavailable.
	embedderHealth *EmbedderHealth

	// toolAnnotations records the MCP ToolAnnotation set on each registered tool.
	// Populated as a side-effect of registerToolWithTimeout. Read by the startup
	// banner (recommended permissions snippet) and by tests to verify that
	// read-only tools carry ReadOnlyHint=true so client-side gating (e.g. Claude
	// Code's plan mode) does not block them.
	toolAnnotations map[string]mcpgo.ToolAnnotation

	// toolDescriptions records the description string passed to each registered
	// tool. Populated alongside toolAnnotations for tests that inspect descriptions
	// (e.g. verifying the embed-degraded suffix is applied — pillar 1C, #611).
	toolDescriptions map[string]string

	// sessionTouchMu and sessionTouchTimes track the last time each session was
	// touched to implement coalescing: at most one DB write per session per 30s.
	// This prevents unbounded goroutine spawning when the same session makes
	// multiple requests in quick succession (#553).
	sessionTouchMu    sync.Mutex
	sessionTouchTimes map[string]time.Time // sessionID -> last TouchSession time

	// embedDegraded is an atomic flag that tracks whether embedding is currently
	// degraded. Unlike cfg.EmbedDegraded (set once at startup), this value is
	// updated by the /health probe loop every 30s so it can flip back to false
	// when LiteLLM recovers (#565).
	embedDegraded *atomic.Bool

	// runtimeCfg holds feature flags that can be reloaded at runtime via SIGHUP (#557).
	// Tool handlers read these without locking to avoid contention.
	runtimeCfg *RuntimeConfig

	// apiKey is the Bearer token required to authenticate requests. Set once by
	// Start() and never modified afterward. Used by handleHealth to gate the
	// detailed topology response from unauthenticated callers (#1210).
	apiKey string

	// tofuGranted tracks whether the one-time localhost bootstrap grant for /setup-token
	// has been issued (#613). Zero value (false) is the correct initial state — no allocation
	// needed. CompareAndSwap ensures exactly one unauthenticated loopback request succeeds.
	tofuGranted atomic.Bool
	// tofuGrantedAt records when the one-time localhost bootstrap grant was
	// issued. It remains for timing/observability and tests. atomic.Int64 holds
	// unix epoch seconds.
	tofuGrantedAt atomic.Int64

	// serverPhase tracks startup lifecycle: 0=starting, 1=warming, 2=warm.
	// /ready returns 503 until phaseWarm is stored. Advanced by the pre-warm goroutine
	// in main.go after pool.WarmProjects completes.
	serverPhase atomic.Int32
}

// readOnlyToolNames returns the canonical set of MCP tool names that perform
// no mutations to memory state. These tools get ReadOnlyHint=true on their
// MCP annotation so clients (notably Claude Code's plan mode) treat them as
// safe to invoke without a permission prompt.
//
// Single source of truth — used by both registerTools (to set the annotation)
// and the startup banner (to print a recommended permissions.allow snippet).
func readOnlyToolNames() map[string]bool {
	return map[string]bool{
		"memory_fetch":              true,
		"memory_list":               true,
		"memory_history":            true,
		"memory_timeline":           true,
		"memory_status":             true,
		"memory_projects":           true,
		"memory_aggregate":          true,
		"memory_diagnose":           true,
		"memory_episode_list":       true,
		"memory_episode_recall":     true,
		"memory_models":             true,
		"memory_embedding_eval":     true,
		"memory_export_all":         true,
		"memory_audit_list_queries": true,
		"memory_audit_compare":      true,
		"memory_weight_history":     true,
		"memory_ingest_status":      true,
		"memory_expand":             true,
		"memory_verify":             true,
		"get_constraints":           true,
		"check_constraints":         true,
		"verify_before_acting":      true,
		"memory_status_ping":        true,
	}
}

// hiddenToolNames returns tools that remain registered (callable via tools/call)
// but are suppressed from the tools/list response. This keeps the MCP surface
// lean for AI clients while preserving full operability for direct HTTP callers
// and bundled skills. Maintenance tasks, ingest, audit, and embedder tools
// are hidden by default; use the bundled skills in skills/ to invoke them.
//
// Some entries also appear in readOnlyToolNames() — a tool can be both
// read-only and hidden. The AfterListTools hook in registerTools() enforces
// the suppression from tools/list.
func hiddenToolNames() map[string]bool {
	return map[string]bool{
		// Audit & weight tuning
		"memory_audit_add_query":        true,
		"memory_audit_list_queries":     true,
		"memory_audit_deactivate_query": true,
		"memory_audit_run":              true,
		"memory_audit_compare":          true,
		"memory_weight_history":         true,
		// Embedder management
		"memory_embedding_eval": true,
		"memory_models":         true,
		// Consolidation & maintenance
		"memory_consolidate": true,
		"memory_sleep":       true,
		"memory_summarize":   true,
		"memory_resummarize": true,
		// Episode management
		"memory_episode_start":  true,
		"memory_episode_end":    true,
		"memory_episode_list":   true,
		"memory_episode_recall": true,
		// Ingest & export
		"memory_ingest":                 true,
		"memory_import_claudemd":        true,
		"memory_ingest_document_stream": true,
		"memory_ingest_export":          true,
		"memory_ingest_status":          true,
		"memory_export_all":             true,
		// Operational / rarely needed
		"memory_expand":         true,
		"memory_adopt":          true,
		"memory_aggregate":      true,
		"memory_diagnose":       true,
		"memory_verify":         true,
		"memory_delete_project": true,
	}
}

// RegisteredToolAnnotations returns a copy of the ToolAnnotation that was
// applied to each registered tool. Intended for tests and for the startup
// banner. The returned map is safe to mutate.
func (s *Server) RegisteredToolAnnotations() map[string]mcpgo.ToolAnnotation {
	out := make(map[string]mcpgo.ToolAnnotation, len(s.toolAnnotations))
	for k, v := range s.toolAnnotations {
		out[k] = v
	}
	return out
}

// RegisteredToolDescriptions returns a copy of the description string recorded
// for each registered tool. Used by tests to verify description mutations such
// as the embed-degraded suffix (pillar 1C, #611). The returned map is safe to mutate.
func (s *Server) RegisteredToolDescriptions() map[string]string {
	out := make(map[string]string, len(s.toolDescriptions))
	for k, v := range s.toolDescriptions {
		out[k] = v
	}
	return out
}

// NewServer constructs a Server with all MCP tools registered.
func NewServer(pool *EnginePool, cfg Config) *Server {
	trustProxy := false
	if v := os.Getenv("ENGRAM_TRUST_PROXY_HEADERS"); v == "1" || strings.EqualFold(v, "true") {
		trustProxy = true
		slog.Warn("ENGRAM_TRUST_PROXY_HEADERS is enabled — ensure a trusted reverse proxy terminates all inbound connections; direct clients can spoof X-Forwarded-For to bypass rate limiting")
	}

	// A-4 (#689): warn loudly when memory_delete_project is callable.
	if v := os.Getenv("ENGRAM_ALLOW_PROJECT_DELETE"); v == "1" || strings.EqualFold(v, "true") {
		slog.Warn("ENGRAM_ALLOW_PROJECT_DELETE is enabled — memory_delete_project is callable by any authenticated MCP client; restart without this env var to re-lock once your delete task is complete (#689)")
	}

	// Create embedder health probe with a 5-second cached check.
	// The check function probes the configured LiteLLM endpoint.
	embedderHealth := NewEmbedderHealth(func(ctx context.Context) (bool, string) {
		// Use context with a 5-second timeout for the health probe.
		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		// Construct a minimal LiteLLMClient for the probe (uses RouterURL).
		// We only use it once per TTL interval.
		client := embed.NewLiteLLMClientNoProbe(cfg.RouterURL, cfg.EmbedModel, "", cfg.EmbedDimensions)
		ok, reason := client.Probe(probeCtx)
		return ok, reason
	}, 5*time.Second)

	// Add embedder health to config so all tool handlers can access it.
	cfg.EmbedderHealth = embedderHealth

	// Initialize embedDegraded from cfg.EmbedDegraded; this will be updated
	// by the /health probe loop as LiteLLM availability changes (#565).
	embedDegradedFlag := &atomic.Bool{}
	embedDegradedFlag.Store(cfg.EmbedDegraded)

	// Initialize runtime config from env vars.
	runtimeCfg := &RuntimeConfig{}
	if v := os.Getenv("ENGRAM_CLAUDE_SUMMARIZE"); v == "true" || v == "1" {
		runtimeCfg.ClaudeSummarize.Store(true)
	}
	if v := os.Getenv("ENGRAM_CLAUDE_CONSOLIDATE"); v == "true" || v == "1" {
		runtimeCfg.ClaudeConsolidate.Store(true)
	}
	if v := os.Getenv("ENGRAM_CLAUDE_RERANK"); v == "true" || v == "1" {
		runtimeCfg.ClaudeRerank.Store(true)
	}
	// Parse log level if set in env; default to 1 (info).
	logLevelVal := int32(1) // info
	if v := os.Getenv("ENGRAM_LOG_LEVEL"); v != "" {
		switch strings.ToLower(v) {
		case "debug":
			logLevelVal = 0
		case "info":
			logLevelVal = 1
		case "warn":
			logLevelVal = 2
		case "error":
			logLevelVal = 3
		}
	}
	runtimeCfg.LogLevel.Store(logLevelVal)
	cfg.RuntimeConfig = runtimeCfg

	s := &Server{
		pool:              pool,
		cfg:               cfg,
		uploads:           make(map[string]*uploadSession),
		trustProxy:        trustProxy,
		embedderHealth:    embedderHealth,
		toolAnnotations:   make(map[string]mcpgo.ToolAnnotation),
		toolDescriptions:  make(map[string]string),
		sessionTouchTimes: make(map[string]time.Time),
		embedDegraded:     embedDegradedFlag,
		runtimeCfg:        runtimeCfg,
	}
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

// ReloadRuntimeConfig reloads runtime configuration from environment variables (#557).
// Called by the SIGHUP handler to update feature flags without restarting the server.
func (s *Server) ReloadRuntimeConfig() {
	if s.runtimeCfg != nil {
		reloadRuntimeConfig(s.runtimeCfg)
		if s.cfg.LogLevelVar != nil {
			switch s.runtimeCfg.LogLevel.Load() {
			case 0:
				s.cfg.LogLevelVar.Set(slog.LevelDebug)
			case 2:
				s.cfg.LogLevelVar.Set(slog.LevelWarn)
			case 3:
				s.cfg.LogLevelVar.Set(slog.LevelError)
			default:
				s.cfg.LogLevelVar.Set(slog.LevelInfo)
			}
		}
	}
}

// setupTokenWindow is the rate-limit window for /setup-token: 3 calls per 5 minutes.
const setupTokenWindow = 5 * time.Minute / 3 // one token every 100 seconds

// sessionFingerprintPepper is the application-specific HMAC key for
// hashAPIKey. Public on purpose: the threat model is "DB dump → recover
// bearer." A 256-bit CSPRNG bearer is already brute-force-infeasible under
// any cryptographic hash; HMAC-SHA-256 with a fixed pepper is used because
// CodeQL's go/weak-sensitive-data-hashing rule flags bare sha256.Sum256 of
// data named like a credential. See engram-go#433.
const sessionFingerprintPepper = "engram-session-fingerprint-v1"

// hashAPIKey returns an HMAC-SHA-256 hex digest of the API key, stored in
// the sessions table instead of the plaintext bearer so the DB does not
// become a credential store. Migration: existing rows hashed with bare
// SHA-256 will not match the new digest; affected sessions are re-keyed
// on next register and unmatched rows expire by TTL.
func hashAPIKey(apiKey string) string {
	mac := hmac.New(sha256.New, []byte(sessionFingerprintPepper))
	mac.Write([]byte(apiKey))
	return hex.EncodeToString(mac.Sum(nil))
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
//
// Only sessions bound to the current API key (by api_key_hash) are rehydrated,
// preventing cross-key session access when the API key rotates (#548).
func (s *Server) RehydrateSessions(ctx context.Context, apiKey string) error {
	if s.cfg.SessionDB == nil {
		return nil
	}
	window := s.cfg.SessionRehydrateWindow
	if window == 0 {
		window = 2 * time.Hour
	}
	ids, err := s.cfg.SessionDB.ListActiveSessions(ctx, window, hashAPIKey(apiKey))
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
		s.sessionTouchMu.Lock()
		delete(s.sessionTouchTimes, sessionID)
		s.sessionTouchMu.Unlock()

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
	if host == "0.0.0.0" {
		slog.Warn("engram MCP server binding to 0.0.0.0 — ensure firewall restricts access (#550)")
	}
	slog.Info("engram MCP server starting", "addr", addr)

	advertised := baseURL
	if advertised == "" {
		advertised = fmt.Sprintf("http://%s", addr)
	}
	slog.Info("SSE base URL", "url", advertised)
	sse := buildSSEServer(s.mcp, advertised)

	s.registerSessionHooks(apiKey)
	s.apiKey = apiKey

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

	// GET /ready — unauthenticated readiness probe. Returns 200 when pool is warm.
	// Unlike /health, /ready tracks startup phase and pre-warming state.
	mux.HandleFunc("/ready", s.handleReady)

	// GET /metrics — requires Bearer authentication (#552).
	mux.Handle("/metrics", s.applyMiddlewareWithRL(
		promhttp.Handler(),
		apiKey,
		newRateLimiterWithConfig(ctx, s.cfg.rateLimitRPS(), s.cfg.rateLimitBurst()),
	))

	// GET /setup-token — TOFU bootstrap on first localhost request; Bearer auth thereafter (#613, #540).
	// Returns the current bearer token so MCP clients can self-configure without manual copy-paste.
	//
	// Security: Prior to #540, /setup-token was protected by IP-based access control
	// (local/RFC1918 only). This was insufficient because:
	// 1. X-Real-IP spoofing could bypass the check (fixed in #551)
	// 2. The token is equivalent in sensitivity to ~/.claude.json which is already on disk
	// 3. Bearer authentication provides cryptographic verification without relying on IP reputation
	//
	// TOFU (#613): the very first unauthenticated request from the loopback interface is
	// auto-approved (one-time-only, raw TCP peer address, not proxy headers). This breaks
	// the chicken-and-egg problem where engram-setup needs the token before it can authenticate.
	//
	// Rate limit: per IP (3 calls per 5-minute window).
	// Route /setup-token through the shared TOFU handler so both production and
	// tests exercise the same code path (#1214).
	mux.Handle("/setup-token", s.setupTokenTOFUHandlerWithLimiter(apiKey, advertised, setupLimiter))

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

	// /mcp — Streamable HTTP transport (stateless POST per tool call).
	// Preferred over /sse for Claude Code: no persistent connection to drop,
	// no keepalive required, no /mcp reconnect needed on idle timeout (#612).
	// Configure in mcp_servers.json: type:"http", url:"http://127.0.0.1:8788/mcp"
	streamable := buildStreamableHTTPServer(s.mcp)
	mux.Handle("/mcp", s.applyMiddleware(streamable, apiKey, rl))
	mux.Handle("/mcp/", s.authenticatedNotFoundHandler(apiKey, rl))

	// /quick-store — sessionless REST endpoint for hook scripts and CLI callers
	// that cannot establish an SSE session (e.g. PreCompact hooks).
	mux.Handle("/quick-store", s.applyMiddleware(http.HandlerFunc(s.handleQuickStore), apiKey, rl))

	// /quick-recall — sessionless REST endpoint for reading memories without an
	// active SSE session (e.g. Python subprocesses in the Clearwatch pipeline).
	mux.Handle("/quick-recall", s.applyMiddleware(http.HandlerFunc(s.handleQuickRecall), apiKey, rl))

	// /atoms — Milestone 1 (#938) atom REST endpoint.
	// POST {"action":"store","project":...,"atom":{...},"embedding":[...]} → stores atom + embedding.
	// POST {"project":...,"atom_type":...,"top_k":N} → returns {"atoms":[...]} for --atom-mode recall.
	mux.Handle("/atoms", s.applyMiddleware(http.HandlerFunc(s.handleAtoms), apiKey, rl))

	// All other authenticated routes are either the legacy SSE endpoint or a
	// structured JSON 404. Unknown paths must not fall through to the SSE
	// transport, which can emit opaque non-object bodies (#1192).
	mux.Handle("/", s.authenticatedFallbackHandler(sse, apiKey, rl))

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

func writeRESTEndpointNotFound(w http.ResponseWriter) {
	writeJSON(w, http.StatusNotFound, map[string]string{
		"error": "not found",
		"hint":  restEndpointRootHint,
	})
}

func (s *Server) authenticatedNotFoundHandler(apiKey string, rl *rateLimiter) http.Handler {
	return s.applyMiddlewareWithRL(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeRESTEndpointNotFound(w)
	}), apiKey, rl)
}

func (s *Server) authenticatedFallbackHandler(sse http.Handler, apiKey string, rl *rateLimiter) http.Handler {
	return s.applyMiddlewareWithRL(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sse" {
			sse.ServeHTTP(w, r)
			return
		}
		writeRESTEndpointNotFound(w)
	}), apiKey, rl)
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
		// Extract or generate a request correlation ID for log threading (#320, #555).
		// Check X-Request-Id first, then traceparent (OpenTelemetry), else generate.
		requestID := r.Header.Get("X-Request-Id")
		if requestID == "" {
			if traceparent := r.Header.Get("traceparent"); traceparent != "" {
				// traceparent format: version-trace_id-parent_id-trace_flags
				// Extract the trace_id (32-char hex) for correlation purposes
				parts := strings.Split(traceparent, "-")
				if len(parts) >= 2 && len(parts[1]) >= 12 {
					requestID = parts[1][:12]
				}
			}
		}
		if requestID == "" {
			// #696: UUIDv4 — 122 bits of entropy, no timing-prediction window,
			// no collision under sub-nanosecond concurrent requests (A-6 advisory).
			requestID = uuid.NewString()
		}

		// Echo the request ID in the response header for client-side correlation (#555)
		w.Header().Set("X-Request-Id", requestID)

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
		//
		// The loopback exemption MUST use the physical network address (r.RemoteAddr),
		// never a client-supplied header such as X-Real-IP or X-Forwarded-For.
		// A remote attacker sending "X-Real-IP: 127.0.0.1" must NOT bypass rate
		// limiting, regardless of the ENGRAM_TRUST_PROXY_HEADERS setting. (#1190)
		remoteHost := s.clientIP(r) // proxy-aware; used as the per-IP RL bucket key
		physicalHost, _, _ := net.SplitHostPort(r.RemoteAddr)
		if !s.cfg.RateLimitDisable && !isLoopbackIP(physicalHost) {
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
				"hint":  buildBearerMismatchHint(r),
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

// setupTokenTOFUHandler returns the /setup-token handler with TOFU (Trust On
// First Use) logic. It creates an internal rate limiter whose eviction goroutine
// is bound to ctx so it stops when the server shuts down (prevents goroutine leak).
// Use setupTokenTOFUHandlerWithLimiter in tests to inject a pre-exhausted limiter.
// advertised is the base URL prefix for the "endpoint" field (e.g. https://engram.example.com).
func (s *Server) setupTokenTOFUHandler(ctx context.Context, apiKey, advertised string) http.Handler {
	return s.setupTokenTOFUHandlerWithLimiter(apiKey, advertised, newRateLimiter(ctx))
}

// setupTokenTOFUHandlerWithLimiter returns the /setup-token handler using the
// provided rate limiter. This is the canonical implementation shared by the
// production path (via Start → mux.Handle) and tests (#1214 unification).
// advertised is the base URL prefix (e.g. "https://engram.example.com"); the
// full endpoint is advertised+"/mcp". Pass "" in tests that do not care about
// the advertised URL — the response will contain just "/mcp".
//
// Security contract:
//  1. Rate limit is checked unconditionally — even TOFU requests are throttled.
//  2. TOFU path uses r.RemoteAddr (raw TCP peer), NOT s.clientIP(r), to prevent
//     X-Forwarded-For spoofing when ENGRAM_TRUST_PROXY_HEADERS=1 (#540).
//  3. Exactly one unauthenticated loopback request is granted via CompareAndSwap.
//  4. All subsequent requests require Bearer authentication (unchanged from #540).
func (s *Server) setupTokenTOFUHandlerWithLimiter(apiKey, advertised string, rl *rateLimiter) http.Handler {
	endpoint := advertised + "/mcp"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Rate limit must use the physical TCP peer address, not the proxy-aware IP.
		// Keying on s.clientIP(r) lets an attacker rotate X-Forwarded-For to mint new
		// rate-limit buckets and exhaust unlimited /setup-token calls. The physical
		// r.RemoteAddr cannot be forged via request headers. (#1209)
		rawPeer, _, _ := net.SplitHostPort(r.RemoteAddr)
		if !rl.allowSetupToken(rawPeer) {
			slog.Warn("setup-token rate limited", "physical_remote", rawPeer)
			w.Header().Set("Retry-After", "100")
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}

		// TOFU: first unauthenticated request from loopback only.
		// CRITICAL: use r.RemoteAddr (raw TCP peer), NOT s.clientIP(r),
		// to prevent X-Forwarded-For spoofing when ENGRAM_TRUST_PROXY_HEADERS=1.
		// rawPeer is already extracted above for the rate limit — reuse it here.
		// Re-grant removed in #1187 — one grant per process lifetime.
		if isLoopbackIP(rawPeer) && s.tofuGranted.CompareAndSwap(false, true) {
			s.tofuGrantedAt.Store(time.Now().Unix())
			slog.Warn("setup-token TOFU: one-time localhost bootstrap grant issued (#613)",
				"remote_ip", rawPeer)
			writeJSON(w, http.StatusOK, map[string]string{
				"token":    apiKey,
				"endpoint": endpoint,
				"name":     "engram",
			})
			return
		}

		// All other requests: require Bearer authentication (unchanged from #540).
		s.applyMiddlewareWithRL(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				slog.Warn("setup-token accessed (authenticated)", "remote_ip", s.clientIP(r))
				writeJSON(w, http.StatusOK, map[string]string{
					"token":    apiKey,
					"endpoint": endpoint,
					"name":     "engram",
				})
			}),
			apiKey,
			rl,
		).ServeHTTP(w, r)
	})
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
		// Coalesce touches: at most one DB write per session per 30 seconds (#553).
		if s.cfg.SessionDB != nil {
			s.sessionTouchMu.Lock()
			lastTouch := s.sessionTouchTimes[sessionID]
			now := time.Now()
			shouldTouch := now.Sub(lastTouch) >= 30*time.Second
			if shouldTouch {
				s.sessionTouchTimes[sessionID] = now
			}
			s.sessionTouchMu.Unlock()

			if shouldTouch {
				go func() {
					dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					if err := s.cfg.SessionDB.TouchSession(dbCtx, sessionID); err != nil {
						slog.Debug("session touch failed", "session_id", sessionID, "err", err)
					}
				}()
			}
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
// All synchronous handlers should return well under 10s in normal operation; 60s
// preserves room for large-project recall while still failing well before the
// 90s HTTP timeout if a handler wedges.
// Tools that trigger background work (migrate_embedder, consolidate) also return
// quickly — the heavy lifting happens in background goroutines, not the handler.
const defaultToolTimeout = 60 * time.Second

func degradedToolMessage(toolName, reason string) string {
	base := toolName + " ran in degraded mode"
	suffix := "results use BM25 text search only. Memory tools remain accessible."
	switch reason {
	case "tool_timeout":
		return base + " (tool deadline exceeded) — " + suffix +
			" Recovery is automatic when recall latency improves."
	case "embed_unavailable", "embed_timeout":
		return base + " (embedding backend unavailable) — " + suffix +
			" Recovery is automatic when embedding service health returns."
	case "embed_error":
		return base + " (embedding backend error) — " + suffix +
			" Recovery is automatic when embedding service health returns."
	case "circuit_open":
		return base + " (embed circuit breaker open) — " + suffix +
			" Recovery is automatic when the embed circuit closes."
	default:
		return base + " (" + reason + ") — " + suffix +
			" Recovery is automatic when the degraded dependency recovers."
	}
}

// registerToolWithTimeout adds a tool to the MCP server with a per-call deadline
// and Prometheus instrumentation. timeout=0 uses defaultToolTimeout. readOnly
// sets the MCP ReadOnlyHint annotation: clients (notably Claude Code's plan
// mode) use that hint to decide whether to invoke a tool without prompting.
// schema declares the tool's JSON input schema (WithString/WithNumber/
// WithArray/WithBoolean/WithObject + Required()) — see #1281: every tool must
// carry a real schema so MCP clients type-coerce arguments correctly instead
// of stringifying array/number params, which handlers then silently drop.
func (s *Server) registerToolWithTimeout(name, desc string, h toolHandler, timeout time.Duration, readOnly bool, schema ...mcpgo.ToolOption) {
	if timeout == 0 {
		timeout = defaultToolTimeout
	}
	pool, cfg := s.pool, s.cfg
	toolName := name
	toolTimeout := timeout
	roCopy := readOnly
	annotation := mcpgo.ToolAnnotation{
		Title:        name,
		ReadOnlyHint: &roCopy,
	}
	if s.toolAnnotations == nil {
		s.toolAnnotations = make(map[string]mcpgo.ToolAnnotation)
	}
	s.toolAnnotations[name] = annotation
	if s.toolDescriptions == nil {
		s.toolDescriptions = make(map[string]string)
	}
	s.toolDescriptions[name] = desc
	opts := make([]mcpgo.ToolOption, 0, len(schema)+2)
	opts = append(opts, mcpgo.WithDescription(desc), mcpgo.WithToolAnnotation(annotation))
	opts = append(opts, schema...)
	s.mcp.AddTool(mcpgo.NewTool(name, opts...),
		func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
			ctx, cancel := context.WithTimeout(ctx, toolTimeout)
			defer cancel()

			timer := prometheus.NewTimer(metrics.ToolDuration.WithLabelValues(toolName))
			defer timer.ObserveDuration()

			result, err := h(ctx, pool, req, cfg)
			if err != nil && ctx.Err() == context.DeadlineExceeded {
				slog.Warn("mcp tool timed out — returning degraded success to prevent false 'user denied' synthesis",
					"tool", toolName, "timeout", toolTimeout, "reason", "tool_timeout")
				metrics.ToolRequests.WithLabelValues(toolName, "timeout").Inc()
				degradedJSON, _ := json.Marshal(map[string]any{
					"_engram_degraded":        true,
					"_engram_degraded_reason": "tool_timeout",
					"_engram_tool":            toolName,
					"status":                  "degraded",
					"message":                 degradedToolMessage(toolName, "tool_timeout"),
				})
				return mcpgo.NewToolResultText(string(degradedJSON)), nil
			}
			status := "ok"
			if err != nil || (result != nil && result.IsError) {
				status = "error"
			}
			metrics.ToolRequests.WithLabelValues(toolName, status).Inc()
			return result, err
		})
}

// registerTool adds a tool with the default dispatch timeout. The readOnly hint
// is sourced from readOnlyToolNames() — single source of truth. schema declares
// the tool's JSON input schema (see registerToolWithTimeout).
func (s *Server) registerTool(name, desc string, h toolHandler, schema ...mcpgo.ToolOption) {
	s.registerToolWithTimeout(name, desc, h, 0, readOnlyToolNames()[name], schema...)
}

func (s *Server) registerTools() {
	// embedSuffix is appended to the description of embedding-dependent tools
	// when the embedder is unavailable at startup. This surfaces the degradation
	// to the AI client so it can communicate the limitation to users (#611).
	embedSuffix := ""
	if s.embedDegraded != nil && s.embedDegraded.Load() {
		embedSuffix = " [EMBEDDING UNAVAILABLE: semantic search degraded, BM25+recency fallback active]"
	}

	type toolDef struct {
		name    string
		desc    string
		handler toolHandler
		schema  []mcpgo.ToolOption
	}
	tools := []toolDef{
		// Core store operations
		{"memory_store", "Store a focused memory (<=10k chars). Optional: pattern_confidence (float 0.0–1.0) for caller-provided confidence that a detected pattern is genuine." + embedSuffix,
			handleMemoryStore, []mcpgo.ToolOption{
				requiredStrProp("content", "The memory content to store (<=10,000 chars)."),
				projectProp(),
				enumStrProp("memory_type", "Type of memory. Defaults to \"context\" (auto-upgraded to \"preference\" when the content reads as a stated preference and memory_type was not explicitly supplied).",
					"decision", "pattern", "error", "context", "architecture", "preference"),
				numberProp("importance", "Retention priority 0–4 (0=Critical .. 4=Low). Defaults to 2."),
				tagsProp("Freeform tags. A tag of the form \"date:YYYY-MM-DD\" sets valid_from."),
				boolProp("immutable", "When true, the memory can never be edited or soft-deleted."),
				numberProp("pattern_confidence", "Caller-provided confidence (0.0–1.0) that a detected pattern is genuine."),
				strProp("episode_id", "Episode to attach this memory to. Falls back to the session's auto-started episode when omitted."),
			}},
		{"memory_store_document", "Store a large document (auto-tiered up to 50 MB via synopsis + raw blob storage)",
			handleMemoryStoreDocument, []mcpgo.ToolOption{
				requiredStrProp("content", "The document content to store."),
				projectProp(),
				enumStrProp("memory_type", "Type of memory. Defaults to \"context\".",
					"decision", "pattern", "error", "context", "architecture", "preference"),
				numberProp("importance", "Retention priority 0–4 (0=Critical .. 4=Low). Defaults to 2."),
				tagsProp("Freeform tags. A tag of the form \"date:YYYY-MM-DD\" sets valid_from."),
				boolProp("immutable", "When true, the memory can never be edited or soft-deleted."),
				strProp("episode_id", "Episode to attach this memory to."),
			}},
		{"memory_ingest_document_stream", "Ingest a very large document via server-local path or chunked base64 upload (auto-tiered, up to 50 MB)",
			func(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
				return handleMemoryIngestDocumentStream(ctx, s, pool, req, cfg)
			}, []mcpgo.ToolOption{
				projectProp(),
				strProp("path", "Server-local file path to ingest directly (mutually exclusive with the chunked-upload action flow)."),
				enumStrProp("action", "Chunked-upload action, used when path is not set.", "start", "append", "finish"),
				strProp("upload_id", "Identifier for the chunked-upload session (required for append/finish; letters, digits, hyphens, underscores, dots only)."),
				numberProp("part", "0-indexed part number for action=append."),
				numberProp("part_index", "Legacy alias for part."),
				strProp("data", "Base64-encoded chunk payload for action=append."),
			}},
		{"memory_store_batch", "Store multiple memories in one call. Each item supports the same optional fields as memory_store, including pattern_confidence (float 0.0–1.0) for per-item caller-provided confidence. If any item fails validation the entire batch is rejected." + embedSuffix,
			handleMemoryStoreBatch, []mcpgo.ToolOption{
				projectProp(),
				mcpgo.WithArray("memories", mcpgo.Required(),
					mcpgo.Description("Array of memory objects to store (max 100). Each item accepts the same fields as memory_store: content (required), memory_type, importance, tags, immutable, pattern_confidence, episode_id."),
					mcpgo.Items(map[string]any{
						"type": "object",
						"properties": map[string]any{
							"content":            map[string]any{"type": "string", "description": "The memory content (required)."},
							"memory_type":        map[string]any{"type": "string", "enum": []string{"decision", "pattern", "error", "context", "architecture", "preference"}},
							"importance":         map[string]any{"type": "number", "description": "0-4, defaults to 2."},
							"tags":               map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
							"immutable":          map[string]any{"type": "boolean"},
							"pattern_confidence": map[string]any{"type": "number"},
							"episode_id":         map[string]any{"type": "string"},
						},
						"required": []string{"content"},
					}),
				),
			}},
		// Recall and retrieval
		{"memory_recall", "Recall memories by semantic + full-text query. Accepts top_k or limit; mode=handle returns lightweight handles. Pass record_event=true to receive an event_id in the response for use with memory_feedback (off by default so recall stays side-effect free)." + embedSuffix,
			withWarnLog("memory_recall", handleMemoryRecall), []mcpgo.ToolOption{
				requiredStrProp("query", "The search query."),
				projectProp(),
				numberProp("top_k", "Maximum number of results to return. Defaults to a server-configured value."),
				numberProp("limit", "Alias for top_k; top_k wins if both are supplied."),
				enumStrProp("detail", "Result verbosity.", "summary", "full", "chunk"),
				boolProp("include_conflicts", "Include conflicting_results enrichment in the response."),
				boolProp("record_event", "Return an event_id usable with memory_feedback. Off by default so recall stays side-effect free. Not supported for federated (projects) recall."),
				enumStrProp("mode", "Response shape.", "handle", "full", "summary", "id_only"),
				strProp("since", "RFC3339 or YYYY-MM-DD lower bound on memory recency (single-project recall only)."),
				strProp("before", "RFC3339 or YYYY-MM-DD upper bound on memory recency (single-project recall only)."),
				boolProp("temporal_window_recall", "Enable server-side date-windowed temporal recall using question_text/question_date."),
				strProp("question_text", "Free text used to parse a temporal anchor for temporal_window_recall."),
				strProp("question_date", "Reference date (RFC3339 or YYYY-MM-DD) used to resolve relative temporal anchors."),
				strProp("atom_recall_as_of", "RFC3339 or YYYY-MM-DD point-in-time bound for atom_recall."),
				boolProp("atom_recall", "Enable atom-level recall."),
				stringArrayProp("projects", "Federated recall: search across these project names instead of the single project arg. Pass [\"*\"] to expand to all known projects."),
				boolProp("rerank", "Opt in to Claude-based reranking (single-project only; requires the server to have reranking enabled)."),
				boolProp("exact_fact_boost", "Boost results containing verbatim identifier matches (URLs, phone numbers, quoted phrases)."),
				boolProp("topic_anchor_boost", "Boost topic-anchor matches for preference-shaped queries."),
				numberProp("session_diversity_n", "Per-session chunk cap on returned results."),
				boolProp("paraphrase_union", "Enable paraphrase-union retrieval for this call."),
				boolProp("rrf_fusion", "Enable reciprocal-rank-fusion for this call."),
				boolProp("evidence_first_pack", "Reorder results so verbatim-identifier matches come first."),
			}},
		{"memory_fetch", "Fetch a single memory by ID; detail=summary|chunk|full",
			handleMemoryFetch, []mcpgo.ToolOption{
				requiredStrProp("id", "ID of the memory to fetch."),
				projectProp(),
				enumStrProp("detail", "Response verbosity. Defaults to \"summary\".", "summary", "full", "chunk"),
				stringArrayProp("chunk_ids", "When detail=chunk, restrict the response to these chunk IDs."),
			}},
		{"memory_list", "List memories with optional filters",
			noConfig(handleMemoryList), []mcpgo.ToolOption{
				projectProp(),
				numberProp("limit", "Maximum number of memories to return (1–500). Defaults to 50; out-of-range values fall back to 50."),
				numberProp("offset", "Number of memories to skip for pagination. Defaults to 0."),
				strProp("memory_type", "Filter to a single memory type."),
				tagsProp("Filter to memories matching any of these tags."),
			}},
		{"memory_history", "Return the full version chain for a memory",
			noConfig(handleMemoryHistory), []mcpgo.ToolOption{
				projectProp(),
				memoryIDProp(""),
			}},
		{"memory_timeline", "Recall memories that were active at a given point in time (as_of param, RFC3339)",
			noConfig(handleMemoryTimeline), []mcpgo.ToolOption{
				projectProp(),
				requiredStrProp("as_of", "RFC3339 timestamp — return memories active at this point in time."),
				numberProp("limit", "Maximum number of memories to return. Defaults to 20."),
			}},
		// Graph operations
		{"memory_connect", "Create a directed relationship between two memories. relation_type values: caused_by, relates_to, depends_on, supersedes, used_in, resolved_by, contradicts, supports, derived_from, part_of, follows",
			noConfig(handleMemoryConnect), []mcpgo.ToolOption{
				projectProp(),
				requiredStrProp("source_id", "ID of the source memory."),
				requiredStrProp("target_id", "ID of the target memory."),
				strProp("relation_type", "Relationship type. Defaults to \"relates_to\"."),
				numberProp("strength", "Relationship strength, 0.0–1.0. Defaults to 1.0."),
			}},
		{"memory_expand", "Explore the relationship graph neighbourhood of a known memory.",
			noConfig(handleMemoryExpand), []mcpgo.ToolOption{
				projectProp(),
				memoryIDProp(""),
				numberProp("depth", "Number of relationship hops to traverse. Defaults to 2."),
			}},
		// Mutations
		{"memory_correct", "Update content, tags, importance, or pattern_confidence (float 0.0–1.0) on an existing memory. Omit pattern_confidence to leave it unchanged. Only-promote-never-nullify rule: omit 'tags' entirely to preserve the existing valid_from; sending tags=[...] recalculates valid_from from date: tags; sending tags=[] clears valid_from to null. See docs/tools.md#memory_correct and issue #765.",
			noConfig(handleMemoryCorrect), []mcpgo.ToolOption{
				projectProp(),
				memoryIDProp(""),
				strProp("content", "New content. Omit to leave content unchanged."),
				tagsProp("New tags. Omit entirely to preserve the existing tags/valid_from; pass [] to clear tags and valid_from."),
				numberProp("importance", "New importance, 0–4. Omit to leave unchanged."),
				numberProp("pattern_confidence", "New pattern confidence, 0.0–1.0. Omit to leave unchanged."),
			}},
		{"memory_forget", "Soft-delete a memory (sets valid_to, preserves history, respects immutability)",
			noConfig(handleMemoryForget), []mcpgo.ToolOption{
				projectProp(),
				memoryIDProp(""),
				strProp("reason", "Optional human-readable reason for the deletion."),
			}},
		// Maintenance
		{"memory_summarize", "Immediately summarize a memory",
			handleMemorySummarize, []mcpgo.ToolOption{
				projectProp(),
				memoryIDProp(""),
			}},
		{"memory_resummarize", "Clear all summaries for a project — they regenerate automatically within 60s",
			noConfig(handleMemoryResummarize), []mcpgo.ToolOption{
				projectProp(),
			}},
		{"memory_status", "Return project statistics",
			noConfig(handleMemoryStatus), []mcpgo.ToolOption{
				projectProp(),
			}},
		{"memory_status_ping", "Lightweight liveness probe — no DB writes, 2s internal timeout. Used by the Claude Code Stop hook to detect MCP disconnection.",
			noConfig(handleMemoryStatusPing), nil},
		{"memory_verify", "Integrity check -- hash coverage and corrupt count",
			noConfig(handleMemoryVerify), []mcpgo.ToolOption{
				projectProp(),
			}},
		// Feedback and aggregation
		{"memory_feedback", "Record retrieval feedback. event_id: the id returned by memory_recall's response when called with record_event=true (required when failure_class is set). failure_class values (for misses): vocabulary_mismatch, aggregation_failure, stale_ranking, missing_content, scope_mismatch, other",
			noConfig(handleMemoryFeedback), []mcpgo.ToolOption{
				projectProp(),
				stringArrayProp("memory_ids", "IDs of memories to reinforce (max 100)."),
				strProp("event_id", "UUID from a prior memory_recall(record_event=true) call. Required when failure_class is set."),
				enumStrProp("failure_class", "Classify a retrieval miss instead of reinforcing.",
					"vocabulary_mismatch", "aggregation_failure", "stale_ranking", "missing_content", "scope_mismatch", "other"),
			}},
		{"memory_aggregate", "Group and count memories. by=tag|type|failure_class. filter: optional ILIKE substring — tag mode only, error for failure_class.",
			noConfig(handleMemoryAggregate), []mcpgo.ToolOption{
				projectProp(),
				requiredEnumStrProp("by", "Dimension to group by.", "tag", "type", "failure_class"),
				strProp("filter", "Optional ILIKE substring filter — tag mode only."),
				numberProp("limit", "Maximum number of groups to return (1–1000). Defaults to 20."),
			}},
		// Consolidation
		{"memory_consolidate", "Prune stale memories, decay edges, merge near-duplicates",
			handleMemoryConsolidate, []mcpgo.ToolOption{
				projectProp(),
			}},
		{"memory_sleep", "Run full sleep-consolidation cycle: infer relationships between semantically related memories",
			handleMemorySleep, []mcpgo.ToolOption{
				projectProp(),
				numberProp("min_similarity", "Minimum cosine similarity to infer a relationship. Defaults to 0.7."),
				numberProp("limit", "Maximum memories to scan for relationship inference (1–5000). Defaults to 500."),
				boolProp("llm_contradiction_detection", "Enable LLM-based contradiction detection (opt-in, default off)."),
				strProp("llm_model", "Ollama model to use for contradiction detection. Defaults to \"llama3.2:3b\"."),
				numberProp("llm_max_calls", "Cap on LLM calls for contradiction detection. Defaults to 10."),
				boolProp("auto_supersede", "Automatically mark detected contradictions as superseded."),
				numberProp("contradiction_limit", "Memories to scan for contradiction detection. Defaults to the value of limit when 0."),
			}},
		{"memory_delete_project", "Delete all memories and project data for a project. IRREVERSIBLE. Requires server started with ENGRAM_ALLOW_PROJECT_DELETE=1 AND a confirm argument exactly matching the project argument (#689). Not for normal use.",
			noConfig(handleMemoryDeleteProject), []mcpgo.ToolOption{
				requiredStrProp("project", "Project to permanently delete."),
				requiredStrProp("confirm", "Must exactly match project — typo guard for this irreversible operation."),
			}},
		// Episodes
		{"memory_episode_start", "Start a named episode to group memories from this session",
			withWarnLog("memory_episode_start", noConfig(handleMemoryEpisodeStart)), []mcpgo.ToolOption{
				projectProp(),
				strProp("description", "Human-readable description of the episode."),
			}},
		{"memory_episode_end", "End an episode with an optional summary",
			noConfig(handleMemoryEpisodeEnd), []mcpgo.ToolOption{
				projectProp(),
				requiredStrProp("episode_id", "ID of the episode to end."),
				strProp("summary", "Optional summary of what happened in this episode."),
			}},
		{"memory_episode_list", "List recent episodes for a project",
			noConfig(handleMemoryEpisodeList), []mcpgo.ToolOption{
				projectProp(),
				numberProp("limit", "Maximum number of episodes to return. Defaults to 20."),
			}},
		{"memory_episode_recall", "Return all memories from a specific episode in chronological order",
			noConfig(handleMemoryEpisodeRecall), []mcpgo.ToolOption{
				projectProp(),
				requiredStrProp("episode_id", "ID of the episode to recall."),
			}},
		// Embedder management
		{"memory_migrate_embedder", "Switch embedding model; triggers background re-embedding. Also resets any learned adaptive weights for the project to compile-time defaults.",
			handleMemoryMigrateEmbedder, []mcpgo.ToolOption{
				projectProp(),
				requiredStrProp("new_model", "Name of the new embedding model."),
				strProp("ollama_url", "Override the server-configured embedder endpoint for this call."),
				boolProp("force", "Bypass the same-model no-op guard."),
				boolProp("dry_run", "Report what would happen without nulling any embeddings."),
				boolProp("confirm", "Explicit confirmation required by some migration guards."),
			}},
		{"memory_models", "List installed and suggested Ollama embedding models. Shows which suggested models are installed, which is current, and flags the recommended upgrade.",
			handleMemoryModels, nil},
		{"memory_embedding_eval", "Compare two Ollama embedding models using probe sentences. model_a defaults to the configured embedding model; model_b defaults to the recommended registry entry. Use this to validate a 1024-dim compatible replacement before migrating. Auto-pulls missing models. Read-only — does not migrate stored embeddings.",
			handleMemoryEmbeddingEval, []mcpgo.ToolOption{
				strProp("model_a", "First model to compare. Defaults to the server's configured embedding model."),
				strProp("model_b", "Second model to compare. Defaults to the recommended registry entry."),
			}},
		// Import / export
		{"memory_export_all", "Export all memories to markdown files",
			handleMemoryExportAll, []mcpgo.ToolOption{
				projectProp(),
				strProp("output_path", "Destination directory, relative to the server's data directory. Defaults to \"./memory-export\"."),
			}},
		{"memory_import_claudemd", "Import a CLAUDE.md file as structured memories",
			handleMemoryImportClaudeMD, []mcpgo.ToolOption{
				projectProp(),
				requiredStrProp("path", "Path to the CLAUDE.md file, relative to the server's data directory."),
			}},
		{"memory_ingest", "Ingest a file or directory as document memories",
			handleMemoryIngest, []mcpgo.ToolOption{
				projectProp(),
				requiredStrProp("path", "File or directory to ingest, relative to the server's data directory."),
			}},
		{"memory_ingest_export",
			"Ingest a server-local AI conversation export file (Slack workspace .zip, Claude.ai conversations.json, or ChatGPT conversations.json). Parses the file, auto-detects format, and stores one memory per conversation or channel.",
			handleMemoryIngestExport, []mcpgo.ToolOption{
				projectProp(),
				requiredStrProp("path", "Path to the export file, relative to the server's data directory."),
			}},
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
			}, []mcpgo.ToolOption{
				requiredStrProp("job_id", "Job ID returned by the queuing call."),
			}},
		// Cross-project federation
		{"memory_projects", "List all projects with memory counts",
			noConfig(handleMemoryProjects), []mcpgo.ToolOption{
				projectProp(),
			}},
		{"memory_adopt", "Create a cross-project reference relationship",
			noConfig(handleMemoryAdopt), []mcpgo.ToolOption{
				projectProp(),
				requiredStrProp("source_id", "ID of the memory in the calling project."),
				requiredStrProp("target_id", "ID of the memory in the other project."),
				strProp("relation_type", "Relationship type. Defaults to \"relates_to\"."),
				numberProp("strength", "Relationship strength, 0.0–1.0. Defaults to 1.0."),
			}},
		// Simplified front-door tools
		{"memory_quick_store", "Store a memory and automatically extract entities. Simplified front door for memory_store.",
			withWarnLog("memory_quick_store", handleMemoryQuickStore), []mcpgo.ToolOption{
				requiredStrProp("content", "The memory content to store."),
				projectProp(),
				enumStrProp("memory_type", "Type of memory. Defaults to \"context\".",
					"decision", "pattern", "error", "context", "architecture", "preference"),
				numberProp("importance", "Retention priority 0–4. Defaults to 2."),
				tagsProp("Freeform tags."),
				boolProp("immutable", "When true, the memory can never be edited or soft-deleted."),
			}},
		{"memory_query", "Simplified front door for memory_recall. Accepts a 'limit' param instead of top_k; sensible defaults applied." + embedSuffix,
			handleMemoryQuery, []mcpgo.ToolOption{
				requiredStrProp("query", "The search query."),
				projectProp(),
				numberProp("limit", "Maximum number of results to return. Defaults to 5. Mapped to top_k internally."),
				numberProp("top_k", "Alias for limit; wins if both are supplied."),
				enumStrProp("detail", "Result verbosity.", "summary", "full", "chunk"),
				boolProp("include_conflicts", "Include conflicting_results enrichment in the response."),
				enumStrProp("mode", "Response shape.", "handle", "full", "summary", "id_only"),
			}},
		// Safety constraint verification
		{"get_constraints", "List constraint and policy memories relevant to an optional query",
			noConfig(handleGetConstraints), []mcpgo.ToolOption{
				projectProp(),
				strProp("query", "Optional free-text query to scope the constraint search."),
				numberProp("limit", "Maximum number of constraints to return (1–50). Defaults to 10."),
				numberProp("stale_after_days", "Days after which a matched constraint is flagged stale (1–3650). Defaults to 180."),
			}},
		{"check_constraints", "Classify a proposed action and return matching constraints with a verification decision",
			noConfig(handleCheckConstraints), []mcpgo.ToolOption{
				projectProp(),
				requiredStrProp("proposed_action", "Description of the action to classify."),
				numberProp("limit", "Maximum number of constraints to return (1–50). Defaults to 10."),
				numberProp("stale_after_days", "Days after which a matched constraint is flagged stale (1–3650). Defaults to 180."),
			}},
		{"verify_before_acting", "Run the full constraint verification pipeline and return a proceed/warn/require_approval/block decision",
			noConfig(handleVerifyBeforeActing), []mcpgo.ToolOption{
				projectProp(),
				requiredStrProp("proposed_action", "Description of the action to verify."),
				numberProp("limit", "Maximum number of constraints to return (1–50). Defaults to 10."),
				numberProp("stale_after_days", "Days after which a matched constraint is flagged stale (1–3650). Defaults to 180."),
			}},
		// Audit and weight tuning
		{"memory_audit_add_query", "Register a canonical query for retrieval drift monitoring",
			handleMemoryAuditAddQuery, []mcpgo.ToolOption{
				requiredProjectProp(),
				requiredStrProp("query", "The canonical query text to monitor."),
				strProp("description", "Optional human-readable description of this canonical query."),
			}},
		{"memory_audit_list_queries", "List canonical queries registered for drift monitoring in a project",
			handleMemoryAuditListQueries, []mcpgo.ToolOption{
				requiredProjectProp(),
			}},
		{"memory_audit_deactivate_query", "Deactivate a canonical query (stops future drift snapshots)",
			handleMemoryAuditDeactivateQuery, []mcpgo.ToolOption{
				requiredStrProp("query_id", "ID of the canonical query to deactivate."),
			}},
		{"memory_audit_run", "Run a decay audit pass for a project immediately and return snapshot summaries",
			handleMemoryAuditRun, []mcpgo.ToolOption{
				requiredProjectProp(),
			}},
		{"memory_audit_compare", "Compare retrieval snapshots for a canonical query to detect ranking drift",
			handleMemoryAuditCompare, []mcpgo.ToolOption{
				requiredStrProp("query_id", "ID of the canonical query."),
				numberProp("limit", "Maximum number of snapshots to return. Defaults to 10."),
			}},
		{"memory_weight_history", "Return current retrieval weights and tuning history for a project",
			handleMemoryWeightHistory, []mcpgo.ToolOption{
				requiredProjectProp(),
			}},
		// Diagnose (always available — no Claude required)
		{"memory_diagnose", "Return evidence map for recalled memories: conflicts, confidence, invalidated sources — no synthesis",
			noConfig(handleMemoryDiagnose), []mcpgo.ToolOption{
				projectProp(),
				requiredStrProp("question", "Query used to recall the memories to diagnose."),
				numberProp("top_k", "Maximum number of memories to recall for diagnosis. Defaults to 10."),
				enumStrProp("detail", "Recall detail level passed through to the underlying recall.", "summary", "full", "chunk"),
			}},
	}
	for _, t := range tools {
		s.registerTool(t.name, t.desc, t.handler, t.schema...)
	}

	// Claude-required tools: registered only when a client is available.
	if s.cfg.ClaudeEnabled {
		s.registerTool("memory_reason",
			"Recall memories and synthesize a grounded answer using Claude",
			handleMemoryReason,
			projectProp(),
			requiredStrProp("question", "The question to answer."),
			numberProp("top_k", "Maximum number of memories to recall (1–100). Defaults to 10."),
			enumStrProp("detail", "Recall detail level.", "summary", "full", "chunk"))
		s.registerTool("memory_explore",
			"Iterative recall+score+synthesis loop — returns a single grounded answer (A3)",
			handleMemoryExplore,
			projectProp(),
			requiredStrProp("question", "The question to answer."),
			numberProp("max_iterations", "Cap on recall+score+synthesis loop iterations (1–10)."),
			numberProp("confidence_threshold", "Stop iterating once this confidence (0.0–1.0) is reached. Defaults to 0.75."),
			numberProp("token_budget", "Cumulative scoring-call token budget. Defaults to a server-configured value."),
			boolProp("include_trace", "Include the full iteration trace in the response."),
			exploreScopeProp())
		// memory_query_document (A5): query a single large document by regex/substring
		// or semantic recall and synthesize an answer with Claude.
		s.registerTool("memory_query_document",
			"Query a large document stored in memory using regex/substring matching or semantic search. Returns relevant spans and an AI-synthesized answer.",
			handleMemoryQueryDocument,
			requiredProjectProp(),
			requiredStrProp("memory_id", "ID of the document memory to query."),
			requiredStrProp("question", "The question to answer against the document."),
			queryDocumentFilterProp(),
			numberProp("window_chars", "Characters of context to include around each match. Defaults to 4000."),
			boolProp("semantic", "Use semantic recall instead of regex/substring matching."),
			numberProp("token_budget", "Token budget for the synthesis call. Defaults to 6000."))
		// memory_ask (P2): retrieval-augmented question answering with numbered citations.
		s.registerTool("memory_ask",
			"Answer a question using stored memories as context. Returns answer + numbered citations.",
			handleMemoryAsk,
			requiredProjectProp(),
			requiredStrProp("question", "The question to answer."),
			numberProp("top_k", "Maximum number of memories to use as context (0–100). 0 or omitted uses the default (10)."))
	}

	// Hide maintenance/operational tools from tools/list. They remain callable
	// via tools/call for bundled skills and direct HTTP access.
	hidden := hiddenToolNames()
	s.mcp.GetHooks().AddAfterListTools(func(_ context.Context, _ any, _ *mcpgo.ListToolsRequest, result *mcpgo.ListToolsResult) {
		filtered := result.Tools[:0] // reuse backing array; filter moves elements left, safe
		for _, t := range result.Tools {
			if !hidden[t.Name] {
				filtered = append(filtered, t)
			}
		}
		result.Tools = filtered
	})
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
		Status       string `json:"status"`
		Postgres     string `json:"postgres"`
		Ollama       string `json:"ollama"`
		CircuitState string `json:"circuit_state"`
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

	// Probe router via InfinityQueueCheck: GETs /v1/models and parses Infinity
	// queue stats to detect GPU thread deadlock (queue_fraction > 1.0,
	// results_pending == 0). Subsumes the former status-code-only check (#649).
	embedLiveOK := false
	if s.cfg.RouterURL != "" {
		embedCtx, embedCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer embedCancel()
		healthHTTPClient := &http.Client{Timeout: 5 * time.Second}
		probeOK, probeReason := embed.InfinityQueueCheck(embedCtx, healthHTTPClient, s.cfg.RouterURL)
		if !probeOK {
			ollamaStatus = "error" //nolint:ineffassign // overwritten if EmbedDegraded
			slog.Warn("health: litellm probe failed", "reason", probeReason)
		} else {
			embedLiveOK = true
		}

		// Check the current embedding degradation status. Update the atomic flag
		// so the state can flip back to false when LiteLLM recovers (#565).
		currentlyDegraded := s.embedDegraded.Load()
		if embedLiveOK {
			// LiteLLM is currently healthy — clear the degraded flag
			ollamaStatus = "ok"
			s.embedDegraded.Store(false)
		} else {
			// LiteLLM is currently unhealthy
			if currentlyDegraded {
				// Was already degraded; stay degraded but report as operational (HTTP 200)
				ollamaStatus = "degraded"
			} else {
				// Was healthy but is now unhealthy — this is a new failure
				ollamaStatus = "error"
			}
			s.embedDegraded.Store(true)
		}
	}

	// Resolve circuit breaker state for the response.
	circuitState := "closed"
	if s.cfg.CircuitStateFunc != nil {
		circuitState = s.cfg.CircuitStateFunc()
	}
	res := result{
		Status:       "ok",
		Postgres:     pgStatus,
		Ollama:       ollamaStatus,
		CircuitState: circuitState,
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

	// Gate the detailed topology (postgres/ollama/circuit_state) behind Bearer
	// authentication so unauthenticated callers (e.g. K8s readiness probes) only
	// learn the overall ok/degraded status without internal topology details (#1210).
	// When s.apiKey is empty (test environments that construct Server without a key),
	// the full response is returned for backward compatibility.
	if s.apiKey != "" {
		got := hmac.New(sha256.New, []byte(s.apiKey))
		got.Write([]byte(r.Header.Get("Authorization")))
		want := hmac.New(sha256.New, []byte(s.apiKey))
		want.Write([]byte("Bearer " + s.apiKey))
		if subtle.ConstantTimeCompare(got.Sum(nil), want.Sum(nil)) != 1 {
			// Unauthenticated: return minimal status without internal topology.
			writeJSON(w, statusCode, map[string]string{"status": res.Status})
			return
		}
	}
	writeJSON(w, statusCode, res)
}

// handleReady reports the server's startup phase and embedding health.
// Returns 200 only when the pool has completed pre-warming (phaseWarm).
// Unlike /health, /ready is specifically designed for startup gating:
// K8s initialDelaySeconds / readinessProbe and MCP client retry logic can
// poll this endpoint to know when to begin routing tool calls.
//
// Unauthenticated — suitable as a K8s readiness probe.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	phase := s.serverPhase.Load()
	embedStatus := "ok"
	if s.embedDegraded.Load() {
		embedStatus = "degraded"
	}

	phaseStr := "starting"
	switch phase {
	case phaseWarming:
		phaseStr = "warming"
	case phaseWarm:
		phaseStr = "warm"
	}

	ready := phase == phaseWarm
	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}

	writeJSON(w, status, map[string]any{
		"ready":          ready,
		"phase":          phaseStr,
		"embed":          embedStatus,
		"transport_hint": "http",
	})
}

// handleQuickStore is a sessionless REST endpoint that stores a single memory
// without requiring an active SSE session. Used by hook scripts (e.g. PreCompact)
// and CLI callers that cannot perform the SSE handshake.
//
// POST /quick-store
// Authorization: Bearer <token>
// {"content":"...","project":"...","tags":[...],"importance":N}.
// Project-level TTL for prune workflows requires explicit opt-in:
// {"set_project_ttl":true,"project_expires_at":"..."}.
func (s *Server) handleQuickStore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Content          string     `json:"content"`
		Project          string     `json:"project"`
		Tags             []string   `json:"tags"`
		Importance       int        `json:"importance"`
		ExpiresAt        *time.Time `json:"expires_at"`
		SetProjectTTL    bool       `json:"set_project_ttl"`
		ProjectExpiresAt *time.Time `json:"project_expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(body.Content) == "" {
		http.Error(w, "content is required", http.StatusBadRequest)
		return
	}

	// Validate input per issue #515/#573.
	project := body.Project
	if project == "" {
		project = "default"
	}
	if err := validateQuickStoreInput(body.Content, project, body.Tags, body.Importance); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.ExpiresAt != nil && !body.ExpiresAt.After(time.Now()) {
		writeJSONError(w, http.StatusBadRequest, "expires_at must be a future timestamp")
		return
	}
	if body.SetProjectTTL && body.ProjectExpiresAt == nil {
		writeJSONError(w, http.StatusBadRequest, "project_expires_at is required when set_project_ttl is true")
		return
	}
	if !body.SetProjectTTL && body.ProjectExpiresAt != nil {
		writeJSONError(w, http.StatusBadRequest, "set_project_ttl must be true when project_expires_at is provided")
		return
	}
	if body.ProjectExpiresAt != nil && !body.ProjectExpiresAt.After(time.Now()) {
		writeJSONError(w, http.StatusBadRequest, "project_expires_at must be a future timestamp")
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

	result, err := handleMemoryQuickStore(r.Context(), s.pool, req, Config{EmbedderHealth: s.embedderHealth})
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

	// Project TTL is a project-level deletion signal, so require explicit
	// opt-in instead of deriving it from memory-level expires_at (#1329).
	// Best-effort: failure is logged but does not affect the store response.
	if body.SetProjectTTL {
		if h, poolErr := s.pool.Get(r.Context(), project); poolErr != nil {
			slog.Warn("quick-store: pool.Get failed for SetProjectTTL", "project", project, "err", poolErr)
		} else if ttlErr := h.Engine.Backend().SetProjectTTL(r.Context(), project, time.Now().UTC(), body.ProjectExpiresAt); ttlErr != nil {
			slog.Warn("quick-store: SetProjectTTL failed", "project", project, "err", ttlErr)
		}
	}
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

	// Validate input per issue #515/#573.
	if err := validateQuickRecallInput(body.Project, body.Query); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
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

// reloadRuntimeConfig re-reads config env vars and updates the atomic fields (#557).
// Called by the SIGHUP handler on each signal.
func reloadRuntimeConfig(cfg *RuntimeConfig) {
	var reloadedKeys []string
	if v := os.Getenv("ENGRAM_CLAUDE_SUMMARIZE"); v != "" {
		newVal := v == "true" || v == "1"
		if cfg.ClaudeSummarize.Load() != newVal {
			cfg.ClaudeSummarize.Store(newVal)
			reloadedKeys = append(reloadedKeys, "ENGRAM_CLAUDE_SUMMARIZE")
		}
	}
	if v := os.Getenv("ENGRAM_CLAUDE_CONSOLIDATE"); v != "" {
		newVal := v == "true" || v == "1"
		if cfg.ClaudeConsolidate.Load() != newVal {
			cfg.ClaudeConsolidate.Store(newVal)
			reloadedKeys = append(reloadedKeys, "ENGRAM_CLAUDE_CONSOLIDATE")
		}
	}
	if v := os.Getenv("ENGRAM_CLAUDE_RERANK"); v != "" {
		newVal := v == "true" || v == "1"
		if cfg.ClaudeRerank.Load() != newVal {
			cfg.ClaudeRerank.Store(newVal)
			reloadedKeys = append(reloadedKeys, "ENGRAM_CLAUDE_RERANK")
		}
	}
	if v := os.Getenv("ENGRAM_LOG_LEVEL"); v != "" {
		// Store log level as an int32: debug=0, info=1, warn=2, error=3
		var level int32
		switch strings.ToLower(v) {
		case "debug":
			level = 0
		case "info":
			level = 1
		case "warn":
			level = 2
		case "error":
			level = 3
		default:
			level = 1 // default to info
		}
		if cfg.LogLevel.Load() != level {
			cfg.LogLevel.Store(level)
			reloadedKeys = append(reloadedKeys, "ENGRAM_LOG_LEVEL")
		}
	}
	if len(reloadedKeys) > 0 {
		slog.Info("config reloaded via SIGHUP", "changed_keys", reloadedKeys)
	}
}

// buildBearerMismatchHint returns a verbose, deployment-specific hint only
// when the request originates from loopback. External callers get a generic
// message so the server doesn't leak filesystem paths or the project name
// to unauthenticated network clients (#704).
func buildBearerMismatchHint(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if host == "127.0.0.1" || host == "::1" || host == "localhost" {
		return "Bearer token mismatch — on the host machine run: cd ~/projects/engram-go && make setup  (or: go run ./cmd/engram-setup). Then run /mcp in Claude Code to reconnect."
	}
	return "Bearer token mismatch — check your client configuration."
}
