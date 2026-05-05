// Command engram runs the Engram MCP memory server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/petersimmons1972/engram/internal/audit"
	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/entity"
	"github.com/petersimmons1972/engram/internal/ingestqueue"
	internalmcp "github.com/petersimmons1972/engram/internal/mcp"
	"github.com/petersimmons1972/engram/internal/metrics"
	"github.com/petersimmons1972/engram/internal/netutil"
	"github.com/petersimmons1972/engram/internal/reembed"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/summarize"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/petersimmons1972/engram/internal/weight"
)

// Version is injected at build time via -ldflags "-X main.Version=$(git describe --tags --always)".
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
	logLevelErr := error(nil)
	if lvl := os.Getenv("ENGRAM_LOG_LEVEL"); lvl != "" {
		if err := logLevel.UnmarshalText([]byte(lvl)); err != nil {
			// Invalid level — keep INFO, but remember to log the issue after handler is set.
			logLevelErr = err
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
	// Log warning about invalid log level after handler is initialized
	if logLevelErr != nil {
		slog.Warn("invalid ENGRAM_LOG_LEVEL value", "value", os.Getenv("ENGRAM_LOG_LEVEL"), "error", logLevelErr, "using", "INFO")
	}

	fs := flag.NewFlagSet("engram", flag.ExitOnError)

	versionFlag := fs.Bool("version", false, "print version and exit")
	databaseURL := fs.String("database-url", envOr("DATABASE_URL", ""), "PostgreSQL DSN (required)")
	litellmURL := fs.String("litellm-url", envOr("LITELLM_URL", "http://litellm:4000"), "LiteLLM base URL (generation)")
	litellmAPIKey := envOr("LITELLM_API_KEY", "")
	// ENGRAM_EMBED_URL overrides LITELLM_URL for embeddings only — use when the
	// embed backend is different from the generation backend (e.g. direct llama.cpp).
	embedURL := envOr("ENGRAM_EMBED_URL", envOr("LITELLM_URL", "http://litellm:4000"))
	embedModel := fs.String("model", envOr("ENGRAM_EMBED_MODEL", envOr("ENGRAM_OLLAMA_MODEL", "")), "Embedding model (required; set ENGRAM_EMBED_MODEL or --model)")
	embedDims := fs.Int("embed-dims", envInt("ENGRAM_EMBED_DIMENSIONS", 0), "MRL truncation target for embedding model (0 = native output)")
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
	decayInterval := fs.Duration("decay-interval", envDuration("ENGRAM_DECAY_INTERVAL", 0), "How often the importance decay worker runs (0 = disabled)")
	auditInterval := fs.Duration("audit-interval", envDuration("ENGRAM_AUDIT_INTERVAL", 6*time.Hour), "How often the decay audit worker runs")
	weightInterval := fs.Duration("weight-interval", envDuration("ENGRAM_WEIGHT_INTERVAL", 24*time.Hour), "How often the weight tuner worker runs")

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
	rateLimit := fs.Float64("rate-limit", envFloat("ENGRAM_RATE_LIMIT", 0), "DEPRECATED: use --rate-limit-rps instead. Per-IP HTTP rate limit in req/s (0 = unlimited, recommended for local use)")
	entityProjectsFlag := fs.String("entity-projects", envOr("ENGRAM_ENTITY_PROJECTS", ""), "Comma-separated list of projects to run entity extraction on")

	// Rate limiter knobs (#387, #560): configurable per-IP rate limit and loopback auto-disable.
	// When both --rate-limit and --rate-limit-rps are set, --rate-limit-rps takes precedence (#560).
	rateLimitRPS := fs.Int("rate-limit-rps", envInt("ENGRAM_RATE_LIMIT_RPS", 0), "Per-IP sustained request rate limit in req/s (0 = default 50) — preferred over --rate-limit")
	rateLimitBurst := fs.Int("rate-limit-burst", envInt("ENGRAM_RATE_LIMIT_BURST", 0), "Per-IP token-bucket burst size (0 = default 200)")
	rateLimitDisable := fs.Bool("rate-limit-disable", envBool("ENGRAM_RATE_LIMIT_DISABLE", false), "Disable HTTP rate limiting entirely (single-user local use)")

	healthcheckFlag := fs.Bool("healthcheck", false, "probe /health and exit 0 (healthy) or 1 (unhealthy) — for use as Docker HEALTHCHECK CMD")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	if *versionFlag {
		fmt.Printf("engram %s\n", Version)
		os.Exit(0)
	}

	// Rate-limit precedence: --rate-limit-rps wins over --rate-limit (#560).
	// Log a warning if both are set so users understand which one is active.
	if *rateLimit != 0 && *rateLimitRPS != 0 {
		slog.Warn("both --rate-limit and --rate-limit-rps are set; using --rate-limit-rps (deprecated flag --rate-limit is ignored)",
			"rate_limit", *rateLimit, "rate_limit_rps", *rateLimitRPS)
	}

	// Docker HEALTHCHECK support — distroless images have no shell or wget,
	// so CMD-SHELL form is unusable. This flag lets the binary probe its own
	// /health endpoint and exit with the appropriate code. See issue #341.
	// Must use *port flag (parsed above), not envInt("ENGRAM_PORT") — fixes #544.
	if *healthcheckFlag {
		hcCtx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		apiKeyForProbe := apiKey
		// Read API key before it gets unset; only add auth header if key was set
		req, err := http.NewRequestWithContext(hcCtx, http.MethodGet, fmt.Sprintf("http://localhost:%d/health", *port), nil)
		if err != nil {
			os.Exit(1)
		}
		if apiKeyForProbe != "" {
			req.Header.Set("Authorization", "Bearer "+apiKeyForProbe)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		_ = resp.Body.Close()
		os.Exit(0)
	}

	if *databaseURL == "" {
		return fmt.Errorf("DATABASE_URL or --database-url is required")
	}
	if apiKey == "" {
		return fmt.Errorf("ENGRAM_API_KEY environment variable is required (--api-key flag intentionally omitted — see issue #136)")
	}
	if strings.TrimSpace(*embedModel) == "" {
		return fmt.Errorf("ENGRAM_EMBED_MODEL or --model is required (no compile-time default)")
	}

	// Warn on inconsistent embed config before spending time connecting to Ollama. (#380)
	if warn := validateEmbedConfig(*embedModel, *embedDims); warn != "" {
		slog.Warn("embed config warning", "detail", warn)
	}

	// Unset secrets from the process environment after reading (#139, #141, #250, #549).
	// Reduces the exposure window for credentials in /proc/self/environ.
	_ = os.Unsetenv("ENGRAM_API_KEY")
	_ = os.Unsetenv("ANTHROPIC_API_KEY")
	_ = os.Unsetenv("DATABASE_URL")
	_ = os.Unsetenv("LITELLM_API_KEY")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Validate LiteLLM URL (SSRF protection — block private/reserved IPs and hostnames that resolve to them) (#549).
	if err := netutil.ValidateUpstreamURL(*litellmURL); err != nil {
		return fmt.Errorf("invalid --litellm-url %q: %w (#549)", *litellmURL, err)
	}
	parsedLiteLLMURL, _ := url.ParseRequestURI(*litellmURL)
	safeLiteLLMURL := *parsedLiteLLMURL
	safeLiteLLMURL.User = nil
	slog.Info("connecting to LiteLLM", "url", safeLiteLLMURL.String(), "model", *embedModel)

	// Validate ENGRAM_EMBED_URL if it differs from LiteLLM URL (#549).
	if embedURL != *litellmURL {
		if err := netutil.ValidateUpstreamURL(embedURL); err != nil {
			return fmt.Errorf("invalid ENGRAM_EMBED_URL %q: %w (#549)", embedURL, err)
		}
	}

	// E6: startup probe is non-fatal. Server starts in degraded mode when
	// LiteLLM is unavailable; /health reports embed:degraded with HTTP 200.
	// BM25+recency fallback keeps recall functional until LiteLLM recovers.
	embedDegraded := false
	embedClient := embed.Client(embed.NewLiteLLMClientNoProbe(embedURL, *embedModel, litellmAPIKey, *embedDims))
	probeCtx, probeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	probeVec, probeErr := embedClient.Embed(probeCtx, "startup probe")
	probeCancel()
	if probeErr != nil {
		slog.Warn("LiteLLM unavailable at startup — embedding degraded; server will start anyway", "error", probeErr)
		embedDegraded = true
	} else if len(probeVec) == 0 {
		slog.Warn("LiteLLM startup probe returned empty vector — embedding degraded")
		embedDegraded = true
	} else {
		slog.Info("LiteLLM embedding verified at startup", "dims", len(probeVec))
	}

	dsn := *databaseURL
	litellmURLVal := *litellmURL
	sumModel := *summarizeModel
	sumEnabled := *summarizeEnabled

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

	// Create a single shared *pgxpool.Pool for the entire server process. All
	// project backends, entity workers, audit/weight workers, and the retention
	// worker share this pool rather than each owning a private 25-connection pool.
	// This prevents connection exhaustion when many projects are active (#363).
	pgxPool, err := db.NewSharedPool(ctx, dsn)
	if err != nil {
		return fmt.Errorf("shared pool: %w", err)
	}
	defer pgxPool.Close()

	retentionBackend, err := db.NewPostgresBackendWithPool(ctx, "default", pgxPool)
	if err != nil {
		return fmt.Errorf("retention worker backend: %w", err)
	}
	// retentionBackend does not own the pool — do not call Close() on it.
	go retentionBackend.StartRetentionWorker(ctx)

	// GlobalReembedder processes unembedded chunks across all projects from a
	// single goroutine, lifecycle-independent of any EnginePool entry (#359).
	// It uses FOR UPDATE SKIP LOCKED so multiple server replicas are safe.
	reembedBatchSize := envInt("ENGRAM_REEMBED_BATCH_SIZE", 100)
	reembedInterval := envDuration("ENGRAM_REEMBED_INTERVAL", 10*time.Second)
	// ENGRAM_REEMBED_BATCH_SIZE=0 disables the GlobalReembedder in this process.
	// Use this when a dedicated engram-reembed container owns all embedding work.
	globalReembedder := reembed.NewGlobalReembedder(pgxPool, embedClient, reembedBatchSize, reembedInterval)
	if reembedBatchSize > 0 {
		globalReembedder.Start(ctx)
		slog.Info("global reembedder started", "batch_size", reembedBatchSize, "interval", reembedInterval)
	} else {
		slog.Info("global reembedder disabled (ENGRAM_REEMBED_BATCH_SIZE=0) — reembed container owns embedding")
	}
	defer globalReembedder.Wait()

	// Audit and weight tuner workers use the shared pool directly.
	sharedPool := pgxPool

	// serverCtx is the outer lifecycle context; captured here so engine background
	// workers (decay, summarize) use a long-lived context, not the 10s-bounded
	// initCtx that Pool.Get passes to the factory.
	serverCtx := ctx
	factory := func(initCtx context.Context, project string) (*internalmcp.EngineHandle, error) {
		backend, err := db.NewPostgresBackendWithPool(initCtx, project, pgxPool)
		if err != nil {
			return nil, fmt.Errorf("postgres backend for project %q: %w", project, err)
		}
		engine := search.New(serverCtx, backend, embedClient, project, litellmURLVal, sumModel, sumEnabled, claudeCompleter, *decayInterval, *embedDims)
		engine.SetGlobalReembedder(globalReembedder)
		return &internalmcp.EngineHandle{Engine: engine}, nil
	}

	pool := internalmcp.NewEnginePool(factory)
	defer pool.Close()

	ingestQ := ingestqueue.New(serverCtx, ingestqueue.Config{Depth: 256, Workers: 4})
	defer ingestQ.Wait()

	// Update IngestQueueDepth gauge every 10 seconds so Prometheus has a
	// meaningful reading. Without this the gauge always reads 0.
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-serverCtx.Done():
				return
			case <-ticker.C:
				metrics.IngestQueueDepth.Set(float64(ingestQ.Depth()))
			}
		}
	}()

	cfg := internalmcp.Config{
		LiteLLMURL:               *litellmURL,
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
		RateLimit:                *rateLimit,
		AllowRFC1918SetupToken:   envBool("ENGRAM_SETUP_TOKEN_ALLOW_RFC1918", false),
		EmbedDimensions:          *embedDims,
		PgPool:                   sharedPool,
		EmbedDegraded:            embedDegraded,
		SessionDB:                retentionBackend, // retentionBackend satisfies db.SessionRegistry
		IngestQueue:              ingestQ,
		RateLimitRPS:             *rateLimitRPS,
		RateLimitBurst:           *rateLimitBurst,
		RateLimitDisable:         *rateLimitDisable,
	}
	// Default EpisodeTTL to 24 h; set ENGRAM_EPISODE_TTL=0 to disable the sweeper.
	if cfg.EpisodeTTL == 0 {
		cfg.EpisodeTTL = 24 * time.Hour
	}

	srv := internalmcp.NewServer(pool, cfg)

	// Track all long-running workers with a WaitGroup so we can wait for them
	// to exit gracefully after SIGTERM before closing the DB connection (#559).
	var workersWg sync.WaitGroup

	// Start audit worker — monitors ranking drift by re-running canonical queries.
	auditRecaller := &auditRecallerAdapter{pool: pool}
	auditWorker := audit.NewAuditWorker(sharedPool, auditRecaller, *embedModel, *auditInterval)
	workersWg.Add(1)
	go func() {
		defer workersWg.Done()
		auditWorker.Run(ctx)
		slog.Debug("audit worker shutdown complete")
	}()
	slog.Info("audit worker started", "interval", auditInterval.String())

	// Start weight tuner worker — adjusts per-project weights on dominant failure classes.
	tunerWorker := weight.NewTunerWorker(sharedPool, *weightInterval)
	workersWg.Add(1)
	go func() {
		defer workersWg.Done()
		tunerWorker.Run(ctx)
		slog.Debug("weight tuner worker shutdown complete")
	}()
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

	if cc != nil && len(entityProjects) > 0 {
		for _, proj := range entityProjects {
			proj := proj // capture for goroutine
			entityBackend, err := db.NewPostgresBackendWithPool(ctx, proj, pgxPool)
			if err != nil {
				slog.Warn("entity worker: could not open backend, skipping project",
					"project", proj, "err", err)
				continue
			}
			adapter := &entityDBAdapter{backend: entityBackend}
			extractor := entity.NewClaudeExtractor(cc)
			w := entity.NewWorker(adapter, extractor, entity.WorkerConfig{
				Projects:     []string{proj},
				PollInterval: time.Duration(envInt("ENGRAM_ENTITY_POLL_SECONDS", 5)) * time.Second,
				BatchSize:    envInt("ENGRAM_ENTITY_BATCH_SIZE", 10),
			})
			workersWg.Add(1)
			go func() {
				defer workersWg.Done()
				w.Run(ctx)
				slog.Debug("entity extraction worker shutdown complete", "project", proj)
			}()
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

	// Rehydrate sessions from DB so clients with pre-restart session IDs can
	// continue making tool calls without reconnecting (#362).
	if err := srv.RehydrateSessions(ctx, apiKey); err != nil {
		slog.Warn("session rehydration failed — clients will need to reconnect", "err", err)
	}

	logRecommendedClientPermissions(srv)

	// Warn if binding to 0.0.0.0 — all network interfaces (#550).
	// This is a valid deployment in a trusted LAN (e.g. behind a reverse proxy),
	// but operators should understand the threat model.
	if *host == "0.0.0.0" {
		slog.Warn("SECURITY: engram is binding to 0.0.0.0 (all network interfaces). " +
			"Ensure it is behind a reverse proxy with authentication, not directly exposed. " +
			"For local development, use --host=127.0.0.1 instead. (#550)")
	}

	slog.Info("engram ready", "host", *host, "port", *port,
		"embed_model", *embedModel, "summarize_model", sumModel)

	err = srv.Start(ctx, *host, *port, apiKey, *baseURL)

	// Wait for all background workers to exit (they listen to ctx.Done(), which
	// is cancelled when SIGTERM is received). Use a timeout to avoid hanging forever (#559).
	slog.Info("waiting for background workers to shut down...")
	workersDone := make(chan struct{})
	go func() {
		workersWg.Wait()
		close(workersDone)
	}()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	select {
	case <-workersDone:
		slog.Info("all background workers shut down cleanly")
	case <-shutdownCtx.Done():
		slog.Warn("background worker shutdown timeout — some workers did not exit gracefully")
	}

	return err
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

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		var n float64
		if _, err := fmt.Sscanf(v, "%f", &n); err == nil {
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

// validateEmbedConfig checks that the embedding dimensions match the vector
// column contract. Returns a non-empty warning string when misconfigured. (#380)
//
// The deployment contract is 1024-dim embeddings, regardless of the specific
// model name. Model upgrades are operational changes as long as they preserve
// that dimension.
func validateEmbedConfig(model string, dims int) string {
	if dims == 1024 {
		return ""
	}
	return "the embedding index expects ENGRAM_EMBED_DIMENSIONS=1024; current value (" +
		fmt.Sprintf("%d", dims) + ") will cause dimension mismatch errors"
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
