-- Manual migration option for issue #1020: IVFFlat index on chunks.embedding.
--
-- DO NOT run during the active LME campaign.
-- DO NOT run from the automatic embedded migration runner.
-- Requires founder/operator approval before execution.
--
-- Expected profile:
--   - Faster build than HNSW, typically minutes rather than hours.
--   - Lower recall quality than HNSW unless probes/lists are tuned carefully.
--   - Good fallback if HNSW build cost is unacceptable on the live corpus.
--   - CREATE INDEX CONCURRENTLY avoids blocking normal reads/writes but still
--     consumes substantial CPU, memory, disk, and WAL.
--
-- Tuning note:
--   - lists=4000 is a middle value in the requested 2000-5000 range for a
--     24.5M-row corpus. Operators should benchmark recall/latency and tune
--     ivfflat.probes at query time before choosing this option permanently.
--
-- Operator preflight:
--   - Run with psql autocommit enabled. CREATE INDEX CONCURRENTLY cannot run
--     inside an explicit transaction block.
--   - Monitor pg_stat_progress_create_index, I/O, WAL volume, and recall latency.

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chunks_embedding_ivfflat
    ON chunks USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 4000)
    WHERE embedding IS NOT NULL;

ANALYZE chunks;
