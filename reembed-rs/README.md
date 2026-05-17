# reembed-rs

High-throughput re-embedding worker in Rust. Shares the same PostgreSQL backend as the Go engram-go server but runs in its own container so that re-embedding concurrency cannot saturate the MCP request path.

## What it does

Polls `chunks` rows where `embedding IS NULL`, sends batches to an OpenAI-compatible embedding endpoint (Infinity / olla), and writes the resulting vectors back to Postgres. One container per GPU; the orchestrator decides which GPU handles which range.

The worker is intentionally a thin DB → HTTP → DB pipeline. No state of its own; all coordination is through Postgres advisory locks and the `chunks` table.

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
| `ENGRAM_EMBED_SUB_BATCH` | Inner batch size for the HTTP call (default 100) | No |
| `ENGRAM_REEMBED_BATCH_SIZE` | Chunks claimed per Postgres lease (default 2000) | No |
| `ENGRAM_REEMBED_INTERVAL` | Sleep between poll cycles when queue is empty | No |
| `ENGRAM_REEMBED_LEASE_SECS` | Advisory-lock TTL for a claimed batch | No |
| `ENGRAM_REEMBED_CONCURRENCY_MIN/MAX` | Adaptive concurrency bounds | No |
| `ENGRAM_REEMBED_LATENCY_HIGH_MS/LOW_MS` | Hysteresis for the concurrency controller | No |

## Tests

```bash
cargo test
```

Integration tests require `TEST_DATABASE_URL`; pure-logic unit tests run unconditionally.

## Healthcheck

The compose-deployed container has a `pg_isready` HEALTHCHECK (see `Dockerfile.reembed`). For deadlock detection (the worker holds DB connections but no longer drains the queue), watch the `engram_chunks_pending_reembed` Prometheus gauge — the container-level healthcheck cannot see this.

## Source layout

- `src/main.rs` — entrypoint, env wiring, signal handling
- `src/worker.rs` — claim → embed → write loop
- `src/embed.rs` — HTTP client for the OpenAI-compatible endpoint
- `src/db.rs` — Postgres claim/release helpers
- `src/metrics.rs` — Prometheus exporter

## Operations

See `docs/runbook.md` at the repo root for incident triage. Closes #683.
