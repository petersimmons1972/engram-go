// Command engram runs the Engram MCP memory server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/entity"
	internalmcp "github.com/petersimmons1972/engram/internal/mcp"
	"github.com/petersimmons1972/engram/internal/netutil"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/summarize"
	"github.com/petersimmons1972/engram/internal/types"

	// Register pprof HTTP handlers at /debug/pprof/ (localhost only).
	_ "net/http/pprof"
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
	// #136: ANTHROPIC_API_KEY is intentionally NOT a CLI flag — secrets in CLI flags
	// are visible in /proc/cmdline to any process on the host. Read from env only.
	claudeAPIKey := envOr("ANTHROPIC_API_KEY", "")
	claudeSummarize := fs.Bool("claude-summarize", envBool("ENGRAM_CLAUDE_SUMMARIZE", false), "Use Claude for background summarization")
	claudeConsolidate := fs.Bool("claude-consolidate", envBool("ENGRAM_CLAUDE_CONSOLIDATE", false), "Use Claude for near-duplicate merge during consolidation")
	claudeRerank := fs.Bool("claude-rerank", envBool("ENGRAM_CLAUDE_RERANK", false), "Enable Claude re-ranking in memory recall")
	port := fs.Int("port", envInt("ENGRAM_PORT", 8788), "MCP SSE port")
	host := fs.String("host", envOr("ENGRAM_HOST", "0.0.0.0"), "Bind address")
	baseURL := fs.String("base-url", envOr("ENGRAM_BASE_URL", ""), "External URL advertised in SSE events (e.g. http://127.0.0.1:8788); defaults to http://<host>:<port>")
	// #136: ENGRAM_API_KEY is intentionally NOT a CLI flag — secrets in CLI flags
	// are visible in /proc/cmdline to any process on the host. Read from env only.
	apiKey := envOr("ENGRAM_API_KEY", "")
	dataDir := fs.String("data-dir", envOr("ENGRAM_DATA_DIR", ""), "Base directory for file operations (required when using export/ingest tools)")
	decayInterval := fs.Duration("decay-interval", envDuration("ENGRAM_DECAY_INTERVAL", 0), "How often the importance decay worker runs (0 = default 8h)")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	if *databaseURL == "" {
		return fmt.Errorf("DATABASE_URL or --database-url is required")
	}
	if apiKey == "" {
		return fmt.Errorf("ENGRAM_API_KEY or --api-key is required; refusing to start without authentication")
	}

	// Unset secrets from the process environment after reading (#139, #141).
	// This limits exposure via /proc/self/environ to the startup window only.
	// Note: this reduces but does not eliminate the window — the kernel may cache
	// the original environ in /proc until the process exits.
	_ = os.Unsetenv("ENGRAM_API_KEY")
	_ = os.Unsetenv("ANTHROPIC_API_KEY")

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
	if ollamaHost := parsedOllamaURL.Hostname(); net.ParseIP(ollamaHost) != nil && netutil.IsPrivateIP(ollamaHost) {
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
	if claudeAPIKey != "" {
		var err error
		cc, err = claude.New(claudeAPIKey)
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
		engine := search.New(ctx, backend, embedClient, project, ollamaURLVal, sumModel, sumEnabled, claudeCompleter, *decayInterval)
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
		RecallDefaultMode:        envOr("ENGRAM_RECALL_DEFAULT_MODE", "handle"),
		FetchMaxBytes:            envInt("ENGRAM_FETCH_MAX_BYTES", 65536),
		ExploreMaxIters:          envInt("ENGRAM_EXPLORE_MAX_ITERS", 5),
		ExploreMaxWorkers:        envInt("ENGRAM_EXPLORE_MAX_WORKERS", 8),
		ExploreTokenBudget:       envInt("ENGRAM_EXPLORE_TOKEN_BUDGET", 20000),
		MaxDocumentBytes:         envInt("ENGRAM_MAX_DOCUMENT_BYTES", 8*1024*1024),
		RawDocumentMaxBytes:      envInt("ENGRAM_RAW_DOCUMENT_MAX_BYTES", 50*1024*1024),
		RAGMaxTokens:             envInt("ENGRAM_RAG_MAX_TOKENS", 4096),
		AllowRFC1918SetupToken:   envBool("ENGRAM_SETUP_TOKEN_ALLOW_RFC1918", false),
	}
	srv := internalmcp.NewServer(pool, cfg)
	if cc != nil {
		srv.SetClaudeClient(cc)
		slog.Info("claude advisor enabled",
			"summarize", *claudeSummarize,
			"consolidate", *claudeConsolidate,
			"rerank", *claudeRerank)
	}

	// Start entity extraction worker if Claude is enabled and projects are configured.
	// The worker polls each project's extraction job queue and runs Claude to identify
	// named entities and relations, building the GraphRAG entity index.
	entityProjects := strings.Split(envOr("ENGRAM_ENTITY_PROJECTS", ""), ",")
	filteredProjects := entityProjects[:0]
	for _, p := range entityProjects {
		if p != "" {
			filteredProjects = append(filteredProjects, p)
		}
	}
	entityProjects = filteredProjects

	var entityBackends []db.Backend
	defer func() {
		for _, b := range entityBackends {
			b.Close()
		}
	}()
	if cc != nil && len(entityProjects) > 0 {
		for _, proj := range entityProjects {
			proj := proj // capture for goroutine
			entityBackend, err := db.NewPostgresBackend(ctx, proj, dsn)
			if err != nil {
				slog.Warn("entity worker: could not open backend, skipping project",
					"project", proj, "err", err)
				continue
			}
			entityBackends = append(entityBackends, entityBackend)
			adapter := &entityDBAdapter{backend: entityBackend}
			extractor := entity.NewClaudeExtractor(cc)
			w := entity.NewWorker(adapter, extractor, entity.WorkerConfig{
				Projects:     []string{proj},
				PollInterval: time.Duration(envInt("ENGRAM_ENTITY_POLL_SECONDS", 5)) * time.Second,
				BatchSize:    envInt("ENGRAM_ENTITY_BATCH_SIZE", 10),
			})
			go w.Run(ctx)
			slog.Info("entity extraction worker started", "project", proj)
		}
	}

	// Start pprof profiling server on localhost:6060.
	// Bound to loopback only — not reachable from outside the host.
	// Requires no auth because it's loopback-only and the API key protects
	// the MCP port; operators can SSH-forward if they need remote access.
	go func() {
		slog.Info("pprof listening", "addr", "localhost:6060")
		if err := http.ListenAndServe("localhost:6060", nil); err != nil {
			slog.Warn("pprof server stopped", "err", err)
		}
	}()

	slog.Info("engram ready", "host", *host, "port", *port,
		"embed_model", *embedModel, "summarize_model", sumModel)
	return srv.Start(ctx, *host, *port, apiKey, *baseURL)
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

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// entityDBAdapter adapts db.Backend to entity.WorkerBackend.
// The only mismatch between the two interfaces is ClaimExtractionJobs:
// db.Backend returns []db.ExtractionJob, while entity.WorkerBackend returns
// []entity.ExtractionJob. Both types have identical fields (ID, MemoryID,
// Project). All other methods are signature-compatible.
type entityDBAdapter struct {
	backend db.Backend
}

func (a *entityDBAdapter) ClaimExtractionJobs(ctx context.Context, project string, limit int) ([]entity.ExtractionJob, error) {
	dbJobs, err := a.backend.ClaimExtractionJobs(ctx, project, limit)
	if err != nil {
		return nil, err
	}
	out := make([]entity.ExtractionJob, len(dbJobs))
	for i, j := range dbJobs {
		out[i] = entity.ExtractionJob{ID: j.ID, MemoryID: j.MemoryID, Project: j.Project}
	}
	return out, nil
}

func (a *entityDBAdapter) CompleteExtractionJob(ctx context.Context, jobID string, err error) error {
	return a.backend.CompleteExtractionJob(ctx, jobID, err)
}

func (a *entityDBAdapter) GetMemory(ctx context.Context, id string) (*types.Memory, error) {
	return a.backend.GetMemory(ctx, id)
}

func (a *entityDBAdapter) GetEntitiesByProject(ctx context.Context, project string) ([]entity.Entity, error) {
	return a.backend.GetEntitiesByProject(ctx, project)
}

func (a *entityDBAdapter) UpsertEntity(ctx context.Context, e *entity.Entity) (string, error) {
	return a.backend.UpsertEntity(ctx, e)
}
