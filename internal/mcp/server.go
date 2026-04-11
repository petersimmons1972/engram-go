// Package mcp registers MCP tools and owns the SSE server lifecycle.
package mcp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/petersimmons1972/engram/internal/claude"
)

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

	// Top-level mux routes unauthenticated utility endpoints before auth middleware.
	mux := http.NewServeMux()

	// GET /health — unauthenticated; returns server status for diagnostics and readiness checks.
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"}) //nolint:errcheck
	})

	// GET /setup-token — localhost-only; returns the current bearer token so MCP clients can
	// self-configure without manual copy-paste. Security: same boundary as ~/.claude.json on disk.
	mux.HandleFunc("/setup-token", func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		if host != "127.0.0.1" && host != "::1" {
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

	// All other routes require Bearer authentication.
	mux.Handle("/", s.applyMiddleware(sse, apiKey))

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

func (s *Server) applyMiddleware(next http.Handler, apiKey string) http.Handler {
	// apiKey is always non-empty — enforced by main.go startup check.
	// This guard is a defence-in-depth backstop; it must never be the primary gate.
	if apiKey == "" {
		panic("engram: auth middleware called with empty apiKey — programming error")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("Authorization"))
		want := []byte("Bearer " + apiKey)
		if subtle.ConstantTimeCompare(got, want) != 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":"unauthorized","hint":"Bearer token mismatch — run: make setup  (or: go run ./cmd/engram-setup)"}`)
			return
		}
		next.ServeHTTP(w, r)
	})
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
		{"memory_store_document", "Store a large document (<=500k chars, auto-chunked)",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryStoreDocument(ctx, pool, req)
			}},
		{"memory_store_batch", "Store multiple memories in one call",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryStoreBatch(ctx, pool, req)
			}},
		{"memory_recall", "Recall memories by semantic + full-text query",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryRecall(ctx, pool, req, cfg)
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
				return handleMemorySleep(ctx, pool, req)
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
	}
}
