package reembed

import (
	"context"
	"errors"
	"testing"
	"time"

	promtest "github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/metrics"
	"github.com/petersimmons1972/engram/internal/testutil"
	"github.com/petersimmons1972/engram/internal/types"
)

// failEmbedder is a mock embed.Client that always returns an error.
type failEmbedder struct{}

func (f *failEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, errors.New("mock embed failure")
}
func (f *failEmbedder) Name() string    { return "fail" }
func (f *failEmbedder) Dimensions() int { return 0 }

var _ embed.Client = (*failEmbedder)(nil)

// TestEmbedFailureCountedInMetrics verifies that when the embedder returns an
// error for a chunk, WorkerErrors{"global_reembed"} is incremented (#370a).
// Requires TEST_DATABASE_URL pointing to a live Postgres+pgvector instance with
// the engram schema already migrated.
func TestEmbedFailureCountedInMetrics(t *testing.T) {
	dsn := testutil.DSN(t)
	ctx := context.Background()

	pool, err := db.NewSharedPool(ctx, dsn)
	if err != nil {
		t.Fatalf("NewSharedPool: %v", err)
	}
	defer pool.Close()

	project := testutil.UniqueProject("reembed-fail")
	backend, err := db.NewPostgresBackendWithPool(ctx, project, pool)
	if err != nil {
		t.Fatalf("NewPostgresBackendWithPool: %v", err)
	}

	// Seed a memory + chunk with NULL embedding so runBatch picks it up.
	mem := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "test content for embed failure",
		MemoryType:  "context",
		Project:     project,
		Importance:  1,
		StorageMode: "focused",
	}
	if err := backend.StoreMemory(ctx, mem); err != nil {
		t.Fatalf("StoreMemory: %v", err)
	}

	chunk := &types.Chunk{
		ID:         types.NewMemoryID(),
		MemoryID:   mem.ID,
		Project:    project,
		ChunkText:  mem.Content,
		ChunkIndex: 0,
		// Embedding left nil → stored as NULL → picked up by runBatch's
		// "WHERE c.embedding IS NULL" query.
	}
	if err := backend.StoreChunks(ctx, []*types.Chunk{chunk}); err != nil {
		t.Fatalf("StoreChunks: %v", err)
	}

	g := &GlobalReembedder{
		pool:      pool,
		embedder:  &failEmbedder{},
		batchSize: 10,
		interval:  1 * time.Hour,
		done:      make(chan struct{}),
	}

	before := promtest.ToFloat64(metrics.WorkerErrors.WithLabelValues("global_reembed"))

	if err := g.runBatch(ctx); err != nil {
		t.Fatalf("runBatch returned unexpected error: %v", err)
	}

	after := promtest.ToFloat64(metrics.WorkerErrors.WithLabelValues("global_reembed"))
	if after <= before {
		t.Errorf("WorkerErrors{global_reembed} did not increase after embed failure: before=%.0f after=%.0f", before, after)
	}
}
