package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

func TestGetStats_ZeroChunks(t *testing.T) {
	ctx := context.Background()
	project := uniqueProject("stats-zero-chunks")
	testDSN(t)

	backend := newTestBackend(t, project)
	// No chunks stored — all three embedding-backlog fields must be zero.
	stats, err := backend.GetStats(ctx, project)
	require.NoError(t, err)
	require.Equal(t, 0, stats.ChunksTotal)
	require.Equal(t, 0, stats.ChunksEmbedded)
	require.Equal(t, 0, stats.ChunksPendingEmbedding)
}

func TestGetStats_AllEmbedded(t *testing.T) {
	ctx := context.Background()
	project := uniqueProject("stats-all-embedded")
	testDSN(t)

	backend := newTestBackend(t, project)
	mem := storeMemory(t, backend, project, "all-embedded test")

	// Store chunks with nil embeddings first, then update them so the DB
	// vector dimension constraint is satisfied (migration 018 sets vector(1024)).
	chunkA := &types.Chunk{
		ID:        types.NewMemoryID(),
		MemoryID:  mem.ID,
		ChunkText: "chunk a",
		ChunkHash: "chunk-hash-a",
		Project:   project,
		ChunkType: "sentence_window",
		Embedding: nil,
	}
	chunkB := &types.Chunk{
		ID:        types.NewMemoryID(),
		MemoryID:  mem.ID,
		ChunkText: "chunk b",
		ChunkHash: "chunk-hash-b",
		Project:   project,
		ChunkType: "sentence_window",
		Embedding: nil,
	}
	err := backend.StoreChunks(ctx, []*types.Chunk{chunkA, chunkB})
	require.NoError(t, err)

	// Populate embeddings using the proper 1024-dim vector.
	vec := make([]float32, 1024)
	for i := range vec {
		vec[i] = float32(i) / 1024.0
	}
	_, err = backend.UpdateChunkEmbedding(ctx, chunkA.ID, vec)
	require.NoError(t, err)
	_, err = backend.UpdateChunkEmbedding(ctx, chunkB.ID, vec)
	require.NoError(t, err)

	stats, err := backend.GetStats(ctx, project)
	require.NoError(t, err)
	require.Equal(t, 2, stats.ChunksTotal)
	require.Equal(t, 0, stats.ChunksPendingEmbedding)
	require.Equal(t, stats.ChunksTotal, stats.ChunksEmbedded)
}

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
