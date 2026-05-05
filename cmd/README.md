# Command-Line Tools

Engram ships six binaries. This page documents what each does, who typically runs it, and when.

---

## starter

**Location:** `cmd/starter`

**Who runs it:** Docker (automatically, as the container ENTRYPOINT). Users never invoke this directly.

**Purpose:** Authenticate to Infisical (optional secret management), inject credentials into the process environment, then exec-replace itself with the `engram` server.

**Usage:**

```bash
starter server          # Start the MCP server (default ENTRYPOINT)
starter migrate         # Run schema migrations only (one-shot database setup)
starter health          # Health check probe (used by Docker healthcheck)
```

**When to run manually:**
- Rarely. Docker handles it automatically.
- `starter migrate` is useful for debugging schema issues in a fresh database.

**Key behavior:**
- If `.env.machine-identity` contains Infisical credentials, `starter` fetches secrets from Infisical before starting the server.
- If `.env.machine-identity` is empty, all secrets must be in `.env` or shell environment.
- Secrets are unset from the process environment after being read, so subprocesses cannot inherit them (security).

---

## engram

**Location:** `cmd/engram`

**Who runs it:** Docker container, or developers running `go run ./cmd/engram`.

**Purpose:** The MCP server itself. Exposes 43 tools over HTTP/SSE, manages the PostgreSQL backend, handles schema migrations, and runs background workers (reembedding, consolidation, decay audit, etc.).

**Usage:**

```bash
go run ./cmd/engram
# OR
./engram (after `make go-build`)
```

**Configuration:** Reads environment variables and `.env` file. See `.env.example` for the full list.

**Key responsibilities:**
- MCP tool handlers (store, recall, consolidate, etc.)
- HTTP endpoints (`/sse`, `/health`, `/setup-token`, `/metrics`)
- Background workers (reembedding, consolidation, decay, weight tuning)
- Authentication (bearer token verification)
- Rate limiting
- Session management

**When to run:**
- Always. This is the core server.

---

## engram-setup

**Location:** `cmd/engram-setup`

**Who runs it:** Users, via `make setup` in the project root.

**Purpose:** Configure Claude Code (and other IDE integrations) to connect to the running engram server. Fetches the current bearer token via the unauthenticated `/setup-token` endpoint, then writes the MCP server config to:

- `~/.claude/mcp_servers.json` (primary, live config)
- `~/.claude.json` (secondary, settings fallback)

Both files are updated so the token stays fresh regardless of which file your Claude Code version reads.

**Usage:**

```bash
go run ./cmd/engram-setup              # Configure with defaults (localhost:8788)
go run ./cmd/engram-setup --dry-run    # Preview changes without writing
go run ./cmd/engram-setup --port 9000  # Non-default port
```

**When to run:**
- Once after first `make up`
- After rotating `ENGRAM_API_KEY` (if the token changes)
- After moving the server to a different port or host

**See also:** `make setup` and `make setup-dry-run` in the project Makefile.

---

## engram-eval

**Location:** `cmd/eval`

**Who runs it:** Developers tuning embedding models or evaluating retrieval quality. Also used in CI for golden-set regression testing.

**Purpose:** Run a retrieval evaluation harness against a live engram server. Reads a golden set (list of queries with known relevant memory IDs), then measures precision, recall, NDCG, and other ranking metrics to evaluate retrieval quality.

**Usage:**

```bash
go run ./cmd/eval/main.go \
  -golden docs/benchmarks/golden-set.json \
  -k 5 \
  -project default
```

**Input:** `golden-set.json` — array of `{query, relevant_ids}`

**Output:** JSON with metrics for each query and aggregate statistics (precision@K, recall@K, MRR, NDCG).

**When to run:**
- Before and after changing the embedding model to verify quality trade-offs
- When debugging poor retrieval results
- In CI as a regression gate for golden-set quality

**Example golden set:**

```json
[
  {
    "query": "deployment procedures",
    "relevant_ids": ["mem-001", "mem-042", "mem-108"]
  },
  {
    "query": "authentication flow",
    "relevant_ids": ["mem-215", "mem-216"]
  }
]
```

See [Deployment Notes → Embedding Model Selection](../docs/deployment-notes.md#embedding-model-selection) for tuning guidance.

---

## instinct

**Location:** `cmd/instinct`

**Who runs it:** Claude Code via hook (PreCompact or PostToolUse hook), or as a standalone daemon for analysis.

**Purpose:** Read tool-use events from a JSONL buffer (written by Claude Code's PostToolUse hook), group them by session, call Claude to identify patterns, and store those patterns in Engram memory.

**Usage:**

```bash
go run ./cmd/instinct/main.go -buffer /tmp/events.jsonl -project myapp
```

**When to run:**
- Automatically, via Claude Code hook configuration (see `~/.claude/hooks/`)
- After a coding session to extract patterns you might reuse
- Standalone for archival / analysis of past sessions

**Key behavior:**
- Reads tool-use events (which tools were called, in what order, with what context)
- Identifies patterns (e.g., "User always checks code coverage after implementing a feature")
- Stores each pattern as a memory with context and timestamps
- Uses Claude Haiku to identify patterns (cheap, fast)

**See also:** [CLAUDE.md](../CLAUDE.md) for hook configuration and pattern detection rationale.

---

## benchmark

**Location:** `cmd/benchmark`

**Who runs it:** Performance engineers, or CI for throughput regression testing.

**Purpose:** Synthetic throughput benchmark for Ollama embedding models. Measures tokens/sec, latency percentiles, and VRAM utilization across different batch sizes and concurrency levels.

**Usage:**

```bash
go run ./cmd/benchmark/main.go \
  -model mxbai-embed-large \
  -batch-size 32 \
  -concurrency 8 \
  -iterations 100
```

**Output:** JSON with throughput stats, latency percentiles (p50, p95, p99), and VRAM telemetry.

**When to run:**
- When evaluating a new embedding model for inclusion in Ollama
- Comparing performance across different batch sizes / concurrency levels
- Benchmarking before/after infra changes (GPU, network, etc.)

**See also:** `make test-explore-soak` in the Makefile for soak tests (high-volume, long-duration stress tests).

---

## Summary Table

| Binary | Purpose | Who Runs It | Frequency |
|--------|---------|------------|-----------|
| `starter` | Bootstrap server, inject secrets from Infisical | Docker ENTRYPOINT | Every container start |
| `engram` | MCP server + background workers | Docker container or developer | Always running |
| `engram-setup` | Configure IDE MCP client | User via `make setup` | Once per install, after key rotation |
| `engram-eval` | Evaluate retrieval quality | Developer, CI | Before/after model changes, CI gate |
| `instinct` | Extract and store tool-use patterns | Claude Code hook or developer | After coding sessions, optional |
| `benchmark` | Synthetic embedding throughput test | Performance engineer, CI | Model evaluation, infra tuning |

---

## Building from Source

All binaries are built with `make go-build`:

```bash
make go-build
```

This produces:
- `./engram` — MCP server
- `./engram-setup` — Setup tool
- `./engram-eval` — Eval harness
- `./instinct-benchmark` — Benchmark binary (note: different name from `cmd/benchmark`)

Docker images are built with `make build` and `make build-postgres`.

See [CONTRIBUTING.md](../CONTRIBUTING.md) for development setup and test policies.
