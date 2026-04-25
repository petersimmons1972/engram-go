// Package mcp registers MCP tools and owns the SSE server lifecycle.
package mcp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
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
	"golang.org/x/time/rate"

	"github.com/petersimmons1972/engram/internal/claude"
)

// rateLimiter holds per-IP token-bucket state for HTTP rate limiting (#140).
type rateLimiter struct {
	mu      sync.Mutex
	clients map[string]*rateLimiterEntry
}

type rateLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// newRateLimiter creates a rate limiter that evicts idle IPs every 5 minutes.
// The eviction goroutine stops when ctx is cancelled.
func newRateLimiter(ctx context.Context) *rateLimiter {
	rl := &rateLimiter{clients: make(map[string]*rateLimiterEntry)}
	go rl.evictLoop(ctx)
	return rl
}

// allow returns true if the request from ip should be allowed.
// Limit: 50 req/s sustained, burst 200. Generous for local/LAN use; the server
// only binds to 127.0.0.1 by default so external abuse is not a concern.
func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	e, ok := rl.clients[ip]
	if !ok {
		e = &rateLimiterEntry{
			limiter: rate.NewLimiter(50, 200), // 50 req/s sustained, burst 200
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

// evict removes entries not seen in the last 5 minutes.
func (rl *rateLimiter) evict() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for ip, e := range rl.clients {
		if time.Since(e.lastSeen) > 5*time.Minute {
			delete(rl.clients, ip)
		}
	}
}

// allowSetupToken is like allow but uses a tighter budget for the /setup-token
// endpoint: 3 requests per 5-minute window (burst 3, one token every ~100s) (#243).
func (rl *rateLimiter) allowSetupToken(ip string) bool {
	rl.mu.Lock()
	e, ok := rl.clients[ip]
	if !ok {
		e = &rateLimiterEntry{
			limiter: rate.NewLimiter(rate.Every(setupTokenWindow), 3),
		}
		rl.clients[ip] = e
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

	// Auto-start a "global" episode on every new SSE client connection (#91).
	// SSE sessions carry no project context, so episodes land in "global" where
	// they can be queried via memory_episode_list/memory_episode_recall.
	//
	// Also store a session→bearer HMAC fingerprint so POST /message can verify
	// the session was established by the same bearer that is now posting (#245).
	s.mcp.GetHooks().AddOnRegisterSession(func(ctx context.Context, sess server.ClientSession) {
		sessionID := sess.SessionID()

		// Compute HMAC(apiKey, sessionID) and store for later verification.
		mac := hmac.New(sha256.New, []byte(apiKey))
		mac.Write([]byte(sessionID))
		s.sessionFingerprints.Store(sessionID, mac.Sum(nil))

		desc := "Claude Code session " + time.Now().UTC().Format(time.RFC3339)
		h, err := s.pool.Get(ctx, "global")
		if err != nil {
			slog.Warn("auto-episode: could not get global engine", "err", err)
			return
		}
		ep, err := h.Engine.Backend().StartEpisode(ctx, "global", desc)
		if err != nil {
			slog.Warn("auto-episode: StartEpisode failed", "err", err)
			return
		}
		slog.Info("auto-episode started", "id", ep.ID, "project", "global", "desc", desc)
	})

	// Remove the fingerprint when a session disconnects (#245).
	s.mcp.GetHooks().AddOnUnregisterSession(func(_ context.Context, sess server.ClientSession) {
		s.sessionFingerprints.Delete(sess.SessionID())
	})

	// setupTokenLimiter enforces 3 calls per 5-minute window per IP (#243).
	setupLimiter := newRateLimiter(ctx) // eviction goroutine shares ctx lifetime

	// Top-level mux routes unauthenticated utility endpoints before auth middleware.
	mux := http.NewServeMux()

	// GET /health — unauthenticated; returns server status for diagnostics and readiness checks.
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"}) //nolint:errcheck
	})

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
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
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
	rl := newRateLimiter(ctx)

	// /message — wrap with session-fingerprint check before the auth middleware.
	msgHandler := s.withSessionFingerprint(sse.MessageHandler(), apiKey)
	mux.Handle("/message", s.applyMiddleware(msgHandler, apiKey, rl))

	// /quick-store — sessionless REST endpoint for hook scripts and CLI callers
	// that cannot establish an SSE session (e.g. PreCompact hooks).
	mux.Handle("/quick-store", s.applyMiddleware(http.HandlerFunc(s.handleQuickStore), apiKey, rl))

	// All other authenticated routes (including /sse) go through the standard middleware.
	mux.Handle("/", s.applyMiddleware(sse, apiKey, rl))

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
		IdleTimeout:       120 * time.Second,
	}

	slog.Info("engram ready — to configure MCP client run: make setup  (or: go run ./cmd/engram-setup)")

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
func (s *Server) applyMiddleware(next http.Handler, apiKey string, rl *rateLimiter) http.Handler {
	// apiKey is always non-empty — enforced by main.go startup check.
	// This guard is a defence-in-depth backstop; it must never be the primary gate.
	if apiKey == "" {
		panic("engram: auth middleware called with empty apiKey — programming error")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Rate limit before auth check to prevent timing-based enumeration.
		remoteHost := s.clientIP(r)
		if !rl.allow(remoteHost) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error":"rate_limited","hint":"too many requests — back off and retry"}`) //nolint:errcheck
			return
		}
		// ConstantTimeCompare leaks length when len(got) != len(want).
		// Use ConstantTimeEq on the HMAC of each side so the comparison is
		// always the same length regardless of input length (#129).
		got := hmac.New(sha256.New, []byte(apiKey))
		got.Write([]byte(r.Header.Get("Authorization")))
		want := hmac.New(sha256.New, []byte(apiKey))
		want.Write([]byte("Bearer " + apiKey))
		if subtle.ConstantTimeCompare(got.Sum(nil), want.Sum(nil)) != 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":"unauthorized","hint":"Bearer token mismatch — run: make setup  (or: go run ./cmd/engram-setup)"}`) //nolint:errcheck
			return
		}
		next.ServeHTTP(w, r)
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
		storedFP, _ := stored.([]byte)

		// Compute the expected fingerprint for the current bearer.
		mac := hmac.New(sha256.New, []byte(apiKey))
		mac.Write([]byte(sessionID))
		expected := mac.Sum(nil)

		if subtle.ConstantTimeCompare(storedFP, expected) != 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":"forbidden","hint":"session bearer mismatch"}`) //nolint:errcheck
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
		if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
			return strings.TrimSpace(realIP)
		}
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			// X-Forwarded-For may be a comma-separated list; leftmost is the client.
			parts := strings.SplitN(forwarded, ",", 2)
			if ip := strings.TrimSpace(parts[0]); ip != "" {
				return ip
			}
		}
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

func (s *Server) registerTools() {
	pool := s.pool
	cfg := s.cfg

	tools := []struct {
		name    string
		desc    string
		handler func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)
	}{
		{"memory_store", "Store a focused memory (<=10k chars)",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				result, err := handleMemoryStore(ctx, pool, req)
				if err != nil {
					return result, err
				}
				// Enqueue entity extraction asynchronously. Runs in a detached
				// goroutine with its own context so it never blocks memory_store.
				// Non-fatal: if the enqueue fails the store has already succeeded.
				args := req.GetArguments()
				project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
				if memID, ok := extractResultID(result); ok {
					go enqueueExtractionAsync(pool, memID, project)
				}
				return result, nil
			}},
		{"memory_store_document", "Store a large document (auto-tiered up to 50 MB via synopsis + raw blob storage)",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryStoreDocument(ctx, pool, req, cfg)
			}},
		{"memory_ingest_document_stream", "Ingest a very large document via server-local path or chunked base64 upload (auto-tiered, up to 50 MB)",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryIngestDocumentStream(ctx, s, pool, req, cfg)
			}},
		{"memory_store_batch", "Store multiple memories in one call",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryStoreBatch(ctx, pool, req)
			}},
		{"memory_recall", "Recall memories by semantic + full-text query",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryRecall(ctx, pool, req, cfg)
			}},
		{"memory_fetch", "Fetch a single memory by ID; detail=summary|chunk|full",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryFetch(ctx, pool, req, cfg)
			}},
		{"memory_list", "List memories with optional filters",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryList(ctx, pool, req)
			}},
		{"memory_connect", "Create a directed relationship between two memories. relation_type values: caused_by, relates_to, depends_on, supersedes, used_in, resolved_by, contradicts, supports, derived_from, part_of, follows",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryConnect(ctx, pool, req)
			}},
		{"memory_correct", "Update content, tags, or importance on an existing memory",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryCorrect(ctx, pool, req)
			}},
		{"memory_forget", "Soft-delete a memory (sets valid_to, preserves history, respects immutability)",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryForget(ctx, pool, req)
			}},
		{"memory_history", "Return the full version chain for a memory",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryHistory(ctx, pool, req)
			}},
		{"memory_timeline", "Recall memories that were active at a given point in time (as_of param, RFC3339)",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryTimeline(ctx, pool, req)
			}},
		{"memory_summarize", "Immediately summarize a memory",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemorySummarize(ctx, pool, req, cfg)
			}},
		{"memory_resummarize", "Clear all summaries for a project — they regenerate automatically within 60s",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryResummarize(ctx, pool, req)
			}},
		{"memory_status", "Return project statistics",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryStatus(ctx, pool, req)
			}},
		{"memory_feedback", "Record retrieval feedback. failure_class values (for misses): vocabulary_mismatch, aggregation_failure, stale_ranking, missing_content, scope_mismatch, other",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryFeedback(ctx, pool, req)
			}},
		{"memory_aggregate", "Group and count memories. by=tag|type|failure_class. filter: optional ILIKE substring — tag mode only, error for failure_class.",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryAggregate(ctx, pool, req)
			}},
		{"memory_consolidate", "Prune stale memories, decay edges, merge near-duplicates",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryConsolidate(ctx, pool, req, cfg)
			}},
		{"memory_sleep", "Run full sleep-consolidation cycle: infer relationships between semantically related memories",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemorySleep(ctx, pool, req, cfg)
			}},
		// Feature 6: Episodic Memory
		{"memory_episode_start", "Start a named episode to group memories from this session",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryEpisodeStart(ctx, pool, req)
			}},
		{"memory_episode_end", "End an episode with an optional summary",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryEpisodeEnd(ctx, pool, req)
			}},
		{"memory_episode_list", "List recent episodes for a project",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryEpisodeList(ctx, pool, req)
			}},
		{"memory_episode_recall", "Return all memories from a specific episode in chronological order",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryEpisodeRecall(ctx, pool, req)
			}},
		{"memory_verify", "Integrity check -- hash coverage and corrupt count",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryVerify(ctx, pool, req)
			}},
		{"memory_migrate_embedder", "Switch embedding model; triggers background re-embedding. Also resets any learned adaptive weights for the project to compile-time defaults.",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryMigrateEmbedder(ctx, pool, req, cfg)
			}},
		{"memory_export_all", "Export all memories to markdown files",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryExportAll(ctx, pool, req, cfg)
			}},
		{"memory_import_claudemd", "Import a CLAUDE.md file as structured memories",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryImportClaudeMD(ctx, pool, req, cfg)
			}},
		{"memory_ingest", "Ingest a file or directory as document memories",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryIngest(ctx, pool, req, cfg)
			}},
		{"memory_ingest_export",
			"Ingest a server-local AI conversation export file (Slack workspace .zip, Claude.ai conversations.json, or ChatGPT conversations.json). Parses the file, auto-detects format, and stores one memory per conversation or channel.",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryIngestExport(ctx, pool, req, cfg)
			}},
		// Feature 4: Cross-Project Knowledge Federation
		{"memory_projects", "List all projects with memory counts",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryProjects(ctx, pool, req)
			}},
		{"memory_adopt", "Create a cross-project reference relationship",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryAdopt(ctx, pool, req)
			}},
		// Simplified front-door tools — wrappers over the expert-surface tools
		// with sensible defaults injected. Designed for LLM orchestrators that
		// do not need the full parameter surface.
		{"memory_quick_store", "Store a memory and automatically extract entities. Simplified front door for memory_store.",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryQuickStore(ctx, pool, req)
			}},
		{"memory_query", "Simplified front door for memory_recall. Accepts a 'limit' param instead of top_k; sensible defaults applied.",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryQuery(ctx, pool, req, cfg)
			}},
		{"memory_expand", "Explore the relationship graph neighbourhood of a known memory.",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryExpand(ctx, pool, req)
			}},
		// Safety constraint verification tools
		{"get_constraints", "List constraint and policy memories relevant to an optional query",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleGetConstraints(ctx, pool, req)
			}},
		{"check_constraints", "Classify a proposed action and return matching constraints with a verification decision",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleCheckConstraints(ctx, pool, req)
			}},
		{"verify_before_acting", "Run the full constraint verification pipeline and return a proceed/warn/require_approval/block decision",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleVerifyBeforeActing(ctx, pool, req)
			}},
		// Phase 5: Pluggable Embedder — model registry and eval
		{"memory_models", "List installed and suggested Ollama embedding models. Shows which suggested models are installed, which is current, and flags the recommended upgrade.",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryModels(ctx, pool, req, cfg)
			}},
		{"memory_embedding_eval", "Compare two Ollama embedding models using probe sentences. model_a defaults to nomic-embed-text; model_b defaults to mxbai-embed-large (recommended). Auto-pulls missing models. Read-only — does not migrate stored embeddings.",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryEmbeddingEval(ctx, pool, req, cfg)
			}},
	}

	for _, t := range tools {
		s.mcp.AddTool(mcpgo.NewTool(t.name, mcpgo.WithDescription(t.desc)), t.handler)
	}

	// Audit and weight tools are always available when PgPool is configured.
	{
		pool := s.pool
		cfg := s.cfg
		s.mcp.AddTool(
			mcpgo.NewTool("memory_audit_add_query",
				mcpgo.WithDescription("Register a canonical query for retrieval drift monitoring")),
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryAuditAddQuery(ctx, pool, req, cfg)
			},
		)
		s.mcp.AddTool(
			mcpgo.NewTool("memory_audit_list_queries",
				mcpgo.WithDescription("List canonical queries registered for drift monitoring in a project")),
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryAuditListQueries(ctx, pool, req, cfg)
			},
		)
		s.mcp.AddTool(
			mcpgo.NewTool("memory_audit_deactivate_query",
				mcpgo.WithDescription("Deactivate a canonical query (stops future drift snapshots)")),
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryAuditDeactivateQuery(ctx, pool, req, cfg)
			},
		)
		s.mcp.AddTool(
			mcpgo.NewTool("memory_audit_run",
				mcpgo.WithDescription("Run a decay audit pass for a project immediately and return snapshot summaries")),
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryAuditRun(ctx, pool, req, cfg)
			},
		)
		s.mcp.AddTool(
			mcpgo.NewTool("memory_audit_compare",
				mcpgo.WithDescription("Compare retrieval snapshots for a canonical query to detect ranking drift")),
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryAuditCompare(ctx, pool, req, cfg)
			},
		)
		s.mcp.AddTool(
			mcpgo.NewTool("memory_weight_history",
				mcpgo.WithDescription("Return current retrieval weights and tuning history for a project")),
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryWeightHistory(ctx, pool, req, cfg)
			},
		)
	}

	// memory_diagnose is always available — no Claude required.
	{
		pool := s.pool
		s.mcp.AddTool(
			mcpgo.NewTool("memory_diagnose",
				mcpgo.WithDescription("Return evidence map for recalled memories: conflicts, confidence, invalidated sources — no synthesis")),
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryDiagnose(ctx, pool, req)
			},
		)
	}

	// memory_reason is registered only when a Claude client is available.
	if s.cfg.ClaudeEnabled {
		pool := s.pool
		cfg := s.cfg
		s.mcp.AddTool(
			mcpgo.NewTool("memory_reason",
				mcpgo.WithDescription("Recall memories and synthesize a grounded answer using Claude")),
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryReason(ctx, pool, req, cfg)
			},
		)
		s.mcp.AddTool(
			mcpgo.NewTool("memory_explore",
				mcpgo.WithDescription("Iterative recall+score+synthesis loop — returns a single grounded answer (A3)")),
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryExplore(ctx, pool, req, cfg)
			},
		)

		// memory_query_document (A5): query a single large document by regex/substring
		// or semantic recall and synthesize an answer with Claude.
		s.mcp.AddTool(
			mcpgo.NewTool("memory_query_document",
				mcpgo.WithDescription("Query a large document stored in memory using regex/substring matching or semantic search. Returns relevant spans and an AI-synthesized answer.")),
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryQueryDocument(ctx, pool, req, cfg)
			},
		)

		// memory_ask (P2): retrieval-augmented question answering with numbered citations.
		s.mcp.AddTool(
			mcpgo.NewTool("memory_ask",
				mcpgo.WithDescription("Answer a question using stored memories as context. Returns answer + numbered citations.")),
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryAsk(ctx, pool, req, cfg)
			},
		)
	}
}

// enqueueExtractionAsync submits memID to the entity extraction queue via pool.
// Intended to run in a detached goroutine; all failures are logged, never surfaced.
func enqueueExtractionAsync(pool *EnginePool, memID, project string) {
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
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
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
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if result.IsError {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"ok": false}) //nolint:errcheck
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
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "id": id}) //nolint:errcheck
}
