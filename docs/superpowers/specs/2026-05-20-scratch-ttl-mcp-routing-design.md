# Design Spec: Route `--scratch-ttl` Through MCP Instead of Direct DB

**Issue:** [#837](https://github.com/petersimmons/engram-go/issues/837) — `lme: --scratch-ttl requires direct DB access; must route through MCP instead`
**Date:** 2026-05-20
**Approach:** A — thread `expires_at` into `/quick-store`
**Status:** Approved

---

## Summary

The `--scratch-ttl` flag in the LME benchmark CLI writes project TTL directly to the `project_ttl` Postgres table via a `--database-url` flag. In any deployed topology where the LME client and Postgres are not co-located — the normal case — this silently does nothing. The fix threads an optional `expires_at` field through the existing `/quick-store` REST endpoint so TTL is set via the Engram server's pooled DB connection, requiring no separate database access from the client.

---

## Problem

`lme run --scratch-ttl 168h` is intended to mark scratch projects as expiring, enabling `lme prune` to clean them up after a benchmark run. The current implementation connects directly to Postgres from the CLI using `--database-url` and writes to the `project_ttl` table via a `ttlStamper` interface. This design has two failure modes:

1. **Silent no-op in remote topologies.** When LME runs against a remote Engram instance without a `--database-url`, the stamp is skipped. No error is surfaced. Projects accumulate without expiry.
2. **Unnecessary credential surface.** Giving the benchmark CLI a direct Postgres credential to the Engram database is a wider blast radius than needed. The server already owns that connection pool.

`lme prune` already routes through MCP and works correctly. The gap is only in the write path during ingest.

---

## Approved Approach

Thread an optional `expires_at` field (RFC3339 string) into the existing `POST /quick-store` request body. The server applies `SetProjectTTL` using the engine's already-pooled backend connection after a successful store. The client computes `now + ScratchTTL` and passes it on every QuickStore call when the flag is set. The direct DB path and `--database-url` flag are removed entirely.

This approach was selected over alternatives because:

- It requires no new endpoints.
- It reuses the engine pool that `handleQuickStore` already acquires.
- It does not extend the `memory_quick_store` MCP tool (which would require coordinating schema changes across the broader MCP surface).
- The idempotency of `SetProjectTTL` (`ON CONFLICT DO UPDATE`) makes it safe to upsert on every call without extra state in the client.

---

## What Changes

### `internal/mcp/server.go` — `handleQuickStore`

The request body struct gains one optional field:

```
ExpiresAt *time.Time  // JSON: "expires_at", RFC3339 string; nil means no TTL
```

After the store succeeds, if `ExpiresAt` is non-nil:

- Validate that `ExpiresAt` is in the future. Return HTTP 400 if it is in the past or present.
- Call `h.Engine.Backend().SetProjectTTL(ctx, project, time.Now().UTC(), *ExpiresAt)` using the engine obtained from `s.pool.Get(ctx, project)` — the same pool call already made earlier in the handler (reference: lines 582, 627, 666 in the current server.go).
- If `SetProjectTTL` returns an error, log at WARN level and continue. Do not return the error to the caller. This preserves the best-effort semantics of the original direct-DB path and avoids failing an otherwise successful store because of a TTL bookkeeping failure.

No new endpoint is introduced. No changes to routing or middleware.

### `internal/longmemeval/engram.go` — `RestClient.QuickStore`

The method signature changes from:

```
QuickStore(ctx context.Context, project, content string, tags []string) error
```

to:

```
QuickStore(ctx context.Context, project, content string, tags []string, expiresAt *time.Time) error
```

When `expiresAt` is non-nil, the POST body includes:

```json
"expires_at": "<RFC3339 timestamp>"
```

When `expiresAt` is nil, the field is omitted from the marshaled body entirely (use `omitempty` or conditional construction — do not send a null JSON value).

### `cmd/longmemeval/ingest.go`

Three changes:

1. **Remove `ttlStamper`.** Delete the `ttlStamper` interface and its entire implementation block, including the `if cfg.ScratchTTL > 0 && cfg.DatabaseURL != ""` conditional.

2. **Remove `stamper` from `ingestWorker` signature.** The worker no longer receives or calls a stamper.

3. **Compute `expiresAt` in `ingestOne`.** When `cfg.ScratchTTL > 0`, compute `expiresAt = time.Now().UTC().Add(cfg.ScratchTTL)` and pass `&expiresAt` to `restClient.QuickStore`. When `cfg.ScratchTTL == 0`, pass `nil`.

The `ingestOne` function receives `cfg` and is therefore the right location for this computation — no threading of a pre-computed value through the worker is needed.

### `cmd/longmemeval/main.go`

Two removals:

1. **Remove `DatabaseURL` from `Config`.** The field is no longer referenced anywhere after the `ttlStamper` removal.

2. **Remove the `--database-url` flag registration.** Any existing documentation or help text referencing `--database-url` in this command should be removed at the same time.

`ScratchTTL` and `--scratch-ttl` are unchanged.

---

## What Does NOT Change

| Component | Reason |
|-----------|--------|
| `project_ttl` schema (migration 022) | The table structure is correct as-is. No migration required. |
| `lme prune` | Already routes through MCP. Finds and deletes expired projects correctly today. |
| `memory_quick_store` MCP tool | Extending the MCP tool is out of scope for this fix. The REST layer is sufficient. |
| `memory_store`, `memory_store_batch`, `memory_store_document` | Not extended. TTL threading is scoped to the quick-store path used by LME ingest. |
| `SetProjectTTL` implementation | The existing `ON CONFLICT DO UPDATE` upsert is correct and requires no change. |

---

## Acceptance Criteria

1. `lme run --scratch-ttl 168h` against a remote Engram instance (no shared DB, no `--database-url`) correctly sets TTL on every project created during the run. Verified by checking `project_ttl` rows via the Engram server after the run.

2. The `--database-url` flag no longer exists in `lme run --help` output.

3. `lme prune` against the same remote instance finds and deletes projects whose `expires_at` has passed. (This already works; regression must not be introduced.)

4. `lme run` with `--scratch-ttl 0` or with the flag omitted behaves identically to the current baseline — projects are durable, no `project_ttl` row is written.

5. A `POST /quick-store` request with a past `expires_at` returns HTTP 400 with a descriptive error message. The store is not written.

6. A `POST /quick-store` request with no `expires_at` field succeeds and writes no `project_ttl` row — preserving the existing behavior for all callers not using TTL.

---

## Tests Required

### `internal/mcp/quick_store_handler_test.go`

| Test | What it verifies |
|------|-----------------|
| Happy path — `expires_at` set, future timestamp | Store succeeds; `project_ttl` row is written with the correct `expires_at`. |
| Invalid `expires_at` — past timestamp | Handler returns HTTP 400; no store write occurs. |
| Invalid `expires_at` — present (now or within clock skew) | Handler returns HTTP 400. |
| Missing `expires_at` | Store succeeds; no `project_ttl` row is written. Existing behavior is preserved. |
| `SetProjectTTL` returns error | Store succeeds (HTTP 200); error is logged at WARN; no 500 returned to caller. |

### `internal/longmemeval/engram_test.go`

| Test | What it verifies |
|------|-----------------|
| `QuickStore` with non-nil `expiresAt` | Request body contains `"expires_at"` field serialized as RFC3339. |
| `QuickStore` with nil `expiresAt` | Request body does not contain an `"expires_at"` key (not null, fully absent). |

### `cmd/longmemeval/ingest_test.go`

| Test | What it verifies |
|------|-----------------|
| Ingest worker, `ScratchTTL > 0` | `QuickStore` is called with a non-nil `expiresAt` equal to `now + ScratchTTL` (within a small delta). |
| Ingest worker, `ScratchTTL == 0` | `QuickStore` is called with nil `expiresAt`. |

---

## Out of Scope

The following are explicitly excluded from this change. File separate issues if any of these are wanted:

- Extending the `memory_quick_store` MCP tool with an `expires_at` parameter.
- Adding TTL support to `memory_store`, `memory_store_batch`, or `memory_store_document`.
- Any changes to `lme prune` — it already works correctly.
- UI or observability surface for per-project TTL (e.g., listing projects with their expiry times).
- Client-side retry or backoff on `SetProjectTTL` failures (the WARN log is sufficient).
