package reembed_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/entity"
	"github.com/petersimmons1972/engram/internal/reembed"
	"github.com/petersimmons1972/engram/internal/types"
)

type fakeEmbedder struct{ dims int }

func (f *fakeEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, f.dims), nil
}
func (f *fakeEmbedder) Name() string    { return "fake" }
func (f *fakeEmbedder) Dimensions() int { return f.dims }

func TestWorker_StartsAndStops(t *testing.T) {
	w := reembed.NewWorker(nil, &fakeEmbedder{dims: 768}, "proj", false)
	w.Start()
	time.Sleep(20 * time.Millisecond)
	w.Stop()
	// reached here without hanging
}

// TestNewWorkerFromMeta_ActivatesOnPendingChunks verifies that NewWorkerFromMeta
// activates the worker when chunks with NULL embeddings exist, even without the
// embedding_migration_in_progress flag. This covers Ollama outage recovery.
func TestNewWorkerFromMeta_ActivatesOnPendingChunks(t *testing.T) {
	backend := &pendingChunkBackend{
		noopBackend:   noopBackend{},
		pendingChunks: []*types.Chunk{{ID: "chunk-1", ChunkText: "test"}},
	}
	w := reembed.NewWorkerFromMeta(context.Background(), backend, &fakeEmbedder{dims: 768}, "proj")
	if !w.IsActive() {
		t.Fatal("expected worker active when pending chunks exist but migration flag unset")
	}
}

// TestNewWorkerFromMeta_InactiveWhenNoPending verifies the worker stays inactive
// when there are no pending chunks and no migration flag.
func TestNewWorkerFromMeta_InactiveWhenNoPending(t *testing.T) {
	backend := &pendingChunkBackend{noopBackend: noopBackend{}}
	w := reembed.NewWorkerFromMeta(context.Background(), backend, &fakeEmbedder{dims: 768}, "proj")
	if w.IsActive() {
		t.Fatal("expected worker inactive when no pending chunks and no migration flag")
	}
}

// pendingChunkBackend embeds noopBackend and overrides GetChunksPendingEmbedding.
type pendingChunkBackend struct {
	noopBackend
	pendingChunks []*types.Chunk
}

func (b *pendingChunkBackend) GetChunksPendingEmbedding(_ context.Context, _ string, _ int) ([]*types.Chunk, error) {
	return b.pendingChunks, nil
}

// noopBackend implements db.Backend with all methods returning zero values.
type noopBackend struct{}

var _ db.Backend = noopBackend{}

func (noopBackend) Close()                                                      {}
func (noopBackend) GetMeta(_ context.Context, _, _ string) (string, bool, error) { return "", false, nil }
func (noopBackend) SetMeta(_ context.Context, _, _, _ string) error             { return nil }
func (noopBackend) SetMetaTx(_ context.Context, _ db.Tx, _, _, _ string) error  { return nil }
func (noopBackend) StoreMemory(_ context.Context, _ *types.Memory) error        { return nil }
func (noopBackend) StoreMemoryTx(_ context.Context, _ db.Tx, _ *types.Memory) error { return nil }
func (noopBackend) GetMemory(_ context.Context, _ string) (*types.Memory, error) { return nil, nil }
func (noopBackend) GetMemoriesByIDs(_ context.Context, _ string, _ []string) ([]*types.Memory, error) {
	return nil, nil
}
func (noopBackend) UpdateMemory(_ context.Context, _ string, _ *string, _ []string, _ *int) (*types.Memory, error) {
	return nil, nil
}
func (noopBackend) DeleteMemory(_ context.Context, _ string) (bool, error)            { return false, nil }
func (noopBackend) DeleteMemoryAtomic(_ context.Context, _, _ string, _ bool) (bool, error) {
	return false, nil
}
func (noopBackend) MergeMemoriesAtomic(_ context.Context, _, _, _, _ string) error { return nil }
func (noopBackend) ListMemories(_ context.Context, _ string, _ db.ListOptions) ([]*types.Memory, error) {
	return nil, nil
}
func (noopBackend) TouchMemory(_ context.Context, _ string) error               { return nil }
func (noopBackend) TouchMemories(_ context.Context, _ []string) error           { return nil }
func (noopBackend) StoreChunks(_ context.Context, _ []*types.Chunk) error       { return nil }
func (noopBackend) StoreChunksTx(_ context.Context, _ db.Tx, _ []*types.Chunk) error { return nil }
func (noopBackend) GetChunksForMemory(_ context.Context, _ string) ([]*types.Chunk, error) {
	return nil, nil
}
func (noopBackend) GetAllChunksWithEmbeddings(_ context.Context, _ string, _ int) ([]*types.Chunk, error) {
	return nil, nil
}
func (noopBackend) GetAllChunkTexts(_ context.Context, _ string, _ int) ([]string, error) {
	return nil, nil
}
func (noopBackend) GetChunksForMemories(_ context.Context, _ []string) ([]*types.Chunk, error) {
	return nil, nil
}
func (noopBackend) ChunkHashExists(_ context.Context, _, _ string) (bool, error) { return false, nil }
func (noopBackend) DeleteChunksForMemory(_ context.Context, _ string) error      { return nil }
func (noopBackend) DeleteChunksForMemoryTx(_ context.Context, _ db.Tx, _ string) error { return nil }
func (noopBackend) DeleteChunksByIDs(_ context.Context, _ []string) (int, error) { return 0, nil }
func (noopBackend) NullAllEmbeddings(_ context.Context, _ string) (int, error)   { return 0, nil }
func (noopBackend) NullAllEmbeddingsTx(_ context.Context, _ db.Tx, _ string) (int, error) {
	return 0, nil
}
func (noopBackend) GetChunksPendingEmbedding(_ context.Context, _ string, _ int) ([]*types.Chunk, error) {
	return nil, nil
}
func (noopBackend) UpdateChunkEmbedding(_ context.Context, _ string, _ []float32) (int, error) {
	return 0, nil
}
func (noopBackend) VectorSearch(_ context.Context, _ string, _ []float32, _ int) ([]db.VectorHit, error) {
	return nil, nil
}
func (noopBackend) ChunkEmbeddingDistance(_ context.Context, _, _ string) (float64, error) {
	return 2.0, nil
}
func (noopBackend) UpdateChunkLastMatched(_ context.Context, _ string) error { return nil }
func (noopBackend) GetPendingEmbeddingCount(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (noopBackend) StoreRelationship(_ context.Context, _ *types.Relationship) error { return nil }
func (noopBackend) GetConnected(_ context.Context, _ string, _ int) ([]db.ConnectedResult, error) {
	return nil, nil
}
func (noopBackend) BoostEdgesForMemory(_ context.Context, _ string, _ float64) (int, error) {
	return 0, nil
}
func (noopBackend) DecayEdgesForMemory(_ context.Context, _ string, _ float64) (int, error) {
	return 0, nil
}
func (noopBackend) GetConnectionCount(_ context.Context, _, _ string) (int, error) { return 0, nil }
func (noopBackend) DecayAllEdges(_ context.Context, _ string, _, _ float64) (int, int, error) {
	return 0, 0, nil
}
func (noopBackend) DeleteRelationshipsForMemory(_ context.Context, _ string) error { return nil }
func (noopBackend) GetRelationships(_ context.Context, _, _ string) ([]types.Relationship, error) {
	return nil, nil
}
func (noopBackend) GetRelationshipsBatch(_ context.Context, _ string, _ []string) (map[string][]types.Relationship, error) {
	return nil, nil
}
func (noopBackend) GetMemoryHistory(_ context.Context, _, _ string) ([]*types.MemoryVersion, error) {
	return nil, nil
}
func (noopBackend) SoftDeleteMemory(_ context.Context, _, _, _ string) (bool, error) {
	return false, nil
}
func (noopBackend) GetMemoriesAsOf(_ context.Context, _ string, _ time.Time, _ int) ([]*types.Memory, error) {
	return nil, nil
}
func (noopBackend) StoreRetrievalEvent(_ context.Context, _ *types.RetrievalEvent) error {
	return nil
}
func (noopBackend) GetRetrievalEvent(_ context.Context, _ string) (*types.RetrievalEvent, error) {
	return nil, nil
}
func (noopBackend) RecordFeedback(_ context.Context, _ string, _ []string) error { return nil }
func (noopBackend) RecordFeedbackWithClass(_ context.Context, _ string, _ []string, _ string) error {
	return nil
}
func (noopBackend) AggregateMemories(_ context.Context, _, _, _ string, _ int) ([]types.AggregateRow, error) {
	return nil, nil
}
func (noopBackend) AggregateFailureClasses(_ context.Context, _ string, _ int) ([]types.AggregateRow, error) {
	return nil, nil
}
func (noopBackend) IncrementTimesRetrieved(_ context.Context, _ []string) error { return nil }
func (noopBackend) UpdateDynamicImportance(_ context.Context, _ string, _, _ float64) error {
	return nil
}
func (noopBackend) SetNextReviewAt(_ context.Context, _ string, _ time.Time) error { return nil }
func (noopBackend) DecayStaleImportance(_ context.Context, _ string, _ float64) (int, error) {
	return 0, nil
}
func (noopBackend) PruneStaleMemories(_ context.Context, _ string, _ float64, _ int) (int, error) {
	return 0, nil
}
func (noopBackend) PruneColdDocuments(_ context.Context, _ string, _ float64, _ int) (int, error) {
	return 0, nil
}
func (noopBackend) FTSSearch(_ context.Context, _, _ string, _ int, _, _ *time.Time) ([]db.FTSResult, error) {
	return nil, nil
}
func (noopBackend) RebuildFTS(_ context.Context) error                                { return nil }
func (noopBackend) GetStats(_ context.Context, _ string) (*types.MemoryStats, error)  { return nil, nil }
func (noopBackend) ListAllProjects(_ context.Context) ([]string, error)               { return nil, nil }
func (noopBackend) GetAllMemoryIDs(_ context.Context, _ string) (map[string]struct{}, error) {
	return nil, nil
}
func (noopBackend) GetMemoryTypeMap(_ context.Context, _ string) (map[string]string, error) {
	return nil, nil
}
func (noopBackend) GetMemoriesPendingSummary(_ context.Context, _ string, _ int) ([]db.IDContent, error) {
	return nil, nil
}
func (noopBackend) StoreSummary(_ context.Context, _, _ string) error { return nil }
func (noopBackend) GetPendingSummaryCount(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (noopBackend) ClearSummaries(_ context.Context, _ string) (int, error) { return 0, nil }
func (noopBackend) GetMemoriesMissingHash(_ context.Context, _ string, _ int) ([]db.IDContent, error) {
	return nil, nil
}
func (noopBackend) UpdateMemoryHash(_ context.Context, _, _ string) error { return nil }
func (noopBackend) ExistsWithContentHash(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
func (noopBackend) GetIntegrityStats(_ context.Context, _ string) (db.IntegrityStats, error) {
	return db.IntegrityStats{}, nil
}
func (noopBackend) StartEpisode(_ context.Context, _, _ string) (*types.Episode, error) {
	return nil, nil
}
func (noopBackend) EndEpisode(_ context.Context, _, _ string) error { return nil }
func (noopBackend) ListEpisodes(_ context.Context, _ string, _ int) ([]*types.Episode, error) {
	return nil, nil
}
func (noopBackend) RecallEpisode(_ context.Context, _ string) ([]*types.Memory, error) {
	return nil, nil
}
func (noopBackend) CloseStaleEpisodes(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}
func (noopBackend) Begin(_ context.Context) (db.Tx, error) { return nil, nil }
func (noopBackend) SearchChunksWithinMemory(_ context.Context, _ []float32, _ string, _ int) ([]*types.Chunk, error) {
	return nil, nil
}
func (noopBackend) StoreDocument(_ context.Context, _, _ string) (string, error) { return "", nil }
func (noopBackend) GetDocument(_ context.Context, _ string) (string, error)      { return "", nil }
func (noopBackend) SetMemoryDocumentID(_ context.Context, _, _ string) error     { return nil }
func (noopBackend) UpsertEntity(_ context.Context, _ *entity.Entity) (string, error) {
	return "", nil
}
func (noopBackend) GetEntitiesByProject(_ context.Context, _ string) ([]entity.Entity, error) {
	return nil, nil
}
func (noopBackend) EnqueueExtractionJob(_ context.Context, _, _ string) error { return nil }
func (noopBackend) ClaimExtractionJobs(_ context.Context, _ string, _ int) ([]db.ExtractionJob, error) {
	return nil, nil
}
func (noopBackend) CompleteExtractionJob(_ context.Context, _ string, _ error) error { return nil }

// concurrentEmbedder records the peak number of Embed calls in-flight simultaneously.
// Each call increments active, records the peak, then waits on the ready channel
// to synchronise with the test. When active reaches totalChunks, it closes the
// release channel so every waiting goroutine unblocks at once.
type concurrentEmbedder struct {
	dims        int
	totalChunks int32
	active      atomic.Int32 // goroutines currently inside Embed
	peak        atomic.Int32 // highest observed value of active
	release     chan struct{} // closed by the last goroutine to enter Embed
	releaseOnce sync.Once
}

func (c *concurrentEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	cur := c.active.Add(1)
	defer c.active.Add(-1)
	// Update peak.
	for {
		old := c.peak.Load()
		if cur <= old {
			break
		}
		if c.peak.CompareAndSwap(old, cur) {
			break
		}
	}
	// When all expected goroutines are in-flight, release the latch so they all
	// return simultaneously.  This lets us observe the maximum concurrency.
	if cur >= c.totalChunks {
		c.releaseOnce.Do(func() { close(c.release) })
	}
	// Block until released.
	<-c.release
	return make([]float32, c.dims), nil
}
func (c *concurrentEmbedder) Name() string    { return "concurrent-fake" }
func (c *concurrentEmbedder) Dimensions() int { return c.dims }

// concurrentUpdateBackend records which chunk IDs were updated, thread-safely.
type concurrentUpdateBackend struct {
	noopBackend
	pendingChunks []*types.Chunk
	mu            sync.Mutex
	updated       []string
}

func (b *concurrentUpdateBackend) GetChunksPendingEmbedding(_ context.Context, _ string, _ int) ([]*types.Chunk, error) {
	return b.pendingChunks, nil
}

func (b *concurrentUpdateBackend) UpdateChunkEmbedding(_ context.Context, id string, _ []float32) (int, error) {
	b.mu.Lock()
	b.updated = append(b.updated, id)
	b.mu.Unlock()
	return 1, nil
}

// deadlineInspectEmbedder records the deadline of the context passed to each Embed
// call.  It does not block — it returns immediately so the test stays fast.
type deadlineInspectEmbedder struct {
	dims      int
	mu        sync.Mutex
	deadlines []time.Time // one per Embed invocation
}

func (d *deadlineInspectEmbedder) Embed(ctx context.Context, _ string) ([]float32, error) {
	dl, ok := ctx.Deadline()
	d.mu.Lock()
	if ok {
		d.deadlines = append(d.deadlines, dl)
	} else {
		d.deadlines = append(d.deadlines, time.Time{}) // zero = no deadline set
	}
	d.mu.Unlock()
	return make([]float32, d.dims), nil
}
func (d *deadlineInspectEmbedder) Name() string    { return "inspect-fake" }
func (d *deadlineInspectEmbedder) Dimensions() int { return d.dims }

// TestRunBatch_EmbedContextHasIndependentDeadline verifies the E5 fix: each Embed
// call receives a context with a deadline ≤ 15s from now, regardless of what
// deadline the parent worker context carries.  We use a 60-second parent so any
// deadline we observe that is ≤ 20s from now must have come from context.Background(),
// not from the parent.
func TestRunBatch_EmbedContextHasIndependentDeadline(t *testing.T) {
	chunk := &types.Chunk{ID: "chunk-dl-check", ChunkText: "deadline check"}
	backend := &concurrentUpdateBackend{
		pendingChunks: []*types.Chunk{chunk},
	}
	embedder := &deadlineInspectEmbedder{dims: 768}

	w := reembed.NewWorker(backend, embedder, "proj", true)

	// Parent context with a 60-second deadline — far beyond the expected 15s embed deadline.
	start := time.Now()
	parentCtx, parentCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer parentCancel()

	w.StartWithContext(parentCtx)

	// Give the worker enough time to call runBatch and process the chunk.
	time.Sleep(200 * time.Millisecond)
	w.Stop()

	embedder.mu.Lock()
	deadlines := append([]time.Time(nil), embedder.deadlines...)
	embedder.mu.Unlock()

	if len(deadlines) == 0 {
		t.Fatal("Embed was never called — worker did not process the chunk")
	}
	for i, dl := range deadlines {
		if dl.IsZero() {
			t.Errorf("Embed call %d: context had no deadline — expected independent 15s deadline", i)
			continue
		}
		// The deadline must be ≤ 16s from start (15s + 1s scheduling slop).
		// If it inherits the 60s parent, it would be ~60s from start.
		elapsed := dl.Sub(start)
		if elapsed > 16*time.Second {
			t.Errorf("Embed call %d: deadline is %v from start, want ≤ 16s — embed is inheriting the parent context deadline", i, elapsed)
		}
	}
}

// TestRunBatch_ConcurrentEmbedding verifies that runBatch dispatches multiple Embed
// calls in parallel rather than one at a time. With 8 chunks and a concurrency limit
// of 8, all 8 goroutines should be in-flight simultaneously before any one returns.
// The concurrentEmbedder acts as a latch: it closes its release channel only when
// all numChunks goroutines have entered Embed, which is impossible if they are
// dispatched sequentially.
func TestRunBatch_ConcurrentEmbedding(t *testing.T) {
	const numChunks = 8

	// Build chunk list.
	chunks := make([]*types.Chunk, numChunks)
	for i := range chunks {
		chunks[i] = &types.Chunk{ID: fmt.Sprintf("chunk-%d", i), ChunkText: "text"}
	}

	embedder := &concurrentEmbedder{
		dims:        768,
		totalChunks: numChunks,
		release:     make(chan struct{}),
	}
	backend := &concurrentUpdateBackend{pendingChunks: chunks}

	w := reembed.NewWorker(backend, embedder, "proj", true)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	w.StartWithContext(ctx)

	// The release channel is closed by the last goroutine to enter Embed.
	// If Embed calls are sequential, the release channel never closes and the
	// embedder blocks forever — the context timeout fires and the test fails.
	// We wait for the release to observe that peak == numChunks before Stop.
	select {
	case <-embedder.release:
		// All goroutines were in-flight simultaneously — concurrent path verified.
	case <-ctx.Done():
		t.Fatal("timed out: embed calls were never all in-flight simultaneously (sequential behavior detected)")
	}

	w.Stop()

	peak := embedder.peak.Load()
	if peak < 2 {
		t.Errorf("expected concurrent Embed calls (peak >= 2), got peak=%d", peak)
	}

	backend.mu.Lock()
	updatedCount := len(backend.updated)
	backend.mu.Unlock()
	if updatedCount != numChunks {
		t.Errorf("expected %d chunk updates, got %d", numChunks, updatedCount)
	}
}
