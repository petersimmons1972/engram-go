-- Migration 026: MemPalace hierarchical clustering tables (LME experiment #9)
--
-- Adds a 2-level coarse→fine hierarchy for hierarchical recall filtering:
--   Level 1 (coarse): memory_clusters — one centroid vector per topic cluster per project
--   Level 2 (fine):   cluster_id column on memories — each memory belongs to one cluster
--
-- RECALL PATH (flag-gated, default OFF):
--   1. Embed query → find top-C nearest cluster centroids (memory_clusters)
--   2. Restrict VectorSearch to chunks whose parent memory.cluster_id IN (top-C clusters)
--   3. Standard composite scoring on the narrowed candidate set
--
-- ABLATION: set ENGRAM_MEMPALACE_HIERARCHICAL_RECALL=false (the default) to bypass
-- the hierarchical path entirely and run the existing flat VectorSearch. No data is
-- lost by disabling the flag — cluster_id is nullable and ignored by all existing queries.
--
-- BACK-FILL NOTE: existing memories have cluster_id = NULL. Hierarchical recall
-- with NULL cluster_id falls back to the flat path automatically (NULL IN (...) = FALSE).
-- Run the back-fill script (see docs/mempalace-backfill.md) AFTER enabling clustering
-- and running longmemeval ingest to populate clusters.
--
-- DOWN: DROP TABLE IF EXISTS memory_clusters; ALTER TABLE memories DROP COLUMN IF EXISTS cluster_id;

-- 1. Cluster centroid table.
--    centroid is VECTOR(1024) to match the deployment contract (migration 018).
CREATE TABLE IF NOT EXISTS memory_clusters (
    id          TEXT         PRIMARY KEY,
    project     TEXT         NOT NULL,
    centroid    vector(1024) NOT NULL,
    label       TEXT         NOT NULL DEFAULT '',
    size        INTEGER      NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_memory_clusters_project
    ON memory_clusters(project);

-- HNSW index on cluster centroids for fast nearest-centroid lookup.
-- ef_construction=64, m=16 matches the chunks HNSW config (migration 018).
CREATE INDEX IF NOT EXISTS idx_memory_clusters_centroid_hnsw
    ON memory_clusters USING hnsw (centroid vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- 2. Add cluster_id to memories (nullable; NULL = unassigned / flat path fallback).
ALTER TABLE memories ADD COLUMN IF NOT EXISTS cluster_id TEXT
    REFERENCES memory_clusters(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_memories_cluster_id
    ON memories(cluster_id)
    WHERE cluster_id IS NOT NULL;

-- Composite index: project + cluster_id for the hierarchical VectorSearch filter.
CREATE INDEX IF NOT EXISTS idx_memories_project_cluster
    ON memories(project, cluster_id)
    WHERE cluster_id IS NOT NULL;
