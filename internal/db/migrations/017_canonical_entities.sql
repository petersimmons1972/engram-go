BEGIN;

CREATE TABLE IF NOT EXISTS canonical_entities (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    name        TEXT NOT NULL,
    aliases     TEXT[] NOT NULL DEFAULT '{}',
    project     TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_canonical_entities_project ON canonical_entities(project);
CREATE UNIQUE INDEX IF NOT EXISTS idx_canonical_entities_name ON canonical_entities(project, lower(name));

CREATE TABLE IF NOT EXISTS entity_extraction_jobs (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    memory_id   TEXT NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    project     TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','done','failed')),
    error       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ,
    CONSTRAINT uq_entity_jobs_pending UNIQUE (memory_id, project)
);

CREATE INDEX IF NOT EXISTS idx_entity_jobs_pending ON entity_extraction_jobs(project, created_at) WHERE status = 'pending';

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_canonical_entities_updated_at
    BEFORE UPDATE ON canonical_entities
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMIT;
