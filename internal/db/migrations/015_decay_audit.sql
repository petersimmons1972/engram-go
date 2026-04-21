-- Decay audit system: canonical query registry and retrieval snapshots.
-- Used to monitor ranking drift over time as embedders and weights change.

CREATE TABLE audit_canonical_queries (
    id          TEXT PRIMARY KEY,
    project     TEXT NOT NULL,
    query       TEXT NOT NULL,
    description TEXT,
    active      BOOLEAN NOT NULL DEFAULT TRUE,
    alert_threshold DOUBLE PRECISION,  -- reserved for post-baseline alerting; NULL means no alerting
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_queries_project ON audit_canonical_queries(project);

CREATE TABLE audit_snapshots (
    id               TEXT PRIMARY KEY,
    query_id         TEXT NOT NULL REFERENCES audit_canonical_queries(id),
    project          TEXT NOT NULL,
    memory_ids       TEXT[] NOT NULL,           -- ordered list of returned memory IDs
    scores           DOUBLE PRECISION[] NOT NULL, -- parallel scores list
    embedding_model  TEXT NOT NULL,             -- e.g. 'nomic-embed-text'
    embedding_model_version TEXT,               -- optional model tag/digest
    run_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    rbo_vs_prev      DOUBLE PRECISION,          -- NULL for baseline snapshot
    jaccard_at_5     DOUBLE PRECISION,
    jaccard_at_10    DOUBLE PRECISION,
    jaccard_full     DOUBLE PRECISION,
    additions        TEXT[],                    -- IDs that appeared vs previous run
    removals         TEXT[]                     -- IDs that dropped vs previous run
);

CREATE INDEX idx_audit_snapshots_query_run ON audit_snapshots(query_id, run_at DESC);
CREATE INDEX idx_audit_snapshots_project_run ON audit_snapshots(project, run_at DESC);
