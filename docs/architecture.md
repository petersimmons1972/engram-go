# Engram Architecture Overview

This document describes the high-level structure, responsibilities, and data flows of the Engram memory engine.

## Package Responsibility Matrix

| Package | Responsibility |
|---------|-----------------|
| `internal/mcp` | MCP protocol implementation, SSE server, tool handlers, HTTP middleware, rate limiting, session management |
| `internal/db` | PostgreSQL backend interface, connection pooling, transactional storage, queries for memories, episodes, sessions |
| `internal/search` | Semantic and BM25 hybrid search, ranking, importance decay, episode recall, memory retrieval pipeline |
| `internal/embed` | Embedding generation via LiteLLM, model management, health probes, dimension truncation (MRL) |
| `internal/types` | Core data types: Memory, SearchResult, Memory ID/Type, constants (MaxContentLength=500KB) |
| `internal/claude` | HTTP client for Anthropic Messages API, advisor tool declarations, request/response marshaling |
| `internal/chunk` | Content splitting into chunks for embedding and storage, chunk table writes |
| `internal/summarize` | Background memory summarization (Ollama or Claude-powered), synopsis generation |
| `internal/consolidate` | Near-duplicate detection and merge, importance decay, entity extraction, background workers |
| `internal/audit` | Retrieval quality auditing, ranking drift detection, canonical query snapshots, weight tuning feedback |
| `internal/weight` | Adaptive retrieval weight tuning via failure feedback and user signals |
| `internal/metrics` | Prometheus instrumentation, tool request counters, duration histograms |
| `internal/entity` | Entity extraction jobs, background entity enrichment via async queue |
| `internal/ingestqueue` | Async job queue for bulk ingestion, status tracking, expiration |
| `internal/reembed` | Background re-embedding worker, embedder migration support |
| `internal/rag` | Retrieval-augmented generation context assembly, token budgeting |
| `internal/llm` | Generic LLM client abstractions (shared by Ollama and LiteLLM paths) |
| `internal/netutil` | Network validation (SSRF protection for upstream URLs) |
| `internal/reporter` | Progress/status reporting for long-running operations |
| `internal/reviewer` | Content review and filtering (used during ingestion) |
| `internal/scorer` | Relevance scoring and ranking |
| `internal/rag` | Token-budgeted context assembly for RAG operations |
| `internal/vram` | VRAM usage estimation and resource budgeting |
| `internal/cache` | In-memory caching for frequently accessed data (e.g., project metadata) |
| `internal/manifest` | Document manifest tracking and versioning |
| `internal/minhash` | MinHash sketches for deduplication and near-duplicate detection |

## Core Data Flow: Memory Store

```
POST /quick-store (or MCP tool memory_store)
  ↓
handleQuickStore (server.go) — validate input, extract fields
  ↓
handleMemoryStore (tools_store.go) — apply defaults, build Memory struct
  ↓
engine.Store (search.go) — coordinate storage pipeline
  ↓
embed.NewClient.Embed — LiteLLM embedding generation
  ↓
chunk.Chunker.Split — tokenize content for chunks table
  ↓
db.Begin().Tx.StoreMemory — PostgreSQL insert
  ↓
async: enqueueExtractionAsync — submit for entity extraction
```

## Core Data Flow: Memory Recall

```
POST /quick-recall (or MCP tool memory_recall)
  ↓
handleQuickRecall (server.go) — validate input, extract fields
  ↓
handleMemoryRecall (tools_recall.go) — parse project/tags/limit
  ↓
engine.Recall (search.go) — semantic + BM25 hybrid ranking
  ↓
embed.NewClient.Embed(query) — embed the query string
  ↓
db.Tx.SearchSemanticBM25 — PostgreSQL vector + full-text search
  ↓
sort by hybrid score, apply importance decay
  ↓
return SearchResult array
```

## Key Entry Points

| Endpoint | Handler | Tool | Purpose |
|----------|---------|------|---------|
| `POST /sse` | buildSSEServer | (all MCP tools) | SSE session management, tool dispatch |
| `POST /message` | MCPServer.MessageHandler | (called via MCP) | MCP message routing with session fingerprint verification |
| `POST /quick-store` | handleQuickStore | N/A | Sessionless memory storage (for hook scripts) |
| `POST /quick-recall` | handleQuickRecall | N/A | Sessionless memory recall (for Python/CLI callers) |
| `GET /health` | handleHealth | N/A | Dependency health probes (PostgreSQL, LiteLLM) |
| `GET /metrics` | promhttp.Handler | N/A | Prometheus metrics (requires Bearer auth) |
| `GET /setup-token` | handleSetupToken | N/A | Return current bearer token + SSE endpoint |

## Concurrency Model

- **Rate Limiter:** Per-IP token-bucket state maintained in `rateLimiter` (evicted every 5 minutes)
- **Session Fingerprints:** `sessionFingerprints` sync.Map (HMAC-SHA256 verification on POST /message)
- **Entity Extraction:** Semaphore-bounded (`extractionSem` max 20 goroutines); non-blocking drop when full
- **Background Workers:**
  - Episode sweeper (hourly): closes crash-orphaned episodes
  - Upload eviction (5-minute interval): cleans expired file upload sessions
  - Decay worker (configurable): importance decay and metrics
  - Audit worker (6-hour interval): retrieval quality snapshots
  - Weight tuner (24-hour interval): adaptive weight tuning via feedback

## Configuration & Environment Variables

Key variables read at startup (see `cmd/engram/main.go`):

| Variable | Default | Purpose |
|----------|---------|---------|
| `DATABASE_URL` | (required) | PostgreSQL DSN |
| `ENGRAM_API_KEY` | (required) | Bearer token for authentication |
| `ENGRAM_EMBED_MODEL` | (required) | Embedding model name (e.g., `snowflake-arctic-embed2`) |
| `ENGRAM_CLAUDE_TOOL_TYPE` | `advisor_20260301` | Claude advisor tool type (see issue #485) |
| `LITELLM_URL` | `http://litellm:4000` | LiteLLM generation endpoint |
| `ENGRAM_EMBED_URL` | (from LITELLM_URL) | LiteLLM embedding endpoint override |
| `ENGRAM_LOG_FORMAT` | (auto-detect) | `json` or text |
| `ENGRAM_LOG_LEVEL` | `info` | Structured logging level |
| `ENGRAM_PORT` | `8788` | SSE bind port |
| `ENGRAM_HOST` | `0.0.0.0` | Bind address |
| `ENGRAM_RATE_LIMIT_RPS` | `50` | Sustained req/s per IP |
| `ENGRAM_RATE_LIMIT_BURST` | `200` | Token-bucket burst size per IP |
| `ENGRAM_RATE_LIMIT_DISABLE` | `false` | Disable rate limiting entirely (local use) |
| `ENGRAM_CLAUDE_SUMMARIZE` | `false` | Use Claude for background summarization |
| `ENGRAM_CLAUDE_CONSOLIDATE` | `false` | Use Claude for consolidation merges |
| `ENGRAM_DECAY_INTERVAL` | `0` (disabled) | How often importance decay runs |

## Input Validation Rules (Issues #515, #573)

The `/quick-store` and `/quick-recall` REST endpoints enforce per-field limits:

- **Content Size:** max 1 MiB (quick-store only)
- **Project Name:** must match `^[a-z0-9_-]{1,64}$` (alphanumeric, underscore, hyphen; no spaces, uppercase, or path traversal)
- **Tags:** max 64 total; each max 256 characters
- **Importance:** 0–100 (not 0–4 like memory_type)
- **Query:** must be non-empty

Validation failures return HTTP 400 with structured error message.

## Error Handling

- **MCP Tool Errors:** Returned via `CallToolResult.IsError=true` with text content
- **HTTP Errors:** JSON body with `{"error":"...", "hint":"..."}`
- **Permanent Embedder Mismatch:** Fast-fail as MCP error (not Go error) via `embed.PermanentError`
- **Timeout:** 15-second per-tool deadline; logged as warning
- **Rate Limit:** HTTP 429 with `Retry-After` header

## Testing Strategy

- **Unit Tests:** `*_test.go` files, use `noopBackend` for isolation
- **Integration Tests:** Can use real PostgreSQL (via test containers) when needed
- **Mock Implementations:** `noopBackend`, `noopEmbedder`, `noopTx` in test files
- **Coverage Gate:** 60% minimum statement coverage enforced by CI

## References

- **Session Management:** `registerSessionHooks()` (OnRegisterSession, OnUnregisterSession)
- **Tool Registration:** `registerTools()` with timeout and read-only annotation
- **Middleware:** `applyMiddleware()` chains rate limiting + Bearer auth
- **Tool Annotations:** `readOnlyToolNames()` defines MCP ReadOnlyHint set
- **Configuration:** `Config` struct in `internal/mcp/config.go`
