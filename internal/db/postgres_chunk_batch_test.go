//go:build integration

package db_test

// postgres_chunk_batch_test.go — integration tests for the SendBatch chunk
// insert path in StoreChunksTx. Requires TEST_DATABASE_URL; skipped otherwise.

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

func newBatchTestBackend(t *testing.T) *db.PostgresBackend {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	ctx := context.Background()
	b, err := db.NewPostgresBackend(ctx, "batch-test", dsn)
	if err != nil {
		t.Fatalf("cannot connect to PostgreSQL (%v) — TEST_DATABASE_URL is set but connection failed (CI misconfiguration)", err)
	}
	t.Cleanup(b.Close)
	return b
}

// makeTestMemory stores a minimal memory and returns its ID. Chunks reference
// this memory via the memory_id foreign key.
func makeTestMemory(t *testing.T, b *db.PostgresBackend, project string) string {
	t.Helper()
	ctx := context.Background()
	m := &types.Memory{
		ID:           uuid.New().String(),
		Content:      "batch test memory " + uuid.New().String(),
		MemoryType:   types.MemoryTypeContext,
		Project:      project,
		Importance:   1,
		LastAccessed: time.Now(),
		CreatedAt:    time.Now(),
	}
	err := b.StoreMemory(ctx, m)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = b.DeleteChunksForMemory(ctx, m.ID) // chunks first due to FK
		_, _ = b.DeleteMemory(ctx, m.ID)
	})
	return m.ID
}

// makeChunk builds a test chunk with a unique ID and hash.
func makeChunk(memoryID, project string, idx int) *types.Chunk {
	text := fmt.Sprintf("chunk text index %d uuid %s", idx, uuid.New().String())
	h := sha256.Sum256([]byte(text))
	return &types.Chunk{
		ID:        uuid.New().String(),
		MemoryID:  memoryID,
		Project:   project,
		ChunkText: text,
		ChunkIndex: idx,
		ChunkHash: fmt.Sprintf("%x", h),
		ChunkType: "paragraph",
	}
}

// TestStoreChunksBatch_LargeBatch stores 200 chunks via StoreChunksTx (which
// routes through storeChunksBatch for counts > chunkBatchThreshold) and
// verifies all 200 are retrievable.
func TestStoreChunksBatch_LargeBatch(t *testing.T) {
	b := newBatchTestBackend(t)
	ctx := context.Background()

	const project = "batch-test"
	memID := makeTestMemory(t, b, project)

	chunks := make([]*types.Chunk, 200)
	for i := range chunks {
		chunks[i] = makeChunk(memID, project, i)
	}

	tx, err := b.Begin(ctx)
	require.NoError(t, err)

	err = b.StoreChunksTx(ctx, tx, chunks)
	require.NoError(t, err)

	err = tx.Commit(ctx)
	require.NoError(t, err)

	got, err := b.GetChunksForMemory(ctx, memID)
	require.NoError(t, err)
	require.Len(t, got, 200, "expected all 200 chunks to be stored")
}

// TestStoreChunksBatch_ConflictIdempotency stores the same chunk twice in
// separate transactions and verifies the count remains 1 (ON CONFLICT DO NOTHING).
func TestStoreChunksBatch_ConflictIdempotency(t *testing.T) {
	b := newBatchTestBackend(t)
	ctx := context.Background()

	const project = "batch-test"
	memID := makeTestMemory(t, b, project)

	// Build a batch of 4 chunks (above threshold so SendBatch is used).
	chunks := make([]*types.Chunk, 4)
	for i := range chunks {
		chunks[i] = makeChunk(memID, project, i)
	}

	// First insert.
	tx1, err := b.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, b.StoreChunksTx(ctx, tx1, chunks))
	require.NoError(t, tx1.Commit(ctx))

	// Second insert of the same chunks — should be silently skipped.
	tx2, err := b.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, b.StoreChunksTx(ctx, tx2, chunks))
	require.NoError(t, tx2.Commit(ctx))

	got, err := b.GetChunksForMemory(ctx, memID)
	require.NoError(t, err)
	require.Len(t, got, 4, "ON CONFLICT DO NOTHING: duplicate insert must not double the row count")
}
