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

	// Insert a minimal chunk row with a NULL embedding so runBatch has something
	// to pick up. We use a conflict-safe upsert to re-run the test idempotently.
	chunkID := "reembed-test-embed-fail-chunk"
	memID := "reembed-test-embed-fail-mem"
	_, err = pool.Exec(ctx, `
		INSERT INTO chunks (id, memory_id, project, chunk_text, chunk_index, embedding)
		VALUES ($1, $2, 'reembed-test', 'test content', 0, NULL)
		ON CONFLICT (id) DO UPDATE SET embedding = NULL`,
		chunkID, memID,
	)
	if err != nil {
		t.Skipf("cannot seed test chunk (schema constraint): %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM chunks WHERE id = $1", chunkID)
	})

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
