-- Partial index to make the global reembedder's NULL-embedding SELECT efficient
-- under large backlogs. Replaces a full-table sort+scan with a pure index scan.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chunks_null_embed_id_desc
    ON chunks (id DESC)
    WHERE embedding IS NULL;
