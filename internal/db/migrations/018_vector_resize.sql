-- Resize the pgvector embedding column to support larger models (e.g. qwen3-embedding:8b).
-- Drops the HNSW index, removes the old fixed-width column, re-adds it at the new dimension,
-- and recreates the index. All embeddings are set to NULL — memory_migrate_embedder will
-- re-embed them with the new model.
--
-- Target dimension: 1536 (qwen3-embedding:8b MRL-truncated via ENGRAM_EMBED_DIMENSIONS=1536).
-- pgvector HNSW supports a maximum of 2000 dimensions; 1536 is well within that limit
-- while providing 2x the representational capacity of the previous 768-dim nomic model.
-- MTEB retrieval at 1536 dims with qwen3-embedding:8b is still ~65-70 vs ~48 baseline.

-- 1. Drop the HNSW index (required before column type change).
DROP INDEX IF EXISTS idx_chunks_embedding_hnsw;

-- 2. Drop the old 768-dim column and re-add at 1536 dims.
ALTER TABLE chunks DROP COLUMN IF EXISTS embedding;
ALTER TABLE chunks ADD COLUMN embedding vector(1536);

-- 3. Update stored dimension metadata so memory_migrate_embedder's pre-flight
--    accepts the new model dimension.
UPDATE project_meta SET value = '1536' WHERE key = 'embedder_dimensions';

-- 4. HNSW index is rebuilt separately after migration (CREATE INDEX CONCURRENTLY
--    outside a transaction, run manually or via a post-migration step) to avoid
--    shared-memory constraints during container startup.
--    Run after server is healthy:
--    CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chunks_embedding_hnsw
--        ON chunks USING hnsw (embedding vector_cosine_ops)
--        WITH (m = 16, ef_construction = 64);
