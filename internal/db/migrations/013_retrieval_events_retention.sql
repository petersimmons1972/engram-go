-- Migration 013: add index for age-based cleanup and retention function.
--
-- retrieval_events accumulates unboundedly with no TTL. This migration adds an
-- index on created_at to make the DELETE fast, and a stored function that the
-- application calls on a daily schedule to evict rows older than 90 days.

CREATE INDEX IF NOT EXISTS idx_retrieval_events_created_at
    ON retrieval_events (created_at);

CREATE OR REPLACE FUNCTION cleanup_old_retrieval_events() RETURNS void
LANGUAGE plpgsql AS $$
BEGIN
  DELETE FROM retrieval_events
  WHERE created_at < NOW() - INTERVAL '90 days';
END;
$$;
