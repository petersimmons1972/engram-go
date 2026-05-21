-- Migration 022: project TTL metadata (created_at, expires_at) — #754
--
-- Projects are not a first-class table in engram-go; they are derived from
-- DISTINCT values of memories.project. TTL metadata is therefore stored in
-- project_meta (the existing per-project key/value store) rather than via a
-- new projects table.
--
-- This migration adds a dedicated table for project-level TTL so we can:
--   1. Index expires_at for efficient range queries without a full table scan
--      of project_meta.
--   2. Store created_at alongside expires_at in a single, typed row.
--
-- NULL expires_at means "durable" (no expiry). Non-NULL means ephemeral.
-- lme-* projects are stamped at creation time by the ingest stage.
--
-- This table is additive — no existing data is affected.
-- Down-migration: DROP TABLE IF EXISTS project_ttl;

CREATE TABLE IF NOT EXISTS project_ttl (
    project    TEXT        PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_project_ttl_expires_at
    ON project_ttl(expires_at)
    WHERE expires_at IS NOT NULL;

-- Existing lme-* projects (if any) get NULL expires_at on INSERT conflict —
-- they are treated as durable until an operator runs the optional backfill:
--
--   UPDATE project_ttl
--      SET expires_at = created_at + INTERVAL '7 days'
--    WHERE project LIKE 'lme-%' AND expires_at IS NULL;
--
-- See docs/lme-benchmark-learnings.md §Operator: scratch retention for the
-- full backfill procedure.
