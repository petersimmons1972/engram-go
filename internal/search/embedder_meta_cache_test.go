package search

// Tests for the process-scoped embedder metadata cache (#929).
//
// These tests are in package search (internal) so they can call the
// unexported checkEmbedderMeta method directly and inspect cache fields.
//
// The tests use an in-memory backend stub with call counting to verify that
// the DB is NOT hit on cache-warm paths.

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/entity"
	"github.com/petersimmons1972/engram/internal/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// cacheTestEmbedder is a minimal embed.Client for metadata cache tests.
type cacheTestEmbedder struct {
	name string
	dims int
}

func (e *cacheTestEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, e.dims), nil
}
func (e *cacheTestEmbedder) EmbedWithModel(ctx context.Context, text string) ([]float32, string, error) {
	v, err := e.Embed(ctx, text)
	return v, e.name, err
}
func (e *cacheTestEmbedder) Name() string    { return e.name }
func (e *cacheTestEmbedder) Dimensions() int { return e.dims }

var _ embed.Client = (*cacheTestEmbedder)(nil)

// stubTx is a no-op db.Tx used by cacheMetaBackend.Begin.
type stubTx struct {
	committed bool
	meta      map[string]string // reference to the backend's meta store
	mu        *sync.Mutex
	project   string
}

func (t *stubTx) Commit(_ context.Context) error {
	t.committed = true
	return nil
}
func (t *stubTx) Rollback(_ context.Context) error { return nil }

// cacheMetaBackend is a minimal db.Backend implementation that:
//   - stores project_meta entries in memory
//   - counts GetMeta calls (to verify cache hit behaviour)
//   - implements Begin/Tx so MigrateEmbedder's gateway path works
type cacheMetaBackend struct {
	mu           sync.Mutex
	meta         map[string]string // key: "project:key"
	getMetaCalls atomic.Int64
}

func newCacheMetaBackend() *cacheMetaBackend {
	return &cacheMetaBackend{meta: make(map[string]string)}
}

func metaKey(project, key string) string { return project + ":" + key }

func (b *cacheMetaBackend) GetMeta(_ context.Context, project, key string) (string, bool, error) {
	b.getMetaCalls.Add(1)
	b.mu.Lock()
	defer b.mu.Unlock()
	v, ok := b.meta[metaKey(project, key)]
	return v, ok, nil
}

func (b *cacheMetaBackend) SetMeta(_ context.Context, project, key, value string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.meta[metaKey(project, key)] = value
	return nil
}

func (b *cacheMetaBackend) SetMetaTx(_ context.Context, tx db.Tx, project, key, value string) error {
	// Write directly for simplicity; the stubTx.Commit is a no-op marker.
	b.mu.Lock()
	defer b.mu.Unlock()
	b.meta[metaKey(project, key)] = value
	return nil
}

func (b *cacheMetaBackend) Begin(_ context.Context) (db.Tx, error) {
	return &stubTx{meta: b.meta, mu: &b.mu}, nil
}

// Satisfy the rest of db.Backend with no-ops. ──────────────────────────────

func (b *cacheMetaBackend) Close() {}

func (b *cacheMetaBackend) StoreMemory(_ context.Context, _ *types.Memory) error { return nil }
func (b *cacheMetaBackend) StoreMemoryTx(_ context.Context, _ db.Tx, _ *types.Memory) error {
	return nil
}
func (b *cacheMetaBackend) GetMemory(_ context.Context, _ string) (*types.Memory, error) {
	return nil, nil
}
func (b *cacheMetaBackend) GetMemoryByID(_ context.Context, _ string) (*types.Memory, error) {
	return nil, nil
}
func (b *cacheMetaBackend) GetMemoriesByIDs(_ context.Context, _ string, _ []string) ([]*types.Memory, error) {
	return nil, nil
}
func (b *cacheMetaBackend) UpdateMemory(_ context.Context, _ string, _ *string, _ []string, _ *int, _ *float64) (*types.Memory, error) {
	return nil, nil
}
func (b *cacheMetaBackend) DeleteMemory(_ context.Context, _ string) (bool, error) { return false, nil }
func (b *cacheMetaBackend) DeleteMemoryAtomic(_ context.Context, _, _ string, _ bool) (bool, error) {
	return false, nil
}
func (b *cacheMetaBackend) MergeMemoriesAtomic(_ context.Context, _, _, _, _ string) error {
	return nil
}
func (b *cacheMetaBackend) ListMemories(_ context.Context, _ string, _ db.ListOptions) ([]*types.Memory, error) {
	return nil, nil
}
func (b *cacheMetaBackend) TouchMemory(_ context.Context, _ string) error  { return nil }
func (b *cacheMetaBackend) TouchMemories(_ context.Context, _ []string) error { return nil }

func (b *cacheMetaBackend) StoreChunks(_ context.Context, _ []*types.Chunk) error { return nil }
func (b *cacheMetaBackend) StoreChunksTx(_ context.Context, _ db.Tx, _ []*types.Chunk) error {
	return nil
}
func (b *cacheMetaBackend) GetChunksForMemory(_ context.Context, _ string) ([]*types.Chunk, error) {
	return nil, nil
}
func (b *cacheMetaBackend) GetAllChunksWithEmbeddings(_ context.Context, _ string, _ int) ([]*types.Chunk, error) {
	return nil, nil
}
func (b *cacheMetaBackend) GetAllChunkTexts(_ context.Context, _ string, _ int) ([]string, error) {
	return nil, nil
}
func (b *cacheMetaBackend) GetChunksForMemories(_ context.Context, _ []string) ([]*types.Chunk, error) {
	return nil, nil
}
func (b *cacheMetaBackend) ChunkHashExists(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
func (b *cacheMetaBackend) DeleteChunksForMemory(_ context.Context, _ string) error { return nil }
func (b *cacheMetaBackend) DeleteChunksForMemoryTx(_ context.Context, _ db.Tx, _ string) error {
	return nil
}
func (b *cacheMetaBackend) DeleteChunksByIDs(_ context.Context, _ []string) (int, error) {
	return 0, nil
}
func (b *cacheMetaBackend) NullAllEmbeddings(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (b *cacheMetaBackend) NullAllEmbeddingsTx(_ context.Context, _ db.Tx, _ string) (int, error) {
	return 0, nil
}
func (b *cacheMetaBackend) GetChunksPendingEmbedding(_ context.Context, _ string, _ int) ([]*types.Chunk, error) {
	return nil, nil
}
func (b *cacheMetaBackend) UpdateChunkEmbedding(_ context.Context, _ string, _ []float32) (int, error) {
	return 0, nil
}
func (b *cacheMetaBackend) ChunkEmbeddingDistance(_ context.Context, _, _ string) (float64, error) {
	return 2.0, nil
}
func (b *cacheMetaBackend) GetPendingEmbeddingCount(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (b *cacheMetaBackend) EnqueueChunkLeases(_ context.Context, _ []string) error { return nil }
func (b *cacheMetaBackend) UpdateChunkLastMatched(_ context.Context, _ string) error {
	return nil
}
func (b *cacheMetaBackend) SearchChunksWithinMemory(_ context.Context, _ []float32, _ string, _ int) ([]*types.Chunk, error) {
	return nil, nil
}

func (b *cacheMetaBackend) VectorSearch(_ context.Context, _ string, _ []float32, _ int) ([]db.VectorHit, error) {
	return nil, nil
}
func (b *cacheMetaBackend) VectorSearchWithDateRange(_ context.Context, _ string, _ []float32, _ int, _, _ *time.Time) ([]db.VectorHit, error) {
	return nil, nil
}
func (b *cacheMetaBackend) FTSSearch(_ context.Context, _ string, _ string, _ int, _, _ *time.Time) ([]db.FTSResult, error) {
	return nil, nil
}
func (b *cacheMetaBackend) GetRelationships(_ context.Context, _, _ string) ([]types.Relationship, error) {
	return nil, nil
}
func (b *cacheMetaBackend) GetRelationshipsBatch(_ context.Context, _ string, _ []string) (map[string][]types.Relationship, error) {
	return nil, nil
}
func (b *cacheMetaBackend) StoreRelationship(_ context.Context, _ *types.Relationship) error {
	return nil
}
func (b *cacheMetaBackend) GetConnected(_ context.Context, _ string, _ int) ([]db.ConnectedResult, error) {
	return nil, nil
}
func (b *cacheMetaBackend) BoostEdgesForMemory(_ context.Context, _ string, _ float64) (int, error) {
	return 0, nil
}
func (b *cacheMetaBackend) DecayEdgesForMemory(_ context.Context, _ string, _ float64) (int, error) {
	return 0, nil
}
func (b *cacheMetaBackend) GetConnectionCount(_ context.Context, _, _ string) (int, error) {
	return 0, nil
}
func (b *cacheMetaBackend) DecayAllEdges(_ context.Context, _ string, _, _ float64) (int, int, error) {
	return 0, 0, nil
}
func (b *cacheMetaBackend) DeleteRelationshipsForMemory(_ context.Context, _ string) error {
	return nil
}

func (b *cacheMetaBackend) GetMemoryHistory(_ context.Context, _, _ string) ([]*types.MemoryVersion, error) {
	return nil, nil
}
func (b *cacheMetaBackend) SoftDeleteMemory(_ context.Context, _, _, _ string) (bool, error) {
	return false, nil
}
func (b *cacheMetaBackend) GetMemoriesAsOf(_ context.Context, _ string, _ time.Time, _ int) ([]*types.Memory, error) {
	return nil, nil
}

func (b *cacheMetaBackend) StoreRetrievalEvent(_ context.Context, _ *types.RetrievalEvent) error {
	return nil
}
func (b *cacheMetaBackend) GetRetrievalEvent(_ context.Context, _ string) (*types.RetrievalEvent, error) {
	return nil, nil
}
func (b *cacheMetaBackend) RecordFeedback(_ context.Context, _ string, _ []string) error {
	return nil
}
func (b *cacheMetaBackend) RecordFeedbackWithClass(_ context.Context, _ string, _ []string, _ string) error {
	return nil
}
func (b *cacheMetaBackend) AggregateMemories(_ context.Context, _, _, _ string, _ int) ([]types.AggregateRow, error) {
	return nil, nil
}
func (b *cacheMetaBackend) AggregateFailureClasses(_ context.Context, _ string, _ int) ([]types.AggregateRow, error) {
	return nil, nil
}
func (b *cacheMetaBackend) IncrementTimesRetrieved(_ context.Context, _ []string) error { return nil }
func (b *cacheMetaBackend) UpdateDynamicImportance(_ context.Context, _ string, _, _ float64) error {
	return nil
}
func (b *cacheMetaBackend) SetNextReviewAt(_ context.Context, _ string, _ time.Time) error {
	return nil
}
func (b *cacheMetaBackend) DecayStaleImportance(_ context.Context, _ string, _ float64) (int, error) {
	return 0, nil
}
func (b *cacheMetaBackend) PruneStaleMemories(_ context.Context, _ string, _ float64, _ int) (int, error) {
	return 0, nil
}
func (b *cacheMetaBackend) PruneColdDocuments(_ context.Context, _ string, _ float64, _ int) (int, error) {
	return 0, nil
}
func (b *cacheMetaBackend) RebuildFTS(_ context.Context) error { return nil }
func (b *cacheMetaBackend) GetStats(_ context.Context, _ string) (*types.MemoryStats, error) {
	return nil, nil
}
func (b *cacheMetaBackend) ListAllProjects(_ context.Context) ([]string, error) { return nil, nil }
func (b *cacheMetaBackend) GetAllMemoryIDs(_ context.Context, _ string) (map[string]struct{}, error) {
	return nil, nil
}
func (b *cacheMetaBackend) GetMemoryTypeMap(_ context.Context, _ string) (map[string]string, error) {
	return nil, nil
}
func (b *cacheMetaBackend) GetMemoriesPendingSummary(_ context.Context, _ string, _ int) ([]db.IDContent, error) {
	return nil, nil
}
func (b *cacheMetaBackend) StoreSummary(_ context.Context, _, _ string) error { return nil }
func (b *cacheMetaBackend) GetPendingSummaryCount(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (b *cacheMetaBackend) ClearSummaries(_ context.Context, _ string) (int, error) { return 0, nil }
func (b *cacheMetaBackend) GetMemoriesMissingHash(_ context.Context, _ string, _ int) ([]db.IDContent, error) {
	return nil, nil
}
func (b *cacheMetaBackend) UpdateMemoryHash(_ context.Context, _, _ string) error { return nil }
func (b *cacheMetaBackend) GetIntegrityStats(_ context.Context, _ string) (db.IntegrityStats, error) {
	return db.IntegrityStats{}, nil
}

func (b *cacheMetaBackend) StartEpisode(_ context.Context, _, _ string) (*types.Episode, error) {
	return nil, nil
}
func (b *cacheMetaBackend) EndEpisode(_ context.Context, _, _ string) error { return nil }
func (b *cacheMetaBackend) ListEpisodes(_ context.Context, _ string, _ int) ([]*types.Episode, error) {
	return nil, nil
}
func (b *cacheMetaBackend) RecallEpisode(_ context.Context, _ string) ([]*types.Memory, error) {
	return nil, nil
}
func (b *cacheMetaBackend) CloseStaleEpisodes(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}
func (b *cacheMetaBackend) ExistsWithContentHash(_ context.Context, _ string, _ string) (bool, error) {
	return false, nil
}
func (b *cacheMetaBackend) StoreDocument(_ context.Context, _, _ string) (string, error) {
	return "", nil
}
func (b *cacheMetaBackend) DeleteDocument(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (b *cacheMetaBackend) DeleteDocumentTx(_ context.Context, _ db.Tx, _ string) (bool, error) {
	return false, nil
}
func (b *cacheMetaBackend) DeleteOrphanedDocumentTx(_ context.Context, _ db.Tx, _ string) (bool, error) {
	return false, nil
}
func (b *cacheMetaBackend) GetDocument(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (b *cacheMetaBackend) SetMemoryDocumentID(_ context.Context, _, _ string) error { return nil }

func (b *cacheMetaBackend) UpsertEntity(_ context.Context, _ *entity.Entity) (string, error) {
	return "", nil
}
func (b *cacheMetaBackend) GetEntitiesByProject(_ context.Context, _ string) ([]entity.Entity, error) {
	return nil, nil
}
func (b *cacheMetaBackend) EnqueueExtractionJob(_ context.Context, _, _ string) error { return nil }
func (b *cacheMetaBackend) ClaimExtractionJobs(_ context.Context, _ string, _ int) ([]db.ExtractionJob, error) {
	return nil, nil
}
func (b *cacheMetaBackend) CompleteExtractionJob(_ context.Context, _ string, _ error) error {
	return nil
}
func (b *cacheMetaBackend) DeleteProject(_ context.Context, _ string) error { return nil }
func (b *cacheMetaBackend) SetProjectTTL(_ context.Context, _ string, _ time.Time, _ *time.Time) error {
	return nil
}
func (b *cacheMetaBackend) ListExpiredProjects(_ context.Context, _ string, _ time.Time, _ int) ([]string, error) {
	return nil, nil
}

// compile-time check: cacheMetaBackend must satisfy db.Backend.
var _ db.Backend = (*cacheMetaBackend)(nil)

// ── noopGateway satisfies the embedGateway interface for testing ──────────────

type noopGateway struct{}

func (g *noopGateway) Enqueue(_ []string) {}

// ── newCacheTestEngine builds a SearchEngine backed by cacheMetaBackend ───────

func newCacheTestEngine(t *testing.T, backend *cacheMetaBackend, emb embed.Client) *SearchEngine {
	t.Helper()
	ctx := context.Background()
	eng := New(ctx, backend, emb, "test-project", "http://localhost:11434", "unused", false, nil, 0)
	// Use the embed-gateway path in MigrateEmbedder to avoid calling
	// embed.NewLiteLLMClient (which requires a live Ollama server).
	eng.SetEmbedGateway(&noopGateway{})
	return eng
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestCheckEmbedderMeta_CacheHitAvoidsDBC verifies that the second call to
// checkEmbedderMeta (after cache is warm) issues zero DB GetMeta round-trips.
func TestCheckEmbedderMeta_CacheHitAvoidsDBC(t *testing.T) {
	backend := newCacheMetaBackend()
	emb := &cacheTestEmbedder{name: "BAAI/bge-m3", dims: 1024}
	eng := newCacheTestEngine(t, backend, emb)
	defer eng.Close()

	ctx := context.Background()

	// First call: cache cold — must hit DB to read/register metadata.
	if err := eng.checkEmbedderMeta(ctx); err != nil {
		t.Fatalf("first checkEmbedderMeta: %v", err)
	}
	callsAfterFirst := backend.getMetaCalls.Load()
	if callsAfterFirst == 0 {
		t.Fatal("expected DB calls on first invocation (cache cold), got 0")
	}

	// Second call: cache warm — must NOT hit DB.
	if err := eng.checkEmbedderMeta(ctx); err != nil {
		t.Fatalf("second checkEmbedderMeta: %v", err)
	}
	callsAfterSecond := backend.getMetaCalls.Load()
	if callsAfterSecond != callsAfterFirst {
		t.Fatalf("expected 0 DB calls on cache-hit path, got %d extra call(s)",
			callsAfterSecond-callsAfterFirst)
	}
}

// TestCheckEmbedderMeta_CacheInvalidatedAfterMigrate_GatewayPath verifies that
// calling MigrateEmbedder (via the embed-gateway early-return path) invalidates
// the cache, so the next checkEmbedderMeta reads fresh values from DB.
func TestCheckEmbedderMeta_CacheInvalidatedAfterMigrate_GatewayPath(t *testing.T) {
	backend := newCacheMetaBackend()
	emb := &cacheTestEmbedder{name: "BAAI/bge-m3", dims: 1024}
	eng := newCacheTestEngine(t, backend, emb)
	defer eng.Close()

	ctx := context.Background()

	// Warm the cache.
	if err := eng.checkEmbedderMeta(ctx); err != nil {
		t.Fatalf("warming checkEmbedderMeta: %v", err)
	}
	eng.embedderMetaMu.RLock()
	cacheValid := eng.embedderMetaCacheValid
	eng.embedderMetaMu.RUnlock()
	if !cacheValid {
		t.Fatal("cache should be valid after first successful checkEmbedderMeta")
	}

	// Migrate to new model (gateway path — no LiteLLM call needed).
	if _, err := eng.MigrateEmbedder(ctx, "new-model-v2"); err != nil {
		t.Fatalf("MigrateEmbedder: %v", err)
	}

	// Cache must be invalidated.
	eng.embedderMetaMu.RLock()
	cacheValidAfter := eng.embedderMetaCacheValid
	eng.embedderMetaMu.RUnlock()
	if cacheValidAfter {
		t.Fatal("cache should be invalidated after MigrateEmbedder")
	}
}

// TestCheckEmbedderMeta_PostMigrationReadsFreshValues verifies that after
// MigrateEmbedder invalidates the cache, calling checkEmbedderMeta again
// re-reads from DB and does NOT return a stale name/dims error.
//
// Specifically: after migration the DB now holds embedder_name=new-model-v2
// and embedding_migration_in_progress=true.  The engine's embedder is still
// the old one — but checkEmbedderMeta should see the migration flag and
// skip the dimension check (returning nil, not an error).
func TestCheckEmbedderMeta_PostMigrationReadsFreshValues(t *testing.T) {
	backend := newCacheMetaBackend()
	emb := &cacheTestEmbedder{name: "BAAI/bge-m3", dims: 1024}
	eng := newCacheTestEngine(t, backend, emb)
	defer eng.Close()

	ctx := context.Background()

	// Warm cache with original embedder metadata.
	if err := eng.checkEmbedderMeta(ctx); err != nil {
		t.Fatalf("initial checkEmbedderMeta: %v", err)
	}

	// Migrate (gateway path writes embedder_name=new-model and
	// embedding_migration_in_progress=true, then invalidates cache).
	const newModel = "BAAI/bge-m3-v2"
	if _, err := eng.MigrateEmbedder(ctx, newModel); err != nil {
		t.Fatalf("MigrateEmbedder: %v", err)
	}

	// Temporarily make the in-memory embedder appear to match the new name
	// so checkEmbedderMeta's name-mismatch check passes (simulating the
	// engine having been updated to the new model).
	eng.embedMu.Lock()
	eng.embedder = &cacheTestEmbedder{name: newModel, dims: 2048}
	eng.embedMu.Unlock()

	// checkEmbedderMeta must return nil — the migration flag is set so the
	// dimension mismatch (1024 vs 2048) is expected and skipped.
	if err := eng.checkEmbedderMeta(ctx); err != nil {
		t.Fatalf("checkEmbedderMeta post-migration: expected nil, got %v", err)
	}

	// Cache should remain invalid while migration is in progress.
	eng.embedderMetaMu.RLock()
	cacheValid := eng.embedderMetaCacheValid
	eng.embedderMetaMu.RUnlock()
	if cacheValid {
		t.Fatal("cache should remain invalid while embedding_migration_in_progress=true")
	}
}

// TestCheckEmbedderMeta_ConcurrentCacheHits exercises the RLock fast path
// under concurrent load to confirm the race detector sees no data races.
func TestCheckEmbedderMeta_ConcurrentCacheHits(t *testing.T) {
	backend := newCacheMetaBackend()
	emb := &cacheTestEmbedder{name: "BAAI/bge-m3", dims: 1024}
	eng := newCacheTestEngine(t, backend, emb)
	defer eng.Close()

	ctx := context.Background()

	// Warm the cache with one sequential call first.
	if err := eng.checkEmbedderMeta(ctx); err != nil {
		t.Fatalf("warm: %v", err)
	}

	const goroutines = 50
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := eng.checkEmbedderMeta(ctx); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent checkEmbedderMeta: %v", err)
	}
}
