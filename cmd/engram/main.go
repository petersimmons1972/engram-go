// Command engram runs the Engram MCP memory server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strings"
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
	claudeRerank := fs.Bool("claude-rerank", envBool("ENGRAM_CLAUDE_RERANK", false), "Enable Claude re-ranking in memory recall")
	port := fs.Int("port", envInt("ENGRAM_PORT", 8788), "MCP SSE port")
	host := fs.String("host", envOr("ENGRAM_HOST", "0.0.0.0"), "Bind address")
	apiKey := fs.String("api-key", envOr("ENGRAM_API_KEY", ""), "Bearer token for auth (required; set ENGRAM_API_KEY)")
	dataDir := fs.String("data-dir", envOr("ENGRAM_DATA_DIR", ""), "Base directory for file operations (required when using export/ingest tools)")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	if *databaseURL == "" {
		return fmt.Errorf("DATABASE_URL or --database-url is required")
	}
	if *apiKey == "" {
		return fmt.Errorf("ENGRAM_API_KEY or --api-key is required; refusing to start without authentication")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Validate and sanitize ollamaURL (#4 SSRF, #27 credential logging)
	parsedOllamaURL, err := url.ParseRequestURI(*ollamaURL)
	if err != nil || (parsedOllamaURL.Scheme != "http" && parsedOllamaURL.Scheme != "https") {
		return fmt.Errorf("invalid --ollama-url %q: must be an http:// or https:// URL", *ollamaURL)
	}
	// Block literal private-IP Ollama URLs to prevent SSRF (#55).
	// Hostnames (e.g. "ollama" in Docker Compose) are intentionally excluded:
	// they resolve to private container IPs by design and are not attacker-controlled.
	if ollamaHost := parsedOllamaURL.Hostname(); net.ParseIP(ollamaHost) != nil && isPrivateIP(ollamaHost) {
		return fmt.Errorf("invalid --ollama-url: IP %q is in a private/reserved range (SSRF protection)", ollamaHost)
	}

	safeOllamaURL := *parsedOllamaURL
	safeOllamaURL.User = nil
	slog.Info("connecting to Ollama", "url", safeOllamaURL.String(), "model", *embedModel)

	embedder, err := embed.NewOllamaClient(ctx, *ollamaURL, *embedModel)
	if err != nil {
		return fmt.Errorf("ollama: %w", err)
	}

	// Dimensional guard: verify embedding model produces the expected vector dimension.
	// The pgvector column is vector(768). A mismatch causes silent corruption.
	const expectedDims = 768
	testVec, err := embedder.Embed(ctx, "dimensional guard test")
	if err != nil {
		return fmt.Errorf("dimensional guard: embed test failed: %w", err)
	}
	if len(testVec) != expectedDims {
		return fmt.Errorf("dimensional guard: embedding model produces %d dimensions, but pgvector column is vector(%d) — use a %d-dimension model or run a schema migration", len(testVec), expectedDims, expectedDims)
	}
	slog.Info("dimensional guard passed", "dims", expectedDims)

	dsn := *databaseURL
	ollamaURLVal := *ollamaURL
	sumModel := *summarizeModel
	sumEnabled := *summarizeEnabled

	// embedder satisfies embed.Client; declare as interface so the factory
	// closure captures the interface value, not the concrete pointer.
	var embedClient embed.Client = embedder

	// Construct Claude client if API key is provided. memory_reason is auto-enabled
	// whenever the key is set; the other advisor features require their own flags.
	var claudeCompleter summarize.ClaudeCompleter
	var cc *claude.Client
	if *claudeAPIKey != "" {
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
		ClaudeEnabled:            cc != nil,
		ClaudeConsolidateEnabled: *claudeConsolidate,
		ClaudeRerankEnabled:      *claudeRerank,
		DataDir:                  *dataDir,
	}
	srv := internalmcp.NewServer(pool, cfg)
	if cc != nil {
		srv.SetClaudeClient(cc)
		slog.Info("claude advisor enabled",
			"summarize", *claudeSummarize,
			"consolidate", *claudeConsolidate,
			"rerank", *claudeRerank)
	}

	slog.Info("engram ready", "host", *host, "port", *port,
		"embed_model", *embedModel, "summarize_model", sumModel)
	return srv.Start(ctx, *host, *port, *apiKey)
}

// isPrivateIP reports whether ipStr is an IP address that falls within a
// private, loopback, or link-local range. Only literal IP addresses are
// checked; hostnames must be resolved before calling this function.
// Used to block SSRF via the Ollama URL flag (#55).
func isPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	// Normalize IPv4-mapped IPv6 addresses (::ffff:x.x.x.x) to their IPv4 form
	// so they are checked against the same IPv4 private ranges. This prevents
	// bypassing the check via notation like "::ffff:127.0.0.1".
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	privateRanges := []string{
		"0.0.0.0/8",      // this-network (RFC 1122)
		"10.0.0.0/8",
		"100.64.0.0/10",  // CGNAT (RFC 6598)
		"127.0.0.0/8",    // loopback
		"169.254.0.0/16", // link-local / AWS metadata endpoint
		"172.16.0.0/12",
		"192.168.0.0/16",
		"198.18.0.0/15",  // benchmark testing (RFC 2544)
		"240.0.0.0/4",    // reserved (RFC 1112)
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 ULA
		"fe80::/10",      // IPv6 link-local
	}
	for _, cidr := range privateRanges {
		_, n, _ := net.ParseCIDR(cidr)
		if n.Contains(ip) {
			return true
		}
	}
	return false
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
	v := strings.ToLower(os.Getenv(key))
	switch v {
	case "1", "true", "yes":
		return true
	case "0", "false", "no":
		return false
	default:
		return def
	}
}
