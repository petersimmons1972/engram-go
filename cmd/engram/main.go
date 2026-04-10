// Command engram runs the Engram MCP memory server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	internalmcp "github.com/petersimmons1972/engram/internal/mcp"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/summarize"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	fs := flag.NewFlagSet("engram", flag.ExitOnError)

	databaseURL := fs.String("database-url", envOr("DATABASE_URL", ""), "PostgreSQL DSN (required)")
	ollamaURL := fs.String("ollama-url", envOr("OLLAMA_URL", "http://ollama:11434"), "Ollama base URL")
	embedModel := fs.String("model", envOr("ENGRAM_OLLAMA_MODEL", "nomic-embed-text"), "Embedding model")
	summarizeModel := fs.String("summarize-model", envOr("ENGRAM_SUMMARIZE_MODEL", "llama3.2"), "Summarization model")
	summarizeEnabled := fs.Bool("summarize", envBool("ENGRAM_SUMMARIZE_ENABLED", true), "Enable background summarization")
	claudeAPIKey := fs.String("claude-api-key", envOr("ANTHROPIC_API_KEY", ""), "Anthropic API key for Claude advisor")
	claudeSummarize := fs.Bool("claude-summarize", envBool("ENGRAM_CLAUDE_SUMMARIZE", false), "Use Claude for background summarization")
	claudeConsolidate := fs.Bool("claude-consolidate", envBool("ENGRAM_CLAUDE_CONSOLIDATE", false), "Use Claude for near-duplicate merge during consolidation")
	port := fs.Int("port", envInt("ENGRAM_PORT", 8788), "MCP SSE port")
	host := fs.String("host", envOr("ENGRAM_HOST", "0.0.0.0"), "Bind address")
	apiKey := fs.String("api-key", envOr("ENGRAM_API_KEY", ""), "Optional bearer token (empty = no auth)")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	if *databaseURL == "" {
		return fmt.Errorf("DATABASE_URL or --database-url is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	slog.Info("connecting to Ollama", "url", *ollamaURL, "model", *embedModel)
	embedder, err := embed.NewOllamaClient(ctx, *ollamaURL, *embedModel)
	if err != nil {
		return fmt.Errorf("ollama: %w", err)
	}

	dsn := *databaseURL
	ollamaURLVal := *ollamaURL
	sumModel := *summarizeModel
	sumEnabled := *summarizeEnabled

	// embedder satisfies embed.Client; declare as interface so the factory
	// closure captures the interface value, not the concrete pointer.
	var embedClient embed.Client = embedder

	// Construct Claude client if API key provided and any Claude advisor feature is enabled.
	var claudeCompleter summarize.ClaudeCompleter
	var cc *claude.Client
	if *claudeAPIKey != "" && (*claudeSummarize || *claudeConsolidate) {
		var err error
		cc, err = claude.New(*claudeAPIKey)
		if err != nil {
			return fmt.Errorf("claude client: %w", err)
		}
		if *claudeSummarize {
			claudeCompleter = cc
		}
	}

	factory := func(ctx context.Context, project string) (*internalmcp.EngineHandle, error) {
		backend, err := db.NewPostgresBackend(ctx, project, dsn)
		if err != nil {
			return nil, fmt.Errorf("postgres backend for project %q: %w", project, err)
		}
		engine := search.New(ctx, backend, embedClient, project, ollamaURLVal, sumModel, sumEnabled, claudeCompleter)
		return &internalmcp.EngineHandle{Engine: engine}, nil
	}

	pool := internalmcp.NewEnginePool(factory)
	defer pool.Close()

	cfg := internalmcp.Config{
		OllamaURL:                *ollamaURL,
		SummarizeModel:           *summarizeModel,
		SummarizeEnabled:         *summarizeEnabled,
		ClaudeConsolidateEnabled: *claudeConsolidate,
	}
	srv := internalmcp.NewServer(pool, cfg)
	if cc != nil {
		srv.SetClaudeClient(cc)
	}

	slog.Info("engram ready", "host", *host, "port", *port,
		"embed_model", *embedModel, "summarize_model", sumModel)
	return srv.Start(ctx, *host, *port, *apiKey)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		return v == "1" || v == "true" || v == "yes"
	}
	return def
}
