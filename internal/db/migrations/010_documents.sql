-- 010_documents.sql — raw document storage for Tier-2 ingestion (A4).
-- Documents > 8 MB are stored here in full; the parent memory holds a
-- synopsis and a foreign key reference.

CREATE TABLE IF NOT EXISTS documents (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project     TEXT NOT NULL,
    content     TEXT NOT NULL,
    sha256      TEXT NOT NULL,
    size_bytes  INTEGER NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS documents_project_idx ON documents(project);

ALTER TABLE memories
    ADD COLUMN IF NOT EXISTS document_id UUID REFERENCES documents(id) ON DELETE SET NULL;
