-- Manual rollback companion for issue #1020 ANN index options.
--
-- Drops either optional ANN index if present. This is safe to run even if only
-- one option was built.
--
-- Run with psql autocommit enabled. DROP INDEX CONCURRENTLY cannot run inside
-- an explicit transaction block.

DROP INDEX CONCURRENTLY IF EXISTS idx_chunks_embedding_hnsw;
DROP INDEX CONCURRENTLY IF EXISTS idx_chunks_embedding_ivfflat;

ANALYZE chunks;
