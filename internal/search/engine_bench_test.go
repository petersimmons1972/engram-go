// Benchmarks for the SearchEngine hot paths.
//
// These benchmarks use a stub backend (no real PostgreSQL) to isolate
// in-process allocations and CPU cost from network/DB latency.
//
// Run all benchmarks:
//
//	go test -bench=. -benchtime=5s -count=3 -benchmem ./internal/search/...
//
// Capture baseline before any optimization:
//
//	go test -bench=. -benchtime=5s -count=3 -benchmem ./internal/search/... | tee bench_before.txt
//
// After implementing optimizations, compare with benchstat:
//
//	benchstat bench_before.txt bench_after.txt
package search_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
)

// ── Synthetic data helpers ───────────────────────────────────────────────────

func syntheticMemoryID(i int) string { return fmt.Sprintf("mem-%04d", i) }
func syntheticChunkID(i, j int) string {
	return fmt.Sprintf("chunk-%04d-%02d", i, j)
}

// syntheticVecHits produces numMemories*chunksPer VectorHit entries with
// different distances per chunk so the best-hit logic must actually evaluate.
func syntheticVecHits(numMemories, chunksPer int) []db.VectorHit {
	hits := make([]db.VectorHit, 0, numMemories*chunksPer)
	heading := "Introduction"
	for i := 0; i < numMemories; i++ {
		for j := 0; j < chunksPer; j++ {
			hits = append(hits, db.VectorHit{
				ChunkID:        syntheticChunkID(i, j),
				MemoryID:       syntheticMemoryID(i),
				Distance:       float64(j+1) * 0.05, // 0.05, 0.10, 0.15 → best is first
				ChunkText:      fmt.Sprintf("chunk text for memory %d chunk %d", i, j),
				ChunkIndex:     j,
				SectionHeading: &heading,
			})
		}
	}
	return hits
}

func syntheticMemories(ids []string) []*types.Memory {
	mems := make([]*types.Memory, len(ids))
	for i, id := range ids {
		now := time.Now()
		imp := 2
		dynImp := 2.5
		mems[i] = &types.Memory{
			ID:               id,
			Content:          fmt.Sprintf("Memory content for %s: some meaningful text about a topic.", id),
			Importance:       imp,
			DynamicImportance: &dynImp,
			LastAccessed:     now,
			CreatedAt:        now,
		}
	}
	return mems
}

func syntheticRelationships(memID string, count int) []types.Relationship {
	rels := make([]types.Relationship, count)
	for i := range rels {
		str := 0.8
		rels[i] = types.Relationship{
			SourceID: memID,
			TargetID: syntheticMemoryID(100 + i), // neighbor memory IDs (outside topK)
			RelType:  "relates_to",
			Strength: str,
		}
	}
	return rels
}

// ── stubBackend ──────────────────────────────────────────────────────────────

// stubBackend is a no-op db.Backend implementation used for benchmarking.
// It returns deterministic synthetic data for the methods called by RecallWithOpts.
// All other methods are safe no-ops.
type stubBackend struct {
	numResults    int // how many memories to return per VectorSearch/FTS call
	relsPerResult int // how many relationships to return per GetRelationships call
}

func newStubBackend(numResults, relsPerResult int) *stubBackend {
	return &stubBackend{numResults: numResults, relsPerResult: relsPerResult}
}

// ── db.Backend interface — methods used by RecallWithOpts ────────────────────

func (s *stubBackend) VectorSearch(_ context.Context, _ string, _ []float32, limit int) ([]db.VectorHit, error) {
	n := s.numResults * 3 // engine requests topK*3
	if limit < n {
		n = limit
	}
	// Return hits: numResults unique memories, 3 chunks each.
	return syntheticVecHits(s.numResults, 3)[:n], nil
}

func (s *stubBackend) FTSSearch(_ context.Context, _ string, _ string, limit int, _, _ *time.Time) ([]db.FTSResult, error) {
	// Return half the memories via FTS with varying scores.
	half := s.numResults / 2
	if half == 0 {
		half = 1
	}
	results := make([]db.FTSResult, half)
	for i := range results {
		id := syntheticMemoryID(i)
		now := time.Now()
		results[i] = db.FTSResult{
			Memory: &types.Memory{ID: id, Content: "fts content", LastAccessed: now, CreatedAt: now},
			Score:  float64(half-i) * 0.1,
		}
	}
	return results, nil
}

func (s *stubBackend) GetMemoriesByIDs(_ context.Context, _ string, ids []string) ([]*types.Memory, error) {
	return syntheticMemories(ids), nil
}

func (s *stubBackend) GetRelationships(_ context.Context, _, memID string) ([]types.Relationship, error) {
	return syntheticRelationships(memID, s.relsPerResult), nil
}

func (s *stubBackend) GetRelationshipsBatch(_ context.Context, _ string, ids []string) (map[string][]types.Relationship, error) {
	result := make(map[string][]types.Relationship, len(ids))
	for _, id := range ids {
		result[id] = syntheticRelationships(id, s.relsPerResult)
	}
	return result, nil
}

func (s *stubBackend) TouchMemories(_ context.Context, _ []string) error { return nil }

func (s *stubBackend) UpdateChunkLastMatched(_ context.Context, _ string) error { return nil }

func (s *stubBackend) SearchChunksWithinMemory(_ context.Context, _ []float32, _ string, _ int) ([]*types.Chunk, error) {
	return nil, nil
}

func (s *stubBackend) GetMeta(_ context.Context, _, _ string) (string, bool, error) {
	return "", false, nil
}

func (s *stubBackend) SetMeta(_ context.Context, _, _, _ string) error { return nil }

// ── Safe no-ops for all remaining db.Backend methods ────────────────────────

func (s *stubBackend) Close() {}

func (s *stubBackend) SetMetaTx(_ context.Context, _ db.Tx, _, _, _ string) error { return nil }

func (s *stubBackend) StoreMemory(_ context.Context, _ *types.Memory) error { return nil }

func (s *stubBackend) StoreMemoryTx(_ context.Context, _ db.Tx, _ *types.Memory) error { return nil }

func (s *stubBackend) GetMemory(_ context.Context, _ string) (*types.Memory, error) { return nil, nil }

func (s *stubBackend) UpdateMemory(_ context.Context, _ string, _ *string, _ []string, _ *int) (*types.Memory, error) {
	return nil, nil
}

func (s *stubBackend) DeleteMemory(_ context.Context, _ string) (bool, error) { return false, nil }

func (s *stubBackend) DeleteMemoryAtomic(_ context.Context, _, _ string, _ bool) (bool, error) {
	return false, nil
}

func (s *stubBackend) MergeMemoriesAtomic(_ context.Context, _, _, _, _ string) error { return nil }

func (s *stubBackend) ListMemories(_ context.Context, _ string, _ db.ListOptions) ([]*types.Memory, error) {
	return nil, nil
}

func (s *stubBackend) TouchMemory(_ context.Context, _ string) error { return nil }

func (s *stubBackend) StoreChunks(_ context.Context, _ []*types.Chunk) error { return nil }

func (s *stubBackend) StoreChunksTx(_ context.Context, _ db.Tx, _ []*types.Chunk) error { return nil }

func (s *stubBackend) GetChunksForMemory(_ context.Context, _ string) ([]*types.Chunk, error) {
	return nil, nil
}

func (s *stubBackend) GetAllChunksWithEmbeddings(_ context.Context, _ string, _ int) ([]*types.Chunk, error) {
	return nil, nil
}

func (s *stubBackend) GetAllChunkTexts(_ context.Context, _ string, _ int) ([]string, error) {
	return nil, nil
}

func (s *stubBackend) GetChunksForMemories(_ context.Context, _ []string) ([]*types.Chunk, error) {
	return nil, nil
}

func (s *stubBackend) ChunkHashExists(_ context.Context, _, _ string) (bool, error) { return false, nil }

func (s *stubBackend) DeleteChunksForMemory(_ context.Context, _ string) error { return nil }

func (s *stubBackend) DeleteChunksForMemoryTx(_ context.Context, _ db.Tx, _ string) error {
	return nil
}

func (s *stubBackend) DeleteChunksByIDs(_ context.Context, _ []string) (int, error) { return 0, nil }

func (s *stubBackend) NullAllEmbeddings(_ context.Context, _ string) (int, error) { return 0, nil }

func (s *stubBackend) NullAllEmbeddingsTx(_ context.Context, _ db.Tx, _ string) (int, error) {
	return 0, nil
}

func (s *stubBackend) GetChunksPendingEmbedding(_ context.Context, _ string, _ int) ([]*types.Chunk, error) {
	return nil, nil
}

func (s *stubBackend) UpdateChunkEmbedding(_ context.Context, _ string, _ []float32) (int, error) {
	return 0, nil
}

func (s *stubBackend) ChunkEmbeddingDistance(_ context.Context, _, _ string) (float64, error) {
	return 2.0, nil
}

func (s *stubBackend) GetPendingEmbeddingCount(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (s *stubBackend) StoreRelationship(_ context.Context, _ *types.Relationship) error { return nil }

func (s *stubBackend) GetConnected(_ context.Context, _ string, _ int) ([]db.ConnectedResult, error) {
	return nil, nil
}

func (s *stubBackend) BoostEdgesForMemory(_ context.Context, _ string, _ float64) (int, error) {
	return 0, nil
}

func (s *stubBackend) DecayEdgesForMemory(_ context.Context, _ string, _ float64) (int, error) {
	return 0, nil
}

func (s *stubBackend) GetConnectionCount(_ context.Context, _, _ string) (int, error) { return 0, nil }

func (s *stubBackend) DecayAllEdges(_ context.Context, _ string, _, _ float64) (int, int, error) {
	return 0, 0, nil
}

func (s *stubBackend) DeleteRelationshipsForMemory(_ context.Context, _ string) error { return nil }

func (s *stubBackend) GetMemoryHistory(_ context.Context, _, _ string) ([]*types.MemoryVersion, error) {
	return nil, nil
}

func (s *stubBackend) SoftDeleteMemory(_ context.Context, _, _, _ string) (bool, error) {
	return false, nil
}

func (s *stubBackend) GetMemoriesAsOf(_ context.Context, _ string, _ time.Time, _ int) ([]*types.Memory, error) {
	return nil, nil
}

func (s *stubBackend) StoreRetrievalEvent(_ context.Context, _ *types.RetrievalEvent) error {
	return nil
}

func (s *stubBackend) GetRetrievalEvent(_ context.Context, _ string) (*types.RetrievalEvent, error) {
	return nil, nil
}

func (s *stubBackend) RecordFeedback(_ context.Context, _ string, _ []string) error { return nil }

func (s *stubBackend) IncrementTimesRetrieved(_ context.Context, _ []string) error { return nil }

func (s *stubBackend) UpdateDynamicImportance(_ context.Context, _ string, _, _ float64) error {
	return nil
}

func (s *stubBackend) SetNextReviewAt(_ context.Context, _ string, _ time.Time) error { return nil }

func (s *stubBackend) DecayStaleImportance(_ context.Context, _ string, _ float64) (int, error) {
	return 0, nil
}

func (s *stubBackend) PruneStaleMemories(_ context.Context, _ string, _ float64, _ int) (int, error) {
	return 0, nil
}

func (s *stubBackend) PruneColdDocuments(_ context.Context, _ string, _ float64, _ int) (int, error) {
	return 0, nil
}

func (s *stubBackend) RebuildFTS(_ context.Context) error { return nil }

func (s *stubBackend) GetStats(_ context.Context, _ string) (*types.MemoryStats, error) {
	return nil, nil
}

func (s *stubBackend) ListAllProjects(_ context.Context) ([]string, error) { return nil, nil }

func (s *stubBackend) GetAllMemoryIDs(_ context.Context, _ string) (map[string]struct{}, error) {
	return nil, nil
}

func (s *stubBackend) GetMemoriesPendingSummary(_ context.Context, _ string, _ int) ([]db.IDContent, error) {
	return nil, nil
}

func (s *stubBackend) StoreSummary(_ context.Context, _, _ string) error { return nil }

func (s *stubBackend) GetPendingSummaryCount(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (s *stubBackend) ClearSummaries(_ context.Context, _ string) (int, error) { return 0, nil }

func (s *stubBackend) GetMemoriesMissingHash(_ context.Context, _ string, _ int) ([]db.IDContent, error) {
	return nil, nil
}

func (s *stubBackend) UpdateMemoryHash(_ context.Context, _, _ string) error { return nil }

func (s *stubBackend) GetIntegrityStats(_ context.Context, _ string) (db.IntegrityStats, error) {
	return db.IntegrityStats{}, nil
}

func (s *stubBackend) StartEpisode(_ context.Context, _, _ string) (*types.Episode, error) {
	return nil, nil
}

func (s *stubBackend) EndEpisode(_ context.Context, _, _ string) error { return nil }

func (s *stubBackend) ListEpisodes(_ context.Context, _ string, _ int) ([]*types.Episode, error) {
	return nil, nil
}

func (s *stubBackend) RecallEpisode(_ context.Context, _ string) ([]*types.Memory, error) {
	return nil, nil
}

func (s *stubBackend) Begin(_ context.Context) (db.Tx, error) { return nil, nil }

func (s *stubBackend) ExistsWithContentHash(_ context.Context, _ string, _ string) (bool, error) {
	return false, nil
}

func (s *stubBackend) StoreDocument(_ context.Context, _, _ string) (string, error) {
	return "", nil
}
func (s *stubBackend) GetDocument(_ context.Context, _ string) (string, error) { return "", nil }
func (s *stubBackend) SetMemoryDocumentID(_ context.Context, _, _ string) error { return nil }

// compile-time check: stubBackend must satisfy db.Backend.
var _ db.Backend = (*stubBackend)(nil)

// ── Micro-benchmarks: inner map-merge loop (Candidate 1) ────────────────────
//
// These benchmarks isolate the best-hit selection loop in RecallWithOpts.
// They do NOT need a running engine — they test the algorithm directly.
// Both approaches are benchmarked here so a single run produces a
// before/after comparison without needing two separate source states.

// BenchmarkRecallMaps_5maps is the CURRENT implementation: five separate maps.
// This is the baseline. Shewhart compares this against BenchmarkRecallMaps_structmap.
func BenchmarkRecallMaps_5maps(b *testing.B) {
	const numMemories = 10
	const chunksPerMemory = 3
	hits := syntheticVecHits(numMemories, chunksPerMemory)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		bestCosine := make(map[string]float64, numMemories)
		bestChunkText := make(map[string]string, numMemories)
		bestChunkIndex := make(map[string]int, numMemories)
		bestChunkSection := make(map[string]*string, numMemories)
		bestChunkID := make(map[string]string, numMemories)
		seen := make(map[string]bool, numMemories)
		uniqueIDs := make([]string, 0, numMemories)

		for _, h := range hits {
			cosine := 1.0 - h.Distance
			if cosine > bestCosine[h.MemoryID] {
				bestCosine[h.MemoryID] = cosine
				bestChunkText[h.MemoryID] = h.ChunkText
				bestChunkIndex[h.MemoryID] = h.ChunkIndex
				bestChunkSection[h.MemoryID] = h.SectionHeading
				bestChunkID[h.MemoryID] = h.ChunkID
			}
			if !seen[h.MemoryID] {
				seen[h.MemoryID] = true
				uniqueIDs = append(uniqueIDs, h.MemoryID)
			}
		}
		// Prevent dead-code elimination.
		_ = uniqueIDs
		_ = bestCosine
		_ = bestChunkText
		_ = bestChunkIndex
		_ = bestChunkSection
		_ = bestChunkID
	}
}

// BenchmarkRecallMaps_structmap is the PROPOSED optimization: one struct map.
// This benchmark is written ahead of implementation (TDD) to establish the
// performance target. It will also serve as the "after" baseline once
// engine.go is updated to use this approach.
func BenchmarkRecallMaps_structmap(b *testing.B) {
	type bestHit struct {
		cosine         float64
		chunkText      string
		chunkIndex     int
		sectionHeading *string
		chunkID        string
	}

	const numMemories = 10
	const chunksPerMemory = 3
	hits := syntheticVecHits(numMemories, chunksPerMemory)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		bestHits := make(map[string]bestHit, numMemories)
		seen := make(map[string]bool, numMemories)
		uniqueIDs := make([]string, 0, numMemories)

		for _, h := range hits {
			cosine := 1.0 - h.Distance
			if existing, ok := bestHits[h.MemoryID]; !ok || cosine > existing.cosine {
				bestHits[h.MemoryID] = bestHit{
					cosine:         cosine,
					chunkText:      h.ChunkText,
					chunkIndex:     h.ChunkIndex,
					sectionHeading: h.SectionHeading,
					chunkID:        h.ChunkID,
				}
			}
			if !seen[h.MemoryID] {
				seen[h.MemoryID] = true
				uniqueIDs = append(uniqueIDs, h.MemoryID)
			}
		}
		_ = uniqueIDs
		_ = bestHits
	}
}

// ── Full-path benchmarks via stub backend ────────────────────────────────────

// newBenchEngine creates a SearchEngine backed by a stubBackend for benchmarking.
// Workers are started but will find no work (stub returns empty pending queues).
func newBenchEngine(b *testing.B, numResults, relsPerResult int) *search.SearchEngine {
	b.Helper()
	backend := newStubBackend(numResults, relsPerResult)
	engine := search.New(
		context.Background(),
		backend,
		&fakeClient{dims: 768},
		"bench-project",
		"http://ollama:11434",
		"llama3.2",
		false, // summarization disabled — no Ollama needed
		nil,   // no claude client
		0,     // default decay interval
	)
	b.Cleanup(func() { engine.Close() })
	return engine
}

// BenchmarkRecallWithOpts_10 benchmarks a full Recall cycle:
// embedding → vector search → FTS → composite score → topK trim.
// GetRelationships is NOT called here (relsPerResult=0).
func BenchmarkRecallWithOpts_10(b *testing.B) {
	engine := newBenchEngine(b, 10, 0)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		results, err := engine.Recall(ctx, "test query for benchmark", 10, "full")
		if err != nil {
			b.Fatal(err)
		}
		_ = results
	}
}

// BenchmarkRecallWithOpts_10_withRelationships benchmarks the full Recall cycle
// including the GetRelationships loop for each of the topK results.
// With the current implementation, this issues K separate DB queries (N+1 pattern).
// After Candidate 2 (GetRelationshipsBatch), this should issue 1 query.
func BenchmarkRecallWithOpts_10_withRelationships(b *testing.B) {
	engine := newBenchEngine(b, 10, 3) // 3 relationships per result → 10 queries
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		results, err := engine.Recall(ctx, "test query for benchmark", 10, "full")
		if err != nil {
			b.Fatal(err)
		}
		_ = results
	}
}

// BenchmarkRecallWithOpts_50 benchmarks a larger recall window (50 results).
// Useful for confirming that optimizations scale with result count.
func BenchmarkRecallWithOpts_50(b *testing.B) {
	engine := newBenchEngine(b, 50, 0)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		results, err := engine.Recall(ctx, "test query for benchmark", 50, "full")
		if err != nil {
			b.Fatal(err)
		}
		_ = results
	}
}
