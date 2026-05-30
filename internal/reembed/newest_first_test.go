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

// TestGlobalReembedder_QueryOrderIsNewestFirst_Source asserts that the
// chunk-selection query in global_worker.go uses DESC ordering so that the
// newest (highest-id) unembedded chunks are processed first.
//
// Rationale: with a large backlog (e.g. 1.34M rows after a model migration),
// ASC ordering means newly-written memories wait up to 18h before becoming
// vector-searchable. Recall falls back to BM25+recency for those chunks,
// which degrades result quality. DESC ordering ensures recent memories are
// embedded promptly (#914).
//
// NOTE on starvation: DESC ordering will not drain oldest backlog rows while
// the write rate meets or exceeds embed throughput. In this system write rate
// is well below embed throughput in steady state, so starvation risk is low.
// Monitor the pending-reembed gauge and flip back to ASC (or adopt a hybrid)
// if the oldest-chunk age grows unboundedly.
func TestGlobalReembedder_QueryOrderIsNewestFirst_Source(t *testing.T) {
	src, err := os.ReadFile("global_worker.go")
	if err != nil {
		t.Fatalf("read global_worker.go: %v", err)
	}
	text := string(src)

	// Must contain DESC ordering for the chunk selection query.
	if !strings.Contains(text, "ORDER BY c.id DESC") {
		t.Error("global_worker.go: chunk-selection query must use ORDER BY c.id DESC (newest-first) — found ASC or missing (#914)")
	}

	// Must NOT contain the old ASC-only ordering (bare ORDER BY c.id without DESC).
	if strings.Contains(text, "ORDER BY c.id\n") {
		t.Error("global_worker.go: found 'ORDER BY c.id' (ASC) — should be 'ORDER BY c.id DESC' (#914)")
	}
}

// TestGlobalReembedder_NewestChunkEmbeddedFirst_Integration verifies that
// when two chunks with NULL embeddings exist, runBatch processes the one with
// the higher id first (newest-first ordering). Requires TEST_DATABASE_URL.
func TestGlobalReembedder_NewestChunkEmbeddedFirst_Integration(t *testing.T) {
	dsn := testutil.DSN(t)
	ctx := context.Background()

	pool, err := db.NewSharedPool(ctx, dsn)
	if err != nil {
		t.Fatalf("NewSharedPool: %v", err)
	}
	defer pool.Close()

	project := testutil.UniqueProject("reembed-newest-first")
	backend, err := db.NewPostgresBackendWithPool(ctx, project, pool)
	if err != nil {
		t.Fatalf("NewPostgresBackendWithPool: %v", err)
	}

	// Seed two memories with NULL-embedding chunks. Because IDs are ULIDs
	// (time-ordered), the second chunk will have a higher id and represent
	// "newer" content. We insert with a small sleep to guarantee ordering.
	mem1 := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "older chunk — should be processed second",
		MemoryType:  "context",
		Project:     project,
		Importance:  1,
		StorageMode: "focused",
	}
	if err := backend.StoreMemory(ctx, mem1); err != nil {
		t.Fatalf("StoreMemory mem1: %v", err)
	}
	chunk1 := &types.Chunk{
		ID:         types.NewMemoryID(),
		MemoryID:   mem1.ID,
		Project:    project,
		ChunkText:  mem1.Content,
		ChunkIndex: 0,
	}
	if err := backend.StoreChunks(ctx, []*types.Chunk{chunk1}); err != nil {
		t.Fatalf("StoreChunks chunk1: %v", err)
	}

	// Brief pause so the second ULID is strictly greater.
	time.Sleep(2 * time.Millisecond)

	mem2 := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "newer chunk — should be processed first",
		MemoryType:  "context",
		Project:     project,
		Importance:  1,
		StorageMode: "focused",
	}
	if err := backend.StoreMemory(ctx, mem2); err != nil {
		t.Fatalf("StoreMemory mem2: %v", err)
	}
	chunk2 := &types.Chunk{
		ID:         types.NewMemoryID(),
		MemoryID:   mem2.ID,
		Project:    project,
		ChunkText:  mem2.Content,
		ChunkIndex: 0,
	}
	if err := backend.StoreChunks(ctx, []*types.Chunk{chunk2}); err != nil {
		t.Fatalf("StoreChunks chunk2: %v", err)
	}

	// Run a batch of exactly 1 so only the highest-id chunk is claimed.
	tracker := &trackingEmbedder{dims: 1024}
	g := &GlobalReembedder{
		pool:      pool,
		embedder:  tracker,
		batchSize: 1, // only pick one chunk
		interval:  time.Hour,
		done:      make(chan struct{}),
		notify:    make(chan struct{}, 1),
	}

	n, err := g.runBatch(ctx)
	if err != nil {
		t.Fatalf("runBatch: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 chunk processed, got %d", n)
	}

	// The newer chunk (chunk2) should now have a non-NULL embedding; the older
	// chunk (chunk1) should still be NULL.
	var chunk1Null, chunk2Null bool
	if err := pool.QueryRow(ctx, "SELECT embedding IS NULL FROM chunks WHERE id = $1", chunk1.ID).Scan(&chunk1Null); err != nil {
		t.Fatalf("check chunk1 embedding: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT embedding IS NULL FROM chunks WHERE id = $1", chunk2.ID).Scan(&chunk2Null); err != nil {
		t.Fatalf("check chunk2 embedding: %v", err)
	}

	if !chunk1Null {
		t.Error("chunk1 (older) should still have NULL embedding — it should not have been processed first (#914)")
	}
	if chunk2Null {
		t.Error("chunk2 (newer/higher id) should have a non-NULL embedding after runBatch with batchSize=1 (#914)")
	}
}

// trackingEmbedder is a fake embed.Client that records which texts were embedded.
type trackingEmbedder struct {
	dims   int
	embeds []string
}

func (e *trackingEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	e.embeds = append(e.embeds, text)
	return make([]float32, e.dims), nil
}
func (e *trackingEmbedder) EmbedWithModel(ctx context.Context, text string) ([]float32, string, error) {
	vec, err := e.Embed(ctx, text)
	return vec, e.Name(), err
}
func (e *trackingEmbedder) Name() string    { return "tracking" }
func (e *trackingEmbedder) Dimensions() int { return e.dims }
