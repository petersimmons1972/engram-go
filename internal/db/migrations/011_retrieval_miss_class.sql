ALTER TABLE retrieval_events ADD COLUMN IF NOT EXISTS failure_class TEXT;

CREATE INDEX IF NOT EXISTS idx_retrieval_events_failure_class
    ON retrieval_events (project, failure_class)
    WHERE failure_class IS NOT NULL;
