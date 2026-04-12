-- Backfill content_hash for existing memories (#153)
--
-- The content_hash column was added in a previous migration and is populated
-- for all new writes. Memories stored before that change have NULL hashes,
-- which causes GetIntegrityStats to report 0% coverage.
--
-- convert_to(content, 'UTF8') returns raw UTF-8 bytes, matching Go's
-- sha256.Sum256([]byte(content)). content::bytea misinterprets backslashes
-- as bytea escape sequences and fails on content containing them.

UPDATE memories
SET content_hash = encode(sha256(convert_to(content, 'UTF8')), 'hex')
WHERE content_hash IS NULL
  AND valid_to IS NULL;
