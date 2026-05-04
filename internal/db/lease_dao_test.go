package db

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// testDSN returns the integration-test DSN, skipping if unset.
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	return dsn
}

// ── TestEnqueueChunkLease ────────────────────────────────────────────────────

// TestEnqueueChunkLease verifies that EnqueueChunkLease sets an initial lease
// on a chunk with NULL embedding.
func TestEnqueueChunkLease(t *testing.T) {
	ctx := context.Background()
	dsn := testDSN(t)

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	// Create a test memory and chunk.
	backend, err := NewPostgresBackend(ctx, "test-lease", dsn)
	require.NoError(t, err)
	defer backend.Close()

	memID := types.NewMemoryID()
	chunkID := types.NewMemoryID()
	chunk := &types.Chunk{
		ID:        chunkID,
		MemoryID:  memID,
		ChunkText: "test chunk",
		ChunkHash: "hash123",
		ChunkType: "sentence_window",
		Project:   "test-lease",
		Embedding: nil,
	}

	mem := &types.Memory{
		ID:      memID,
		Content: "test memory",
		Project: "test-lease",
	}

	// Store memory and chunk.
	err = backend.StoreMemory(ctx, mem)
	require.NoError(t, err)

	err = backend.StoreChunks(ctx, []*types.Chunk{chunk})
	require.NoError(t, err)

	// Enqueue the chunk.
	err = EnqueueChunkLease(ctx, pool, chunkID, "test-lease")
	require.NoError(t, err)

	// Verify the lease was set.
	var leaseUntil *time.Time
	err = pool.QueryRow(ctx, `
		SELECT embed_lease_until FROM chunks WHERE id = $1
	`, chunkID).Scan(&leaseUntil)
	require.NoError(t, err)
	require.NotNil(t, leaseUntil, "embed_lease_until should be set")
	require.True(t, leaseUntil.After(time.Now()), "lease should be in the future")
}

// ── TestEnqueueChunkLease_Idempotent ────────────────────────────────────────

// TestEnqueueChunkLease_Idempotent verifies that calling EnqueueChunkLease
// multiple times does not cause an error.
func TestEnqueueChunkLease_Idempotent(t *testing.T) {
	ctx := context.Background()
	dsn := testDSN(t)

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	backend, err := NewPostgresBackend(ctx, "test-lease-idempotent", dsn)
	require.NoError(t, err)
	defer backend.Close()

	memID := types.NewMemoryID()
	chunkID := types.NewMemoryID()
	chunk := &types.Chunk{
		ID:        chunkID,
		MemoryID:  memID,
		ChunkText: "test chunk",
		ChunkHash: "hash456",
		ChunkType: "sentence_window",
		Project:   "test-lease-idempotent",
		Embedding: nil,
	}

	mem := &types.Memory{
		ID:      memID,
		Content: "test memory",
		Project: "test-lease-idempotent",
	}

	err = backend.StoreMemory(ctx, mem)
	require.NoError(t, err)

	err = backend.StoreChunks(ctx, []*types.Chunk{chunk})
	require.NoError(t, err)

	// Call EnqueueChunkLease twice.
	err = EnqueueChunkLease(ctx, pool, chunkID, "test-lease-idempotent")
	require.NoError(t, err)

	err = EnqueueChunkLease(ctx, pool, chunkID, "test-lease-idempotent")
	require.NoError(t, err, "second call should also succeed")
}

// ── TestEnqueueChunkLeases_Batch ────────────────────────────────────────────

// TestEnqueueChunkLeases_Batch verifies that EnqueueChunkLeases sets leases
// on multiple chunks in a single call.
func TestEnqueueChunkLeases_Batch(t *testing.T) {
	ctx := context.Background()
	dsn := testDSN(t)

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	backend, err := NewPostgresBackend(ctx, "test-lease-batch", dsn)
	require.NoError(t, err)
	defer backend.Close()

	memID := types.NewMemoryID()
	chunkIDs := []string{types.NewMemoryID(), types.NewMemoryID(), types.NewMemoryID()}

	mem := &types.Memory{
		ID:      memID,
		Content: "test memory",
		Project: "test-lease-batch",
	}

	err = backend.StoreMemory(ctx, mem)
	require.NoError(t, err)

	var chunks []*types.Chunk
	for i, chunkID := range chunkIDs {
		chunk := &types.Chunk{
			ID:        chunkID,
			MemoryID:  memID,
			ChunkText: "test chunk " + string(rune(i)),
			ChunkHash: "hash" + string(rune(i)),
			ChunkType: "sentence_window",
			Project:   "test-lease-batch",
			Embedding: nil,
		}
		chunks = append(chunks, chunk)
	}

	err = backend.StoreChunks(ctx, chunks)
	require.NoError(t, err)

	// Enqueue all chunks in one call.
	err = EnqueueChunkLeases(ctx, pool, chunkIDs)
	require.NoError(t, err)

	// Verify all leases were set.
	var count int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM chunks WHERE id = ANY($1) AND embed_lease_until IS NOT NULL
	`, chunkIDs).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, len(chunkIDs), count, "all chunks should have leases set")
}
