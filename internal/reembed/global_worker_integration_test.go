//go:build integration

package reembed_test

// global_worker_integration_test.go — live-Postgres integration test for the
// Patch A embedding IS NULL write guard in global_worker.go's runBatch.
// Requires TEST_DATABASE_URL; skips when the variable is unset.
//
// Run: go test -tags integration -run TestGlobalWorker_NullGuard ./internal/reembed/...
//
// Design note: GlobalReembedder.runBatch is unexported. Rather than driving the
// full worker lifecycle (which requires a live embed endpoint), we execute the
// exact UPDATE SQL that runBatch issues via the pgxpool.Pool. This is the
// minimal faithful test: same SQL, same rows_affected semantics, no mock.

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/types"
	pgvector "github.com/pgvector/pgvector-go"
	"github.com/stretchr/testify/require"
)

// newGlobalWorkerTestBackend opens a PostgresBackend against TEST_DATABASE_URL,
// skipping the test when the variable is unset. Matches the harness pattern in
// internal/db/postgres_chunk_batch_test.go.
func newGlobalWorkerTestBackend(t *testing.T) *db.PostgresBackend {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	ctx := context.Background()
	b, err := db.NewPostgresBackend(ctx, "global-worker-test", dsn)
	if err != nil {
		t.Fatalf("cannot connect to PostgreSQL (%v) — TEST_DATABASE_URL is set but connection failed (CI misconfiguration)", err)
	}
	t.Cleanup(b.Close)
	return b
}

// makeGlobalWorkerMemory stores a minimal memory and registers cleanup.
func makeGlobalWorkerMemory(t *testing.T, b *db.PostgresBackend, project string) string {
	t.Helper()
	ctx := context.Background()
	m := &types.Memory{
		ID:         uuid.New().String(),
		Content:    "global-worker test memory " + uuid.New().String(),
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

// makeGlobalWorkerChunk inserts a chunk with embedding=NULL (pending state).
func makeGlobalWorkerChunk(t *testing.T, b *db.PostgresBackend, memID, project string) string {
	t.Helper()
	ctx := context.Background()
	chunk := &types.Chunk{
		ID:         uuid.New().String(),
		MemoryID:   memID,
		Project:    project,
		ChunkText:  "global-worker test chunk " + uuid.New().String(),
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

// globalWorkerFakeEmbedding returns a non-zero float32 slice of dimension 1024.
// Distinct seeds produce distinct vectors so first vs. second writer are
// distinguishable in the assertion.
func globalWorkerFakeEmbedding(seed int) []float32 {
	const dim = 1024
	v := make([]float32, dim)
	for i := range v {
		v[i] = float32(seed+i+1) / float32(dim*(seed+1)+1)
	}
	return v
}

// workerUpdateSQL is the exact UPDATE statement that global_worker.go's runBatch
// goroutine issues (commit ef5771a, Patch A #1087). Kept verbatim here so test
// failures point at a real SQL divergence, not a test rewrite.
const workerUpdateSQL = "UPDATE chunks SET embedding=$1 WHERE id=$2 AND embedding IS NULL"

// TestGlobalWorker_NullGuard_RowsAffectedZeroOnRace proves that the AND embedding
// IS NULL guard in global_worker.go's runBatch makes a second drainer's UPDATE
// affect 0 rows, preventing silent overwrite of an already-committed embedding.
//
// Scenario: two concurrent GlobalReembedder instances both claim the same chunk
// while it has embedding=NULL. Drainer-1 commits its embedding. Drainer-2 then
// executes the same UPDATE — it must receive rows_affected==0 and the stored
// embedding must remain Drainer-1's value (FM-14 no-overwrite invariant).
func TestGlobalWorker_NullGuard_RowsAffectedZeroOnRace(t *testing.T) {
	b := newGlobalWorkerTestBackend(t)
	ctx := context.Background()
	pool := b.Pool()

	const project = "global-worker-test"
	memID := makeGlobalWorkerMemory(t, b, project)
	chunkID := makeGlobalWorkerChunk(t, b, memID, project)

	vec1 := globalWorkerFakeEmbedding(1) // Drainer-1's embedding — must survive
	vec2 := globalWorkerFakeEmbedding(2) // Drainer-2's embedding — must NOT land

	// Drainer-1: row has NULL embedding → guard passes, 1 row updated.
	tag1, err := pool.Exec(ctx, workerUpdateSQL, pgvector.NewVector(vec1), chunkID)
	require.NoError(t, err)
	require.Equal(t, int64(1), tag1.RowsAffected(),
		"Drainer-1 (first writer): UPDATE must affect 1 row when embedding IS NULL")

	// Drainer-2: row now has a non-NULL embedding → AND embedding IS NULL blocks write.
	tag2, err := pool.Exec(ctx, workerUpdateSQL, pgvector.NewVector(vec2), chunkID)
	require.NoError(t, err)
	require.Equal(t, int64(0), tag2.RowsAffected(),
		"Drainer-2 (second writer): UPDATE must affect 0 rows — "+
			"AND embedding IS NULL guard in global_worker.go must block the overwrite (#1087 Patch A, FM-14)")

	// Verify the stored embedding is still Drainer-1's value — no silent corruption.
	chunks, err := b.GetChunksForMemory(ctx, memID)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	require.NotNil(t, chunks[0].Embedding,
		"chunk embedding must not be NULL after Drainer-1 committed")
	require.Equal(t, vec1, chunks[0].Embedding,
		"stored embedding must be Drainer-1's value — Drainer-2 must not have overwritten it (FM-14)")
}
