-- Engram Go schema v1 — clean sheet
-- Primary keys are UUID (stored as TEXT in 36-char dash format, UUID v7).
-- This schema is NOT compatible with the Python "engram v1" schema.
-- Use `engram migrate-from-v1` to convert an existing Python database.
--
-- Key differences from Python schema v9:
--   - memories.id  : UUID (36-char) instead of hex TEXT (32-char)
--   - project_meta : PRIMARY KEY (project, key) — per-project isolation
--   - chunks.project: denormalized from memories for faster per-project queries
--   - embeddings   : BYTEA (pgvector can be layered on top later)

CREATE TABLE IF NOT EXISTS memories (
    id            TEXT        PRIMARY KEY,
    content       TEXT        NOT NULL,
    memory_type   TEXT        NOT NULL DEFAULT 'context',
    project       TEXT        NOT NULL DEFAULT 'default',
    tags          JSONB       NOT NULL DEFAULT '[]'::jsonb,
    importance    INTEGER     NOT NULL DEFAULT 2,
    access_count  INTEGER     NOT NULL DEFAULT 0,
    last_accessed TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL,
    immutable     BOOLEAN     NOT NULL DEFAULT FALSE,
    expires_at    TIMESTAMPTZ,
    summary       TEXT,
    content_hash  TEXT,
    storage_mode  TEXT        NOT NULL DEFAULT 'focused',
    search_vector TSVECTOR GENERATED ALWAYS AS (
        to_tsvector('english', content)
    ) STORED
);

CREATE INDEX IF NOT EXISTS idx_memories_search       ON memories USING GIN (search_vector);
CREATE INDEX IF NOT EXISTS idx_memories_project      ON memories(project);
CREATE INDEX IF NOT EXISTS idx_memories_last_accessed ON memories(last_accessed);
CREATE INDEX IF NOT EXISTS idx_memories_updated_at   ON memories(updated_at);
CREATE INDEX IF NOT EXISTS idx_memories_project_type ON memories(project, memory_type);
CREATE INDEX IF NOT EXISTS idx_memories_summary_null ON memories(id) WHERE summary IS NULL;
CREATE INDEX IF NOT EXISTS idx_memories_content_hash ON memories(content_hash) WHERE content_hash IS NOT NULL;

CREATE TABLE IF NOT EXISTS chunks (
    id              TEXT        PRIMARY KEY,
    memory_id       TEXT        NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    project         TEXT        NOT NULL DEFAULT '',
    chunk_text      TEXT        NOT NULL,
    chunk_index     INTEGER     NOT NULL,
    chunk_hash      TEXT        NOT NULL DEFAULT '',
    embedding       BYTEA,
    section_heading TEXT,
    chunk_type      TEXT        NOT NULL DEFAULT 'sentence_window',
    last_matched    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_chunks_memory  ON chunks(memory_id);
CREATE INDEX IF NOT EXISTS idx_chunks_hash    ON chunks(chunk_hash);
CREATE INDEX IF NOT EXISTS idx_chunks_project ON chunks(project);

CREATE TABLE IF NOT EXISTS relationships (
    id         TEXT             PRIMARY KEY,
    source_id  TEXT             NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    target_id  TEXT             NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    rel_type   TEXT             NOT NULL DEFAULT 'relates_to',
    strength   DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    project    TEXT             NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ      NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_rel_source  ON relationships(source_id);
CREATE INDEX IF NOT EXISTS idx_rel_target  ON relationships(target_id);
CREATE INDEX IF NOT EXISTS idx_rel_project ON relationships(project);
CREATE UNIQUE INDEX IF NOT EXISTS idx_rel_pair ON relationships(source_id, target_id, rel_type);

-- project_meta: per-project key/value store.
-- PRIMARY KEY (project, key) prevents cross-project collision (Python schema bug fix).
CREATE TABLE IF NOT EXISTS project_meta (
    project TEXT NOT NULL,
    key     TEXT NOT NULL,
    value   TEXT NOT NULL,
    PRIMARY KEY (project, key)
);

-- Record schema version for this database.
INSERT INTO project_meta (project, key, value)
VALUES ('_engram', 'go_schema_version', '1')
ON CONFLICT (project, key) DO NOTHING;
