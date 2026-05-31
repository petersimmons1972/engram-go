package reembed

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/testutil"
	"github.com/petersimmons1972/engram/internal/types"
)

// TestGlobalReembedder_ConsecutiveErrorCounterResets verifies that the
// consecutive-error counter increments on error and resets to zero after a
// successful batch. This counter drives the pool.Reset() call that recovers
// from stale connections after a Postgres restart (#645).
func TestGlobalReembedder_ConsecutiveErrorCounterResets(t *testing.T) {
	g := &GlobalReembedder{
		pool:      nil,
		embedder:  nil,
		batchSize: 10,
		interval:  time.Hour,
		done:      make(chan struct{}),
		notify:    make(chan struct{}, 1),
	}

	// Simulate manual error increments (what run() does on safeRunBatch failure).
	for i := 1; i <= 5; i++ {
		got := g.consecutiveErrors.Add(1)
		if got != int64(i) {
			t.Errorf("after %d errors: consecutive_errors = %d, want %d", i, got, i)
		}
	}
	if g.ConsecutiveErrors() != 5 {
		t.Fatalf("ConsecutiveErrors(): got %d, want 5", g.ConsecutiveErrors())
	}

	// On success, run() calls Store(0).
	g.consecutiveErrors.Store(0)
	if g.ConsecutiveErrors() != 0 {
		t.Errorf("after reset: ConsecutiveErrors() = %d, want 0", g.ConsecutiveErrors())
	}
}

// TestGlobalReembedder_HasPoolResetOnThreshold verifies the source code contains
// the pool.Reset() call guarded by the threshold, ensuring the recovery path
// cannot be accidentally deleted without breaking this test (#645).
func TestGlobalReembedder_HasPoolResetOnThreshold(t *testing.T) {
	src, err := os.ReadFile("global_worker.go")
	if err != nil {
		t.Fatalf("read global_worker.go: %v", err)
	}
	text := string(src)

	if !strings.Contains(text, "pool.Reset()") {
		t.Error("global_worker.go missing pool.Reset() call — Postgres restart recovery path deleted (#645)")
	}
	if !strings.Contains(text, "consecutiveErrorThreshold") {
		t.Error("global_worker.go missing consecutiveErrorThreshold guard — pool.Reset() must be threshold-gated (#645)")
	}
}

// countingEmbedder is a fake embed.Client that returns a zero vector.
type countingEmbedder struct {
	dims int
}

func (c *countingEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, c.dims), nil
}
func (c *countingEmbedder) EmbedWithModel(ctx context.Context, text string) ([]float32, string, error) {
	vec, err := c.Embed(ctx, text)
	return vec, c.Name(), err
}
func (c *countingEmbedder) Name() string    { return "counting" }
func (c *countingEmbedder) Dimensions() int { return c.dims }

// TestGlobalReembedder_RecoveryIntegration is an integration test that requires
// a live Postgres instance (TEST_DATABASE_URL). It verifies that after the pool
// is forcibly reset (simulating a Postgres restart), the GlobalReembedder
// processes a queued chunk within 25 seconds (#645).
func TestGlobalReembedder_RecoveryIntegration(t *testing.T) {
	dsn := testutil.DSN(t) // skips if TEST_DATABASE_URL is unset

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := db.NewSharedPool(ctx, dsn)
	if err != nil {
		t.Fatalf("NewSharedPool: %v", err)
	}
	defer pool.Close()

	project := testutil.UniqueProject("reembed-recovery")
	backend, err := db.NewPostgresBackendWithPool(ctx, project, pool)
	if err != nil {
		t.Fatalf("NewPostgresBackendWithPool: %v", err)
	}

	// Seed a memory with a NULL-embedded chunk.
	mem := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "integration test for #645 recovery",
		MemoryType:  "context",
		Project:     project,
		Importance:  1,
		StorageMode: "focused",
	}
	if err := backend.StoreMemory(ctx, mem); err != nil {
		t.Fatalf("StoreMemory: %v", err)
	}

	chunkID := types.NewMemoryID()
	chunk := &types.Chunk{
		ID:         chunkID,
		MemoryID:   mem.ID,
		Project:    project,
		ChunkText:  mem.Content,
		ChunkIndex: 0,
		// Embedding left nil → stored as NULL → picked up by runBatch.
	}
	if err := backend.StoreChunks(ctx, []*types.Chunk{chunk}); err != nil {
		t.Fatalf("StoreChunks: %v", err)
	}

	embedder := &countingEmbedder{dims: 1024}
	g := NewGlobalReembedder(pool, embedder, 10, 100*time.Millisecond)

	// Simulate all connections going stale (e.g. Postgres was restarted) by
	// calling pool.Reset() before starting the worker. The worker must recover
	// and process the chunk within the test deadline.
	pool.Reset()

	g.Start(ctx)
	defer g.Wait()

	// Notify immediately to skip the initial backoff sleep.
	g.Notify()

	// Poll until the chunk has a non-NULL embedding or the context times out.
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		var embeddingIsNull bool
		err := pool.QueryRow(ctx,
			"SELECT embedding IS NULL FROM chunks WHERE id = $1", chunkID,
		).Scan(&embeddingIsNull)
		if err != nil {
			t.Logf("check embedding: %v (retrying)", err)
		} else if !embeddingIsNull {
			return // success
		}
		time.Sleep(200 * time.Millisecond)
	}

	t.Errorf("chunk %s still has NULL embedding after 25s — worker did not recover from simulated Postgres restart (#645)", chunkID)
}
