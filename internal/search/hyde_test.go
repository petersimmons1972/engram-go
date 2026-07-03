package search

import (
	"context"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/entity"
	"github.com/petersimmons1972/engram/internal/types"
)

// --- stub LLM question generator ---

// constQuestionGen always returns the same question string.
type constQuestionGen struct {
	question string
	calls    []string // memory content passed to each call
}

func (g *constQuestionGen) GenerateHydeQuestion(_ context.Context, content string) (string, error) {
	g.calls = append(g.calls, content)
	return g.question, nil
}

// --- stub embedder that satisfies embed.Client ---

// constEmbedder always returns the same constant vector.
type constEmbedder struct {
	name string
	dims int
	vec  []float32
}

func (c *constEmbedder) Name() string    { return c.name }
func (c *constEmbedder) Dimensions() int { return c.dims }
func (c *constEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	v := make([]float32, c.dims)
	copy(v, c.vec)
	return v, nil
}
func (c *constEmbedder) EmbedWithModel(ctx context.Context, text string) ([]float32, string, error) {
	v, err := c.Embed(ctx, text)
	return v, c.name, err
}

var _ embed.Client = (*constEmbedder)(nil)

// --- hydeStubBackend ---

// hydeStubBackend satisfies both db.Backend and hydeBackend.
// It records calls to HyDE methods.
type hydeStubBackend struct {
	noopBackend

	upsertCalls  []hydeUpsertCall
	searchCalls  []hydeSearchCall
	searchResult []db.HydeVectorHit
	searchErr    error
}

type hydeUpsertCall struct {
	memoryID  string
	project   string
	question  string
	embedding []float32
}

type hydeSearchCall struct {
	project string
	vec     []float32
	limit   int
}

func (s *hydeStubBackend) UpsertHydeEmbedding(_ context.Context, memoryID, project, question string, embedding []float32) error {
	s.upsertCalls = append(s.upsertCalls, hydeUpsertCall{
		memoryID:  memoryID,
		project:   project,
		question:  question,
		embedding: embedding,
	})
	return nil
}

func (s *hydeStubBackend) HydeVectorSearch(_ context.Context, project string, vec []float32, limit int) ([]db.HydeVectorHit, error) {
	s.searchCalls = append(s.searchCalls, hydeSearchCall{project: project, vec: vec, limit: limit})
	return s.searchResult, s.searchErr
}

// compile-time checks
var _ hydeBackend = (*hydeStubBackend)(nil)
var _ db.Backend = (*hydeStubBackend)(nil)

// --- noopBackend: satisfies db.Backend with zero-value returns ---

type noopBackend struct{}

// MemPalace interface methods (LME exp #9 widened db.Backend). These stubs
// return zero values; HyDE unit tests do not exercise cluster behavior.
func (noopBackend) StoreMemoryCluster(_ context.Context, _ *db.MemoryCluster) error { return nil }
func (noopBackend) SetMemoryClusterID(_ context.Context, _, _ string) error         { return nil }
func (noopBackend) FindNearestClusters(_ context.Context, _ string, _ []float32, _ int) ([]string, error) {
	return nil, nil
}
func (noopBackend) VectorSearchWithClusters(_ context.Context, _ string, _ []float32, _ int, _ []string, _, _ *time.Time) ([]db.VectorHit, error) {
	return nil, nil
}
func (noopBackend) TableExists(_ context.Context, _ string) (bool, error)     { return false, nil }
func (noopBackend) ColumnExists(_ context.Context, _, _ string) (bool, error) { return false, nil }

func (noopBackend) Close() {}
func (noopBackend) GetMeta(_ context.Context, _, _ string) (string, bool, error) {
	return "", false, nil
}
func (noopBackend) SetMeta(_ context.Context, _, _, _ string) error                  { return nil }
func (noopBackend) SetMetaTx(_ context.Context, _ db.Tx, _, _, _ string) error       { return nil }
func (noopBackend) StoreMemory(_ context.Context, _ *types.Memory) error             { return nil }
func (noopBackend) StoreMemoryTx(_ context.Context, _ db.Tx, _ *types.Memory) error  { return nil }
func (noopBackend) GetMemory(_ context.Context, _ string) (*types.Memory, error)     { return nil, nil }
func (noopBackend) GetMemoryByID(_ context.Context, _ string) (*types.Memory, error) { return nil, nil }
func (noopBackend) GetMemoryByIDInProject(_ context.Context, _, _ string) (*types.Memory, error) {
	return nil, nil
}
func (noopBackend) GetMemoriesByIDs(_ context.Context, _ string, _ []string) ([]*types.Memory, error) {
	return nil, nil
}
func (noopBackend) UpdateMemory(_ context.Context, _ string, _ *string, _ []string, _ *int, _ *float64) (*types.Memory, error) {
	return nil, nil
}
func (noopBackend) DeleteMemory(_ context.Context, _ string) (bool, error) { return false, nil }
func (noopBackend) DeleteMemoryAtomic(_ context.Context, _, _ string, _ bool) (bool, error) {
	return false, nil
}
func (noopBackend) MergeMemoriesAtomic(_ context.Context, _, _, _, _ string) error { return nil }
func (noopBackend) ListMemories(_ context.Context, _ string, _ db.ListOptions) ([]*types.Memory, error) {
	return nil, nil
}
func (noopBackend) TouchMemory(_ context.Context, _ string) error                    { return nil }
func (noopBackend) TouchMemories(_ context.Context, _ []string) error                { return nil }
func (noopBackend) StoreChunks(_ context.Context, _ []*types.Chunk) error            { return nil }
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
func (noopBackend) ChunkHashExists(_ context.Context, _, _ string) (bool, error)       { return false, nil }
func (noopBackend) DeleteChunksForMemory(_ context.Context, _ string) error            { return nil }
func (noopBackend) DeleteChunksForMemoryTx(_ context.Context, _ db.Tx, _ string) error { return nil }
func (noopBackend) DeleteChunksByIDs(_ context.Context, _ []string) (int, error)       { return 0, nil }
func (noopBackend) NullAllEmbeddings(_ context.Context, _ string) (int, error)         { return 0, nil }
func (noopBackend) NullAllEmbeddingsTx(_ context.Context, _ db.Tx, _ string) (int, error) {
	return 0, nil
}
func (noopBackend) CountProjectChunks(_ context.Context, _ string) (int, error) { return 0, nil }
func (noopBackend) GetChunksPendingEmbedding(_ context.Context, _ string, _ int) ([]*types.Chunk, error) {
	return nil, nil
}
func (noopBackend) UpdateChunkEmbedding(_ context.Context, _ string, _ []float32) (int, error) {
	return 0, nil
}
func (noopBackend) VectorSearch(_ context.Context, _ string, _ []float32, _ int) ([]db.VectorHit, error) {
	return nil, nil
}
func (noopBackend) VectorSearchWithDateRange(_ context.Context, _ string, _ []float32, _ int, _, _ *time.Time) ([]db.VectorHit, error) {
	return nil, nil
}
func (noopBackend) SearchChunksWithinMemory(_ context.Context, _ []float32, _ string, _ int) ([]*types.Chunk, error) {
	return nil, nil
}
func (noopBackend) ChunkEmbeddingDistance(_ context.Context, _, _ string) (float64, error) {
	return 2.0, nil
}
func (noopBackend) UpdateChunkLastMatched(_ context.Context, _ string) error          { return nil }
func (noopBackend) GetPendingEmbeddingCount(_ context.Context, _ string) (int, error) { return 0, nil }
func (noopBackend) EnqueueChunkLeases(_ context.Context, _ []string) error            { return nil }
func (noopBackend) StoreRelationship(_ context.Context, _ *types.Relationship) error { return nil }
func (noopBackend) StoreRelationshipTx(_ context.Context, _ db.Tx, _ *types.Relationship) error {
	return nil
}
func (noopBackend) SoftDeleteMemoryTx(_ context.Context, _ db.Tx, _, _, _ string) (bool, error) {
	return false, nil
}
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
func (noopBackend) StoreRetrievalEvent(_ context.Context, _ *types.RetrievalEvent) error { return nil }
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
func (noopBackend) DeleteProject(_ context.Context, _ string) error { return nil }
func (noopBackend) SetProjectTTL(_ context.Context, _ string, _ time.Time, _ *time.Time) error {
	return nil
}
func (noopBackend) ListExpiredProjects(_ context.Context, _ string, _ time.Time, _ int) ([]string, error) {
	return nil, nil
}
func (noopBackend) FTSSearch(_ context.Context, _, _ string, _ int, _, _ *time.Time) ([]db.FTSResult, error) {
	return nil, nil
}
func (noopBackend) RebuildFTS(_ context.Context) error                               { return nil }
func (noopBackend) GetStats(_ context.Context, _ string) (*types.MemoryStats, error) { return nil, nil }
func (noopBackend) ListAllProjects(_ context.Context) ([]string, error)              { return nil, nil }
func (noopBackend) GetAllMemoryIDs(_ context.Context, _ string) (map[string]struct{}, error) {
	return nil, nil
}
func (noopBackend) GetMemoryTypeMap(_ context.Context, _ string) (map[string]string, error) {
	return nil, nil
}
func (noopBackend) GetMemoriesPendingSummary(_ context.Context, _ string, _ int) ([]db.IDContent, error) {
	return nil, nil
}
func (noopBackend) StoreSummary(_ context.Context, _, _ string) error               { return nil }
func (noopBackend) GetPendingSummaryCount(_ context.Context, _ string) (int, error) { return 0, nil }
func (noopBackend) ClearSummaries(_ context.Context, _ string) (int, error)         { return 0, nil }
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
func (noopBackend) StoreDocument(_ context.Context, _, _ string) (string, error) { return "", nil }
func (noopBackend) GetDocument(_ context.Context, _ string) (string, error)      { return "", nil }
func (noopBackend) DeleteDocument(_ context.Context, _ string) (bool, error)     { return false, nil }
func (noopBackend) DeleteDocumentTx(_ context.Context, _ db.Tx, _ string) (bool, error) {
	return false, nil
}
func (noopBackend) DeleteOrphanedDocumentTx(_ context.Context, _ db.Tx, _ string) (bool, error) {
	return false, nil
}
func (noopBackend) SetMemoryDocumentID(_ context.Context, _, _ string) error         { return nil }
func (noopBackend) UpsertEntity(_ context.Context, _ *entity.Entity) (string, error) { return "", nil }
func (noopBackend) GetEntitiesByProject(_ context.Context, _ string) ([]entity.Entity, error) {
	return nil, nil
}
func (noopBackend) EnqueueExtractionJob(_ context.Context, _, _ string) error { return nil }
func (noopBackend) ClaimExtractionJobs(_ context.Context, _ string, _ int) ([]db.ExtractionJob, error) {
	return nil, nil
}
func (noopBackend) CompleteExtractionJob(_ context.Context, _ string, _ error) error { return nil }
func (noopBackend) Begin(_ context.Context) (db.Tx, error)                           { return nil, nil }

var _ db.Backend = (*noopBackend)(nil)

// --- Tests ---

// TestHydeIndexer_WriteTime_CallsUpsert verifies that IndexHydeForMemory
// generates a question from the memory content, embeds it, and calls
// UpsertHydeEmbedding with the correct arguments.
func TestHydeIndexer_WriteTime_CallsUpsert(t *testing.T) {
	stub := &hydeStubBackend{}
	emb := &constEmbedder{name: "test-model", dims: 3, vec: []float32{0.1, 0.2, 0.3}}
	gen := &constQuestionGen{question: "What is the user's preferred coffee?"}

	idx := NewHydeIndexer(stub, emb, gen)
	mem := &types.Memory{ID: "mem-001", Content: "I prefer oat milk flat whites", Project: "proj"}

	if err := idx.IndexHydeForMemory(context.Background(), mem); err != nil {
		t.Fatalf("IndexHydeForMemory: %v", err)
	}

	if len(stub.upsertCalls) != 1 {
		t.Fatalf("want 1 upsert call, got %d", len(stub.upsertCalls))
	}
	got := stub.upsertCalls[0]
	if got.memoryID != "mem-001" {
		t.Errorf("upsert memoryID = %q, want mem-001", got.memoryID)
	}
	if got.question != "What is the user's preferred coffee?" {
		t.Errorf("upsert question = %q, want the generated question", got.question)
	}
	if len(got.embedding) != 3 {
		t.Errorf("upsert embedding len = %d, want 3", len(got.embedding))
	}
	if len(gen.calls) != 1 || gen.calls[0] != "I prefer oat milk flat whites" {
		t.Errorf("question generator called with %v, want [%q]", gen.calls, mem.Content)
	}
}

// TestHydeIndexer_EmptyContent skips indexing without error.
func TestHydeIndexer_EmptyContent(t *testing.T) {
	stub := &hydeStubBackend{}
	emb := &constEmbedder{name: "test-model", dims: 3, vec: []float32{0.1, 0.2, 0.3}}
	gen := &constQuestionGen{question: "irrelevant"}

	idx := NewHydeIndexer(stub, emb, gen)
	mem := &types.Memory{ID: "mem-002", Content: "", Project: "proj"}

	if err := idx.IndexHydeForMemory(context.Background(), mem); err != nil {
		t.Fatalf("IndexHydeForMemory on empty content: %v", err)
	}
	if len(stub.upsertCalls) != 0 {
		t.Errorf("want 0 upserts for empty content, got %d", len(stub.upsertCalls))
	}
}

// TestHydeScores_VocabMismatch is the core HyDE hypothesis test.
//
// A memory whose raw text does NOT contain the query vocabulary still
// surfaces via HyDE because the hypothetical question shares vocabulary
// with the query (vocabulary-mismatch failure class in LME).
func TestHydeScores_VocabMismatch(t *testing.T) {
	memID := "mem-vocab-mismatch"
	stub := &hydeStubBackend{
		searchResult: []db.HydeVectorHit{
			{MemoryID: memID, Distance: 0.05, Question: "What hot beverage does the user prefer?"},
		},
	}
	emb := &constEmbedder{name: "test-model", dims: 3, vec: []float32{0.9, 0.1, 0.0}}

	hs := NewHydeScorer(stub, emb)
	results, err := hs.Score(context.Background(), "What hot beverage does the user prefer?", "proj", 10)
	if err != nil {
		t.Fatalf("HydeScorer.Score: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("want >= 1 result; HyDE failed to surface the memory")
	}
	if results[0].MemoryID != memID {
		t.Errorf("first result memoryID = %q, want %q", results[0].MemoryID, memID)
	}
	// distance=0.05 → score = 1 - 0.05 = 0.95
	if results[0].Score < 0.9 {
		t.Errorf("HyDE score = %.3f, want >= 0.9 for high-similarity hit", results[0].Score)
	}
}

// TestHydeScores_FlagOff verifies that when the backend does NOT implement
// hydeBackend (flag off or test stub without the optional interface), Score
// returns an empty slice without error.
func TestHydeScores_FlagOff(t *testing.T) {
	noop := &noopBackend{}
	emb := &constEmbedder{name: "test-model", dims: 3, vec: []float32{0.5, 0.5, 0.0}}

	hs := NewHydeScorer(noop, emb)
	results, err := hs.Score(context.Background(), "any query", "proj", 10)
	if err != nil {
		t.Fatalf("HydeScorer.Score with no hydeBackend: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("want 0 results when hydeBackend absent, got %d", len(results))
	}
}

// TestRRFMerge verifies that Reciprocal Rank Fusion correctly promotes a
// memory that scores well on HyDE but poorly on raw chunk cosine.
func TestRRFMerge(t *testing.T) {
	rawScores := map[string]float64{
		"mem-a": 0.9, // high raw similarity
		"mem-b": 0.3, // low raw similarity — vocabulary mismatch
	}
	hydeScores := []HydeScore{
		{MemoryID: "mem-b", Score: 0.95}, // mem-b ranks #1 on HyDE
		{MemoryID: "mem-a", Score: 0.50},
	}
	merged := mergeRRF(rawScores, hydeScores, 60)

	scoreA, scoreB := merged["mem-a"], merged["mem-b"]
	if scoreA == 0 || scoreB == 0 {
		t.Fatalf("expected both mem-a and mem-b in merged map; a=%.3f b=%.3f", scoreA, scoreB)
	}
	// With k=60 RRF the ranks dominate over raw magnitudes.
	// mem-b: HyDE rank=1, raw rank=2  → RRF ≈ 1/61 + 1/62 ≈ 0.0326
	// mem-a: HyDE rank=2, raw rank=1  → RRF ≈ 1/62 + 1/61 ≈ 0.0326
	// Near-tie is the expected outcome; mem-b should not be much worse.
	if scoreB < scoreA*0.75 {
		t.Errorf("RRF should boost HyDE-top mem-b near mem-a; a=%.4f b=%.4f", scoreA, scoreB)
	}
}

// TestRRFMerge_EmptyHyde verifies raw ordering is preserved when no HyDE hits.
func TestRRFMerge_EmptyHyde(t *testing.T) {
	rawScores := map[string]float64{"mem-x": 0.8, "mem-y": 0.6}
	merged := mergeRRF(rawScores, nil, 60)
	if len(merged) != 2 {
		t.Fatalf("want 2 merged entries, got %d", len(merged))
	}
	if merged["mem-x"] <= merged["mem-y"] {
		t.Errorf("raw ordering not preserved with empty HyDE: x=%.4f y=%.4f",
			merged["mem-x"], merged["mem-y"])
	}
}
