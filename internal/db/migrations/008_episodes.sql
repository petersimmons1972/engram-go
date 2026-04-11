-- Episodic Memory / Session Context Binding (Feature 6)
--
-- Every SSE connection can be associated with an episode. Memories stored
-- during that connection carry the episode_id for later recall. Episodes can
-- also be named and summarised explicitly via the MCP tools.

CREATE TABLE IF NOT EXISTS episodes (
    id          TEXT        PRIMARY KEY,
    project     TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    started_at  TIMESTAMPTZ NOT NULL,
    ended_at    TIMESTAMPTZ,
    summary     TEXT        NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_episodes_project ON episodes (project, started_at DESC);

ALTER TABLE memories
    ADD COLUMN IF NOT EXISTS episode_id TEXT REFERENCES episodes(id);

CREATE INDEX IF NOT EXISTS idx_memories_episode ON memories (episode_id) WHERE episode_id IS NOT NULL;
