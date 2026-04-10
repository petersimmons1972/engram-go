// Package mcp registers MCP tools and owns the SSE server lifecycle.
package mcp

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
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

// NewServer constructs a Server with all 18 tools registered.
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
// Must be called before Start. Will be expanded to a ClaudeDoer interface in Task 4.3.
func (s *Server) SetClaudeClient(client *claude.Client) {
	s.cfg.claudeClient = client
}

// Start begins serving SSE on host:port. Blocks until ctx is cancelled.
func (s *Server) Start(ctx context.Context, host string, port int, apiKey string) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	slog.Info("engram MCP server starting", "addr", addr)

	sse := server.NewSSEServer(s.mcp, server.WithBaseURL(fmt.Sprintf("http://%s", addr)))
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           s.applyMiddleware(sse, apiKey),
		ReadHeaderTimeout: 10 * time.Second,
	}

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
	if apiKey == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("Authorization"))
		want := []byte("Bearer " + apiKey)
		if subtle.ConstantTimeCompare(got, want) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
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
				return handleMemoryRecall(ctx, pool, req)
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
		{"memory_forget", "Delete a memory (respects immutability)",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryForget(ctx, pool, req)
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
				return handleMemoryExportAll(ctx, pool, req)
			}},
		{"memory_import_claudemd", "Import a CLAUDE.md file as structured memories",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryImportClaudeMD(ctx, pool, req)
			}},
		{"memory_dump", "Dump raw memory files to a directory",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryDump(ctx, pool, req)
			}},
		{"memory_ingest", "Ingest a file or directory as document memories",
			func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return handleMemoryIngest(ctx, pool, req)
			}},
	}

	for _, t := range tools {
		s.mcp.AddTool(mcpgo.NewTool(t.name, mcpgo.WithDescription(t.desc)), t.handler)
	}
}
