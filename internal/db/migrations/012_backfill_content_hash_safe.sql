-- Migration 012: re-run content_hash backfill in safe batches.
--
-- Migration 009 already ran on most deployments and will find zero rows to
-- update. Large deployments that skipped 009 get this non-blocking catch-up
-- instead of the single unbounded UPDATE that could lock the full table for
-- minutes.
--
-- Uses FOR UPDATE SKIP LOCKED so live reader/writer traffic is not stalled,
-- and pg_sleep(0.01) between batches to yield scheduling time to other
-- transactions. The loop exits as soon as a batch returns 0 rows.

DO $$
DECLARE
  batch_size CONSTANT int := 1000;
  rows_updated int;
BEGIN
  LOOP
    UPDATE memories
    SET content_hash = encode(sha256(convert_to(content, 'UTF8')), 'hex')
    WHERE id IN (
      SELECT id FROM memories
      WHERE content_hash IS NULL AND valid_to IS NULL
      LIMIT batch_size
      FOR UPDATE SKIP LOCKED
    );
    GET DIAGNOSTICS rows_updated = ROW_COUNT;
    EXIT WHEN rows_updated = 0;
    PERFORM pg_sleep(0.01);
  END LOOP;
END $$;
