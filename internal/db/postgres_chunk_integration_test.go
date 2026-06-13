//go:build integration

package db_test

// postgres_chunk_integration_test.go — live-Postgres integration tests for
// the Patch A embedding IS NULL write guard in UpdateChunkEmbedding.
// Requires TEST_DATABASE_URL; tests skip when the variable is unset.
//
// Run: go test -tags integration -run 'TestUpdateChunkEmbedding_NullGuard|TestUpdateChunkEmbedding_NullRow' ./internal/db/...

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// newNullGuardTestBackend opens a backend against TEST_DATABASE_URL, skipping
// the test if the env var is absent. Matches the harness pattern in
// postgres_chunk_batch_test.go (newBatchTestBackend).
func newNullGuardTestBackend(t *testing.T) *db.PostgresBackend {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	ctx := context.Background()
	b, err := db.NewPostgresBackend(ctx, "null-guard-test", dsn)
	if err != nil {
		t.Fatalf("cannot connect to PostgreSQL (%v) — TEST_DATABASE_URL is set but connection failed (CI misconfiguration)", err)
	}
	t.Cleanup(b.Close)
	return b
}

// makeNullGuardMemory stores a minimal memory owned by project and registers a
// cleanup to delete it (and its chunks) at test end.
func makeNullGuardMemory(t *testing.T, b *db.PostgresBackend, project string) string {
	t.Helper()
	ctx := context.Background()
	m := &types.Memory{
		ID:         uuid.New().String(),
		Content:    "null-guard test memory " + uuid.New().String(),
		MemoryType: types.MemoryTypeContext,
		Project:    project,
		Importance: 1,
	}
	require.NoError(t, b.StoreMemory(ctx, m))
	t.Cleanup(func() {
		_ = b.DeleteChunksForMemory(ctx, m.ID)
		_, _ = b.DeleteMemory(ctx, m.ID)
	})
	return m.ID
}

// makeNullEmbeddingChunk inserts a chunk with embedding=NULL (the pending-reembed
// state) and returns its ID.
func makeNullEmbeddingChunk(t *testing.T, b *db.PostgresBackend, memID, project string) string {
	t.Helper()
	ctx := context.Background()
	chunk := &types.Chunk{
		ID:         uuid.New().String(),
		MemoryID:   memID,
		Project:    project,
		ChunkText:  "null-guard test chunk " + uuid.New().String(),
		ChunkIndex: 0,
		ChunkHash:  uuid.New().String(),
		ChunkType:  "paragraph",
		// Embedding intentionally nil — pending-reembed state
	}
	tx, err := b.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, b.StoreChunksTx(ctx, tx, []*types.Chunk{chunk}))
	require.NoError(t, tx.Commit(ctx))
	return chunk.ID
}

// fakeEmbedding returns a non-zero float32 slice of dimension 1024 with values
// derived from seed so that first-writer and second-writer embeddings are distinct.
func fakeEmbedding(seed int) []float32 {
	const dim = 1024
	v := make([]float32, dim)
	for i := range v {
		v[i] = float32(seed+i+1) / float32(dim*(seed+1)+1)
	}
	return v
}

// TestUpdateChunkEmbedding_NullGuard_RowsAffectedZeroOnRace proves that the
// AND embedding IS NULL guard (Patch A, #1087) makes a second drainer's UPDATE
// affect 0 rows and preserves the first committed embedding.
//
// Scenario: two concurrent drainers both claim the same chunk while it has
// embedding=NULL. Drainer-1 commits its embedding first. Drainer-2 then calls
// UpdateChunkEmbedding on the same chunk — it must get rows_affected==0 and
// the embedding in the DB must still be Drainer-1's value (FM-14 corruption gate).
func TestUpdateChunkEmbedding_NullGuard_RowsAffectedZeroOnRace(t *testing.T) {
	b := newNullGuardTestBackend(t)
	ctx := context.Background()

	const project = "null-guard-test"
	memID := makeNullGuardMemory(t, b, project)
	chunkID := makeNullEmbeddingChunk(t, b, memID, project)

	vec1 := fakeEmbedding(1) // Drainer-1's embedding — must survive
	vec2 := fakeEmbedding(2) // Drainer-2's embedding — must NOT overwrite

	// Drainer-1: row has NULL embedding → UPDATE must match and return 1.
	n1, err := b.UpdateChunkEmbedding(ctx, chunkID, vec1)
	require.NoError(t, err)
	require.Equal(t, 1, n1,
		"Drainer-1 (first writer): UpdateChunkEmbedding must affect 1 row when embedding IS NULL")

	// Drainer-2: row now has a non-NULL embedding → AND embedding IS NULL blocks the write.
	n2, err := b.UpdateChunkEmbedding(ctx, chunkID, vec2)
	require.NoError(t, err)
	require.Equal(t, 0, n2,
		"Drainer-2 (second writer): UpdateChunkEmbedding must affect 0 rows — "+
			"AND embedding IS NULL guard must block the overwrite (#1087 Patch A, FM-14)")

	// Confirm the stored embedding is Drainer-1's value — no silent corruption.
	chunks, err := b.GetChunksForMemory(ctx, memID)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	require.NotNil(t, chunks[0].Embedding,
		"chunk embedding must not be NULL after Drainer-1 committed")
	require.Equal(t, vec1, chunks[0].Embedding,
		"stored embedding must be Drainer-1's value — Drainer-2 must not have overwritten it (FM-14)")
}

// TestUpdateChunkEmbedding_NullRow_StillWrites proves that the AND embedding IS
// NULL guard does NOT block a legitimate first write when the row genuinely has
// embedding=NULL (rows_affected must be 1, and the value must be stored).
func TestUpdateChunkEmbedding_NullRow_StillWrites(t *testing.T) {
	b := newNullGuardTestBackend(t)
	ctx := context.Background()

	const project = "null-guard-test"
	memID := makeNullGuardMemory(t, b, project)
	chunkID := makeNullEmbeddingChunk(t, b, memID, project)

	vec := fakeEmbedding(7)

	// Row has embedding=NULL — a normal first write must succeed.
	n, err := b.UpdateChunkEmbedding(ctx, chunkID, vec)
	require.NoError(t, err)
	require.Equal(t, 1, n,
		"UpdateChunkEmbedding on a NULL-embedding row must affect 1 row — "+
			"guard must not block legitimate first writes (#1087 Patch A)")

	// Confirm the embedding is persisted with the correct value.
	chunks, err := b.GetChunksForMemory(ctx, memID)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	require.Equal(t, vec, chunks[0].Embedding,
		"embedding written by the first writer must be stored verbatim")
}
