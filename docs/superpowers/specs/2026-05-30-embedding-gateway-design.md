# Embedding Gateway — Design Specification

**Status:** APPROVED by founder 2026-05-30
**Date:** 2026-05-30
**Author:** Claude (coordinator)
**Form-factor decision:** In-process module (`internal/embedgateway`) — founder approved; Codex may raise structural blockers to the sidecar fallback described in §7.

---

## 1. Context and Problem

### What exists today

engram-go has three separate sites that either call olla/LiteLLM for embeddings or drive chunk-embedding work:

| Site | Location | Problem |
|------|----------|---------|
| `storeChunksForMemory` (sync path) | `internal/search/engine.go` | Calls `embed.Client.Embed()` inline when `ENGRAM_STORE_EMBED_MODE=sync`. Blocks the MCP call for the duration of the Ollama round-trip. Previously caused a documented 48-minute hang. |
| `GlobalReembedder.runBatch` | `internal/reembed/global_worker.go` | Calls `embed.Client.Embed()` directly. Uses `FOR UPDATE SKIP LOCKED` with `time.After(backoff)` polling — defaults to 10s interval (`ENGRAM_REEMBED_INTERVAL`). Busy-polls even when the queue is empty. No model-identity or dimension validation before writing vectors back. |
| `reembed-rs` sidecar | `reembed-rs/src/main.rs` | Separate Rust process calling LiteLLM independently. 10s default poll interval. No alias-aware model validation. Adds a second independent drain path that can interleave with the Go worker, producing race conditions on the same chunk batch. |

### Failure modes this design eliminates

1. **Sync-embed-on-store latency / hang.** When olla is degraded, every `memory_store` call blocks until the embed call times out (up to 15s in current config; historically hung indefinitely). With the gateway, `memory_store` returns after DB write latency (~1–5ms). The embed happens out-of-band.

2. **Reembed DB contention / pool starvation.** `GlobalReembedder` uses the shared `pgxpool` (currently capped at 50 connections). During a large backfill, the reembedder competes with user-serving `memory_recall` and `memory_store` calls for the same connection budget. Under load this can exhaust the pool and cause user-facing latency spikes.

3. **768-dim / wrong-model drift.** The current `GlobalReembedder` writes whatever vector the embedder returns. If the configured embedder drifts (wrong env var, wrong olla routing, or a temporary fallback to a 768-dim model), corrupted vectors enter the store silently. The only check today is `checkEmbedderMeta` at store/recall time — it catches mismatches in the client config, not in the actual embed response.

4. **Mixed-dimension risk.** The `embedderAliases` table in `engine.go` covers GGUF vs. HuggingFace name variants but does not validate the dimension of the returned vector. A `bge-m3-Q4_K_M.gguf` build producing 768-dim output (e.g., misconfigured olla serving) would pass the name check and write a 768-dim vector into a 1024-dim pgvector column — causing a runtime INSERT panic or silent truncation depending on the pgvector version.

---

## 2. Goals / Non-Goals

### Goals

- Centralise all olla/LiteLLM embed calls into one object (`EmbedGateway`).
- Make `memory_store` fully decoupled from Ollama: returns in ~DB-write latency.
- Validate every embed response (model identity in alias set AND `len(vec)==1024`) before writing. Reject and alarm on anything else.
- Guarantee background embedding never exhausts user-serving connection budget.
- Eliminate busy-polling; use Postgres LISTEN/NOTIFY for wake + exponential backoff idle sleep.
- Absorb `reembed-rs` sidecar logic — one canonical drain implementation.
- Define a planned `docs/EMBEDDING.md` as the single source of truth for model identity, alias table, and dimension contract. The gateway imports its constants from that specification.

### Non-Goals

- Changing the pgvector column schema or running destructive migrations. Backfill of NULL-embedding rows is additive only.
- Supporting multiple concurrent embedding models. bge-m3 is the canonical model; this design does not accommodate multi-model concurrency.
- Changing the MCP tool surface (`memory_store` arguments, return shape).
- Implementing recall-time embedding (that path stays in `SearchEngine.RecallWithOpts` with its existing 500ms timeout and BM25 fallback — it is a read-only, synchronous embed call that does not go through the queue).

---

## 3. Architecture

### 3.1 The `EmbedGateway` object

**Package:** `internal/embedgateway`

```
type EmbedGateway struct {
    // owns
    pool        *pgxpool.Pool     // dedicated, size-bounded (see §6)
    embedder    embed.Client      // bge-m3 client — validated at construction
    notify      chan struct{}      // buffered size 1; woken by Enqueue + pg NOTIFY
    done        chan struct{}      // closed when drain goroutine exits
    startOnce   sync.Once

    // adaptive throttle (see §6)
    throttle    *AdaptiveThrottle

    // metrics / alarming
    metrics     gatewayMetrics
}
```

**Interface exposed to callers:**

```go
// Enqueue records chunk IDs as needing embedding.
// Non-blocking: writes to a buffered channel to wake the drain loop.
// The actual work happens asynchronously.
func (g *EmbedGateway) Enqueue(chunkIDs []string)

// Start launches the drain goroutine. Call once after construction.
func (g *EmbedGateway) Start(ctx context.Context)

// Stop signals the drain goroutine and waits for it to exit (up to 15s).
func (g *EmbedGateway) Stop()

// WaitDrained blocks until the pending-chunk queue reaches zero.
// Used by tests and migration scripts. Not for production callers.
func (g *EmbedGateway) WaitDrained(ctx context.Context) error
```

The gateway does NOT expose `Embed()` directly. All embedding is internal.

### 3.2 Store path

`SearchEngine.StoreWithRawBody` (the single path all store operations flow through) becomes:

```
1. checkEmbedderMeta(ctx)                    // unchanged — model/dim guard
2. storeChunksForMemory(ctx, m, rawBody)     // unchanged — produce Chunk records with nil embedding
3. tx: StoreMemoryTx + StoreChunksTx        // unchanged DB write
4. tx.Commit()                               // return to caller here (~5ms)
5. gateway.Enqueue(chunkIDs)                 // non-blocking; wakes drain loop
   return nil                               // ~DB-write latency only
```

The `storeEmbedSync` flag and its inline embed path are **deprecated** and removed. Migration scripts that previously relied on sync mode must use `gateway.WaitDrained(ctx)` after batch store operations.

`StoreBatch` follows the same pattern.

**`Correct()` with content change:** same pattern — re-chunk without embedding, store, enqueue new chunk IDs.

### 3.3 Drain loop

The drain loop runs inside a single goroutine started by `gateway.Start(ctx)`:

```
func (g *EmbedGateway) drain(ctx context.Context) {
    defer close(g.done)
    listenConn := g.pool.Acquire(ctx)         // dedicated LISTEN connection
    listenConn.Exec(ctx, "LISTEN embed_queue")

    backoff := minIdle
    for {
        n, err := g.drainBatch(ctx)
        if err != nil { /* log, adaptive backoff, pool.Reset() after threshold */ }
        if n == 0 {
            backoff = min(backoff*2, maxIdle)   // exponential backoff up to 5 min
        } else {
            backoff = minIdle                   // reset on work found
            if n == batchSize { continue }      // full batch: drain immediately
        }

        select {
        case <-ctx.Done():       return
        case <-g.notify:         backoff = minIdle   // direct in-process notify
        case <-listenConn.Conn().WaitForNotification(backoffCtx):
            backoff = minIdle                        // pg NOTIFY from another writer
        case <-time.After(backoff):
        }
    }
}
```

`LISTEN embed_queue` + `pg_notify('embed_queue', '')` replaces the per-interval `time.After`. The `g.notify` channel handles the common case where the writer and drain loop are in the same process (direct wake, zero DB round-trip). The `pg_notify` path handles multi-instance deployments and the `reembed-rs` replacement scenario.

### 3.4 `drainBatch`

The batch query is unchanged from `GlobalReembedder.runBatch` — `FOR UPDATE SKIP LOCKED` is the correct claim strategy for multi-instance safety. The additions are:

1. **Validate response before write** — see §4.
2. **Use the gateway's dedicated pool** — not the shared user pool.
3. **Respect adaptive throttle** — see §6.

### 3.5 Centralised olla call

The single olla/LiteLLM call site in the gateway:

```go
func (g *EmbedGateway) embedAndValidate(ctx context.Context, text string) ([]float32, error) {
    vec, modelID, err := g.embedder.EmbedWithModel(ctx, text)
    if err != nil { return nil, err }
    if err := validateEmbedResponse(vec, modelID); err != nil {
        // alarm + return error — do NOT write vector
        metrics.EmbedValidationRejections.Inc()
        slog.Error("embed response rejected", "model_id", modelID, "dims", len(vec), "err", err)
        return nil, err
    }
    return vec, nil
}
```

`validateEmbedResponse` is defined in §4. No other code in the process calls `embedder.Embed()` for background/store purposes. Recall-time embedding in `SearchEngine` is read-only (query vectorisation, not stored), so it is exempt from gateway routing — but it uses the same `embed.Client` and the same model.

### 3.6 Where per-project `reembed.Worker` fits

The per-project `reembed.Worker` (one per `SearchEngine`) is **retired** as an active drain path. It is kept as a dormant struct for the `Notify()` and `Stop()` call sites in `SearchEngine.Close()` and `MigrateEmbedder()` — those are wired to no-ops or to `gateway.Enqueue()`. The `GlobalReembedder` is replaced by `EmbedGateway`. `reembed-rs` is decommissioned.

---

## 4. Model Invariant Enforcement

### 4.1 The reference document

A new file `docs/EMBEDDING.md` (not yet written — this design depends on it) will be the single source of truth for:

- Canonical model identity: `BAAI/bge-m3`
- Accepted alias set: `bge-m3`, `bge-m3-Q8_0.gguf`, `bge-m3-Q4_K_M.gguf`, `BAAI/bge-m3`
- Required output dimension: `1024`
- Rejected models (for alarm/audit): anything else

The gateway imports typed constants from a Go package `internal/embedmodel` that mirrors the EMBEDDING.md table. EMBEDDING.md is the human-readable source; `embedmodel` is the machine-readable import. When the alias table is updated, both are updated together in the same commit.

### 4.2 Validation function

```go
// validateEmbedResponse returns an error if the response does not conform to
// the bge-m3 model contract defined in docs/EMBEDDING.md.
func validateEmbedResponse(vec []float32, reportedModelID string) error {
    if embedmodel.CanonicalName(reportedModelID) != embedmodel.CanonicalBGEM3 {
        return fmt.Errorf("embed response rejected: model %q not in bge-m3 alias set", reportedModelID)
    }
    if len(vec) != embedmodel.RequiredDims {   // 1024
        return fmt.Errorf("embed response rejected: got %d dims, want %d (model: %q)",
            len(vec), embedmodel.RequiredDims, reportedModelID)
    }
    return nil
}
```

### 4.3 Rejection path

If `validateEmbedResponse` returns an error:
1. The chunk is NOT updated in the DB. Its `embedding` column remains NULL.
2. `metrics.EmbedValidationRejections.Inc()` is called with a label for rejection class (`wrong_model` or `wrong_dims`).
3. `slog.Error` is emitted with model ID, dims, and chunk ID.
4. After `consecutiveValidationThreshold` (suggested: 5) consecutive rejections from the same embedder, the gateway enters a **degraded hold**: it stops issuing embed requests for a configurable `holdDuration` (default 2 minutes), logs a watchdog-level error, and increments `metrics.EmbedGatewayDegraded`.
5. On reconnect / hold expiry, the gateway attempts a single probe embed. If the probe passes validation, the drain loop resumes. If not, the hold extends.

**Wrong vectors can never enter the store.** The existing `checkEmbedderMeta` guard at store/recall time is a configuration-level check; `validateEmbedResponse` is a response-level check. Both are required.

### 4.4 `embed.Client` interface extension

The current `embed.Client.Embed(ctx, text) ([]float32, error)` does not return the model ID reported by the server. The interface needs a new method or the existing method must return the model ID alongside the vector. Proposal:

```go
type Client interface {
    Embed(ctx context.Context, text string) ([]float32, error)   // unchanged; recall path
    EmbedWithModel(ctx context.Context, text string) (vec []float32, modelID string, err error)  // new; gateway path
    Name() string
    Dimensions() int
}
```

`LiteLLMClient` already parses the response JSON — adding `model` extraction from the response body is a one-field change. This is an additive, backward-compatible interface extension.

---

## 5. Self-Pacing: LISTEN/NOTIFY

### Problem with current approach

`GlobalReembedder` wakes on `time.After(backoff)`. After exponential backoff reaches max (5 min), an empty queue costs one `SELECT COUNT(*)` and one `FOR UPDATE SKIP LOCKED` query every 5 minutes — acceptable. But the *base* interval before backoff is 10 seconds (`ENGRAM_REEMBED_INTERVAL`), which means a newly queued chunk can wait up to 10 seconds before being noticed — only avoidable by the explicit `Notify()` call from `StoreWithRawBody`. The `reembed-rs` sidecar default is also 10 seconds with no in-process notify path.

The bigger issue: under a sustained empty queue, the current code still wakes every 10s minimum (before backoff kicks in). With the LISTEN/NOTIFY approach, an empty queue costs nothing — the goroutine is parked in `WaitForNotification`.

### Design

**Postgres channel:** `embed_queue` (string constant in `internal/embedmodel`)

**Wake sources (priority order):**
1. `g.notify <- struct{}{}` — direct buffered-channel wake from same-process `gateway.Enqueue()`. Zero latency, zero DB cost. This is the common case.
2. `listenConn.Conn().WaitForNotification(ctx)` — wakes when `pg_notify('embed_queue', '')` is issued by another process (e.g., a migration script or a second server replica). One dedicated connection held open for LISTEN.
3. `time.After(maxIdleBackoff)` — safety net. Default `maxIdleBackoff` = 5 minutes. Ensures the drain loop catches any chunks that were queued before the LISTEN connection was established (startup race), or if `pg_notify` was missed.

**`StoreWithRawBody` change:**
After `gateway.Enqueue(chunkIDs)`, emit `pg_notify('embed_queue', '')` in a best-effort goroutine (non-blocking, fire-and-forget). This costs one DB round-trip per store call — acceptable given it replaces blocking embed calls. For in-process deployments, the `g.notify` channel wake is faster and this step is redundant but harmless.

**Idle backoff curve:**
```
minIdle = ENGRAM_EMBED_GW_MIN_IDLE (default 5s)
maxIdle = ENGRAM_EMBED_GW_MAX_IDLE (default 300s = 5 min)
growth  = 2× per empty drain
```
On a fully empty queue the goroutine reaches maxIdle in ~5 doublings and parks there until a notify arrives. This replaces the 10s busy-poll.

---

## 6. DB Backpressure Design

### 6.1 Dedicated connection budget

The gateway owns its own `*pgxpool.Pool` created with `pgxpool.NewWithConfig` against the same DSN as the shared user pool. It is NEVER the shared pool.

**Proposed sizes (relative to the in-flight DB scaling change):**

| Pool | Current | Proposed after DB scaling | Rationale |
|------|---------|--------------------------|-----------|
| User-serving pool (`configureSharedPool`) | `MaxConns: 50` | `MaxConns: 60–80` | User traffic; recall + store path. Raised because the 256GB/40T host supports Postgres `max_connections: 200–400`. |
| Gateway dedicated pool | — (new) | `MaxConns: 8`, `MinConns: 1` | Background embed drain. 8 = `globalConcurrency` constant in current `global_worker.go`. Hard ceiling; can never grow. |
| CLI tools (`configurePool`) | `MaxConns: 25` | unchanged | Migration scripts, CLI commands. |

Total maximum: 80 (user) + 8 (gateway) + 25 (CLI tools, not concurrent with server) = 88–105 connections well within the proposed 200–400 `max_connections`.

**Why 8 for the gateway?** The current `GlobalReembedder` uses `errgroup.SetLimit(globalConcurrency)` = 8 concurrent embed calls per batch. Each running goroutine holds one connection briefly for the `UPDATE chunks SET embedding=$1 WHERE id=$2` write. 8 connections covers the maximum concurrent writes with one spare. The embedder call itself does NOT hold a DB connection (it's an HTTP call to olla).

**Hard floor guarantee:** The gateway pool is constructed separately from the user pool. Even if the gateway pool is fully saturated (8 connections all held), the user pool's 60–80 connections are unaffected. This is structural isolation, not a soft limit.

### 6.2 Adaptive throttle

The `AdaptiveThrottle` struct observes user-traffic signals and adjusts the gateway's drain concurrency:

```go
type AdaptiveThrottle struct {
    mu              sync.Mutex
    concurrency     int           // current goroutine limit [1, maxConcurrency]
    maxConcurrency  int           // = 8 (gateway pool max)
    minConcurrency  int           // = 1 (always make progress)
    // signal inputs
    userPoolWait    time.Duration // p95 acquire wait on user pool (from pgxpool.Stat)
    userActiveConns int32         // pool.Stat().AcquiredConns()
    userPoolCap     int32         // pool max
}

func (t *AdaptiveThrottle) Update(stat pgxpool.Stat) {
    t.mu.Lock()
    defer t.mu.Unlock()
    utilisation := float64(stat.AcquiredConns()) / float64(t.userPoolCap)
    switch {
    case utilisation > 0.80:   t.concurrency = t.minConcurrency  // user traffic heavy
    case utilisation > 0.60:   t.concurrency = max(t.concurrency-1, t.minConcurrency)
    case utilisation < 0.30:   t.concurrency = min(t.concurrency+1, t.maxConcurrency)
    }
}

func (t *AdaptiveThrottle) Concurrency() int { /* locked read */ }
```

The drain loop calls `errgroup.SetLimit(t.Concurrency())` at the start of each batch. The throttle is updated by a 5-second ticker in the gateway goroutine (one `pgxpool.Stat()` call per tick — cheap, non-blocking).

**Throttle signal choice rationale:** Pool utilisation is the right signal because it directly measures whether user traffic is competing for DB resources. CPU load and embed latency are olla-side signals; they do not represent DB contention. Postgres `active_connections` from `pg_stat_activity` would be more accurate but requires a DB query; `pgxpool.Stat()` is in-process and zero-cost.

**Background embedding ALWAYS yields.** When `utilisation > 0.80`, gateway concurrency drops to 1 (not 0 — it always makes some progress). This prevents permanent queue growth under sustained user load while never starving user traffic.

---

## 7. Form Factor: In-Process Module (Recommendation)

**Recommendation: in-process package `internal/embedgateway` in `cmd/engram`.**

**Rationale:**

The codebase is already architecturally in-process for the Go layer. `GlobalReembedder` runs as a goroutine sharing the same `pgxpool.Pool`. The `embedderAliases` table and `checkEmbedderMeta` already live in `internal/search/engine.go`. The model-invariant enforcement requirement argues strongly for keeping everything in one process — splitting across a process boundary creates a distributed agreement problem for the alias table.

The LISTEN/NOTIFY wake mechanism is most efficient in-process (direct channel wake, no pg_notify round-trip for the common case). The dedicated pool isolation (§6.1) gives the same hard floor guarantee in-process as a sidecar would, without the deployment and lifecycle complexity of a separate container.

The `reembed-rs` sidecar (`engram-reembed-7900xt`, `engram-reembed-w6800` containers) was deployed to use different GPU-bearing olla endpoints. The gateway design can call any remote olla URL, so the multi-GPU pattern is preserved: the gateway's `embed.Client` is configured with the priority-LB endpoint (as per the in-flight `fix/olla-embed-priority` work), not a host-local GPU. The sidecar containers become redundant.

**Pending:** Codex input and founder ratification. See https://github.com/petersimmons1972/claude-codex/issues/2#issuecomment-4583218973

If Codex identifies a concrete reason why in-process fails (e.g., the multi-GPU endpoint pattern requires separate processes), the sidecar option is: rewrite `reembed-rs` in Go as `cmd/engram-embed-worker`, sharing `internal/embedgateway` as a library. The model-invariant and validation logic would still live in the shared package; the form factor would change but not the architecture.

---

## 8. Migration: Absorbing the Reembed Sidecar

### 8.1 Additive backfill of NULL/wrong-dim rows

On gateway startup, after `Start(ctx)` is called, a one-time scan enqueues all chunks with `embedding IS NULL`:

```sql
SELECT id FROM chunks WHERE embedding IS NULL
```

This is a read-only scan — no destructive operation. Chunk records are not deleted or modified; they are enqueued for the drain loop. The drain loop fills them in over time. This replaces the current `NewWorkerFromMeta` startup logic.

**No destructive migration.** The spec explicitly forbids destructive re-embedding (nulling existing vectors and rewriting). The backfill targets only rows that are already NULL — rows with existing 1024-dim bge-m3 vectors are untouched.

Wrong-dim rows (e.g., 768-dim vectors from a previous `nomic-embed-text` era) are detected by the pgvector column type mismatch and would have already caused INSERTs to fail. If any survive in the DB, they require a separate investigation before the gateway handles them.

### 8.2 Decommissioning `reembed-rs`

1. Remove `engram-reembed-7900xt` and `engram-reembed-w6800` service entries from `docker-compose.yml` and `docker-compose.lan.yml` (or rename with `_disabled` suffix pending founder confirmation).
2. Set `ENGRAM_REEMBED_BATCH_SIZE=0` in the MCP server container env (already done per `docker-compose.yml` comment; this setting disables the in-process GlobalReembedder). After the gateway replaces GlobalReembedder, this env var is renamed to `ENGRAM_EMBED_GW_ENABLED=true/false`.
3. `Dockerfile.reembed` is kept in the repo but not built in CI until founder explicitly archives it.

### 8.3 Cutover sequence (zero data-loss)

1. Deploy engram-go with gateway **disabled** (`ENGRAM_EMBED_GW_ENABLED=false`). GlobalReembedder continues as-is.
2. Enable gateway (`ENGRAM_EMBED_GW_ENABLED=true`), GlobalReembedder disabled (`ENGRAM_REEMBED_BATCH_SIZE=0`). Both drain from the same queue (FOR UPDATE SKIP LOCKED prevents double-processing). Monitor `embed_validation_rejections_total` — expect zero.
3. Scale down `reembed-rs` sidecar containers once pending queue is observed at zero for 24h.
4. Remove GlobalReembedder wiring from `main.go`.

---

## 9. Risks and Testing Approach

### Risks

| Risk | Likelihood | Mitigation |
|------|-----------|------------|
| LISTEN connection drops (network flap, PG restart) | Medium | `time.After(maxIdle=5m)` safety net catches any missed notifies. Pool health-check at 15s evicts dead connections. Gateway re-acquires LISTEN conn on reconnect. |
| Adaptive throttle over-suppresses background embeds | Low | `minConcurrency=1` hard floor. If queue grows unboundedly, `metrics.ChunksPendingReembed` alerts. |
| `EmbedWithModel` not returning model ID from olla | Medium | LiteLLM `/v1/embeddings` response includes `model` field in the response body. Olla may or may not echo it. If model ID is unavailable, fall back to validating dims only (1024-dim check still blocks wrong vectors); log a warning that model validation is degraded. |
| Multi-instance (two engram-go replicas) competing for queue | Low | `FOR UPDATE SKIP LOCKED` handles this correctly — already proven by current GlobalReembedder. Each gateway instance claims a non-overlapping batch. |
| `docs/EMBEDDING.md` not yet written | High (dependency) | Gateway implementation must be blocked on EMBEDDING.md authorship. This is an open item for the spec. The alias constants already exist in `engine.go:embedderAliases` — EMBEDDING.md formalises them. |

### Testing approach (TDD)

Following the engram-go test policy: failing tests before implementation, ≥60% function coverage on new files.

**Unit tests for `internal/embedgateway`:**
- `TestValidateEmbedResponse_AcceptsAllAliases` — all four alias variants return no error for 1024-dim vecs.
- `TestValidateEmbedResponse_RejectsWrongModel` — non-bge-m3 model ID returns error.
- `TestValidateEmbedResponse_RejectsWrongDims` — 768-dim vec with correct model ID returns error.
- `TestGateway_EnqueueWakesLoop` — enqueue after Start wakes drain within 100ms (using stub embedder).
- `TestGateway_DrainBatch_SkipsOnValidationFailure` — inject a stub embedder returning wrong dims; verify chunk embedding NOT updated in DB.
- `TestGateway_AdaptiveThrottle_BacksOffUnderLoad` — mock high pool utilisation; verify concurrency drops to 1.
- `TestGateway_Stop_DrainsInFlight` — verify Stop waits for in-flight batch to complete.
- `TestGateway_ListenNotify_WakesFromPgNotify` — integration test (requires pgxpool); emit `pg_notify` and verify drain is triggered.
- `TestGateway_NullBackfillOnStart` — pre-populate chunks with NULL embedding; Start gateway; WaitDrained; verify all embeddings set.

**Integration tests:**
- `TestStoreReturnsBeforeEmbed` — time `memory_store`; assert returns in <20ms even with a 500ms mock embedder.
- `TestGateway_ReplacesGlobalReembedder` — end-to-end: store memory with null embedding via gateway mode; recall returns vector match.

### Rollout plan

The gateway is gated behind a feature flag: `ENGRAM_EMBED_GW_ENABLED` (default `false` at first deploy, `true` after validation). All existing code paths remain active when the flag is false. This makes the rollout reversible by env-var change without a redeployment.

**Stage 1 (flag=false, default):** Deploy new binary. Gateway code compiled in but not started. Zero behaviour change. Baseline metrics collected.

**Stage 2 (flag=true, sidecar running):** Gateway active, GlobalReembedder disabled (BATCH_SIZE=0), reembed-rs sidecar still running. Monitor `embed_validation_rejections_total` (expect 0), `chunks_pending_reembed` (expect declining), store latency (expect <20ms p99). 24-48h soak.

**Stage 3 (flag=true, sidecar stopped):** Scale reembed-rs containers to 0. Monitor pending-queue gauge for 24h. If queue drains and no regressions: declare stable.

**Rollback:** Set `ENGRAM_EMBED_GW_ENABLED=false` and restart. GlobalReembedder resumes. Restart reembed-rs containers if needed. No data loss risk — chunks with NULL embedding are simply re-drained.

---

## 10. Open Questions for Founder

1. **EMBEDDING.md** — this design depends on a canonical `docs/EMBEDDING.md`. Should authoring that be the first deliverable before gateway implementation begins?

2. **`EmbedWithModel` interface** — should the `embed.Client` interface extension (`EmbedWithModel` returning model ID) be a breaking change (update all implementations at once) or additive (add new method, keep `Embed` as-is)? The `LiteLLMClient`, `OllamaClient`, and any test stubs all need updating.

3. **Multi-GPU endpoint routing** — the current `reembed-rs` sidecar containers point at GPU-specific olla endpoints (`REEMBED_7900XT_URL`, `REEMBED_W6800_URL`). Should the gateway support multiple upstream endpoints with load-balancing (matching the current olla priority-LB fix on `fix/olla-embed-priority`)? Or does the gateway always use the single priority-LB URL?

4. **`pg_notify` per-store cost** — is the one DB round-trip per `memory_store` for `pg_notify` acceptable, or should it be fire-and-forget via a goroutine? (The in-process channel wake makes it redundant for single-instance deployments; only needed for multi-instance.)

5. **Form factor** — pending Codex input (https://github.com/petersimmons1972/claude-codex/issues/2#issuecomment-4583218973). Codex may have observed structural issues in the engram-go codebase that affect this choice.

6. **DB scaling interaction** — this spec proposes user pool `MaxConns: 60–80` and gateway pool `MaxConns: 8`, against a target Postgres `max_connections: 200–400`. The DB scaling change is in-flight. Should the gateway pool sizes be set conservatively (8/60) for the initial deployment, with a follow-on tuning pass after the DB is scaled?

---

## Appendix: Key File Locations (as of 2026-05-30)

| Component | Location |
|-----------|----------|
| Alias table (current) | `internal/search/engine.go:embedderAliases` |
| GlobalReembedder | `internal/reembed/global_worker.go` |
| Per-project Worker | `internal/reembed/worker.go` |
| Shared pool config | `internal/db/postgres.go:configureSharedPool` |
| Store path | `internal/search/engine.go:StoreWithRawBody` |
| MCP store handler | `internal/mcp/tools_store.go:handleMemoryStore` |
| Rust sidecar | `reembed-rs/src/main.rs` |
| Docker services | `docker-compose.yml` (engram-reembed-7900xt, engram-reembed-w6800) |
| Embedder config doc | `docs/configuration/embedders.md` |
| Planned: canonical model doc | `docs/EMBEDDING.md` (not yet written) |
| Planned: gateway package | `internal/embedgateway/` (not yet written) |
| Planned: model constants package | `internal/embedmodel/` (not yet written) |
