package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/petersimmons1972/engram/internal/types"
)

// StoreRelationship upserts a directed relationship between two memories.
// Both existence checks and the upsert run inside a single transaction (#110)
// so a concurrent DeleteMemory between the check and the insert is prevented
// by the SELECT ... FOR UPDATE row locks.
func (b *PostgresBackend) StoreRelationship(ctx context.Context, rel *types.Relationship) error {
	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var dummy int
	if err := tx.QueryRow(ctx,
		"SELECT 1 FROM memories WHERE id=$1 AND project=$2 AND valid_to IS NULL FOR UPDATE",
		rel.SourceID, b.project,
	).Scan(&dummy); err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("source memory %q does not exist or is invalidated", rel.SourceID)
		}
		return fmt.Errorf("check source memory: %w", err)
	}
	if err := tx.QueryRow(ctx,
		"SELECT 1 FROM memories WHERE id=$1 AND project=$2 AND valid_to IS NULL FOR UPDATE",
		rel.TargetID, b.project,
	).Scan(&dummy); err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("target memory %q does not exist or is invalidated", rel.TargetID)
		}
		return fmt.Errorf("check target memory: %w", err)
	}

	rel.Project = b.project
	if _, err := tx.Exec(ctx, `
		INSERT INTO relationships
		  (id, source_id, target_id, rel_type, strength, project, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (source_id, target_id, rel_type)
		DO UPDATE SET strength = EXCLUDED.strength`,
		rel.ID, rel.SourceID, rel.TargetID,
		rel.RelType, rel.Strength, rel.Project, rel.CreatedAt,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// GetConnected performs BFS from memoryID via a single recursive CTE (#113).
// Returns all connected memories up to maxHops hops away, with the shortest-path
// metadata (rel_type, direction, strength) for each discovered node.
func (b *PostgresBackend) GetConnected(ctx context.Context, memoryID string, maxHops int) ([]ConnectedResult, error) {
	if maxHops <= 0 {
		return nil, nil
	}
	rows, err := b.pool.Query(ctx, `
WITH RECURSIVE bfs(neighbor_id, rel_type, direction, strength, hop, path) AS (
  -- Hop 1: direct outgoing neighbors of the seed node.
  SELECT target_id, rel_type, 'outgoing'::text, strength, 1,
         ARRAY[$1::text, target_id::text]
  FROM relationships
  WHERE source_id = $1 AND project = $2
  UNION ALL
  -- Hop 1: direct incoming neighbors of the seed node.
  SELECT source_id, rel_type, 'incoming'::text, strength, 1,
         ARRAY[$1::text, source_id::text]
  FROM relationships
  WHERE target_id = $1 AND project = $2
  UNION ALL
  -- Deeper hops: expand outgoing from current frontier, skip visited nodes.
  SELECT r.target_id, r.rel_type, 'outgoing'::text, r.strength, b.hop + 1,
         b.path || r.target_id::text
  FROM relationships r
  JOIN bfs b ON b.neighbor_id = r.source_id
  WHERE r.project = $2 AND b.hop < $3 AND NOT r.target_id = ANY(b.path)
  UNION ALL
  -- Deeper hops: expand incoming from current frontier, skip visited nodes.
  SELECT r.source_id, r.rel_type, 'incoming'::text, r.strength, b.hop + 1,
         b.path || r.source_id::text
  FROM relationships r
  JOIN bfs b ON b.neighbor_id = r.target_id
  WHERE r.project = $2 AND b.hop < $3 AND NOT r.source_id = ANY(b.path)
)
SELECT DISTINCT ON (bfs.neighbor_id)
  bfs.neighbor_id, bfs.rel_type, bfs.direction, bfs.strength,
  m.id, m.content, m.memory_type, m.project, m.tags, m.importance,
  m.access_count, m.last_accessed, m.created_at, m.updated_at,
  m.immutable, m.expires_at, m.summary, m.content_hash, m.storage_mode,
  m.search_vector, m.valid_from, m.valid_to, m.invalidation_reason,
  m.dynamic_importance, m.retrieval_interval_hrs, m.next_review_at,
  m.times_retrieved, m.times_useful, m.retrieval_precision, m.episode_id
FROM bfs
JOIN memories m ON m.id = bfs.neighbor_id AND m.valid_to IS NULL
ORDER BY bfs.neighbor_id, bfs.hop ASC`,
		memoryID, b.project, maxHops,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ConnectedResult
	for rows.Next() {
		var neighborID, relType, direction string
		var strength float64
		var m types.Memory
		var tagsJSON []byte
		var searchVector, invalidationReason *string
		var expiresAt, validTo, nextReviewAt *time.Time
		err := rows.Scan(
			&neighborID, &relType, &direction, &strength,
			&m.ID, &m.Content, &m.MemoryType, &m.Project, &tagsJSON, &m.Importance,
			&m.AccessCount, &m.LastAccessed, &m.CreatedAt, &m.UpdatedAt,
			&m.Immutable, &expiresAt, &m.Summary, &m.ContentHash, &m.StorageMode,
			&searchVector, &m.ValidFrom, &validTo, &invalidationReason,
			&m.DynamicImportance, &m.RetrievalIntervalHrs, &nextReviewAt,
			&m.TimesRetrieved, &m.TimesUseful, &m.RetrievalPrecision, &m.EpisodeID,
		)
		if err != nil {
			return nil, err
		}
		if tagsJSON != nil {
			_ = json.Unmarshal(tagsJSON, &m.Tags)
		}
		m.ExpiresAt = expiresAt
		m.ValidTo = validTo
		m.InvalidationReason = invalidationReason
		m.NextReviewAt = nextReviewAt
		results = append(results, ConnectedResult{
			Memory:    &m,
			RelType:   relType,
			Direction: direction,
			Strength:  strength,
		})
	}
	return results, rows.Err()
}

func (b *PostgresBackend) BoostEdgesForMemory(ctx context.Context, memoryID string, factor float64) (int, error) {
	tag, err := b.pool.Exec(ctx, `
		UPDATE relationships SET strength=LEAST(1.0, strength*$1)
		WHERE source_id=$2 OR target_id=$2`, factor, memoryID,
	)
	return int(tag.RowsAffected()), err
}

func (b *PostgresBackend) DecayEdgesForMemory(ctx context.Context, memoryID string, factor float64) (int, error) {
	tag, err := b.pool.Exec(ctx, `
		UPDATE relationships SET strength=GREATEST(0.0, strength-$1)
		WHERE source_id=$2 OR target_id=$2`, factor, memoryID,
	)
	return int(tag.RowsAffected()), err
}

func (b *PostgresBackend) GetConnectionCount(ctx context.Context, memoryID, project string) (int, error) {
	var count int
	err := b.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM relationships WHERE (source_id=$1 OR target_id=$1) AND project=$2",
		memoryID, project,
	).Scan(&count)
	return count, err
}

// DecayAllEdges decays all edges for a project and prunes those below minStrength.
// Both operations run inside a single transaction (#109) so a crash between them
// cannot leave partially-decayed but un-pruned "zombie" edges.
func (b *PostgresBackend) DecayAllEdges(ctx context.Context, project string, decayFactor, minStrength float64) (int, int, error) {
	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		UPDATE relationships SET strength=GREATEST(0.0, strength-$1)
		WHERE project=$2`, decayFactor, project,
	)
	if err != nil {
		return 0, 0, err
	}
	decayed := int(tag.RowsAffected())

	tag, err = tx.Exec(ctx,
		"DELETE FROM relationships WHERE strength<$1 AND project=$2",
		minStrength, project,
	)
	if err != nil {
		return decayed, 0, err
	}
	pruned := int(tag.RowsAffected())
	return decayed, pruned, tx.Commit(ctx)
}

func (b *PostgresBackend) DeleteRelationshipsForMemory(ctx context.Context, memoryID string) error {
	_, err := b.pool.Exec(ctx,
		"DELETE FROM relationships WHERE source_id=$1 OR target_id=$1", memoryID,
	)
	return err
}

func (b *PostgresBackend) GetRelationships(ctx context.Context, project, memoryID string) ([]types.Relationship, error) {
	rows, err := b.pool.Query(ctx, `
		SELECT id, source_id, target_id, rel_type, strength, project, created_at
		FROM relationships
		WHERE project = $1 AND (source_id = $2 OR target_id = $2)`,
		project, memoryID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rels []types.Relationship
	for rows.Next() {
		var r types.Relationship
		if err := rows.Scan(&r.ID, &r.SourceID, &r.TargetID, &r.RelType, &r.Strength, &r.Project, &r.CreatedAt); err != nil {
			return nil, err
		}
		rels = append(rels, r)
	}
	return rels, rows.Err()
}
