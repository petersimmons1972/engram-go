# chunks.embedding ANN Index Rollout Note

Issue: #1020

These files are manual, founder-gated migration artifacts. They are intentionally
stored under `ops/manual-migrations/` so the embedded startup migration runner
cannot execute them automatically.

## Options

### HNSW

File: `20260605_chunks_embedding_hnsw.sql`

- Best recall/latency option for unscoped or federated vector recall.
- Heavy build on the production corpus: 24.5M rows x 1024 dimensions.
- Expected to take hours and produce substantial I/O and WAL.
- Uses `CREATE INDEX CONCURRENTLY IF NOT EXISTS`.
- Partial index predicate: `WHERE embedding IS NOT NULL`.

### IVFFlat

File: `20260605_chunks_embedding_ivfflat.sql`

- Faster build, with lower recall quality than HNSW unless tuned.
- Uses `lists = 4000`, inside the requested 2000-5000 range.
- Requires `ANALYZE chunks` after build and query-time `ivfflat.probes` tuning.
- Uses `CREATE INDEX CONCURRENTLY IF NOT EXISTS`.
- Partial index predicate: `WHERE embedding IS NOT NULL`.

## Rollback

File: `20260605_chunks_embedding_ann_rollback.sql`

Drops both optional index names with `DROP INDEX CONCURRENTLY IF EXISTS`, then
runs `ANALYZE chunks`.

## Guardrails

- Do not run any of these files during the active LME campaign.
- Do not run both HNSW and IVFFlat unless the operator intentionally wants to
  compare two large ANN indexes on the same column.
- Run with psql autocommit enabled; `CREATE INDEX CONCURRENTLY` and
  `DROP INDEX CONCURRENTLY` cannot run inside an explicit transaction block.
- Monitor `pg_stat_progress_create_index`, disk, WAL, CPU, memory, and recall
  latency while the build runs.
- Prefer HNSW if recall quality is the priority and the host can absorb the
  build cost. Prefer IVFFlat only when build cost is the binding constraint.
