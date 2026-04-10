<p align="center"><img src="docs/hero.svg" alt="Engram — Persistent Memory for AI Agents" width="100%"></p>

**Your AI agents forget everything between sessions. Engram fixes that.**

---

## The Problem

You're deep into a project with Claude Code or Cursor. The agent knows your architecture, knows the bug you spent two hours tracking down, knows you always use `RS256` for JWT and hate trailing commas. You close the tab. You open it again. The agent introduces the same bug. It suggests the same pattern you already rejected. It asks you to re-explain the same architecture decision for the third time.

Every session starts from a blank slate. The agent isn't getting worse — it's getting reset.

**Engram is the memory layer.** It runs alongside your AI tools and gives every agent a shared, persistent, searchable store of everything learned about your projects. Store a decision once; every future agent finds it. Fix a bug on Monday; Tuesday's agent already knows about it.

---

## What Makes Engram Different

Most "memory" solutions for AI tools are basic key-value stores — you save a note and it either matches exactly or it doesn't. Engram runs four search signals simultaneously:

**Keyword search (BM25)** — Full-text search with Porter stemming. "authentication token expiry" matches "auth token expiry." Fast, exact, no embedding required.

**Semantic vector search** — Each memory is chunked and embedded. Search for "database lock contention" and it finds the memory you stored about "WAL mode timeout under heavy load" — no shared words, but the vectors are close.

**Recency decay** — Recent memories score higher. Exponential decay at 1% per hour. The decision you made yesterday outweighs the one from six months ago. Nothing is deleted; things step back.

**Knowledge graph enrichment** — When you recall a memory about authentication, Engram traverses its graph connections and pulls in related memories automatically: the bug caused by the auth flow, the pattern you use to test it, the JWT library decision. You get the result plus its neighborhood in one call.

All four signals combine on every recall. You get useful results whether you remember the exact phrase or only roughly what the problem was.

---

## What's New in v2.0

Engram v2.0 is a full rewrite in Go. The Python prototype proved the concept. v2.0 makes it production-grade:

| | v1 (Python) | v2 (Go) |
|---|---|---|
| Container size | ~200 MB | ~10 MB (Chainguard static) |
| Cold start | ~3s | ~200ms |
| Memory footprint | ~120 MB | ~18 MB |
| Concurrency | Sequential tool calls | Goroutine-per-connection |
| Binary | Python runtime required | Single static binary |

**New in v2.0:**
- `memory_store_document` — ingest large documents (up to 500k chars) with automatic chunking
- `memory_reason` — recall memories and synthesize a grounded answer using Claude (requires `ANTHROPIC_API_KEY`)
- Claude Advisor strategy — opt-in AI-powered summarization, consolidation, and re-ranking
- Chainguard minimal base image — zero-CVE container by default

---

## What's Inside

Engram exposes tools that any MCP-compatible client — Cursor, Claude Code, VS Code, Windsurf — can call directly:

### Core Memory Operations

| Tool | What it does |
|---|---|
| `memory_store` | Save a focused memory (up to 10k chars). Auto-chunks, embeds, and indexes it. |
| `memory_store_document` | ⭐ **v2.0** — Ingest a large document (up to 500k chars) with automatic chunking. |
| `memory_store_batch` | Store multiple memories in one efficient call with batched embedding. |
| `memory_recall` | Search across keyword, semantic, and graph layers. Returns summaries by default. |
| `memory_list` | Browse memories with optional type, tag, or importance filters. |
| `memory_correct` | Supersede a wrong or outdated memory with a corrected version. |
| `memory_forget` | Delete a memory and all its graph connections. |
| `memory_connect` | Link two memories with a typed relationship. |
| `memory_feedback` | Tell the system which recall results were useful. |
| `memory_consolidate` | Deduplicate, decay weak connections, prune stale memories. |
| `memory_status` | View stats: memory count, chunks, summarization progress, embedding migration status. |
| `memory_verify` | Sweep all memories for SHA-256 integrity mismatches. `fix=true` backfills missing hashes. |
| `memory_summarize` | Manually trigger summarization for memories without summaries. |
| `memory_migrate_embedder` | Switch embedding providers. Re-embeds in background; BM25 continues during migration. |

### Data Portability

| Tool | What it does |
|---|---|
| `memory_export_all` | Export all project memories as markdown files. |
| `memory_import_claudemd` | Import a CLAUDE.md file as structured memories. |
| `memory_dump` | Dump raw memory files to a directory. |
| `memory_ingest` | Ingest a file or directory as document memories. |

### Claude Advisor

| Tool | What it does |
|---|---|
| `memory_reason` | ⭐ **v2.0** — Recall memories and synthesize a grounded answer using Claude. Requires `ANTHROPIC_API_KEY`. |

---

## How Projects Work

Engram organizes memories by **project namespace**. Every tool accepts a `project` parameter. Memories stored in `project="my-web-app"` are invisible to `project="cli-tool"` — your web app's database schema won't bleed into your CLI agent's context.

One special namespace: `project="global"`. Use it for preferences that apply everywhere — code style, tooling conventions, infrastructure rules. Any agent on any project can recall from `global`.

Each project namespace is a separate notebook. They share the same server, but nothing crosses between them. `global` is the one notebook every agent can read.

---

## Architecture

<p align="center">
  <img src="docs/architecture.svg" alt="Engram Architecture" width="900">
</p>

### The Four Search Signals

<p align="center">
  <img src="docs/scoring.svg" alt="Engram Search Scoring" width="900">
</p>

When you call `memory_recall`, Engram runs four searches simultaneously and combines their scores:

**1. BM25 Keyword Search** — Full-text search using PostgreSQL's `tsvector` with Porter stemming. Store "Authentication uses RS256 JWT" and a search for "RS256" or "JWT auth" scores high here.

**2. Vector Semantic Search** — Each memory is split into chunks and each chunk is embedded into a vector. At recall time, your query is embedded too, and Engram finds chunks close in that space. Store "WAL mode timeout under load," search for "database lock contention" — no shared words, but the vectors match.

**3. Recency Decay** — Exponential decay at 1% per hour. A memory from today scores significantly higher than one from last month. Old memories stay; they just step back.

**4. Knowledge Graph Enrichment** — High-scoring memories pull in their graph neighbors. You get the result plus its context in one call.

The final composite score:

```
composite = (vector × 0.50) + (bm25 × 0.35) + (recency × 0.15)
final     = composite × importance_multiplier
```

### Context-Efficient Recall

<p align="center">
  <img src="docs/context-reduction.svg" alt="Context reduction: summary vs full" width="900">
</p>

Every `memory_recall` returns a `detail` field you control:

| `detail=` | What you get | Typical size | When to use |
|---|---|---|---|
| `"summary"` **(default)** | 1–2 sentence summary | ~150 chars | AI agents — maximum context efficiency |
| `"chunk"` | The matched excerpt | ~200 chars | When you need the exact passage that matched |
| `"full"` | Original content, unmodified | 200–50,000 chars | Export, debugging, or when full fidelity is required |

Summaries are generated asynchronously by a background goroutine using your configured Ollama model (`ENGRAM_SUMMARIZE_MODEL`, default: `llama3.2`). New memories fall back to the matched chunk until summarized — typically within 30 seconds of storage.

At scale, `detail="summary"` reduces context window consumption by ~13× compared to `detail="full"`.

**Importance multipliers:**

| Importance | Label | Multiplier | When to use |
|---|---|---|---|
| 0 | Critical | 2.0× | Core preferences, system-wide decisions |
| 1 | High | 1.5× | Key project decisions, frequently-needed patterns |
| 2 | Medium | 1.0× | Most memories — the default |
| 3 | Low | 0.8× | Minor notes, temporary context |
| 4 | Trivial | 0.6× | Auto-pruned after 30 days if never accessed |

Without vector embeddings, weights redistribute: BM25 gets `0.85` and recency gets `0.15`. Recall still works — you just lose the semantic similarity layer.

### Claude Advisor Strategy

<p align="center">
  <img src="docs/claude-advisor.svg" alt="Claude Advisor Strategy" width="900">
</p>

Engram v2.0 integrates the Anthropic `advisor_20260301` strategy. A fast Sonnet executor handles routine operations and automatically escalates to Opus when a decision is hard. You get Opus quality where it matters at Sonnet cost for everything else.

Four advisor-powered features, all opt-in:

| Feature | Flag | What changes |
|---|---|---|
| Summarization | `ENGRAM_CLAUDE_SUMMARIZE=true` | Background summarizer uses Claude instead of Ollama |
| Consolidation | `ENGRAM_CLAUDE_CONSOLIDATE=true` | `memory_consolidate` uses Claude to review near-duplicate merge candidates |
| Re-ranking | `ENGRAM_CLAUDE_RERANK=true` | `memory_recall` accepts `rerank=true` to re-score results after retrieval |
| Reasoning | *(auto when API key set)* | `memory_reason` tool — recall + synthesize a grounded answer |

All four default to off. Setting `ANTHROPIC_API_KEY` only enables `memory_reason`. The other three require explicit opt-in.

---

## Memory Types

Six types let agents filter by context:

| Type | Use it for | Example |
|---|---|---|
| `decision` | Choices made and the reasoning behind them | "Chose PostgreSQL — needed JSONB and array columns" |
| `pattern` | Recurring code or architecture patterns | "All DB access goes through a Repository, never raw SQL" |
| `error` | Bugs, gotchas, known failures, and their fixes | "Port 3000 is taken on this server — always use 3001" |
| `context` | General project or environment facts | "Running on Ubuntu 22.04, deployed to K8s on 3 nodes" |
| `architecture` | System design, data flow, integrations | "Auth: client → /api/login → JWT (RS256) → httpOnly cookie" |
| `preference` | User style and convention preferences | "Always tabs, 120-char lines, no trailing commas" |

---

## Embedding Options

engram-go uses Ollama for local embedding inference. A running Ollama instance is the only external dependency for full semantic search:

| Mode | Model | Dimensions | Quality | Cost | Privacy |
|---|---|---|---|---|---|
| `ollama` **(default)** | `nomic-embed-text` | 768 | Good | Free | Fully local |
| `none` | — | — | BM25 only | Free | Fully local |

**Switching embedding models:** Once a project stores its first embedding, Engram locks the project to that model. To switch, call `memory_migrate_embedder(project="my-app", new_model="mxbai-embed-large")`. This triggers background re-embedding. Vector search degrades to BM25+recency during migration — typically a few minutes for large projects.

**Running without Ollama:** Start with `OLLAMA_URL` unset or unreachable. Engram falls back to BM25+recency only. All tools work; you lose semantic similarity.

---

## Installation

Engram runs as three Docker services: **PostgreSQL** for storage, **Ollama** for local embedding inference, and **engram-go** for the MCP server.

### Prerequisites

- Docker Engine 20.10+
- Docker Compose 2.0+
- 1 GB RAM minimum (4 GB recommended if running Ollama in the same stack)

### Quick Start

```bash
git clone https://github.com/petersimmons1972/engram-go.git
cd engram-go

# Copy the example config and edit it
cp .env.example .env
```

Open `.env` and set at minimum:

```bash
POSTGRES_PASSWORD=a-strong-password-here
```

Then start:

```bash
docker compose up -d
```

Engram listens on `http://localhost:8788/sse`. Connect your IDE.

**GPU acceleration for Ollama:** See the comments in `docker-compose.yml` for NVIDIA and AMD ROCm setup. Mac users: run Ollama natively with `brew install ollama && ollama serve`, then set `OLLAMA_URL=http://host.docker.internal:11434`.

---

## Connecting Your IDE

### Claude Code

```bash
claude mcp add engram --transport sse http://localhost:8788/sse
```

If you set `ENGRAM_API_KEY`, add a header:

```bash
claude mcp add engram --transport sse http://localhost:8788/sse \
  --header "Authorization: Bearer your-api-key"
```

### Cursor / VS Code

Add to `~/.cursor/mcp.json` (or `.vscode/mcp.json`):

```json
{
  "mcpServers": {
    "engram": {
      "url": "http://localhost:8788/sse"
    }
  }
}
```

### Windsurf

Add to `~/.codeium/windsurf/mcp_config.json`:

```json
{
  "mcpServers": {
    "engram": {
      "serverUrl": "http://localhost:8788/sse"
    }
  }
}
```

### Claude Desktop

Add to `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS) or `%APPDATA%\Claude\claude_desktop_config.json` (Windows):

```json
{
  "mcpServers": {
    "engram": {
      "command": "docker",
      "args": ["exec", "-i", "engram-go-app", "/engram", "server", "--transport", "stdio"],
      "disabled": false
    }
  }
}
```

---

## Configuration Reference

Create `.env` in the engram-go directory. All variables have working defaults — change only what you need:

```bash
# .env

# === Database (required) ===
POSTGRES_PASSWORD=change-me-in-production
DATABASE_URL=postgresql://engram:${POSTGRES_PASSWORD}@postgres:5432/engram

# === Embeddings ===
OLLAMA_URL=http://ollama:11434          # Docker service (default)
# OLLAMA_URL=http://host.docker.internal:11434  # Mac: native Ollama via brew
ENGRAM_OLLAMA_MODEL=nomic-embed-text   # Any Ollama embedding model

# === Summarization ===
ENGRAM_SUMMARIZE_MODEL=llama3.2        # Ollama model for background summarization

# === Server ===
ENGRAM_PORT=8788
ENGRAM_API_KEY=your-secret-token       # Optional: require Bearer auth

# === Claude Advisor (all opt-in) ===
ANTHROPIC_API_KEY=sk-ant-...           # Enables memory_reason tool automatically
ENGRAM_CLAUDE_SUMMARIZE=false          # Use Claude for background summarization
ENGRAM_CLAUDE_CONSOLIDATE=false        # Use Claude for near-duplicate merge review
ENGRAM_CLAUDE_RERANK=false             # Enable Claude re-ranking in recall
```

Full reference:

| Variable | Default | What it does |
|---|---|---|
| `DATABASE_URL` | *(set by Compose)* | PostgreSQL DSN. Set automatically by docker compose. |
| `POSTGRES_PASSWORD` | `engram` | PostgreSQL password. **Change this in production.** |
| `OLLAMA_URL` | `http://ollama:11434` | Ollama endpoint for embedding inference. |
| `ENGRAM_OLLAMA_MODEL` | `nomic-embed-text` | Embedding model name. |
| `ENGRAM_SUMMARIZE_MODEL` | `llama3.2` | Ollama model for background summarization. |
| `ENGRAM_SUMMARIZE_ENABLED` | `true` | Enable background summarization daemon. |
| `ENGRAM_PORT` | `8788` | HTTP port for SSE server. |
| `ENGRAM_API_KEY` | *(unset)* | Bearer token to secure the SSE endpoint. |
| `ANTHROPIC_API_KEY` | *(unset)* | Anthropic API key. Enables `memory_reason` tool and Claude features. |
| `ENGRAM_CLAUDE_SUMMARIZE` | `false` | Use Claude for background memory summarization instead of Ollama. |
| `ENGRAM_CLAUDE_CONSOLIDATE` | `false` | Use Claude for near-duplicate merge review during consolidation. |
| `ENGRAM_CLAUDE_RERANK` | `false` | Enable Claude re-ranking in `memory_recall`. |

---

## How Agents Use Engram

Engram works best when agents follow a consistent session pattern:

### At the Start of Every Session

Before touching code, the agent recalls where things stand:

```
# Find out where the last agent left off
memory_recall("session handoff", project="my-app", detail="full")

# Get context relevant to today's work
memory_recall("authentication flow", project="my-app")

# Check preferences that apply everywhere
memory_recall("code style preferences", project="global")
```

The session handoff note is a structured summary the previous agent stored before finishing. It tells the current agent what was done, what's next, and what's blocked. You stop re-explaining the current state of the project.

### During Work

When the agent makes a decision, hits a bug, or spots a pattern:

```
# Store an architectural decision
memory_store(
    content="Auth uses RS256 JWT. Tokens issued at /api/login, 24h expiry, stored in httpOnly cookie. Do not use localStorage.",
    memory_type="decision",
    tags="auth,security,jwt",
    importance=1,
    project="my-app"
)

# Store a bug that burned you
memory_store(
    content="The config loader silently ignores unknown keys — typos in config names fail silently. Always validate against the schema.",
    memory_type="error",
    tags="config,gotcha",
    importance=1,
    project="my-app"
)

# Link related memories
memory_connect(
    source_id="decision-id-here",
    target_id="error-id-here",
    rel_type="relates_to",
    project="my-app"
)
```

### At the End of Every Session

Before stopping, store a handoff note:

```
memory_store(
    content="SESSION HANDOFF: Implemented OAuth2 login flow | NEXT: Add logout endpoint and token refresh | BLOCKED: Nothing | FILES CHANGED: internal/auth/login.go, internal/auth/middleware.go",
    memory_type="context",
    tags="session-handoff",
    importance=1,
    project="my-app"
)
```

The next agent — same tool tomorrow or a different IDE entirely — calls `memory_recall("session handoff")` and picks up where work stopped.

### With Claude Advisor (v2.0)

When you need more than recalled facts — a synthesized answer grounded in your memory store:

```
memory_reason(
    question="Why did we switch from Redis to PostgreSQL for session storage?",
    project="my-app"
)
```

Engram recalls the relevant memories, passes them to Claude, and returns a cited answer. The response names which memory IDs it drew from so you can follow up.

### After Recall, Give Feedback

```
memory_feedback(memory_ids="id1,id2", helpful=true, project="my-app")
```

Positive feedback reinforces graph connections. Over many sessions, the graph self-optimizes toward what actually surfaces useful results.

---

## Data Portability

### Exporting Your Data

Export all memories as editable markdown files:

```
memory_dump(project="my-app", output_dir="/tmp/my-memories")
```

Or via the `memory_export_all` tool to export all projects at once. You get a directory of `.md` files, each with YAML frontmatter — greppable, committable, readable without any tools.

### Importing Existing Docs

Ingest a file or directory as document memories:

```
memory_ingest(path="/path/to/docs", project="my-app")
```

Or import a CLAUDE.md file directly:

```
memory_import_claudemd(content="...", project="my-app")
```

---

## Security

- **Network exposure:** By default, engram-go binds to `0.0.0.0` inside Docker, but the host mapping in `docker-compose.yml` is `127.0.0.1:8788:8788` — port is not externally reachable unless you change this.
- **Authentication:** Set `ENGRAM_API_KEY` to require `Authorization: Bearer <key>` on every request.
- **Database:** PostgreSQL data is stored in a named Docker volume (`pgdata`). The volume is never deleted by `docker compose down` — only `docker compose down -v` removes it. **Never use `docker compose down -v` unless you intend to wipe all memory data.**
- **Claude API calls:** The `ANTHROPIC_API_KEY` is used only when Claude features are explicitly enabled. It is never logged and is not stored in the database.

---

## Backup & Recovery

### Creating Backups

PostgreSQL is the single source of truth. Back it up with:

```bash
docker exec engram-postgres pg_dump -U engram engram | gzip > backups/engram-$(date +%Y%m%d).sql.gz
```

Or use the included script:

```bash
bash bin/backup-postgres.sh
```

Backups land in `backups/` with a datestamp. Keep at least the last 7 days.

### Restoring from Backup

```bash
# Decompress and restore
gunzip -c backups/engram-20260101.sql.gz | docker exec -i engram-postgres psql -U engram engram
```

### Volume Backup (full data directory)

For a complete backup including indexes and configuration:

```bash
docker run --rm -v pgdata:/data -v $(pwd)/backups:/backup \
  alpine tar czf /backup/pgdata-$(date +%Y%m%d).tar.gz /data
```

---

## Database Layout

Five tables:

| Table | Purpose |
|---|---|
| `memories` | Core memory records: content, type, tags, importance, summary, SHA-256 hash |
| `memory_fts` | PostgreSQL `tsvector` full-text search index (BM25) |
| `chunks` | Content chunks with vector embeddings (768-dim float32 array) |
| `relationships` | Knowledge graph edges: source, target, type, weight, created_at |
| `project_meta` | Per-project metadata: embedder lock, migration state, stats |

PostgreSQL pgvector extension handles vector similarity search (`<=>` cosine distance operator). Chunking splits content at sentence boundaries to keep semantic coherence within each vector.

---

## Knowledge Graph

Six relationship types for connecting memories:

| Type | Meaning |
|---|---|
| `causes` | Memory A is a cause of Memory B |
| `caused_by` | Memory A was caused by Memory B |
| `relates_to` | General bidirectional relationship |
| `supersedes` | Memory A replaces Memory B (used by `memory_correct`) |
| `supports` | Memory A provides evidence for Memory B |
| `contradicts` | Memory A and Memory B conflict |

Relationships have a `weight` (0–1) that decays over time via `memory_consolidate`. Edges below 0.1 are pruned. Positive feedback from `memory_feedback` strengthens edges. Graph traversal returns results up to two hops from the top-scoring memory.

---

## Compatible Clients

Any MCP-compatible client works with engram-go:

| Client | Transport | Connection |
|---|---|---|
| Claude Code | SSE | `claude mcp add engram --transport sse http://localhost:8788/sse` |
| Cursor | SSE | Add `url` to `mcp.json` |
| VS Code (Copilot) | SSE | Add `url` to `.vscode/mcp.json` |
| Windsurf | SSE | Add `serverUrl` to `mcp_config.json` |
| Claude Desktop | stdio | Docker exec into container |
| Any MCP client | SSE | `http://localhost:8788/sse` |

---

## CLI Reference

The `engram` binary exposes one command: `server`.

```
engram server [flags]

Flags:
  --database-url string   PostgreSQL DSN (required) [$DATABASE_URL]
  --ollama-url string     Ollama endpoint (default "http://ollama:11434") [$OLLAMA_URL]
  --model string          Embedding model (default "nomic-embed-text") [$ENGRAM_OLLAMA_MODEL]
  --summarize-model string  Summarization model (default "llama3.2") [$ENGRAM_SUMMARIZE_MODEL]
  --summarize             Enable background summarization (default true) [$ENGRAM_SUMMARIZE_ENABLED]
  --claude-api-key string Anthropic API key [$ANTHROPIC_API_KEY]
  --claude-summarize      Use Claude for summarization (default false) [$ENGRAM_CLAUDE_SUMMARIZE]
  --claude-consolidate    Use Claude for consolidation (default false) [$ENGRAM_CLAUDE_CONSOLIDATE]
  --claude-rerank         Enable Claude re-ranking (default false) [$ENGRAM_CLAUDE_RERANK]
  --port int              HTTP port (default 8788) [$ENGRAM_PORT]
  --host string           Bind address (default "0.0.0.0") [$ENGRAM_HOST]
  --api-key string        Bearer auth token [$ENGRAM_API_KEY]
```

**Building from source:**

```bash
git clone https://github.com/petersimmons1972/engram-go.git
cd engram-go
go build -o engram ./cmd/engram
./engram server --database-url postgres://engram:password@localhost:5432/engram
```

**Docker:**

```bash
docker pull ghcr.io/petersimmons1972/engram-go:2.0.0
# or build locally:
docker build -t engram-go:2.0.0 .
```

---

## License

[MIT](LICENSE)
