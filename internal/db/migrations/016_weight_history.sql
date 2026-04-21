-- Adaptive weight tuning: per-project weight config and tuning history.

-- WeightConfig holds one active weight set per project.
CREATE TABLE weight_config (
    project          TEXT PRIMARY KEY,
    weight_vector    DOUBLE PRECISION NOT NULL DEFAULT 0.45,
    weight_bm25      DOUBLE PRECISION NOT NULL DEFAULT 0.30,
    weight_recency   DOUBLE PRECISION NOT NULL DEFAULT 0.10,
    weight_precision DOUBLE PRECISION NOT NULL DEFAULT 0.15,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- WeightHistory documents what drove each adjustment.
CREATE TABLE weight_history (
    id               TEXT PRIMARY KEY,
    project          TEXT NOT NULL,
    applied_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    weight_vector    DOUBLE PRECISION NOT NULL,
    weight_bm25      DOUBLE PRECISION NOT NULL,
    weight_recency   DOUBLE PRECISION NOT NULL,
    weight_precision DOUBLE PRECISION NOT NULL,
    trigger_data     JSONB,
    notes            TEXT
);

CREATE INDEX idx_weight_history_project_applied ON weight_history(project, applied_at DESC);
