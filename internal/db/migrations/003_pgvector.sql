-- pgvector migration: BYTEA → vector(768) with HNSW index.
-- Multi-step for zero-downtime rollback safety.

-- 1. Ensure pgvector extension.
CREATE EXTENSION IF NOT EXISTS vector;

-- 2. Add new vector column alongside existing BYTEA.
ALTER TABLE chunks ADD COLUMN IF NOT EXISTS embedding_vec vector(768);

-- 3. Mark backfill pending — Go code handles BYTEA→vector conversion
-- because the BYTEA format is little-endian float32 (needs byte-swap).
INSERT INTO project_meta (project, key, value)
VALUES ('_engram', 'pgvector_backfill_pending', 'true')
ON CONFLICT (project, key) DO NOTHING;
