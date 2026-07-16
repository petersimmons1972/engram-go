// preference_mmr.go — Centroid-MMR diversity pass for H-NEW-2.
//
// Experiment: When PreferenceMMR is enabled in RecallOpts, a post-processing
// pass re-scores preference-query results using Maximal Marginal Relevance
// against the dominant-topic centroid. This surfaces domain-specific preference
// sessions that pure cosine similarity buries under the user's dominant topic
// (e.g. books/reading matching a generic preference query).
//
// Algorithm (Approach A from advisory-gate decision):
//  1. Sort primary results by composite score.
//  2. Compute centroid of top-N (default 10) result embeddings.
//  3. Re-score ALL candidates: mmr_score = λ·relevance - (1-λ)·sim(doc,centroid)
//  4. Re-sort by mmr_score, return topK.
//
// λ default: 0.7 (70% relevance, 30% anti-centroid diversity penalty).
// Ablatable: flag-off (PreferenceMMR=false) → baseline unchanged.
// Deterministic: centroid + linear formula produces identical output for
// identical inputs.
package search

import (
	"math"
	"sort"
)

const (
	// mmrLambdaDefault is the relevance weight in the MMR formula.
	// Advisory recommendation: 0.7 (conservative start; lower if 0 improvement).
	mmrLambdaDefault = 0.7

	// mmrCentroidPoolSize is the number of top results used to compute the
	// dominant-cluster centroid. Using top-10 captures the dominant topic cluster
	// without diluting the centroid with lower-ranked, less-representative results.
	mmrCentroidPoolSize = 10
)

// mmrCandidate bundles the fields needed for MMR re-scoring.
type mmrCandidate struct {
	memoryID  string
	relevance float64   // composite score from primary recall
	embedding []float32 // best-chunk embedding for this memory; nil = no embedding
}

// computeCentroid returns the element-wise mean of vecs. Vectors shorter than
// the first vector's length are skipped (dimension mismatch guard). Returns nil
// when vecs is empty.
func computeCentroid(vecs [][]float32) []float32 {
	if len(vecs) == 0 {
		return nil
	}
	// Determine dimensionality from the first vector.
	dims := len(vecs[0])
	if dims == 0 {
		return nil
	}

	sum := make([]float64, dims)
	count := 0
	for _, v := range vecs {
		if len(v) < dims {
			continue // skip dim-mismatched vectors
		}
		for i := 0; i < dims; i++ {
			sum[i] += float64(v[i])
		}
		count++
	}
	if count == 0 {
		return nil
	}
	out := make([]float32, dims)
	for i, s := range sum {
		out[i] = float32(s / float64(count))
	}
	return out
}

// cosineSimilarityF32 computes the cosine similarity between two float32 vectors.
// Returns 0.0 for zero-length vectors or dimension mismatches.
func cosineSimilarityF32(a, b []float32) float32 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0.0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0.0
	}
	return float32(dot / denom)
}

// mmrReScore re-scores candidates using the centroid-MMR formula:
//
//	score = lambda * relevance - (1-lambda) * cosineSim(embedding, centroid)
//
// A high score means the candidate is relevant AND far from the dominant cluster.
// Returns the candidates sorted descending by MMR score.
//
// Nil centroid or lambda==1.0 degrades to pure relevance ordering (no-op baseline).
// Input slice is not modified; a new sorted slice is returned.
func mmrReScore(candidates []mmrCandidate, centroid []float32, lambda float64) []mmrCandidate {
	if len(candidates) == 0 {
		return nil
	}
	type scored struct {
		c     mmrCandidate
		score float64
	}
	out := make([]scored, len(candidates))
	for i, c := range candidates {
		var antiCentroid float64
		if centroid != nil && lambda < 1.0 {
			sim := cosineSimilarityF32(c.embedding, centroid)
			antiCentroid = float64(sim)
		}
		mmr := lambda*c.relevance - (1-lambda)*antiCentroid
		out[i] = scored{c: c, score: mmr}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].score > out[j].score
	})
	result := make([]mmrCandidate, len(out))
	for i, s := range out {
		result[i] = s.c
	}
	return result
}
