-- Backfill content_hash for existing memories (#153)
--
-- The content_hash column was added in a previous migration and is populated
-- for all new writes. Memories stored before that change have NULL hashes,
-- which causes GetIntegrityStats to report 0% coverage.
--
-- Postgres's built-in sha256() function produces the same value as the Go
-- contentHash() helper in postgres_memory.go: encode(sha256(content::bytea),'hex').

UPDATE memories
SET content_hash = encode(sha256(content::bytea), 'hex')
WHERE content_hash IS NULL
  AND valid_to IS NULL;
