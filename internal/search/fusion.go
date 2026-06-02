// fusion.go — Reciprocal Rank Fusion (RRF) candidate merging for RecallWithOpts.
//
// LME experiment #6 — issue #938 improvement #1.
//
// When RecallOpts.Fusion is true, the engine pre-computes 1-based rank
// positions for each memory ID from the vector (ANN) and BM25/FTS retrieval
// legs, then computes an RRF score for each candidate. The RRF score is stored
// in bestHit.rrfScore and passed to CompositeScoreRRF in place of the raw
// cosine and BM25 signals.
//
// Flag-gate: when Fusion=false (default), applyFusion is a strict no-op.
package search

import (
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/types"
)

const rrfK = 60 // standard RRF constant (Cormack et al. 2009)

// rankVectorHits assigns 1-based rank positions to memory IDs from a slice of
// VectorHit values ordered by ascending cosine distance (pgvector order).
// When a memory ID appears multiple times (multiple chunks), only the first
// occurrence is ranked — it corresponds to the best (lowest-distance) chunk.
func rankVectorHits(hits []db.VectorHit) map[string]int {
	ranks := make(map[string]int, len(hits))
	rank := 0
	for _, h := range hits {
		if _, seen := ranks[h.MemoryID]; !seen {
			rank++
			ranks[h.MemoryID] = rank
		}
	}
	return ranks
}

// rankFTSResults assigns 1-based rank positions to memory IDs from a slice of
// FTSResult values ordered by descending BM25 score (highest score = rank 1).
// Results with nil Memory fields are skipped.
func rankFTSResults(results []db.FTSResult) map[string]int {
	ranks := make(map[string]int, len(results))
	rank := 0
	for _, r := range results {
		if r.Memory == nil {
			continue
		}
		rank++
		if _, seen := ranks[r.Memory.ID]; !seen {
			ranks[r.Memory.ID] = rank
		}
	}
	return ranks
}

// applyFusion computes and stores RRF scores into the bestHit map for every
// memory in memories. When opts.Fusion is false it is a strict no-op.
//
// After applyFusion, the scoring loop in RecallWithOpts detects opts.Fusion and
// calls CompositeScoreRRF instead of CompositeScoreWithWeights, passing
// hit.rrfScore in place of the raw cosine and BM25 signals.
func applyFusion(opts RecallOpts, memories map[string]*types.Memory, bestHits map[string]bestHit, vecRanks, ftsRanks map[string]int) {
	if !opts.Fusion {
		return
	}
	for id := range memories {
		vr := vecRanks[id] // 0 if absent
		fr := ftsRanks[id] // 0 if absent
		score := RRFScore(vr, fr, rrfK)
		h := bestHits[id]
		h.rrfScore = score
		bestHits[id] = h
	}
}
