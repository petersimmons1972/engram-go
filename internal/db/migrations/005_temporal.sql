-- Temporal Knowledge Versioning (Feature 1 / Issue #82)
--
-- Adds a bi-temporal layer to the memory system:
--   memories.valid_to       -- NULL while memory is active; set on soft-delete
--   memories.valid_from     -- optional start of validity window
--   memories.invalidation_reason -- why the memory was soft-deleted
--
--   memory_versions         -- full audit trail; a row is inserted before every
--                              update and on every soft-delete

CREATE TABLE IF NOT EXISTS memory_versions (
    id                  TEXT PRIMARY KEY,
    memory_id           TEXT NOT NULL,
    content             TEXT NOT NULL,
    memory_type         TEXT NOT NULL,
    tags                JSONB DEFAULT '[]',
    importance          INTEGER NOT NULL,
    system_from         TIMESTAMPTZ NOT NULL,
    system_to           TIMESTAMPTZ,          -- when this version was superseded
    valid_from          TIMESTAMPTZ,
    valid_to            TIMESTAMPTZ,
    change_type         TEXT NOT NULL DEFAULT 'create',  -- create|update|invalidate
    change_reason       TEXT,
    project             TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_memory_versions_memory_id
    ON memory_versions (memory_id);
CREATE INDEX IF NOT EXISTS idx_memory_versions_project_memory
    ON memory_versions (project, memory_id);

-- Extend the memories table with temporal columns.
ALTER TABLE memories
    ADD COLUMN IF NOT EXISTS valid_from          TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS valid_to            TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS invalidation_reason TEXT;

-- Partial index: active-memory fast-path queries hit this.
CREATE INDEX IF NOT EXISTS idx_memories_active
    ON memories (project, updated_at DESC)
    WHERE valid_to IS NULL;
