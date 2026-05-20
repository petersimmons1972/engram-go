# Persistence Audit — "Parsed but Never Persisted" — 2026-05-19

**SHA audited:** c409c22  
**Auditor:** Claude Sonnet 4.6 (sub-agent, read-only)  
**Wall time:** ~40 min  
**Scope:** `internal/db/*.go`, `internal/mcp/tools_store.go`, `internal/types/types.go`, `internal/search/engine.go`, `internal/db/migrations/*.sql`, `internal/entity/entity.go`

---

## Summary Table

| # | Severity | File:Line | Finding | GH Issue |
|---|----------|-----------|---------|----------|
| F1 | **blocker** | `internal/mcp/tools_store.go:300–310` | `memory_store_batch` never calls `parseDateTag` → `ValidFrom` is always `nil` for all batch items | [#762](https://github.com/petersimmons1972/engram-go/issues/762) |
| F2 | **blocker** | `internal/mcp/tools_store.go:185–192` | `memory_store_document` never calls `parseDateTag` → `ValidFrom` nil; also never calls `episodeIDFromContextOrArgs` → `EpisodeID` always empty | [#763](https://github.com/petersimmons1972/engram-go/issues/763) |
| F3 | nice-to-have | `internal/mcp/tools_store.go:300–310` | `memory_store_batch` silently drops per-item `immutable=true` flag | [#764](https://github.com/petersimmons1972/engram-go/issues/764) |
| F4 | nice-to-have | `internal/db/postgres_memory.go:229–237` | `memory_correct` does not recalculate `valid_from` when `date:` tags are updated | [#765](https://github.com/petersimmons1972/engram-go/issues/765) |

---

## Methodology

### Dimension 1 — INSERT vs struct fields

Enumerated every `INSERT INTO <table>` in `internal/db/*.go` and cross-checked against the corresponding `types.*` struct fields and the `rowToMemory` scanner.

**memories INSERT** (`postgres_memory.go:78–88`): 19 columns. Cross-checked against `types.Memory` (24 exported fields). Discrepancies investigated:

- `search_vector`: generated column, not in INSERT. Correct by design.
- `RawBody`: `json:"-"`, explicitly documented as transient. False positive.
- `valid_from`: NOW IN INSERT (fix from #747, commit c409c22). ✅

**chunks INSERT** (`postgres_chunk.go:44–46`): 9 columns. `Chunk.LastMatched` is not in INSERT (starts NULL, updated on first vector-search hit). Correct by design — the `UPDATE chunks SET last_matched=NOW()` call in `UpdateChunkLastMatched` is the intended write path.

**memory_versions INSERT** (`postgres_memory.go:434–441`): snapshots `valid_from`, `valid_to`, `change_type`, `change_reason`, `project`. `PatternConfidence` and `DynamicImportance` are NOT snapshotted. This is an intentional scope limitation (version table tracks content/type/tags/importance changes only), not a bug. Confirmed by code comment pattern.

**episodes INSERT** (`postgres_episode.go:20–22`): 4 columns. `EndedAt` and `Summary` are NULL at creation, populated by `EndEpisode`. Correct.

**relationships INSERT** (`postgres_relationship.go:47–54`): all `Relationship` fields persisted. ✅

**retrieval_events INSERT** (`postgres_feedback.go:27–31`): 5 columns. `failure_class`, `feedback_ids`, `feedback_at` are NULL at creation — populated by `RecordFeedbackWithClass`. Correct by design.

**canonical_entities INSERT** (`postgres_entity.go:17–23`): `created_at` uses DB DEFAULT NOW(). `Entity` struct has no `CreatedAt` field — intentional; `updated_at` is in INSERT as `NOW()`. ✅

### Dimension 2 — Default fallbacks masking absence

**`temporalAnchorHours` (`internal/search/engine.go:1030–1034`)** — the archetype from #747. After the #747 fix, `ValidFrom` is correctly persisted by `storeMemoryExec`. However findings F1 and F2 show that two other store paths (`memory_store_batch`, `memory_store_document`) never populate `ValidFrom` before calling into the engine, so those paths still exhibit the pre-#747 silent degradation.

No other `if x == nil { x = time.Now() }` patterns found in hot scoring/recall paths that would mask absent fields.

### Dimension 3 — Migration vs ORM drift

Compared latest migration (021_pattern_confidence.sql) against `types.Memory`:

- `memories` DB columns: `id`, `content`, `memory_type`, `project`, `tags`, `importance`, `access_count`, `last_accessed`, `created_at`, `updated_at`, `search_vector`, `immutable`, `expires_at`, `summary`, `content_hash`, `storage_mode`, `valid_from`, `valid_to`, `invalidation_reason`, `dynamic_importance`, `retrieval_interval_hrs`, `next_review_at`, `times_retrieved`, `times_useful`, `retrieval_precision`, `episode_id`, `document_id`, `pattern_confidence` — **28 columns total**.
- `rowToMemory` scanner at `postgres.go:631–639` scans 28 positional parameters in migration-documented order. ✅
- `types.Memory` has 24 exported fields (plus `RawBody json:"-"`). Every DB column maps to a struct field; `search_vector` is scanned into a `[]byte` dummy that is discarded. ✅

No column-without-field or field-without-column drift found.

### Dimension 4 — Tag parsers → struct → INSERT

`parseDateTag` (`tools_store.go:515–531`) returns `*time.Time` parsed from `date:<value>` tags.

Trace:
- `handleMemoryStore` line 92: `ValidFrom: parseDateTag(tags)` → struct field set → reaches `storeMemoryExec` line 87: `m.ValidFrom` in INSERT position 19. ✅ (after #747 fix)
- `handleMemoryStoreBatch` lines 300–310: **`parseDateTag` NOT called**. `ValidFrom` field absent from struct literal → `nil` in DB. **F1 (blocker)**
- `handleMemoryStoreDocument` lines 185–192: **`parseDateTag` NOT called**. Same gap. **F2 (blocker)**
- `handleMemoryQuickStore` lines 485–504: delegates to `handleMemoryStore` → `parseDateTag` called. ✅

### Dimension 5 — MCP tool inputs vs storage

| Tool | `valid_from` | `episode_id` | `immutable` | `pattern_confidence` |
|------|-------------|-------------|------------|---------------------|
| `memory_store` | ✅ via `parseDateTag` | ✅ `episodeIDFromContextOrArgs` | ✅ `getBool` | ✅ validated |
| `memory_store_batch` (per-item) | ❌ **F1** | ✅ `batchEpisodeID` fallback | ❌ **F3** | ✅ validated |
| `memory_store_document` | ❌ **F2** | ❌ **F2** | ✅ `getBool` | not applicable |
| `memory_quick_store` | ✅ delegates to `memory_store` | ✅ same | ✅ same | ✅ same |

---

## Findings Detail

### F1 — BLOCKER: `memory_store_batch` drops `ValidFrom` for all batch items

**File:line:** `internal/mcp/tools_store.go:300–310` (SHA c409c22)

**Evidence:** `handleMemoryStore` calls `parseDateTag(tags)` at line 92 and assigns the result to `ValidFrom`. `handleMemoryStoreBatch` constructs the same `types.Memory` literal but omits this call entirely. The batch path is used by the LongMemEval haystack ingest pipeline (`internal/longmemeval/engram.go` — though the current LME ingest happens to use `QuickStore` which delegates correctly, the MCP-level `memory_store_batch` API is broken for any caller using `date:` tags).

**Scoring impact:** `temporalAnchorHours` at `internal/search/engine.go:1031-1034` uses `ValidFrom` as the primary temporal anchor. With `ValidFrom=nil`, it falls back to `LastAccessed = NOW()` (set in `storeMemoryExec` line 44), making every batch-ingested dated memory appear to have been created at ingest time, destroying recency scoring for historical memories.

**Proposed fix:** Add `ValidFrom: parseDateTag(itemTags)` to the `types.Memory` literal at line 300. One-line change.

**GH:** [#762](https://github.com/petersimmons1972/engram-go/issues/762)

---

### F2 — BLOCKER: `memory_store_document` drops `ValidFrom` and `EpisodeID`

**File:line:** `internal/mcp/tools_store.go:185–192` (SHA c409c22)

**Evidence:** The `types.Memory` struct built at lines 185–192 does not include `ValidFrom` or `EpisodeID`. Compare with `handleMemoryStore` lines 82–94 which populates both.

**`ValidFrom` impact:** Same as F1. Documents stored with date-tagged content will have `nil` ValidFrom, causing recency scorer to use ingest time.

**`EpisodeID` impact:** Documents stored during an active named episode (auto-episode context or explicit `episode_id` arg) will not be linked to that episode. `memory_episode_recall` will silently omit them.

**Proposed fix:**
```go
m := &types.Memory{
    // ... existing fields ...
    EpisodeID: episodeIDFromContextOrArgs(ctx, args),
    ValidFrom: parseDateTag(docTags),
}
```

**GH:** [#763](https://github.com/petersimmons1972/engram-go/issues/763)

---

### F3 — NICE-TO-HAVE: `memory_store_batch` ignores per-item `immutable` flag

**File:line:** `internal/mcp/tools_store.go:300–310` (SHA c409c22)

**Evidence:** `getBool(mmap, "immutable", false)` is never called in the batch item construction loop. The `Immutable` struct field is always `false`. Tool documentation promises parity with `memory_store`.

**Proposed fix:** Add `Immutable: getBool(mmap, "immutable", false)` to the struct literal.

**GH:** [#764](https://github.com/petersimmons1972/engram-go/issues/764)

---

### F4 — NICE-TO-HAVE: `memory_correct` does not update `valid_from` when tags change

**File:line:** `internal/db/postgres_memory.go:229–237` (SHA c409c22)

**Evidence:** Both UPDATE statements in `UpdateMemory` (one with content, one without) update `content`, `tags`, `importance`, `updated_at`, `pattern_confidence`, optionally `content_hash` — but NOT `valid_from`. A caller who adds `date:2023-01-01` via `memory_correct` will see tags updated in the DB but `valid_from` unchanged (still NULL if not set at creation).

**Assessment:** Borderline intentional vs bug. If `valid_from` is intended to be immutable after store, this should be documented. If it should follow tags, a one-line fix in the UPDATE adds `valid_from = $N`.

**GH:** [#765](https://github.com/petersimmons1972/engram-go/issues/765)

---

## False Positives Investigated

| Candidate | Conclusion |
|-----------|------------|
| `chunks.last_matched` not in INSERT | Correct — starts NULL, set on first vector hit by `UpdateChunkLastMatched` |
| `memory_versions` missing `PatternConfidence` / `DynamicImportance` | Intentional scope limit; version table tracks content/type/tags/importance only |
| `retrieval_events` missing `failure_class` at INSERT | Correct — populated by `RecordFeedbackWithClass`; starts NULL |
| `canonical_entities` missing `created_at` in struct | Correct — DB DEFAULT NOW(); no need in Go struct |
| `Memory.RawBody` never in INSERT | Correct — `json:"-"`, explicitly documented as transient ingest-path field |
| `StorageMode` default fallback in `rowToMemory` | Correct — backward compat for rows written before `storage_mode` column existed |

---

## Advisory Gate

No advisory-gate consultation was needed. All findings were clear #747-class bugs or intentional design choices confirmable from code comments, with no architecturally divergent paths.
