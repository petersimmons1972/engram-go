-- Resize the pgvector embedding column to match the deployment contract.
-- Drops the HNSW index, removes the old fixed-width column, re-adds it at the new dimension,
-- and recreates the index. All embeddings are set to NULL — memory_migrate_embedder will
-- re-embed them with the new model.
--
-- Target dimension: 1024 (the deployment contract documented in cmd/engram/main.go's
-- validateEmbedConfig: "The deployment contract is 1024-dim embeddings, regardless of
-- the specific model name. Model upgrades are operational changes as long as they
-- preserve that dimension.").
--
-- Production has been running at 1024 since the snowflake-arctic-embed2 / jina-v4
-- rollout. CI was previously misconfigured to expect 1536, which caused all
-- TEST_DATABASE_URL-gated tests in internal/consolidate to fail with
-- "expected 1536 dimensions, not 768". This migration brings the schema back in
-- line with both production and the application contract.

-- 1. Drop the HNSW index (required before column type change).
DROP INDEX IF EXISTS idx_chunks_embedding_hnsw;

-- 2. Drop the old fixed-width column and re-add at 1024 dims.
ALTER TABLE chunks DROP COLUMN IF EXISTS embedding;
ALTER TABLE chunks ADD COLUMN embedding vector(1024);

-- 3. Update stored dimension metadata so memory_migrate_embedder's pre-flight
--    accepts the contract dimension.
UPDATE project_meta SET value = '1024' WHERE key = 'embedder_dimensions';

-- 4. HNSW index is rebuilt separately after migration (CREATE INDEX CONCURRENTLY
--    outside a transaction, run manually or via a post-migration step) to avoid
--    shared-memory constraints during container startup.
--    Run after server is healthy:
--    CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chunks_embedding_hnsw
--        ON chunks USING hnsw (embedding vector_cosine_ops)
--        WITH (m = 16, ef_construction = 64);
