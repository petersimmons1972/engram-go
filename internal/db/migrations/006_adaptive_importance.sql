-- Adaptive Importance via Spaced Repetition (Feature 2 / Issue #83)
--
-- Adds three columns to memories:
--   dynamic_importance      -- learned score; replaces static importance in composite scoring
--   retrieval_interval_hrs  -- spaced-repetition interval in hours (grows on positive feedback)
--   next_review_at          -- when the memory should next be retrieved to avoid decay

ALTER TABLE memories
    ADD COLUMN IF NOT EXISTS dynamic_importance     DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS retrieval_interval_hrs DOUBLE PRECISION DEFAULT 168,
    ADD COLUMN IF NOT EXISTS next_review_at         TIMESTAMPTZ;

-- Backfill: map static importance [0,4] → dynamic_importance using the same
-- formula as ImportanceBoost so existing memories start at their current rank.
UPDATE memories
SET dynamic_importance = GREATEST(0.1, (5.0 - importance) / 3.0)
WHERE dynamic_importance IS NULL;

-- Index for the decay worker: find stale memories efficiently.
CREATE INDEX IF NOT EXISTS idx_memories_stale_review
    ON memories (project, next_review_at)
    WHERE next_review_at IS NOT NULL AND valid_to IS NULL;
