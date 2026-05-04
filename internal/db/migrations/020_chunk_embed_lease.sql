-- 020_chunk_embed_lease.sql — per-card lease columns for distributed reembed workers.
--
-- Each `engram-reembed` worker process now runs one async task per backend GPU,
-- pulling work independently from this table via a claim-lease pattern. These
-- columns let workers atomically claim a batch of pending chunks (FOR UPDATE
-- SKIP LOCKED) and stamp them with a time-bounded lease. If the worker dies
-- mid-flight, the lease expires and another worker reclaims the chunks.
--
-- Both DDL operations are metadata-only on Postgres (ADD COLUMN with no
-- DEFAULT, partial index over a small subset) and safe to run on the
-- multi-million row chunks table. Fully additive — rollback is a clean
-- DROP COLUMN.

ALTER TABLE chunks
    ADD COLUMN IF NOT EXISTS embed_lease_until TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS embed_lease_owner TEXT;

-- Partial index over the pending set. Postgres requires immutable predicates
-- in index expressions, so the time-based lease component lives in the query
-- not the index — at the chunk-table size we expect, the embed_lease_until
-- column is cheap to evaluate against the much smaller pending subset.
CREATE INDEX IF NOT EXISTS idx_chunks_embed_pending
    ON chunks (id, embed_lease_until)
    WHERE embedding IS NULL;
