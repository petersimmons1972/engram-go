# Engram-Go Postgres Durability Posture

**Verdict: C — Hybrid (keep sync=disabled, add ZFS snapshot replication)**

The database contains a mix of rebuildable-cache data (embeddings, scoring signals, retrieval telemetry) and authoritative data that cannot be reconstructed from any external source (user-authored memories, manual corrections, constraint policies, episode metadata, and weight-tuning decisions). The loss window with `sync=disabled` is up to ~5 seconds of committed writes. That is acceptable for cache data but not for authoritative rows.

---

## Durability Posture

| Setting | Value | Implication |
|---------|-------|-------------|
| ZFS `sync` | `disabled` | fsync calls from Postgres are ignored by the OS/ZFS layer |
| Max data loss window | ~5 s | Any committed transaction in the last 5 s before power failure may not reach persistent storage |
| ZFS recordsize | **128K** (inherited from `zp1`) | Not the planned 16K — see issue #740 for the corrective action |
| Postgres WAL | Written, but fsyncs silently no-ops | WAL is present but not guaranteed durable below the 5 s window |

`sync=disabled` is the TrueNAS SCALE default for NFS-attached pools and is appropriate for scratch workloads. For Engram it is defensible only because:

1. The **bulk** of the database (embeddings, retrieval events, chunk text) is reconstructable from the source chat transcripts and re-running the embed pipeline.
2. A ZFS snapshot schedule (see Recovery Runbook below) provides a point-in-time backstop that reduces effective data loss to at most the snapshot interval, not 5 seconds.

The risk that remains is **up to one snapshot interval of authoritative writes** (new memories, corrections, feedback, episodes, weight adjustments). Flipping to `sync=standard` (Verdict B) would close that window entirely but at a measurable throughput cost on spinning-disk pools. Verdict C (keep `sync=disabled`, add snapshot replication) is the recommended path — it trades a small authoritative-write risk for operational simplicity.

---

## Table-by-Table Classification

### Authoritative — data loss is unrecoverable

| Table | Why authoritative |
|-------|-------------------|
| `memories` | User-authored content, manual corrections, constraint policies (stored as memories with `constraint`/`policy` tags), `immutable` flag, `invalidation_reason`, `valid_from`/`valid_to` temporal windows, `pattern_confidence` |
| `memory_versions` | Full audit trail of every edit and soft-delete; source of truth for change history — cannot be reconstructed once the parent row is overwritten |
| `relationships` | User-declared or system-inferred edges between memories; deletion or creation by the user represents intent with no external source |
| `episodes` | User-named work sessions with descriptions and summaries; `started_at`/`ended_at` timestamps represent real session boundaries |
| `weight_config` | Per-project composite scoring weights, actively tuned by the adaptive weight system; losing the current config causes scoring drift |
| `weight_history` | Log of every weight adjustment with `trigger_data` and `notes`; required for auditing retrieval quality regressions |
| `canonical_entities` | Deduplicated entity names and aliases maintained by the user/operator; no external authoritative source |
| `project_meta` | Schema version, embedder contract (`embedder_dimensions`), operator-set flags; losing this can prevent server startup |
| `project_ttl` | Operator-set expiry timestamps for projects; losing these silently extends ephemeral-project lifetimes |

### Rebuildable — can be reconstructed by re-running pipelines

| Table | Rebuild path |
|-------|-------------|
| `chunks` | Re-run the chunking pipeline against `memories.content`; all chunk text is derived from the memory content |
| `chunks.embedding` | Re-run `engram reembed` after chunks are restored; requires the embedding model to be available |
| `documents` | Large-document storage (>8 MB) — content is derived from the ingest source; re-ingest from the original file |
| `audit_canonical_queries` | Re-register canonical queries from operator runbook; loss is inconvenient but not data-loss |
| `audit_snapshots` | Historical retrieval-rank snapshots; baseline is lost but future baselines can be re-established |
| `memories.dynamic_importance` | Recomputed by the spaced-repetition decay worker from `importance` and access history |
| `memories.times_retrieved` / `times_useful` / `retrieval_precision` | Resets to 0 — loses learned precision signal; recovers over time from new retrieval events |
| `memories.content_hash` | Recomputed by migration 009/012 from `memories.content` |
| `memories.search_vector` | GENERATED ALWAYS — rebuilt automatically by Postgres on any write |

### Operational — transient state, safe to lose

| Table | Why safe |
|-------|---------|
| `retrieval_events` | Ephemeral retrieval telemetry; 90-day TTL, used only for scoring — loss causes a scoring warm-up period, not data loss |
| `mcp_sessions` | Active SSE session registrations; clients reconnect automatically on server restart |
| `entity_extraction_jobs` | Work queue for entity extraction; pending jobs are re-queued on the next ingest cycle |
| `chunks.embed_lease_until` / `embed_lease_owner` | Distributed reembed worker leases; expire naturally, workers reclaim |

---

## Verdict: C — Hybrid

**Recommendation:** Keep `sync=disabled`. Add:

1. **ZFS snapshot schedule on `zp1/postgres-engram`** — at least hourly send/receive to a second pool or offsite target. This bounds authoritative data loss to the snapshot interval, not 5 seconds.
2. **Daily `pg_dump` backup** — logical backup is snapshot-interval-independent and survives pool-level corruption. Store on a different dataset.

**Do not** flip to `sync=standard` unless throughput testing on the current pool confirms the write-amplification penalty is acceptable. On spinning-disk HDD vdevs this is typically a 3–10× write throughput reduction.

---

## Recovery Runbook — After TrueNAS Power Loss

### Step 1: Assess data loss window

```bash
# On the Postgres host, check the last checkpoint timestamp.
psql -h trunas.petersimmons.com -p 5434 -U engram engram -c \
  "SELECT pg_last_xact_replay_timestamp(), NOW() - pg_last_xact_replay_timestamp() AS lag;"

# Check ZFS snapshot timestamps.
ssh trunas "zfs list -t snapshot zp1/postgres-engram | tail -5"
```

### Step 2: Start the server and verify schema integrity

```bash
# Start engram-go; it runs migrations on boot.
kubectl rollout restart deployment/engram -n engram

# Verify schema version matches expectations.
psql -h trunas.petersimmons.com -p 5434 -U engram engram -c \
  "SELECT * FROM project_meta WHERE project='_engram';"
```

### Step 3: Check for truncated authoritative rows

```bash
# Memories with content_hash mismatch indicate partial writes.
psql -h trunas.petersimmons.com -p 5434 -U engram engram -c "
  SELECT id, project, created_at, updated_at
  FROM memories
  WHERE content_hash IS NOT NULL
    AND content_hash != encode(sha256(convert_to(content, 'UTF8')), 'hex')
    AND valid_to IS NULL
  ORDER BY updated_at DESC
  LIMIT 20;
"

# Open memory_versions rows (system_to IS NULL) with no matching current memory.
psql -h trunas.petersimmons.com -p 5434 -U engram engram -c "
  SELECT mv.memory_id, mv.change_type, mv.system_from
  FROM memory_versions mv
  LEFT JOIN memories m ON mv.memory_id = m.id
  WHERE mv.system_to IS NULL AND m.id IS NULL
  LIMIT 20;
"
```

### Step 4: Reconcile corrupt authoritative rows

```bash
# Option A: Roll back corrupt memories to the last good version snapshot.
# Identify last good version for a corrupt memory_id:
psql -c "
  SELECT id, system_from, system_to, change_type, content
  FROM memory_versions
  WHERE memory_id = '<corrupt_id>'
  ORDER BY system_from DESC;
"

# Option B: If no version snapshot exists, restore from the most recent pg_dump backup.
pg_restore -h trunas.petersimmons.com -p 5434 -U engram -d engram \
  --table=memories --data-only /path/to/backup.dump
```

### Step 5: Rebuild rebuildable cache tables

```bash
# Re-run content_hash backfill.
psql -h trunas.petersimmons.com -p 5434 -U engram engram \
  -f internal/db/migrations/012_backfill_content_hash_safe.sql

# Re-embed chunks (nulls set by power-loss mid-embed).
engram reembed --project=all --null-only

# Rebuild HNSW index after reembed completes.
psql -h trunas.petersimmons.com -p 5434 -U engram engram -c "
  CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chunks_embedding_hnsw
      ON chunks USING hnsw (embedding vector_cosine_ops)
      WITH (m = 16, ef_construction = 64);
"
```

### Step 6: Verify retrieval health

```bash
# Run a smoke-test recall against each active project.
for project in clearwatch homelab engram global; do
  engram recall --project=$project "recent work" --top=3
done

# Confirm weight_config is present for active projects.
psql -h trunas.petersimmons.com -p 5434 -U engram engram -c \
  "SELECT project, updated_at FROM weight_config ORDER BY updated_at DESC;"
```

---

## Related Issues

- **#830** — This audit (closes)
- **#740** — ZFS recordsize=128K (inherited, should be 16K for Postgres) — tracked separately; does not affect durability classification, only I/O alignment efficiency

---

*Last updated: 2026-05-21. Audit by: Claude Code (claude-sonnet-4-6).*
