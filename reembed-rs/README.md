# reembed-rs

High-throughput re-embedding worker in Rust. Shares the same PostgreSQL backend as the Go engram-go server but runs in its own container so that re-embedding concurrency cannot saturate the MCP request path.

## What it does

Polls `chunks` rows where `embedding IS NULL`, sends batches to an OpenAI-compatible embedding endpoint (Infinity / olla), and writes the resulting vectors back to Postgres. One container per GPU; the orchestrator decides which GPU handles which range.

The worker is intentionally a thin DB → HTTP → DB pipeline. A fixed worker pool (`ENGRAM_REEMBED_CONCURRENCY_MAX`) claims small slices via `FOR UPDATE SKIP LOCKED`, calls the shared embed endpoint once per slice, persists vectors, and immediately loops. No worker tracks which GPU backend to use; throughput differences come from endpoint scheduling outside this process.

## Prerequisites

- Rust 1.75+ (`rustc --version`)
- Cargo (installed with Rust)
- A running engram-postgres instance reachable via `DATABASE_URL`
- A reachable embedding endpoint reachable via `LITELLM_URL` (the name is historical — it's just an OpenAI-compatible embeddings endpoint)

## Build

From the engram-go repo root:

```bash
cd reembed-rs
cargo build --release
# Binary is at target/release/engram-reembed
```

For a containerized build, use `Dockerfile.reembed` at the repo root (multi-stage; Alpine runtime; pinned non-root UID).

## Run (local development)

```bash
DATABASE_URL=postgresql://engram:$PG_PASSWORD@localhost:5432/engram?sslmode=disable \
LITELLM_URL=http://localhost:8004 \
ENGRAM_EMBED_MODEL=BAAI/bge-m3 \
ENGRAM_EMBED_DIMENSIONS=1024 \
./target/release/engram-reembed
```

## Environment variables

| Var | Purpose | Required |
|---|---|---|
| `DATABASE_URL` | Postgres DSN | Yes |
| `LITELLM_URL` | OpenAI-compatible embed endpoint | Yes |
| `LITELLM_API_KEY` | Bearer for `LITELLM_URL` (if endpoint requires) | No |
| `ENGRAM_EMBED_MODEL` | Embedding model name | Yes |
| `ENGRAM_EMBED_DIMENSIONS` | Output dimension (1024 for bge-m3) | Yes |
| `ENGRAM_EMBED_SUB_BATCH` | Preferred per-claim slice size for the worker-pool DB claim loop (default 8). Also mirrors `ENGRAM_REEMBED_BATCH_SIZE` for compatibility. | No |
| `ENGRAM_REEMBED_BATCH_SIZE` | Legacy alias for claim slice size (default 8, mirrors `ENGRAM_EMBED_SUB_BATCH`) | No |
| `ENGRAM_REEMBED_INTERVAL` | Base poll interval used when no rows are claimed (and seed for empty-queue backoff) | No |
| `ENGRAM_REEMBED_CONCURRENCY_MAX` | Worker count in pool (capped by startup probe warmup success count) | No |
| `ENGRAM_REEMBED_CONCURRENCY_MIN/MAX` | Adaptive concurrency fields kept for compatibility; runtime concurrency is fixed by `ENGRAM_REEMBED_CONCURRENCY_MAX` and startup warmup | No |
| `ENGRAM_REEMBED_LATENCY_HIGH_MS/LOW_MS` | Reserved for compatibility; adaptive controller remains in code for historical reference and can be re-enabled in future releases | No |

## Tests

```bash
cargo test
```

Integration tests require `TEST_DATABASE_URL`; pure-logic unit tests run unconditionally.

## Healthcheck

The compose-deployed container has a `pg_isready` HEALTHCHECK (see `Dockerfile.reembed`). For deadlock detection (the worker holds DB connections but no longer drains the queue), watch the `engram_chunks_pending_reembed` Prometheus gauge — the container-level healthcheck cannot see this.

## Source layout

- `src/main.rs` — entrypoint, env wiring, startup probe, worker orchestration, and structured runtime logs
- `src/claim.rs` — SKIP LOCKED claim + per-slice embed + update unit used by all worker loops

## Operations

See `docs/runbook.md` at the repo root for incident triage. Closes #683.
