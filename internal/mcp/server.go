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
func newRateLimiter() *rateLimiter {
	rl := &rateLimiter{clients: make(map[string]*rateLimiterEntry)}
	go rl.evict()
	return rl
}

// allow returns true if the request from ip should be allowed.
// Limit: 60 requests/minute with a burst of 20.
func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	e, ok := rl.clients[ip]
	if !ok {
		e = &rateLimiterEntry{
			limiter: rate.NewLimiter(rate.Every(time.Second), 20), // 1 req/s sustained, burst 20
		}
		rl.clients[ip] = e
	}
	e.lastSeen = time.Now()
	ok = e.limiter.Allow()
	rl.mu.Unlock()
	return ok
}

// evict removes entries not seen in the last 5 minutes.
func (rl *rateLimiter) evict() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		for ip, e := range rl.clients {
			if time.Since(e.lastSeen) > 5*time.Minute {
				delete(rl.clients, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// Server wraps the MCP SSE server and owns the EnginePool.
type Server struct {
	pool *EnginePool
	cfg  Config
	mcp  *server.MCPServer
}

// NewServer constructs a Server with all MCP tools registered.
func NewServer(pool *EnginePool, cfg Config) *Server {
	s := &Server{pool: pool, cfg: cfg}
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
	s.mcp.GetHooks().AddOnRegisterSession(func(ctx context.Context, _ server.ClientSession) {
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
	mux.HandleFunc("/setup-token", func(w http.ResponseWriter, r *http.Request) {
		remoteHost, _, _ := net.SplitHostPort(r.RemoteAddr)
		if !isLocalAddress(remoteHost) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
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
	rl := newRateLimiter()
	mux.Handle("/", s.applyMiddleware(sse, apiKey, rl))

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
		remoteHost, _, _ := net.SplitHostPort(r.RemoteAddr)
		if !rl.allow(remoteHost) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error":"rate_limited","hint":"too many requests — back off and retry"}`)
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
			fmt.Fprint(w, `{"error":"unauthorized","hint":"Bearer token mismatch — run: make setup  (or: go run ./cmd/engram-setup)"}`)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isLocalAddress reports whether the IP string is a loopback or RFC1918 private address.
// Used by /setup-token to accept requests arriving via Docker's NAT gateway (which presents
// as a Docker bridge IP, not 127.0.0.1, even when the port is host-loopback-bound).
func isLocalAddress(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	local := []string{
		"127.0.0.0/8",    // loopback
		"::1/128",        // IPv6 loopback
		"10.0.0.0/8",     // RFC1918 — Docker bridge networks (10.x.x.x)
		"172.16.0.0/12",  // RFC1918 — Docker default bridge (172.17-31.x.x)
		"192.168.0.0/16", // RFC1918 — Docker custom networks
	}
	for _, cidr := range local {
		_, n, _ := net.ParseCIDR(cidr)
		if n.Contains(ip) {
			return true
		}
	}
	return false
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
				return handleMemoryStore(ctx, pool, req)
			}},
		{"memory_store_document", "Store a large document (auto-tiered up to 50 MB via synopsis + raw blob storage)",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryStoreDocument(ctx, pool, req, cfg)
			}},
		{"memory_ingest_document_stream", "Ingest a very large document via server-local path or chunked base64 upload (auto-tiered, up to 50 MB)",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryIngestDocumentStream(ctx, pool, req, cfg)
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
		{"memory_connect", "Create a directed relationship between two memories",
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
		{"memory_feedback", "Record positive access signal for memories",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryFeedback(ctx, pool, req)
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
		{"memory_migrate_embedder", "Switch embedding model; triggers background re-embedding",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryMigrateEmbedder(ctx, pool, req)
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
		// Feature 4: Cross-Project Knowledge Federation
		{"memory_projects", "List all projects with memory counts",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryProjects(ctx, pool, req)
			}},
		{"memory_adopt", "Create a cross-project reference relationship",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryAdopt(ctx, pool, req)
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
	}

	for _, t := range tools {
		s.mcp.AddTool(mcpgo.NewTool(t.name, mcpgo.WithDescription(t.desc)), t.handler)
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
	}
}
