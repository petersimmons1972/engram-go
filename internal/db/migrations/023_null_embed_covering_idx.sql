-- 023_null_embed_covering_idx.sql — narrow index for null-embedding reembed scans.
--
-- Reembed and readiness pre-warm paths scan only rows with NULL embeddings.
-- A covering partial index on the columns used for backpressure and work ordering
-- keeps those queries off full-table scans under large backlog scenarios.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chunks_null_embed_proj_created
    ON chunks (project, created_at)
    WHERE embedding IS NULL;
