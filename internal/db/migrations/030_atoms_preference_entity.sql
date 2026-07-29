-- 030_atoms_preference_entity.sql — Preference-atom entity capture (issue #1181)
--
-- Adds polarity, entity, and domain columns to the atoms table so that
-- preference atoms extracted at ingest time carry verbatim named items
-- (e.g. "dark chocolate", "cilantro") rather than requiring the generator
-- to synthesise them from the statement alone. This is the structural fix
-- for FM-PG (#1183): the generator confabulates specific entities absent
-- from retrieved context; recording the verbatim entity at extraction time
-- removes that dependency.
--
-- All three columns are nullable TEXT so the migration is fully additive
-- and backward-compatible with existing atoms (which will have NULL values
-- read back as empty strings by the application layer).
--
-- Idempotent: ADD COLUMN IF NOT EXISTS guards re-runs.

BEGIN;

ALTER TABLE atoms
    ADD COLUMN IF NOT EXISTS polarity TEXT
        CHECK (polarity IS NULL OR polarity IN ('like', 'dislike', ''));

ALTER TABLE atoms
    ADD COLUMN IF NOT EXISTS entity TEXT;

ALTER TABLE atoms
    ADD COLUMN IF NOT EXISTS domain TEXT;

-- Partial index to make entity-filtered preference lookups efficient.
-- Covers the common case: find all atoms where a named entity is recorded.
CREATE INDEX IF NOT EXISTS idx_atoms_entity_active
    ON atoms (project, entity)
    WHERE valid_to IS NULL AND entity IS NOT NULL AND entity <> '';

COMMIT;
