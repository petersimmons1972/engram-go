-- Retrieval Outcome Tracking (Feature 5 / Issue #86)
--
-- Every recall generates a retrieval_event row. When the caller provides
-- feedback, the event is updated with which results were actually useful.
-- Per-memory precision = times_useful / times_retrieved is recomputed on
-- each feedback call and used as a 5th composite scoring signal.

CREATE TABLE IF NOT EXISTS retrieval_events (
    id           TEXT PRIMARY KEY,
    project      TEXT NOT NULL,
    query        TEXT NOT NULL,
    result_ids   JSONB NOT NULL DEFAULT '[]',
    feedback_ids JSONB,
    created_at   TIMESTAMPTZ NOT NULL,
    feedback_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_retrieval_events_project
    ON retrieval_events (project, created_at DESC);

-- Three new columns on memories for precision tracking.
ALTER TABLE memories
    ADD COLUMN IF NOT EXISTS times_retrieved   INTEGER DEFAULT 0,
    ADD COLUMN IF NOT EXISTS times_useful      INTEGER DEFAULT 0,
    ADD COLUMN IF NOT EXISTS retrieval_precision DOUBLE PRECISION;
