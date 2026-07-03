-- 029_layerb.sql — Additive Layer B deterministic aggregation cache.
--
-- Two tables:
--   layer_b_atoms   — exact provenance spans extracted deterministically from recalled memories
--   layer_b_events  — count-style aggregation events derived from those spans
--
-- Design goals:
--   1. Additive only: no changes to existing memories/chunks/retrieval tables.
--   2. Provenance-preserving: exact char span and verbatim span text round-trip.
--   3. Idempotent re-ingest: unique constraints make repeated indexing a no-op/update.
--   4. No orphans: ON DELETE CASCADE from memories.

BEGIN;

CREATE TABLE IF NOT EXISTS layer_b_atoms (
    id                  TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    project             TEXT NOT NULL,
    provenance_memory_id TEXT NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    provenance_span     TEXT NOT NULL,
    span_text           TEXT NOT NULL,
    statement           TEXT NOT NULL,
    normalized_text     TEXT NOT NULL,
    event_time          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_layer_b_atoms UNIQUE (
        project, provenance_memory_id, provenance_span, normalized_text
    )
);

CREATE INDEX IF NOT EXISTS idx_layer_b_atoms_project_memory
    ON layer_b_atoms (project, provenance_memory_id);

CREATE TABLE IF NOT EXISTS layer_b_events (
    id                  TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    project             TEXT NOT NULL,
    provenance_memory_id TEXT NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    provenance_span     TEXT NOT NULL,
    span_text           TEXT NOT NULL,
    anchor              TEXT NOT NULL,
    normalized_text     TEXT NOT NULL,
    event_time          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_layer_b_events UNIQUE (
        project, provenance_memory_id, provenance_span, anchor
    )
);

CREATE INDEX IF NOT EXISTS idx_layer_b_events_project_memory
    ON layer_b_events (project, provenance_memory_id, event_time);

COMMIT;
