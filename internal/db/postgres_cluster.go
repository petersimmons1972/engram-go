package db

// postgres_cluster.go — PostgreSQL implementation of MemPalace cluster methods.
//
// Implements the Backend interface additions from LME experiment #9:
//   StoreMemoryCluster  — upsert a cluster centroid row
//   SetMemoryClusterID  — assign cluster_id on a memory
//   FindNearestClusters — top-K centroid lookup by cosine distance
//   VectorSearchWithClusters — VectorSearchWithDateRange filtered to cluster set
//   TableExists, ColumnExists — schema diagnostics used by tests and health probes

import (
	"context"
	"fmt"
	"time"

	"github.com/petersimmons1972/engram/internal/types"
	pgvector "github.com/pgvector/pgvector-go"
)

// StoreMemoryCluster upserts a cluster centroid row for the project.
// On conflict on (id) the centroid, label, size, and updated_at are updated.
func (b *PostgresBackend) StoreMemoryCluster(ctx context.Context, c *MemoryCluster) error {
	if c.ID == "" {
		c.ID = types.NewMemoryID()
	}
	_, err := b.pool.Exec(ctx, `
		INSERT INTO memory_clusters (id, project, centroid, label, size, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (id) DO UPDATE
		  SET centroid   = EXCLUDED.centroid,
		      label      = EXCLUDED.label,
		      size       = EXCLUDED.size,
		      updated_at = NOW()`,
		c.ID, c.Project, pgvector.NewVector(c.Centroid), c.Label, c.Size,
	)
	if err != nil {
		return fmt.Errorf("store memory cluster %q: %w", c.ID, err)
	}
	return nil
}

// SetMemoryClusterID sets memories.cluster_id = clusterID for memoryID.
// Pass clusterID="" to set NULL (un-assign from any cluster).
func (b *PostgresBackend) SetMemoryClusterID(ctx context.Context, memoryID, clusterID string) error {
	var err error
	if clusterID == "" {
		_, err = b.pool.Exec(ctx,
			`UPDATE memories SET cluster_id = NULL WHERE id = $1`, memoryID)
	} else {
		_, err = b.pool.Exec(ctx,
			`UPDATE memories SET cluster_id = $1 WHERE id = $2`, clusterID, memoryID)
	}
	if err != nil {
		return fmt.Errorf("set cluster_id on memory %q: %w", memoryID, err)
	}
	return nil
}

// FindNearestClusters returns the IDs of the top-K clusters whose centroids are
// closest to queryVec by cosine distance, scoped to project.
// Returns an empty slice (not an error) when no clusters exist.
func (b *PostgresBackend) FindNearestClusters(ctx context.Context, project string, queryVec []float32, topK int) ([]string, error) {
	if topK <= 0 {
		topK = 3
	}
	rows, err := b.pool.Query(ctx, `
		SELECT id
		FROM memory_clusters
		WHERE project = $1
		ORDER BY centroid <=> $2::vector
		LIMIT $3`,
		project, pgvector.NewVector(queryVec), topK,
	)
	if err != nil {
		return nil, fmt.Errorf("find nearest clusters: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan cluster id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// VectorSearchWithClusters is like VectorSearchWithDateRange but adds a
// WHERE m.cluster_id = ANY($clusterIDs) filter to restrict the candidate set
// to chunks belonging to the given clusters.
//
// When clusterIDs is empty the function falls back to the unconstrained search
// (equivalent to calling VectorSearchWithDateRange) so callers never get an
// empty result set purely because the cluster filter was empty.
func (b *PostgresBackend) VectorSearchWithClusters(ctx context.Context, project string, queryVec []float32, limit int, clusterIDs []string, since, before *time.Time) ([]VectorHit, error) {
	if len(clusterIDs) == 0 {
		return b.VectorSearchWithDateRange(ctx, project, queryVec, limit, since, before)
	}

	if limit > hnswDefaultEfSearch {
		tx, err := b.pool.Begin(ctx)
		if err != nil {
			return nil, fmt.Errorf("begin vector search tx (cluster): %w", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		efSearch := efSearchForLimit(limit)
		if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL hnsw.ef_search = %d", efSearch)); err != nil {
			return nil, fmt.Errorf("set hnsw.ef_search (cluster): %w", err)
		}
		rows, err := tx.Query(ctx, `
			SELECT c.id, c.memory_id,
			       c.embedding <=> $1::vector AS distance,
			       c.chunk_text, c.chunk_index, c.section_heading
			FROM chunks c
			JOIN memories m ON m.id = c.memory_id AND m.project = c.project
			WHERE c.project = $2 AND c.embedding IS NOT NULL
			  AND m.cluster_id = ANY($6::text[])
			  AND ($4::timestamptz IS NULL OR m.valid_from >= $4::timestamptz)
			  AND ($5::timestamptz IS NULL OR m.valid_from < $5::timestamptz)
			ORDER BY c.embedding <=> $1::vector
			LIMIT $3`,
			pgvector.NewVector(queryVec), project, limit, since, before, clusterIDs,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanVectorHits(rows)
	}

	rows, err := b.pool.Query(ctx, `
		SELECT c.id, c.memory_id,
		       c.embedding <=> $1::vector AS distance,
		       c.chunk_text, c.chunk_index, c.section_heading
		FROM chunks c
		JOIN memories m ON m.id = c.memory_id AND m.project = c.project
		WHERE c.project = $2 AND c.embedding IS NOT NULL
		  AND m.cluster_id = ANY($6::text[])
		  AND ($4::timestamptz IS NULL OR m.valid_from >= $4::timestamptz)
		  AND ($5::timestamptz IS NULL OR m.valid_from < $5::timestamptz)
		ORDER BY c.embedding <=> $1::vector
		LIMIT $3`,
		pgvector.NewVector(queryVec), project, limit, since, before, clusterIDs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanVectorHits(rows)
}

// TableExists returns true if the named table exists in the public schema.
func (b *PostgresBackend) TableExists(ctx context.Context, table string) (bool, error) {
	var exists bool
	err := b.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = $1
		)`, table,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check table exists %q: %w", table, err)
	}
	return exists, nil
}

// ColumnExists returns true if the named column exists on the given table in the public schema.
func (b *PostgresBackend) ColumnExists(ctx context.Context, table, column string) (bool, error) {
	var exists bool
	err := b.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name   = $1
			  AND column_name  = $2
		)`, table, column,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check column exists %q.%q: %w", table, column, err)
	}
	return exists, nil
}
