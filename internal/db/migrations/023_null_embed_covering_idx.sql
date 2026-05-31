-- 023_null_embed_covering_idx.sql — cover null-embedding scans by project/time.
--
-- Reembed workers claim pending rows using FOR UPDATE SKIP LOCKED where
-- embedding IS NULL. At high row counts, this partial covering index avoids
-- broad scans on the chunks table and reduces lock contention with warmup.

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chunks_null_embed_proj_created
    ON chunks (project, created_at)
    WHERE embedding IS NULL;
