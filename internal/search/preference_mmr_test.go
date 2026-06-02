// preference_mmr_test.go — pure unit tests for H-NEW-2 centroid-MMR helpers.
// All tests run without a DB. The key property under test:
//   - computeCentroid correctly averages float32 vectors
//   - cosineSimilarityF32 returns 1.0 for identical vectors, 0.0 for orthogonal
//   - mmrReScore promotes a domain-specific result that pure relevance buried
//   - mmrReScore with flag off (lambda=0 shortcircuit) produces unchanged order
//   - determinism: same inputs → same output every call
//
// Integration tests for applyPreferenceMMR (engine method):
//   - Bug 1: MMR ordering survives when a downstream sortResults pass is active
//     (r.Score is written with MMR-derived rank so sort preserves MMR order)
//   - Bug 2: centroid reflects the dominant-topic (general) pool, not the
//     preference-front-loaded results slice
package search

import (
	"context"
	"math"
	"testing"

	"github.com/petersimmons1972/engram/internal/types"
)

// ---------------------------------------------------------------------------
// mmrChunkBackend overrides GetChunksForMemories on cacheMetaBackend so tests
// can inject specific per-memory embeddings without a real database.
// All other Backend methods delegate to cacheMetaBackend's no-op
// implementations.
// ---------------------------------------------------------------------------

type mmrChunkBackend struct {
	cacheMetaBackend
	// chunksByMemID maps memory_id → embedding vector to return.
	chunksByMemID map[string][]float32
}

func newMMRChunkBackend(embeddings map[string][]float32) *mmrChunkBackend {
	return &mmrChunkBackend{
		cacheMetaBackend: cacheMetaBackend{meta: make(map[string]string)},
		chunksByMemID:    embeddings,
	}
}

func (b *mmrChunkBackend) GetChunksForMemories(_ context.Context, memIDs []string) ([]*types.Chunk, error) {
	var out []*types.Chunk
	for _, id := range memIDs {
		if emb, ok := b.chunksByMemID[id]; ok {
			out = append(out, &types.Chunk{
				MemoryID:   id,
				ChunkIndex: 0,
				Embedding:  emb,
			})
		}
	}
	return out, nil
}

// makeSearchResult is a test helper that builds a SearchResult with a fixed
// Score and optional memory_type.
func makeSearchResult(id string, score float64, memType string) types.SearchResult {
	return types.SearchResult{
		Memory: &types.Memory{ID: id, MemoryType: memType},
		Score:  score,
	}
}

// ---------------------------------------------------------------------------
// Bug 1: MMR ordering survives a downstream sortResults pass
//
// Setup: three preference results where raw-score order is dominant-a >
// dominant-b > domain-x, but MMR (using topic centroid [1,0]) promotes domain-x
// above dominant-b because domain-x is orthogonal to the dominant cluster.
//
// A (stub) reranker would call sortResults on the output of applyPreferenceMMR.
// The bug was: applyPreferenceMMR reordered results but left r.Score at the
// original composite score, so sortResults reverted to score order, silently
// discarding the MMR ordering.
//
// The fix writes the MMR-derived rank into r.Score, so sortResults preserves it.
//
// We prove: after applyPreferenceMMR,
//   (a) domain-x ranks above dominant-b in the returned slice, AND
//   (b) domain-x.Score > dominant-b.Score (so a subsequent sortResults call
//       keeps domain-x above dominant-b — the reranker path is safe).
//
// Embeddings and topicPool are chosen so the centroid from topicPool is clearly
// [1,0], making domain-x's [0,1] get minimal penalty and dominant-b's [0.95,0.05]
// get near-maximum penalty.
// ---------------------------------------------------------------------------

func TestApplyPreferenceMMR_OrderingsSurviveSortResults_Bug1(t *testing.T) {
	embeddings := map[string][]float32{
		"dominant-a": {1.0, 0.0},
		"dominant-b": {0.95, 0.05},
		"domain-x":   {0.0, 1.0},
		// topicPool source memories:
		"general-1": {1.0, 0.0},
		"general-2": {0.98, 0.02},
	}
	backend := newMMRChunkBackend(embeddings)
	emb := &cacheTestEmbedder{name: "test", dims: 2}
	eng := newCacheTestEngine(t, &backend.cacheMetaBackend, emb)
	// Wire the overridden GetChunksForMemories by replacing the backend field
	// directly — SearchEngine.backend is package-private so we set it here.
	eng.backend = backend

	results := []types.SearchResult{
		makeSearchResult("dominant-a", 0.90, "preference"),
		makeSearchResult("dominant-b", 0.85, "preference"),
		makeSearchResult("domain-x", 0.60, "preference"),
	}

	// topicPool: general memories with embeddings tightly clustered around [1,0].
	// This makes the centroid ≈ [1,0], giving domain-x (embedding [0,1]) near-zero
	// cosine similarity to the centroid and thus near-zero penalty.
	// dominant-b (embedding [0.95,0.05]) has cosine_sim ≈ 0.999 to [1,0] centroid
	// → high penalty. With lambda=0.7:
	//   dominant-a mmr ≈ 0.7*0.9 - 0.3*1.0   = 0.63 - 0.30 = 0.33
	//   dominant-b mmr ≈ 0.7*0.85 - 0.3*0.999 = 0.595 - 0.300 = 0.295
	//   domain-x   mmr ≈ 0.7*0.6 - 0.3*0.0   = 0.42 - 0.00 = 0.42
	// → MMR order: domain-x > dominant-a > dominant-b
	topicPool := []types.SearchResult{
		makeSearchResult("general-1", 0.95, "context"),
		makeSearchResult("general-2", 0.90, "context"),
	}

	ctx := context.Background()
	out := eng.applyPreferenceMMR(ctx, results, topicPool)

	if len(out) != 3 {
		t.Fatalf("applyPreferenceMMR returned %d results, want 3", len(out))
	}

	// Find ranks in output.
	rankOf := func(id string) int {
		for i, r := range out {
			if r.Memory != nil && r.Memory.ID == id {
				return i
			}
		}
		return -1
	}
	domainXRank := rankOf("domain-x")
	dominantBRank := rankOf("dominant-b")
	if domainXRank == -1 || dominantBRank == -1 {
		t.Fatalf("expected both domain-x and dominant-b in output; got %v", out)
	}
	if domainXRank >= dominantBRank {
		t.Errorf("Bug 1 (ordering): domain-x rank=%d, dominant-b rank=%d — MMR did not promote domain-x before dominant-b",
			domainXRank, dominantBRank)
	}

	// Key assertion for Bug 1: r.Score must encode MMR rank order so that a
	// subsequent sortResults call (from the reranker block) preserves MMR order.
	// domain-x ranked above dominant-b under MMR → domain-x.Score must be larger.
	scoreOf := func(id string) float64 {
		for _, r := range out {
			if r.Memory != nil && r.Memory.ID == id {
				return r.Score
			}
		}
		t.Fatalf("id %q not found", id)
		return 0
	}
	domainXScore := scoreOf("domain-x")
	dominantBScore := scoreOf("dominant-b")
	if domainXScore <= dominantBScore {
		t.Errorf("Bug 1 (score): domain-x.Score=%.4f is not > dominant-b.Score=%.4f — sortResults would revert MMR ordering",
			domainXScore, dominantBScore)
	}

	// Confirm: sortResults on the MMR output must still place domain-x before dominant-b.
	sortResults(out)
	domainXRankAfterSort := rankOf("domain-x")
	dominantBRankAfterSort := rankOf("dominant-b")
	if domainXRankAfterSort >= dominantBRankAfterSort {
		t.Errorf("Bug 1 (sort-stable): after sortResults domain-x rank=%d, dominant-b rank=%d — MMR ordering was lost",
			domainXRankAfterSort, dominantBRankAfterSort)
	}
}

// ---------------------------------------------------------------------------
// Bug 2: centroid reflects dominant topic (general/topicPool), not the
// preference-front-loaded results.
//
// Setup: results has preference memories front-loaded, all with embeddings
// pointing in direction [0,1] (the "preference cluster"). topicPool has
// general memories with embeddings pointing in direction [1,0] (the "topic
// cluster"). When the bug was present, the centroid was computed from results
// (preference-front-loaded) → centroid ≈ [0,1] → MMR penalised the [0,1]
// preference memories rather than the [1,0] topic cluster, inverting intent.
// With the fix, centroid is computed from topicPool → centroid ≈ [1,0] →
// preference memories (orthogonal to centroid) are promoted.
//
// We verify: the memory with embedding [0,1] (preference-shaped, far from topic
// centroid [1,0]) gets a higher MMR score (lower rank position) than the memory
// with embedding [1,0] (aligned with topic centroid, thus penalised).
// ---------------------------------------------------------------------------

func TestApplyPreferenceMMR_CentroidFromTopicPool_Bug2(t *testing.T) {
	// Two preference memories (in results, front-loaded):
	//   "pref-ortho": embedding [0,1] — orthogonal to topic centroid [1,0]
	//     → should be PROMOTED by MMR (low centroid similarity → low penalty)
	//   "pref-aligned": embedding [0.95,0.1] — aligned with topic centroid [1,0]
	//     → should be DEMOTED by MMR (high centroid similarity → high penalty)
	// Both start with equal relevance scores so any reordering is purely MMR-driven.
	//
	// Two general memories (in topicPool only — they define the dominant topic):
	//   "general-a": embedding [1,0]
	//   "general-b": embedding [0.9,0.1]
	embeddings := map[string][]float32{
		"pref-ortho":   {0.0, 1.0},
		"pref-aligned": {0.95, 0.1},
		"general-a":    {1.0, 0.0},
		"general-b":    {0.9, 0.1},
	}
	backend := newMMRChunkBackend(embeddings)
	emb := &cacheTestEmbedder{name: "test", dims: 2}
	eng := newCacheTestEngine(t, &backend.cacheMetaBackend, emb)
	eng.backend = backend

	// results: preference memories front-loaded (as after preference-first split).
	// Equal scores so ordering is purely MMR-driven.
	results := []types.SearchResult{
		makeSearchResult("pref-ortho", 0.80, "preference"),
		makeSearchResult("pref-aligned", 0.80, "preference"),
	}

	// topicPool: general memories only, relevance-ranked — represents dominant topic.
	topicPool := []types.SearchResult{
		makeSearchResult("general-a", 0.75, "context"),
		makeSearchResult("general-b", 0.70, "context"),
	}

	ctx := context.Background()
	out := eng.applyPreferenceMMR(ctx, results, topicPool)

	if len(out) != 2 {
		t.Fatalf("applyPreferenceMMR returned %d results, want 2", len(out))
	}

	// With the fix (centroid from topicPool ≈ [1,0]):
	//   pref-ortho [0,1] has cosine_sim ≈ 0 to centroid → low penalty → higher MMR
	//   pref-aligned [0.95,0.1] has cosine_sim ≈ 0.95 to centroid → high penalty → lower MMR
	// So pref-ortho should rank first (rank 0), pref-aligned second (rank 1).
	//
	// If the bug were present (centroid from preference-front-loaded results):
	//   centroid ≈ ([0,1] + [0.95,0.1]) / 2 ≈ [0.475, 0.55]
	//   Both memories have similar cosine_sim to this centroid → ordering is arbitrary
	//   and does NOT reliably promote pref-ortho. In the worst case pref-aligned wins.

	if out[0].Memory == nil {
		t.Fatal("out[0].Memory is nil")
	}
	if out[0].Memory.ID != "pref-ortho" {
		t.Errorf("Bug 2: expected pref-ortho at rank 0 (promoted via topic centroid), got %q",
			out[0].Memory.ID)
	}
	if out[1].Memory.ID != "pref-aligned" {
		t.Errorf("Bug 2: expected pref-aligned at rank 1 (penalised by topic centroid), got %q",
			out[1].Memory.ID)
	}
}

// ---------------------------------------------------------------------------
// computeCentroid
// ---------------------------------------------------------------------------

func TestComputeCentroid_Empty(t *testing.T) {
	c := computeCentroid(nil)
	if c != nil {
		t.Errorf("computeCentroid(nil) = %v, want nil", c)
	}
	c2 := computeCentroid([][]float32{})
	if c2 != nil {
		t.Errorf("computeCentroid([]) = %v, want nil", c2)
	}
}

func TestComputeCentroid_SingleVector(t *testing.T) {
	v := []float32{1.0, 0.0, 0.0}
	c := computeCentroid([][]float32{v})
	if len(c) != 3 {
		t.Fatalf("centroid len = %d, want 3", len(c))
	}
	if math.Abs(float64(c[0])-1.0) > 1e-6 || math.Abs(float64(c[1])) > 1e-6 || math.Abs(float64(c[2])) > 1e-6 {
		t.Errorf("centroid of single [1,0,0] = %v, want [1,0,0]", c)
	}
}

func TestComputeCentroid_TwoVectors(t *testing.T) {
	v1 := []float32{1.0, 0.0}
	v2 := []float32{0.0, 1.0}
	c := computeCentroid([][]float32{v1, v2})
	if len(c) != 2 {
		t.Fatalf("centroid len = %d, want 2", len(c))
	}
	if math.Abs(float64(c[0])-0.5) > 1e-6 || math.Abs(float64(c[1])-0.5) > 1e-6 {
		t.Errorf("centroid([1,0],[0,1]) = %v, want [0.5,0.5]", c)
	}
}

func TestComputeCentroid_DimMismatch_SkipsShort(t *testing.T) {
	// Vectors with different dims: shorter ones are skipped (truncated).
	v1 := []float32{2.0, 4.0}
	v2 := []float32{0.0} // too short — only 1 dim
	c := computeCentroid([][]float32{v1, v2})
	// Only v1 contributes because v2 has dim < len(v1).
	if len(c) != 2 {
		t.Fatalf("centroid len = %d, want 2", len(c))
	}
	if math.Abs(float64(c[0])-2.0) > 1e-6 || math.Abs(float64(c[1])-4.0) > 1e-6 {
		t.Errorf("centroid with dim-mismatch = %v, want [2,4]", c)
	}
}

// ---------------------------------------------------------------------------
// cosineSimilarityF32
// ---------------------------------------------------------------------------

func TestCosineSimilarityF32_Identical(t *testing.T) {
	v := []float32{1.0, 2.0, 3.0}
	sim := cosineSimilarityF32(v, v)
	if math.Abs(float64(sim)-1.0) > 1e-5 {
		t.Errorf("cosineSimilarityF32(v,v) = %v, want 1.0", sim)
	}
}

func TestCosineSimilarityF32_Orthogonal(t *testing.T) {
	a := []float32{1.0, 0.0, 0.0}
	b := []float32{0.0, 1.0, 0.0}
	sim := cosineSimilarityF32(a, b)
	if math.Abs(float64(sim)) > 1e-5 {
		t.Errorf("cosineSimilarityF32([1,0,0],[0,1,0]) = %v, want 0.0", sim)
	}
}

func TestCosineSimilarityF32_Opposite(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{-1.0, 0.0}
	sim := cosineSimilarityF32(a, b)
	if math.Abs(float64(sim)+1.0) > 1e-5 {
		t.Errorf("cosineSimilarityF32([1,0],[-1,0]) = %v, want -1.0", sim)
	}
}

func TestCosineSimilarityF32_ZeroVector(t *testing.T) {
	a := []float32{0.0, 0.0}
	b := []float32{1.0, 0.0}
	sim := cosineSimilarityF32(a, b)
	if math.IsNaN(float64(sim)) || math.IsInf(float64(sim), 0) {
		t.Errorf("cosineSimilarityF32(zero, v) = %v, want finite (0)", sim)
	}
	if sim != 0.0 {
		t.Errorf("cosineSimilarityF32(zero, v) = %v, want 0", sim)
	}
}

func TestCosineSimilarityF32_DimMismatch_ReturnsZero(t *testing.T) {
	a := []float32{1.0, 2.0}
	b := []float32{1.0}
	sim := cosineSimilarityF32(a, b)
	if sim != 0.0 {
		t.Errorf("cosineSimilarityF32 dim mismatch = %v, want 0.0", sim)
	}
}

// ---------------------------------------------------------------------------
// mmrReScore: the key property
//
// Setup: three candidates
//   - "dominant-A": high relevance (cosine=0.9), closely aligned with centroid
//   - "dominant-B": high relevance (cosine=0.85), closely aligned with centroid
//   - "domain-specific": lower relevance (cosine=0.6), far from centroid
//
// Without MMR: order is dominant-A, dominant-B, domain-specific.
// With MMR (lambda=0.7): domain-specific gets promoted because it avoids centroid.
// ---------------------------------------------------------------------------

func TestMMRReScore_DomainSpecificPromoted(t *testing.T) {
	// Centroid points in direction [1,0] (dominant cluster = books/reading).
	centroid := []float32{1.0, 0.0}

	// dominant-A: aligned with centroid, high relevance
	dominantA := mmrCandidate{
		memoryID:  "dominant-a",
		relevance: 0.90,
		embedding: []float32{0.95, 0.05}, // nearly parallel to centroid
	}
	// dominant-B: aligned with centroid, slightly lower relevance
	dominantB := mmrCandidate{
		memoryID:  "dominant-b",
		relevance: 0.85,
		embedding: []float32{0.92, 0.10},
	}
	// domain-specific: lower relevance but orthogonal to centroid
	domainSpecific := mmrCandidate{
		memoryID:  "domain-specific",
		relevance: 0.60,
		embedding: []float32{0.05, 0.95}, // near-orthogonal to centroid
	}

	candidates := []mmrCandidate{dominantA, dominantB, domainSpecific}
	const lambda = 0.7

	reScored := mmrReScore(candidates, centroid, lambda)

	// Find domain-specific rank in reScored output.
	domainRank := -1
	for i, c := range reScored {
		if c.memoryID == "domain-specific" {
			domainRank = i
			break
		}
	}
	if domainRank == -1 {
		t.Fatal("domain-specific not found in reScored output")
	}
	// With MMR, domain-specific should outrank at least dominant-B.
	// dominant-A has relevance 0.9 vs domain-specific 0.6 — check relative to B.
	dominantBRank := -1
	for i, c := range reScored {
		if c.memoryID == "dominant-b" {
			dominantBRank = i
			break
		}
	}
	if domainRank >= dominantBRank {
		t.Errorf("domain-specific rank=%d is not better than dominant-b rank=%d; MMR did not promote it",
			domainRank, dominantBRank)
	}
}

// TestMMRReScore_FlagOff verifies that lambda=1.0 (no diversity penalty) preserves
// relevance-only order — used to confirm the flag-off baseline is identical.
func TestMMRReScore_FlagOff_NoChange(t *testing.T) {
	centroid := []float32{1.0, 0.0}

	candidates := []mmrCandidate{
		{memoryID: "a", relevance: 0.9, embedding: []float32{1.0, 0.0}},
		{memoryID: "b", relevance: 0.7, embedding: []float32{0.5, 0.5}},
		{memoryID: "c", relevance: 0.5, embedding: []float32{0.0, 1.0}},
	}

	// lambda=1.0 means pure relevance: MMR score = relevance, no penalty.
	reScored := mmrReScore(candidates, centroid, 1.0)

	// Order should be a > b > c (descending relevance).
	wantOrder := []string{"a", "b", "c"}
	for i, want := range wantOrder {
		if reScored[i].memoryID != want {
			t.Errorf("position %d: got %q, want %q (lambda=1.0 should preserve relevance order)",
				i, reScored[i].memoryID, want)
		}
	}
}

// TestMMRReScore_Deterministic verifies identical inputs produce identical outputs.
func TestMMRReScore_Deterministic(t *testing.T) {
	centroid := []float32{1.0, 0.0}
	candidates := []mmrCandidate{
		{memoryID: "x", relevance: 0.9, embedding: []float32{0.9, 0.1}},
		{memoryID: "y", relevance: 0.8, embedding: []float32{0.1, 0.9}},
		{memoryID: "z", relevance: 0.7, embedding: []float32{0.5, 0.5}},
	}

	result1 := mmrReScore(candidates, centroid, 0.7)
	result2 := mmrReScore(candidates, centroid, 0.7)

	for i := range result1 {
		if result1[i].memoryID != result2[i].memoryID {
			t.Errorf("position %d: run1=%q run2=%q — not deterministic",
				i, result1[i].memoryID, result2[i].memoryID)
		}
	}
}

// TestMMRReScore_EmptyCandidates verifies no panic on empty input.
func TestMMRReScore_EmptyCandidates(t *testing.T) {
	centroid := []float32{1.0, 0.0}
	result := mmrReScore(nil, centroid, 0.7)
	if result != nil {
		t.Errorf("mmrReScore(nil,...) = %v, want nil", result)
	}
}

// TestMMRReScore_NilCentroid verifies graceful handling when centroid is nil
// (falls back to relevance-only ordering).
func TestMMRReScore_NilCentroid(t *testing.T) {
	candidates := []mmrCandidate{
		{memoryID: "a", relevance: 0.9, embedding: []float32{1.0, 0.0}},
		{memoryID: "b", relevance: 0.6, embedding: []float32{0.0, 1.0}},
	}
	result := mmrReScore(candidates, nil, 0.7)
	// With nil centroid: anti-centroid penalty is 0 for all; pure relevance order.
	if len(result) != 2 || result[0].memoryID != "a" {
		t.Errorf("mmrReScore with nil centroid = %v, want a>b (relevance order)", result)
	}
}
