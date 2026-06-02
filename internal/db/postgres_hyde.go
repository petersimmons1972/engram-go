package db

import (
	"context"
	"fmt"
	"time"

	pgvector "github.com/pgvector/pgvector-go"
)

// HydeEmbedding is a single HyDE (hypothetical document embedding) row.
type HydeEmbedding struct {
	MemoryID  string
	Project   string
	Embedding []float32
	Question  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// HydeVectorHit is a single result from a HyDE ANN search.
type HydeVectorHit struct {
	MemoryID string
	Distance float64 // cosine distance (0 = identical, 2 = opposite)
	Question string
}

// hydeEfSearchForLimit mirrors efSearchForLimit for the hyde index.
// The hyde index has fewer rows (one per memory), so a smaller cap is fine.
func hydeEfSearchForLimit(limit int) int {
	v := limit * 2
	if v > 500 {
		v = 500
	}
	return v
}

const hnswHydeDefaultEfSearch = 100

// UpsertHydeEmbedding inserts or updates the HyDE embedding for a memory.
// The question field is the generated hypothetical question (stored for audit).
func (b *PostgresBackend) UpsertHydeEmbedding(ctx context.Context, memoryID, project, question string, embedding []float32) error {
	now := time.Now().UTC()
	_, err := b.pool.Exec(ctx, `
		INSERT INTO hyde_embeddings (memory_id, project, embedding, question, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5)
		ON CONFLICT (memory_id) DO UPDATE
		  SET embedding  = EXCLUDED.embedding,
		      question   = EXCLUDED.question,
		      updated_at = EXCLUDED.updated_at`,
		memoryID, project, pgvector.NewVector(embedding), question, now,
	)
	if err != nil {
		return fmt.Errorf("upsert hyde_embedding for %s: %w", memoryID, err)
	}
	return nil
}

// DeleteHydeEmbedding removes the HyDE embedding for a memory.
// Returns true if a row was deleted, false if not found.
func (b *PostgresBackend) DeleteHydeEmbedding(ctx context.Context, memoryID string) (bool, error) {
	tag, err := b.pool.Exec(ctx,
		"DELETE FROM hyde_embeddings WHERE memory_id = $1", memoryID)
	if err != nil {
		return false, fmt.Errorf("delete hyde_embedding %s: %w", memoryID, err)
	}
	return tag.RowsAffected() > 0, nil
}

// HydeVectorSearch returns the nearest memories by cosine distance on the
// hyde_embeddings HNSW index. Returned distances are cosine distances (0–2).
func (b *PostgresBackend) HydeVectorSearch(ctx context.Context, project string, queryVec []float32, limit int) ([]HydeVectorHit, error) {
	if limit > hnswHydeDefaultEfSearch {
		tx, err := b.pool.Begin(ctx)
		if err != nil {
			return nil, fmt.Errorf("begin hyde vector search tx: %w", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		efSearch := hydeEfSearchForLimit(limit)
		if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL hnsw.ef_search = %d", efSearch)); err != nil {
			return nil, fmt.Errorf("set hnsw.ef_search for hyde: %w", err)
		}
		rows, err := tx.Query(ctx, `
			SELECT memory_id,
			       embedding <=> $1::vector AS distance,
			       COALESCE(question, '')
			FROM hyde_embeddings
			WHERE project = $2 AND embedding IS NOT NULL
			ORDER BY embedding <=> $1::vector
			LIMIT $3`,
			pgvector.NewVector(queryVec), project, limit,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanHydeHits(rows)
	}

	rows, err := b.pool.Query(ctx, `
		SELECT memory_id,
		       embedding <=> $1::vector AS distance,
		       COALESCE(question, '')
		FROM hyde_embeddings
		WHERE project = $2 AND embedding IS NOT NULL
		ORDER BY embedding <=> $1::vector
		LIMIT $3`,
		pgvector.NewVector(queryVec), project, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHydeHits(rows)
}

func scanHydeHits(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]HydeVectorHit, error) {
	var hits []HydeVectorHit
	for rows.Next() {
		var h HydeVectorHit
		if err := rows.Scan(&h.MemoryID, &h.Distance, &h.Question); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}
