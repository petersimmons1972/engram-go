-- Manual migration option for issue #1020: HNSW index on chunks.embedding.
--
-- DO NOT run during the active LME campaign.
-- DO NOT run from the automatic embedded migration runner.
-- Requires founder/operator approval before execution.
--
-- Expected profile:
--   - Best recall/latency option for unscoped or federated vector recall.
--   - Heavy build on the production corpus: 24.5M rows x 1024 dimensions.
--   - Likely hours of build time with high I/O and maintenance_work_mem demand.
--   - CREATE INDEX CONCURRENTLY avoids blocking normal reads/writes but still
--     consumes substantial CPU, memory, disk, and WAL.
--
-- Operator preflight:
--   - Run with psql autocommit enabled. CREATE INDEX CONCURRENTLY cannot run
--     inside an explicit transaction block.
--   - Consider increasing maintenance_work_mem for the session, sized to the
--     host and concurrent workload:
--       SET maintenance_work_mem = '8GB';
--   - Monitor pg_stat_progress_create_index, I/O, WAL volume, and recall latency.

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chunks_embedding_hnsw
    ON chunks USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64)
    WHERE embedding IS NOT NULL;

ANALYZE chunks;
