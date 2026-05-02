# engram-go Phases 3–7 Design Spec

**Date:** 2026-04-10  
**Status:** Approved  
**Scope:** Embedding client, search engine, background workers, MCP server, CLI — complete Go port of the Python engram service

---

## Context

The Python engram service (`petersimmons1972/engram`) is a PostgreSQL-backed MCP memory server.
Phases 1–2 of the Go rewrite delivered core types, semantic chunker, vector math, and the full
PostgreSQL backend interface + implementation. This spec covers the remaining phases to produce a
deployable, wire-compatible MCP server in Go.

**Non-goals:**
- OpenAI or any remote/cloud embedding provider — out of scope permanently
- Null/BM25-only mode — all deployments have Ollama
- pgvector extension — exact cosine in Go is sufficient at Engram scale
- Python schema migration (`migrate-from-v1`) — deferred to a future phase
- Multi-binary architecture — single binary, single container

---

## Package Structure

```
cmd/engram/
  main.go               CLI entry point; parses flags/env; wires all packages; starts MCP server

internal/embed/
  ollama.go             HTTP client for Ollama /api/embed
  ollama_test.go

internal/search/
  engine.go             SearchEngine: store, recall, consolidate, graph ops
  score.go              Composite scoring formulas
  engine_test.go

internal/summarize/
  worker.go             Background goroutine: fills summary IS NULL via Ollama generate
  worker_test.go

internal/reembed/
  worker.go             Background goroutine: fills embedding IS NULL chunks
  worker_test.go

internal/markdown/
  io.go                 Export memories→markdown files; import markdown→memories
  io_test.go

internal/mcp/
  server.go             Registers MCP tools; owns SSE server lifecycle
  tools.go              One func per MCP tool; thin dispatch over SearchEngine
  tools_test.go
```

---

## Phase 3 — Embedding Client (`internal/embed/ollama.go`)

### Interface

```go
type Client interface {
    Embed(ctx context.Context, text string) ([]float32, error)
    Name() string
    Dimensions() int
}
```

### OllamaClient

- `NewOllamaClient(ctx, ollamaURL, model string) (*OllamaClient, error)`
  - On construction: `GET /api/tags` to confirm Ollama is reachable
  - If model absent: `POST /api/pull` with streaming response, wait for completion (5 min timeout)
  - First successful embed stores `embedder_name` and `embedder_dimensions` in `project_meta`
- Calls `POST /api/embed` (current endpoint; `/api/embeddings` is deprecated)
- Request body: `{"model": "<model>", "input": "<text>"}`
- Response: `{"embeddings": [[float32...]]}`  — take `[0]`
- HTTP transport: `IdleConnTimeout: 30s`, `MaxIdleConnsPerHost: 2`
  - Short idle timeout ensures DNS changes (host DNS server changes) propagate within 30s
  - Discrete embed calls don't benefit from persistent keepalives

### Configuration

| Env var | Default | Description |
|---|---|---|
| `OLLAMA_URL` | `http://ollama:11434` | Ollama base URL |
| `ENGRAM_OLLAMA_MODEL` | *(required)* | Embedding model name; must produce 1024-dim vectors |

---

## Phase 4 — Search Engine (`internal/search/`)

### SearchEngine

```go
type SearchEngine struct {
    db         db.Backend
    embedder   embed.Client
    project    string
    summarizer *summarize.Worker
    reembedder *reembed.Worker
}

func New(ctx context.Context, backend db.Backend, embedder embed.Client, project string) *SearchEngine
func (e *SearchEngine) Close()
```

`New` starts the background workers. `Close` stops them and releases resources.

### Store

1. Validate immutability if updating
2. Check embedder metadata consistency via `project_meta` (embedder name + dimensions)
3. Chunk content via `chunk.ChunkDocument()`
4. For each chunk: check `db.ChunkHashExists()` — skip duplicates
5. Embed non-duplicate chunks via `embedder.Embed()`
6. `db.Begin()` → `StoreMemoryTx` + `StoreChunksTx` → `Commit()`
7. Return stored Memory

### Recall

1. Embed query text
2. In parallel:
   - **Vector path:** `db.GetAllChunksWithEmbeddings()` → cosine similarity → top candidates
   - **FTS path:** `db.FTSSearch()` → BM25 scores
3. Merge by memory ID, compute composite score:
   ```
   score = 0.50 × cosine + 0.35 × bm25_norm + 0.15 × recency_decay
   recency_decay = exp(-DECAY_RATE × hours_since_last_access)   // DECAY_RATE = 0.01
   ```
4. Apply importance boost: multiply by `(importance / 3.0)`
5. Sort descending, return top-K with matched chunk text
6. `db.TouchMemory()` on each returned memory
7. `db.UpdateChunkLastMatched()` on matched chunks

### Other SearchEngine methods

All 17 MCP tools dispatch through SearchEngine methods. Key ones:

- `Connect(src, dst, relType, strength)` → `db.StoreRelationship()`
- `Consolidate(project)` → prune stale memories, decay edges, merge near-duplicates (Jaccard > 0.85)
- `Verify(project)` → integrity check via `db.GetIntegrityStats()`
- `MigrateEmbedder(newModel)` → sets migration flag, nulls all embeddings, updates project_meta, reembedder picks up

---

## Phase 5 — MCP Server (`internal/mcp/`)

### Library

`github.com/mark3labs/mcp-go` v0.45.0

### Transport

SSE on `0.0.0.0:8788` — identical to Python; existing Claude Code MCP config works unchanged.

Optional bearer token gate via `ENGRAM_API_KEY` env var (same as Python).

### Tool registration

All 17 tools registered at startup. Each tool function:
1. Extracts typed parameters from `mcp.CallToolRequest`
2. Resolves `project` (default: `"default"`)
3. Calls the corresponding `SearchEngine` method
4. Returns `mcp.NewToolResultText(json.Marshal(result))`

### Tool surface (17 tools)

| Tool | Description |
|---|---|
| `memory_store` | Store a focused memory (≤10k chars) |
| `memory_store_document` | Store a large document (≤500k chars, auto-chunked) |
| `memory_store_batch` | Store multiple memories in one call |
| `memory_recall` | Recall memories by semantic + FTS query |
| `memory_list` | List memories with filters (type, tags, importance) |
| `memory_connect` | Create a directed relationship between two memories |
| `memory_correct` | Update content, tags, or importance on an existing memory |
| `memory_forget` | Delete a memory (respects immutability) |
| `memory_summarize` | Trigger immediate summarization of a memory |
| `memory_status` | Return project statistics |
| `memory_feedback` | Record access feedback (boost edge strength) |
| `memory_consolidate` | Prune stale memories, decay edges, merge near-duplicates |
| `memory_verify` | Integrity check (hash coverage, corrupt count) |
| `memory_migrate_embedder` | Switch embedding model; triggers background re-embedding |
| `memory_export_all` | Export all memories to JSON or markdown |
| `memory_import_claudemd` | Import a CLAUDE.md file as structured memories |
| `memory_dump` | Dump raw memory JSON to a directory |
| `memory_ingest` | Ingest a file or directory as document memories |

---

## Phase 6 — Background Workers

### summarize.Worker (`internal/summarize/worker.go`)

- Ticker: every 60s
- Fetches up to 10 `summary IS NULL` rows via `db.GetMemoriesPendingSummary()`
- Calls Ollama generate endpoint (`POST /api/generate`) with a fixed summarization prompt
- Stores result via `db.StoreSummary()`
- Stops cleanly on context cancellation

### reembed.Worker (`internal/reembed/worker.go`)

- Ticker: every 30s
- Fetches up to 20 `embedding IS NULL` chunks via `db.GetChunksPendingEmbedding()`
- Embeds each via `embed.Client.Embed()`
- Stores via `db.UpdateChunkEmbedding()`
- Stops cleanly on context cancellation

Both workers receive a `context.Context` from `SearchEngine.New()`. Cancellation (via `SearchEngine.Close()`) is the only shutdown path.

---

## Phase 7 — CLI Entry Point (`cmd/engram/main.go`)

```
engram server   — start the MCP SSE server (default subcommand)
```

**Flag/env precedence:** flags override env vars.

| Flag | Env | Default |
|---|---|---|
| `--database-url` | `DATABASE_URL` | (required) |
| `--ollama-url` | `OLLAMA_URL` | `http://ollama:11434` |
| `--model` | `ENGRAM_OLLAMA_MODEL` | *(required)* |
| `--port` | `ENGRAM_PORT` | `8788` |
| `--host` | `ENGRAM_HOST` | `0.0.0.0` |
| `--api-key` | `ENGRAM_API_KEY` | `""` (no auth) |
| `--project` | `ENGRAM_PROJECT` | `default` |

Startup sequence:
1. Parse config
2. Connect PostgreSQL (`db.NewPostgresBackend`) — exit on failure
3. Connect Ollama (`embed.NewOllamaClient`) — exit on failure (no fallback)
4. Construct `SearchEngine` (starts background workers)
5. Register MCP tools, start SSE server
6. Block on signal (`SIGTERM`, `SIGINT`) → `SearchEngine.Close()` → exit 0

---

## Docker / Container

### Dockerfile (multi-stage, Chainguard)

```dockerfile
FROM cgr.dev/chainguard/go:latest AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /engram ./cmd/engram

FROM cgr.dev/chainguard/static:latest
COPY --from=build /engram /engram
ENTRYPOINT ["/engram", "server"]
```

### docker-compose additions

Ollama bundled as a sibling service (already present from Phase 2 re-enable). No changes needed for the engram service beyond swapping the image from the Python build to this Go build.

---

## Testing Strategy

- **Unit tests** alongside each package (table-driven, `-race`)
- **Integration tests** in `internal/db/` against a real PostgreSQL (test profile already in docker-compose)
- **MCP tool tests** in `internal/mcp/tools_test.go` — use an in-process SearchEngine against test-postgres
- Python hash/score compatibility verified via constants (already established in Phase 1)

---

## Explicitly Out of Scope

- OpenAI, Cohere, or any remote embedding provider
- BM25-only / no-vector mode
- pgvector extension
- Python v9 schema migration
- Multi-project server pooling (one project per server instance)
- HTTP/REST API (MCP SSE only)
