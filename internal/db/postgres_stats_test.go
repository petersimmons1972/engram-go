package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

func TestGetStats_ReportsEmbeddingBacklog(t *testing.T) {
	ctx := context.Background()
	project := uniqueProject("stats-embedding-backlog")
	dsn := testDSN(t)

	backend := newTestBackend(t, project)
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	mem := storeMemory(t, backend, project, "embedding backlog test")
	err = backend.StoreChunks(ctx, []*types.Chunk{
		{
			ID:        types.NewMemoryID(),
			MemoryID:  mem.ID,
			ChunkText: "chunk 1",
			ChunkHash: "chunk-hash-1",
			Project:   project,
			ChunkType: "sentence_window",
			Embedding: nil,
		},
		{
			ID:        types.NewMemoryID(),
			MemoryID:  mem.ID,
			ChunkText: "chunk 2",
			ChunkHash: "chunk-hash-2",
			Project:   project,
			ChunkType: "sentence_window",
			Embedding: nil,
		},
		{
			ID:        types.NewMemoryID(),
			MemoryID:  mem.ID,
			ChunkText: "chunk 3",
			ChunkHash: "chunk-hash-3",
			Project:   project,
			ChunkType: "sentence_window",
			Embedding: nil,
		},
	})
	require.NoError(t, err)

	stats, err := backend.GetStats(ctx, project)
	require.NoError(t, err)
	require.Equal(t, 3, stats.TotalChunks)
	require.Equal(t, 3, stats.ChunksTotal)
	require.Equal(t, 0, stats.ChunksEmbedded)
	require.Equal(t, 3, stats.ChunksPendingEmbedding)

	var dbCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM chunks c
		JOIN memories m ON m.id = c.memory_id
		WHERE m.project=$1 AND m.valid_to IS NULL AND c.embedding IS NULL`, project).Scan(&dbCount)
	require.NoError(t, err)
	require.Equal(t, dbCount, stats.ChunksPendingEmbedding)
}
