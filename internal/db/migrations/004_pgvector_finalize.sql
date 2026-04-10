-- Finalize pgvector migration. Only runs after Go backfill is complete
-- (pgvector_backfill_pending = 'false' in project_meta).
-- The runMigrations code checks this before applying this file.

-- 4. Drop old BYTEA column.
ALTER TABLE chunks DROP COLUMN IF EXISTS embedding;

-- 5. Rename vector column to canonical name.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_name='chunks' AND column_name='embedding_vec') THEN
        ALTER TABLE chunks RENAME COLUMN embedding_vec TO embedding;
    END IF;
END $$;

-- 6. Create HNSW index for cosine distance.
CREATE INDEX IF NOT EXISTS idx_chunks_embedding_hnsw
    ON chunks USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);
