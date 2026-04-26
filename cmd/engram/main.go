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

	"github.com/petersimmons1972/engram/internal/audit"
	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/entity"
	internalmcp "github.com/petersimmons1972/engram/internal/mcp"
	"github.com/petersimmons1972/engram/internal/netutil"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/summarize"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/petersimmons1972/engram/internal/weight"

	// Register pprof HTTP handlers at /debug/pprof/ (localhost only).
	_ "net/http/pprof"
)

// Version is injected at build time via -ldflags "-X main.Version=$(git describe --tags --always)"
var Version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	// Configure structured JSON logging when running in a container or when
	// ENGRAM_LOG_FORMAT=json. Auto-detects container by checking TERM absence.
	logFormat := os.Getenv("ENGRAM_LOG_FORMAT")
	logLevel := slog.LevelInfo
	if lvl := os.Getenv("ENGRAM_LOG_LEVEL"); lvl != "" {
		if err := logLevel.UnmarshalText([]byte(lvl)); err != nil {
			// Invalid level — keep INFO, log the issue after handler is set.
			_ = err
		}
	}
	if logFormat == "json" || (logFormat == "" && os.Getenv("TERM") == "") {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			AddSource: true,
			Level:     logLevel,
		})))
	} else if logLevel != slog.LevelInfo {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: logLevel,
		})))
	}

	fs := flag.NewFlagSet("engram", flag.ExitOnError)

	versionFlag := fs.Bool("version", false, "print version and exit")
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
	auditInterval := fs.Duration("audit-interval", envDuration("ENGRAM_AUDIT_INTERVAL", 6*time.Hour), "How often the decay audit worker runs (default 6h)")
	weightInterval := fs.Duration("weight-interval", envDuration("ENGRAM_WEIGHT_INTERVAL", 24*time.Hour), "How often the weight tuner worker runs (default 24h)")

	// Knobs previously only configurable via environment variables — registered as flags
	// so they appear in --help. Env vars remain supported as defaults (closes #306).
	recallDefaultMode := fs.String("recall-default-mode", envOr("ENGRAM_RECALL_DEFAULT_MODE", "handle"), "Default recall output mode (handle|full|summary|id_only)")
	fetchMaxBytes := fs.Int("fetch-max-bytes", envInt("ENGRAM_FETCH_MAX_BYTES", 65536), "Maximum bytes returned by memory_fetch")
	exploreMaxIters := fs.Int("explore-max-iters", envInt("ENGRAM_EXPLORE_MAX_ITERS", 5), "Maximum iterations for explore_context")
	exploreMaxWorkers := fs.Int("explore-max-workers", envInt("ENGRAM_EXPLORE_MAX_WORKERS", 8), "Maximum parallel workers for explore_context")
	exploreTokenBudget := fs.Int("explore-token-budget", envInt("ENGRAM_EXPLORE_TOKEN_BUDGET", 20000), "Token budget for explore_context")
	maxDocumentBytes := fs.Int("max-document-bytes", envInt("ENGRAM_MAX_DOCUMENT_BYTES", 8*1024*1024), "Maximum document size for ingest operations")
	rawDocumentMaxBytes := fs.Int("raw-document-max-bytes", envInt("ENGRAM_RAW_DOCUMENT_MAX_BYTES", 50*1024*1024), "Maximum raw document size before rejection")
	ragMaxTokens := fs.Int("rag-max-tokens", envInt("ENGRAM_RAG_MAX_TOKENS", 4096), "Maximum tokens for RAG context assembly")
	entityProjectsFlag := fs.String("entity-projects", envOr("ENGRAM_ENTITY_PROJECTS", ""), "Comma-separated list of projects to run entity extraction on")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	if *versionFlag {
		fmt.Printf("engram %s\n", Version)
		os.Exit(0)
	}

	if *databaseURL == "" {
		return fmt.Errorf("DATABASE_URL or --database-url is required")
	}
	if apiKey == "" {
		return fmt.Errorf("ENGRAM_API_KEY environment variable is required (--api-key flag intentionally omitted — see issue #136)")
	}

	// Unset secrets from the process environment after reading (#139, #141, #250).
	// Reduces the exposure window for credentials in /proc/self/environ.
	_ = os.Unsetenv("ENGRAM_API_KEY")
	_ = os.Unsetenv("ANTHROPIC_API_KEY")
	_ = os.Unsetenv("DATABASE_URL")

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

	// Dimensional guard: verify the embedding model returns a non-empty vector.
	// Schema compatibility (pgvector column width) is enforced at insert time —
	// a dimension mismatch produces a clear error on the first Store call.
	//
	// E6: the probe is bounded to 5 seconds and is non-fatal. If Ollama is
	// unavailable at startup (slow pull, not yet ready, transient outage), the
	// server starts in degraded mode: /health reports "ollama":"degraded" with
	// HTTP 200 so the pod is considered live. Embedding calls will fail until
	// Ollama recovers, which is preferable to refusing to start at all.
	ollamaDegraded := false
	embedCtx, embedCancel := context.WithTimeout(context.Background(), 5*time.Second)
	testVec, embedErr := embedder.Embed(embedCtx, "startup probe")
	embedCancel()
	if embedErr != nil {
		slog.Warn("Ollama unavailable at startup — embedding degraded; server will start anyway", "error", embedErr)
		ollamaDegraded = true
	} else if len(testVec) == 0 {
		slog.Warn("Ollama startup probe returned empty vector — embedding degraded; server will start anyway")
		ollamaDegraded = true
	} else {
		slog.Info("Ollama embedding verified at startup", "dims", len(testVec))
	}

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

	// Start the retrieval_events retention worker on a shared backend.
	// A dedicated backend is used so the worker lifecycle is independent of
	// per-project factory calls. Project "default" is used for the connection;
	// the DELETE query itself targets all projects via no project filter.
	retentionBackend, err := db.NewPostgresBackend(ctx, "default", dsn)
	if err != nil {
		return fmt.Errorf("retention worker backend: %w", err)
	}
	defer retentionBackend.Close()
	go retentionBackend.StartRetentionWorker(ctx)

	// Audit and weight tuner workers use the shared retention backend pool.
	// A recaller adapter is wired after the pool is constructed below.
	sharedPool := retentionBackend.Pool()

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
		EmbedModel:               *embedModel,
		SummarizeModel:           *summarizeModel,
		SummarizeEnabled:         *summarizeEnabled,
		ClaudeEnabled:            cc != nil,
		ClaudeConsolidateEnabled: *claudeConsolidate,
		ClaudeRerankEnabled:      *claudeRerank,
		DataDir:                  *dataDir,
		RecallDefaultMode:        *recallDefaultMode,
		FetchMaxBytes:            *fetchMaxBytes,
		ExploreMaxIters:          *exploreMaxIters,
		ExploreMaxWorkers:        *exploreMaxWorkers,
		ExploreTokenBudget:       *exploreTokenBudget,
		MaxDocumentBytes:         *maxDocumentBytes,
		RawDocumentMaxBytes:      *rawDocumentMaxBytes,
		RAGMaxTokens:             *ragMaxTokens,
		AllowRFC1918SetupToken:   envBool("ENGRAM_SETUP_TOKEN_ALLOW_RFC1918", false),
		PgPool:                   sharedPool,
		OllamaDegraded:           ollamaDegraded,
	}
	srv := internalmcp.NewServer(pool, cfg)

	// Start audit worker — monitors ranking drift by re-running canonical queries.
	auditRecaller := &auditRecallerAdapter{pool: pool}
	auditWorker := audit.NewAuditWorker(sharedPool, auditRecaller, *embedModel, *auditInterval)
	go auditWorker.Run(ctx)
	slog.Info("audit worker started", "interval", auditInterval.String())

	// Start weight tuner worker — adjusts per-project weights on dominant failure classes.
	tunerWorker := weight.NewTunerWorker(sharedPool, *weightInterval)
	go tunerWorker.Run(ctx)
	slog.Info("weight tuner started", "interval", weightInterval.String())
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
	entityProjects := strings.Split(*entityProjectsFlag, ",")
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

	// Start pprof profiling server on localhost:6060 when ENGRAM_PPROF=1.
	// Bound to loopback only — not reachable from outside the host.
	// Operators can SSH-forward if they need remote access.
	if os.Getenv("ENGRAM_PPROF") != "" {
		go func() {
			slog.Info("pprof listening", "addr", "localhost:6060")
			if err := http.ListenAndServe("localhost:6060", nil); err != nil { //nolint // nosemgrep: go.lang.security.audit.net.use-tls.use-tls -- loopback-only, gated by ENGRAM_PPROF env var
				slog.Warn("pprof server stopped", "err", err)
			}
		}()
	}

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

// auditRecallerAdapter adapts the engine pool to the audit.Recaller interface.
type auditRecallerAdapter struct {
	pool *internalmcp.EnginePool
}

func (a *auditRecallerAdapter) Recall(ctx context.Context, project, query string, topK int) ([]string, error) {
	h, err := a.pool.Get(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("audit recaller: get engine for project %q: %w", project, err)
	}
	if h == nil || h.Engine == nil {
		return nil, fmt.Errorf("audit recaller: no engine for project %q", project)
	}
	results, err := h.Engine.Recall(ctx, query, topK, "id_only")
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.Memory.ID
	}
	return ids, nil
}
