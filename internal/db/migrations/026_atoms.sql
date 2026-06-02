-- 026_atoms.sql — Atom extraction layer (Milestone 1: preference atoms)
--
-- Adds three tables that form the atom extraction pipeline:
--   atoms                   — the stored typed atoms (subject/predicate/value)
--   atom_embeddings         — vector index for statement-level recall
--   atom_extraction_jobs    — async work queue (mirrors entity_extraction_jobs)
--
-- Design mirrors migrations 017 (canonical_entities) and 005 (temporal) with
-- the bi-temporal contract (valid_from/valid_to) from migration 005.
-- Idempotent: all DDL uses IF NOT EXISTS / IF EXISTS guards.

BEGIN;

-- ── atoms ────────────────────────────────────────────────────────────────────
-- Each row is one extracted belief/fact/preference.
-- "statement" is the canonical NL sentence that gets embedded.
-- Bi-temporal columns (valid_from/valid_to) mirror the memories table (005).

CREATE TABLE IF NOT EXISTS atoms (
    id                    TEXT PRIMARY KEY,
    project               TEXT NOT NULL,
    atom_type             TEXT NOT NULL CHECK (atom_type IN (
                              'preference','fact','event','attribute','relationship'
                          )),
    subject               TEXT NOT NULL,
    predicate             TEXT NOT NULL,
    value                 TEXT NOT NULL,
    statement             TEXT NOT NULL,  -- canonical NL sentence; this gets embedded
    scope                 TEXT NOT NULL DEFAULT 'global'
                              CHECK (scope = 'global'
                                  OR scope LIKE 'session:%'
                                  OR scope LIKE 'entity:%'),
    valid_from            TIMESTAMPTZ,
    valid_to              TIMESTAMPTZ,    -- NULL while atom is active
    confidence            FLOAT NOT NULL DEFAULT 1.0
                              CHECK (confidence >= 0 AND confidence <= 1),
    provenance_memory_id  TEXT REFERENCES memories(id) ON DELETE SET NULL,
    provenance_span       TEXT,           -- e.g. "chars:120-180"
    supersedes            TEXT REFERENCES atoms(id) ON DELETE SET NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_atoms_project_type
    ON atoms (project, atom_type)
    WHERE valid_to IS NULL;

CREATE INDEX IF NOT EXISTS idx_atoms_subject_predicate
    ON atoms (project, subject, predicate)
    WHERE valid_to IS NULL;

CREATE INDEX IF NOT EXISTS idx_atoms_provenance_memory
    ON atoms (provenance_memory_id)
    WHERE provenance_memory_id IS NOT NULL;

-- Partial index for active atoms (bi-temporal fast-path, mirrors idx_memories_active).
CREATE INDEX IF NOT EXISTS idx_atoms_active
    ON atoms (project, created_at DESC)
    WHERE valid_to IS NULL;

-- ── atom_embeddings ───────────────────────────────────────────────────────────
-- One embedding per atom (statement text → vector).
-- Stored as a separate table to mirror how chunks/chunk_embeddings work, keeping
-- the atoms table lean and the embedding column alterations isolated.

CREATE TABLE IF NOT EXISTS atom_embeddings (
    atom_id      TEXT PRIMARY KEY REFERENCES atoms(id) ON DELETE CASCADE,
    embedding    vector(1024),
    embedder     TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- HNSW index on atom statement embeddings (cosine distance).
-- Uses IF NOT EXISTS guard so re-running the migration is safe.
-- The new-table case does not require CONCURRENTLY (no live traffic to block).
CREATE INDEX IF NOT EXISTS idx_atom_embeddings_hnsw
    ON atom_embeddings USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- ── atom_extraction_jobs ──────────────────────────────────────────────────────
-- Async work queue. Mirrors entity_extraction_jobs (017) exactly.

CREATE TABLE IF NOT EXISTS atom_extraction_jobs (
    id           TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    memory_id    TEXT NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    project      TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending','processing','done','failed')),
    error        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ,
    CONSTRAINT uq_atom_jobs_pending UNIQUE (memory_id, project)
);

CREATE INDEX IF NOT EXISTS idx_atom_jobs_pending
    ON atom_extraction_jobs (project, created_at)
    WHERE status = 'pending';

COMMIT;
