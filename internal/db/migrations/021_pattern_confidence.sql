-- Add pattern_confidence column for caller-provided float confidence.
-- See Phase 1 Track E1 of the instinct migration campaign.
--
-- Semantically distinct from:
--   importance        (INTEGER 0-4, static caller priority — never pruned through 4)
--   dynamic_importance (DOUBLE PRECISION, engine-learned spaced-repetition score)
--
-- pattern_confidence holds the consolidator's float belief [0.0, 1.0] that
-- a detected pattern is genuine. NULL means "unknown" — we do NOT default to
-- 0.0 because that would conflate "no confidence data" with "lowest confidence".
--
-- ALTER TABLE ADD COLUMN with a nullable column and no default is safe in
-- PostgreSQL: it requires no table rewrite and acquires only a brief ACCESS
-- EXCLUSIVE lock to update the catalog. Existing rows surface NULL.
--
-- Down-migration (if needed):
--   ALTER TABLE memories DROP COLUMN IF EXISTS pattern_confidence;
ALTER TABLE memories ADD COLUMN IF NOT EXISTS pattern_confidence DOUBLE PRECISION
    CONSTRAINT memories_pattern_confidence_range
    CHECK (pattern_confidence IS NULL OR (pattern_confidence >= 0.0 AND pattern_confidence <= 1.0));
