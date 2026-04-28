package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	pgvector "github.com/pgvector/pgvector-go"
	"github.com/petersimmons1972/engram/internal/types"
)

// chunkCols is an explicit column list for chunks SELECTs. Using SELECT c.*
// breaks positional scanning when migrations reorder columns (e.g. after
// 003_pgvector.sql drops the old BYTEA embedding and adds vector(768)).
const chunkCols = "c.id, c.memory_id, c.project, c.chunk_text, c.chunk_index, c.chunk_hash, c.embedding, c.section_heading, c.chunk_type, c.last_matched"

func (b *PostgresBackend) StoreChunks(ctx context.Context, chunks []*types.Chunk) error {
	return b.storeChunksExec(ctx, b.pool, chunks)
}

func (b *PostgresBackend) StoreChunksTx(ctx context.Context, tx Tx, chunks []*types.Chunk) error {
	raw, err := unwrapTx(tx)
	if err != nil {
		return err
	}
	return b.storeChunksExec(ctx, raw, chunks)
}

func (b *PostgresBackend) storeChunksExec(ctx context.Context, ex execer, chunks []*types.Chunk) error {
	if len(chunks) == 0 {
		return nil
	}
	const chunkSQL = `
		INSERT INTO chunks (id, memory_id, project, chunk_text, chunk_index,
		                    chunk_hash, embedding, section_heading, chunk_type)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (id) DO NOTHING`
	for _, c := range chunks {
		var embParam any
		if len(c.Embedding) > 0 {
			embParam = pgvector.NewVector(c.Embedding)
		}
		_, err := ex.Exec(ctx, chunkSQL,
			c.ID, c.MemoryID, c.Project,
			c.ChunkText, c.ChunkIndex, c.ChunkHash,
			embParam, c.SectionHeading, c.ChunkType,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *PostgresBackend) GetChunksForMemory(ctx context.Context, memoryID string) ([]*types.Chunk, error) {
	rows, err := b.pool.Query(ctx,
		"SELECT "+chunkCols+" FROM chunks c WHERE c.memory_id=$1 ORDER BY c.chunk_index", memoryID,
	)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, rowToChunk)
}

func (b *PostgresBackend) GetAllChunksWithEmbeddings(ctx context.Context, project string, limit int) ([]*types.Chunk, error) {
	rows, err := b.pool.Query(ctx, `
		SELECT `+chunkCols+` FROM chunks c
		JOIN memories m ON m.id = c.memory_id
		WHERE c.embedding IS NOT NULL AND m.project=$1 AND m.valid_to IS NULL
		ORDER BY m.last_accessed DESC
		LIMIT $2`, project, limit,
	)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, rowToChunk)
}

func (b *PostgresBackend) GetAllChunkTexts(ctx context.Context, project string, limit int) ([]string, error) {
	rows, err := b.pool.Query(ctx, `
		SELECT c.chunk_text FROM chunks c
		JOIN memories m ON m.id = c.memory_id
		WHERE m.project=$1 AND m.valid_to IS NULL LIMIT $2`, project, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var texts []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		texts = append(texts, t)
	}
	return texts, rows.Err()
}

func (b *PostgresBackend) GetChunksForMemories(ctx context.Context, memoryIDs []string) ([]*types.Chunk, error) {
	if len(memoryIDs) == 0 {
		return nil, nil
	}
	rows, err := b.pool.Query(ctx, `
		SELECT `+chunkCols+` FROM chunks c
		WHERE c.memory_id = ANY($1) AND c.embedding IS NOT NULL
		ORDER BY c.chunk_index`, memoryIDs,
	)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, rowToChunk)
}

func (b *PostgresBackend) ChunkHashExists(ctx context.Context, chunkHash, _ string) (bool, error) {
	var exists bool
	err := b.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM chunks c
			JOIN memories m ON m.id = c.memory_id
			WHERE c.chunk_hash=$1 AND m.project=$2
		)`, chunkHash, b.project,
	).Scan(&exists)
	return exists, err
}

func (b *PostgresBackend) DeleteChunksForMemory(ctx context.Context, memoryID string) error {
	_, err := b.pool.Exec(ctx, "DELETE FROM chunks WHERE memory_id=$1", memoryID)
	return err
}

func (b *PostgresBackend) DeleteChunksForMemoryTx(ctx context.Context, tx Tx, memoryID string) error {
	raw, err := unwrapTx(tx)
	if err != nil {
		return err
	}
	_, err = raw.Exec(ctx, "DELETE FROM chunks WHERE memory_id=$1", memoryID)
	return err
}

func (b *PostgresBackend) DeleteChunksByIDs(ctx context.Context, chunkIDs []string) (int, error) {
	if len(chunkIDs) == 0 {
		return 0, nil
	}
	tag, err := b.pool.Exec(ctx, "DELETE FROM chunks WHERE id=ANY($1)", chunkIDs)
	return int(tag.RowsAffected()), err
}

func (b *PostgresBackend) NullAllEmbeddings(ctx context.Context, project string) (int, error) {
	tag, err := b.pool.Exec(ctx,
		"UPDATE chunks SET embedding=NULL WHERE memory_id IN (SELECT id FROM memories WHERE project=$1)",
		project,
	)
	return int(tag.RowsAffected()), err
}

func (b *PostgresBackend) NullAllEmbeddingsTx(ctx context.Context, tx Tx, project string) (int, error) {
	raw, err := unwrapTx(tx)
	if err != nil {
		return 0, err
	}
	tag, err := raw.Exec(ctx,
		"UPDATE chunks SET embedding=NULL WHERE memory_id IN (SELECT id FROM memories WHERE project=$1)",
		project,
	)
	return int(tag.RowsAffected()), err
}

func (b *PostgresBackend) GetChunksPendingEmbedding(ctx context.Context, project string, limit int) ([]*types.Chunk, error) {
	rows, err := b.pool.Query(ctx, `
		SELECT `+chunkCols+` FROM chunks c
		JOIN memories m ON m.id = c.memory_id
		WHERE m.project=$1 AND c.embedding IS NULL
		ORDER BY m.last_accessed DESC
		LIMIT $2`, project, limit,
	)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, rowToChunk)
}

func (b *PostgresBackend) UpdateChunkEmbedding(ctx context.Context, chunkID string, embedding []float32) (int, error) {
	tag, err := b.pool.Exec(ctx,
		"UPDATE chunks SET embedding=$1 WHERE id=$2", pgvector.NewVector(embedding), chunkID,
	)
	return int(tag.RowsAffected()), err
}

// hnswDefaultEfSearch is the HNSW index default ef_search (query-time candidate
// set size). When callers request more candidates than this threshold, we tune
// ef_search upward so the index can actually return the requested number of rows
// rather than silently truncating.
const hnswDefaultEfSearch = 64

// efSearchForLimit returns the ef_search value to set for a given query limit.
// It doubles the limit to overscan, capped at 1000 to prevent pathological
// HNSW scans on very large limits (#370).
func efSearchForLimit(limit int) int {
	v := limit * 2
	if v > 1000 {
		v = 1000
	}
	return v
}

func (b *PostgresBackend) VectorSearch(ctx context.Context, project string, queryVec []float32, limit int) ([]VectorHit, error) {
	// When the requested limit exceeds the HNSW default ef_search, the index
	// silently returns fewer rows than requested. Wrap in a transaction so that
	// SET LOCAL hnsw.ef_search is scoped to this query only and does not bleed
	// into other queries on the same pooled connection (#360).
	if limit > hnswDefaultEfSearch {
		tx, err := b.pool.Begin(ctx)
		if err != nil {
			return nil, fmt.Errorf("begin vector search tx: %w", err)
		}
		defer func() { _ = tx.Rollback(ctx) }() // read-only — rollback == commit here
		efSearch := efSearchForLimit(limit)
		if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL hnsw.ef_search = %d", efSearch)); err != nil {
			return nil, fmt.Errorf("set hnsw.ef_search: %w", err)
		}
		rows, err := tx.Query(ctx, `
			SELECT c.id, c.memory_id,
			       c.embedding <=> $1::vector AS distance,
			       c.chunk_text, c.chunk_index, c.section_heading
			FROM chunks c
			WHERE c.project = $2 AND c.embedding IS NOT NULL
			ORDER BY c.embedding <=> $1::vector
			LIMIT $3`,
			pgvector.NewVector(queryVec), project, limit,
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
		WHERE c.project = $2 AND c.embedding IS NOT NULL
		ORDER BY c.embedding <=> $1::vector
		LIMIT $3`,
		pgvector.NewVector(queryVec), project, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanVectorHits(rows)
}

func scanVectorHits(rows pgx.Rows) ([]VectorHit, error) {
	var hits []VectorHit
	for rows.Next() {
		var h VectorHit
		if err := rows.Scan(&h.ChunkID, &h.MemoryID, &h.Distance,
			&h.ChunkText, &h.ChunkIndex, &h.SectionHeading); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

// SearchChunksWithinMemory returns the nearest chunks by cosine distance to
// queryVec, scoped to a single memory. Used by A5 memory_query_document's
// semantic path to narrow vector search to one document's chunks.
func (b *PostgresBackend) SearchChunksWithinMemory(ctx context.Context, embedding []float32, memoryID string, topK int) ([]*types.Chunk, error) {
	if topK <= 0 {
		topK = 10
	}
	rows, err := b.pool.Query(ctx, `
		SELECT `+chunkCols+` FROM chunks c
		WHERE c.memory_id = $2 AND c.embedding IS NOT NULL
		ORDER BY c.embedding <=> $1::vector
		LIMIT $3`,
		pgvector.NewVector(embedding), memoryID, topK,
	)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, rowToChunk)
}

// ChunkEmbeddingDistance returns the minimum cosine distance between any chunk of
// memAID and any chunk of memBID. Uses a LATERAL join so the HNSW index on
// chunks.embedding is used for each probe — O(N·log M) instead of O(N×M) (#114).
func (b *PostgresBackend) ChunkEmbeddingDistance(ctx context.Context, memAID, memBID string) (float64, error) {
	var dist *float64
	err := b.pool.QueryRow(ctx, `
		SELECT MIN(sq.dist)
		FROM chunks ca
		JOIN LATERAL (
			SELECT ca.embedding <=> cb.embedding AS dist
			FROM chunks cb
			WHERE cb.memory_id = $2 AND cb.embedding IS NOT NULL
			ORDER BY dist ASC
			LIMIT 1
		) sq ON true
		WHERE ca.memory_id = $1 AND ca.embedding IS NOT NULL`,
		memAID, memBID,
	).Scan(&dist)
	if err != nil {
		return 2.0, err
	}
	if dist == nil {
		return 2.0, nil // no embedded chunks
	}
	return *dist, nil
}

func (b *PostgresBackend) UpdateChunkLastMatched(ctx context.Context, chunkID string) error {
	_, err := b.pool.Exec(ctx,
		"UPDATE chunks SET last_matched=NOW() WHERE id=$1", chunkID,
	)
	return err
}

func (b *PostgresBackend) GetPendingEmbeddingCount(ctx context.Context, project string) (int, error) {
	var count int
	err := b.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM chunks c
		JOIN memories m ON m.id = c.memory_id
		WHERE m.project=$1 AND m.valid_to IS NULL AND c.embedding IS NULL`, project,
	).Scan(&count)
	return count, err
}


