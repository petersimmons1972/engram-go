-- Performance indexes for critical query paths.
-- All are CONCURRENTLY-safe; no table lock held during build.
-- Added by get-well Phase 1.

CREATE INDEX IF NOT EXISTS idx_memories_project
    ON memories (project);

CREATE INDEX IF NOT EXISTS idx_chunks_memory_id
    ON chunks (memory_id);

CREATE INDEX IF NOT EXISTS idx_chunks_last_accessed
    ON chunks (last_accessed DESC);

CREATE INDEX IF NOT EXISTS idx_chunks_project
    ON chunks (project);

CREATE INDEX IF NOT EXISTS idx_relationships_source
    ON relationships (source_id);

CREATE INDEX IF NOT EXISTS idx_relationships_target
    ON relationships (target_id);
