// fusion_test.go — TDD tests for composite retrieval fusion (RRF flag-gate).
//
// Tests verify three invariants:
//  1. Flag OFF → rrfScore stays 0 (no-op, baseline unchanged).
//  2. Flag ON  → a gold candidate that only one retriever surfaces (absent from
//     the other leg) still receives a non-zero RRF score (not suppressed).
//  3. Ordering: both-legs candidate scores higher than single-leg candidate.
//  4. Determinism: identical inputs → identical outputs.
//
// All tests are pure (no-DB); they exercise rankVectorHits, rankFTSResults,
// and applyFusion via the package-internal test file.
package search

import (
	"sort"
	"testing"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/types"
)

// ---------------------------------------------------------------------------
// rankVectorHits
// ---------------------------------------------------------------------------

func TestRankVectorHits_Empty(t *testing.T) {
	ranks := rankVectorHits(nil)
	if len(ranks) != 0 {
		t.Fatalf("expected empty map for nil input, got len=%d", len(ranks))
	}
}

func TestRankVectorHits_UniqueMemories(t *testing.T) {
	hits := []db.VectorHit{
		{MemoryID: "mem-a", Distance: 0.1}, // best → rank 1
		{MemoryID: "mem-b", Distance: 0.5}, // second → rank 2
		{MemoryID: "mem-c", Distance: 0.9}, // third → rank 3
	}
	ranks := rankVectorHits(hits)
	if ranks["mem-a"] != 1 {
		t.Errorf("mem-a rank = %d, want 1", ranks["mem-a"])
	}
	if ranks["mem-b"] != 2 {
		t.Errorf("mem-b rank = %d, want 2", ranks["mem-b"])
	}
	if ranks["mem-c"] != 3 {
		t.Errorf("mem-c rank = %d, want 3", ranks["mem-c"])
	}
}

func TestRankVectorHits_BestChunkPerMemory(t *testing.T) {
	// Same memory ID appears twice; rank should reflect first (best) occurrence.
	hits := []db.VectorHit{
		{MemoryID: "mem-x", Distance: 0.2, ChunkID: "chunk-1"}, // first → rank 1
		{MemoryID: "mem-y", Distance: 0.4, ChunkID: "chunk-2"}, // rank 2
		{MemoryID: "mem-x", Distance: 0.6, ChunkID: "chunk-3"}, // duplicate mem-x, ignored for rank
	}
	ranks := rankVectorHits(hits)
	if ranks["mem-x"] != 1 {
		t.Errorf("mem-x rank = %d, want 1 (best chunk)", ranks["mem-x"])
	}
	if ranks["mem-y"] != 2 {
		t.Errorf("mem-y rank = %d, want 2", ranks["mem-y"])
	}
	if len(ranks) != 2 {
		t.Errorf("expected 2 unique memory ranks, got %d", len(ranks))
	}
}

// ---------------------------------------------------------------------------
// rankFTSResults
// ---------------------------------------------------------------------------

func TestRankFTSResults_Empty(t *testing.T) {
	ranks := rankFTSResults(nil)
	if len(ranks) != 0 {
		t.Fatalf("expected empty map for nil input, got len=%d", len(ranks))
	}
}

func TestRankFTSResults_Ordering(t *testing.T) {
	results := []db.FTSResult{
		{Memory: &types.Memory{ID: "fts-1"}, Score: 3.5}, // rank 1
		{Memory: &types.Memory{ID: "fts-2"}, Score: 2.0}, // rank 2
		{Memory: &types.Memory{ID: "fts-3"}, Score: 0.5}, // rank 3
	}
	ranks := rankFTSResults(results)
	if ranks["fts-1"] != 1 {
		t.Errorf("fts-1 rank = %d, want 1", ranks["fts-1"])
	}
	if ranks["fts-2"] != 2 {
		t.Errorf("fts-2 rank = %d, want 2", ranks["fts-2"])
	}
	if ranks["fts-3"] != 3 {
		t.Errorf("fts-3 rank = %d, want 3", ranks["fts-3"])
	}
}

func TestRankFTSResults_NilMemorySkipped(t *testing.T) {
	results := []db.FTSResult{
		{Memory: nil, Score: 5.0},
		{Memory: &types.Memory{ID: "fts-valid"}, Score: 1.0},
	}
	ranks := rankFTSResults(results)
	if _, ok := ranks[""]; ok {
		t.Error("nil Memory should not produce a rank entry")
	}
	if ranks["fts-valid"] != 1 {
		t.Errorf("fts-valid rank = %d, want 1", ranks["fts-valid"])
	}
}

// ---------------------------------------------------------------------------
// applyFusion: flag OFF = strict no-op
// ---------------------------------------------------------------------------

func TestApplyFusion_FlagOff_NoChange(t *testing.T) {
	// When Fusion is false, applyFusion must not modify rrfScore.
	memories := map[string]*types.Memory{
		"m1": {ID: "m1"},
		"m2": {ID: "m2"},
	}
	bh := map[string]bestHit{
		"m1": {cosine: 0.9},
		"m2": {cosine: 0.7},
	}
	vecRanks := map[string]int{"m1": 1, "m2": 2}
	ftsRanks := map[string]int{"m1": 2, "m2": 1}

	opts := RecallOpts{Fusion: false}
	applyFusion(opts, memories, bh, vecRanks, ftsRanks)

	for id, h := range bh {
		if h.rrfScore != 0 {
			t.Errorf("flag OFF: %s rrfScore = %v, want 0 (no-op)", id, h.rrfScore)
		}
	}
}

// ---------------------------------------------------------------------------
// applyFusion: flag ON surfaces BM25-only candidate
// ---------------------------------------------------------------------------

func TestApplyFusion_FlagOn_SurfacesBM25OnlyCandidate(t *testing.T) {
	// gold-mem: absent from vector (vecRank=0), present at ftsRank=3.
	// With pure weighted-sum, cosine=0 → very low score.
	// With RRF: gold-mem gets 1/(60+3)=0.0159 — non-zero.
	memories := map[string]*types.Memory{
		"gold-mem": {ID: "gold-mem"},
		"vec-only": {ID: "vec-only"},
	}
	bh := map[string]bestHit{
		"gold-mem": {cosine: 0.0},
		"vec-only": {cosine: 0.95},
	}
	vecRanks := map[string]int{"vec-only": 1}
	ftsRanks := map[string]int{"gold-mem": 3}

	opts := RecallOpts{Fusion: true}
	applyFusion(opts, memories, bh, vecRanks, ftsRanks)

	goldHit, ok := bh["gold-mem"]
	if !ok {
		t.Fatal("gold-mem missing from bestHits after fusion")
	}
	if goldHit.rrfScore <= 0 {
		t.Errorf("gold-mem rrfScore = %v, want > 0 (BM25-only candidate must not be zeroed)", goldHit.rrfScore)
	}
}

func TestApplyFusion_FlagOn_BothLegsHigherThanSingle(t *testing.T) {
	// champion: vecRank=1, ftsRank=1 → RRF = 1/61 + 1/61 ≈ 0.0328.
	// single-leg: vecRank=2, ftsRank=0 → RRF = 1/62 ≈ 0.0161.
	memories := map[string]*types.Memory{
		"champion":   {ID: "champion"},
		"single-leg": {ID: "single-leg"},
	}
	bh := map[string]bestHit{
		"champion":   {cosine: 0.9},
		"single-leg": {cosine: 0.7},
	}
	vecRanks := map[string]int{"champion": 1, "single-leg": 2}
	ftsRanks := map[string]int{"champion": 1}

	opts := RecallOpts{Fusion: true}
	applyFusion(opts, memories, bh, vecRanks, ftsRanks)

	champScore := bh["champion"].rrfScore
	singleScore := bh["single-leg"].rrfScore
	if champScore <= singleScore {
		t.Errorf("champion rrfScore %v should exceed single-leg rrfScore %v", champScore, singleScore)
	}
}

func TestApplyFusion_FlagOn_AbsentFromBothLegsGetsZero(t *testing.T) {
	// A memory in the memories map but absent from both legs → rrfScore=0.
	memories := map[string]*types.Memory{
		"ghost": {ID: "ghost"},
		"real":  {ID: "real"},
	}
	bh := map[string]bestHit{
		"ghost": {cosine: 0.3},
		"real":  {cosine: 0.8},
	}
	vecRanks := map[string]int{"real": 1}
	ftsRanks := map[string]int{"real": 1}

	opts := RecallOpts{Fusion: true}
	applyFusion(opts, memories, bh, vecRanks, ftsRanks)

	if bh["ghost"].rrfScore != 0 {
		t.Errorf("ghost rrfScore = %v, want 0 (absent from both legs)", bh["ghost"].rrfScore)
	}
	if bh["real"].rrfScore <= 0 {
		t.Errorf("real rrfScore = %v, want > 0 (present in both legs)", bh["real"].rrfScore)
	}
}

// ---------------------------------------------------------------------------
// Determinism
// ---------------------------------------------------------------------------

func TestApplyFusion_Deterministic(t *testing.T) {
	// Run applyFusion twice with identical inputs; rrfScore must be stable.
	setup := func() (map[string]*types.Memory, map[string]bestHit, map[string]int, map[string]int) {
		return map[string]*types.Memory{
				"m1": {ID: "m1"}, "m2": {ID: "m2"}, "m3": {ID: "m3"},
			},
			map[string]bestHit{
				"m1": {cosine: 0.8}, "m2": {cosine: 0.6}, "m3": {cosine: 0.4},
			},
			map[string]int{"m1": 1, "m2": 2, "m3": 3},
			map[string]int{"m2": 1, "m1": 2}
	}

	opts := RecallOpts{Fusion: true}

	mems1, bh1, vr1, fr1 := setup()
	applyFusion(opts, mems1, bh1, vr1, fr1)

	mems2, bh2, vr2, fr2 := setup()
	applyFusion(opts, mems2, bh2, vr2, fr2)

	for id := range bh1 {
		if bh1[id].rrfScore != bh2[id].rrfScore {
			t.Errorf("non-deterministic: %s rrfScore run1=%v run2=%v", id, bh1[id].rrfScore, bh2[id].rrfScore)
		}
	}
}

// ---------------------------------------------------------------------------
// RecallOpts.Fusion default
// ---------------------------------------------------------------------------

func TestRecallOpts_FusionDefaultOff(t *testing.T) {
	var opts RecallOpts
	if opts.Fusion {
		t.Error("RecallOpts zero value must have Fusion=false (default OFF)")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func sortedKeys(m map[string]*types.Memory) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
