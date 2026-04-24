// Package eval provides precision/recall metrics for retrieval evaluation.
package eval

import "math"

// PrecisionAtK computes the fraction of the top-k retrieved items that are relevant.
// Returns 0 for k<=0 or empty retrieved.
func PrecisionAtK(retrieved []string, relevant map[string]bool, k int) float64 {
	if k <= 0 || len(retrieved) == 0 {
		return 0
	}
	limit := k
	if limit > len(retrieved) {
		limit = len(retrieved)
	}
	hits := 0
	for i := 0; i < limit; i++ {
		if relevant[retrieved[i]] {
			hits++
		}
	}
	return float64(hits) / float64(limit)
}

// MRR computes the Reciprocal Rank of the first relevant item.
// Returns 0 when no relevant item is found or retrieved is empty.
func MRR(retrieved []string, relevant map[string]bool) float64 {
	for i, id := range retrieved {
		if relevant[id] {
			return 1.0 / float64(i+1)
		}
	}
	return 0
}

// NDCG computes Normalized Discounted Cumulative Gain at k.
// Returns 0 when retrieved or relevant is empty, or idealDCG is 0.
func NDCG(retrieved []string, relevant map[string]bool, k int) float64 {
	if len(retrieved) == 0 || len(relevant) == 0 {
		return 0
	}
	limit := k
	if limit > len(retrieved) {
		limit = len(retrieved)
	}
	dcg := 0.0
	for i := 0; i < limit; i++ {
		if relevant[retrieved[i]] {
			dcg += 1.0 / math.Log2(float64(i+2))
		}
	}
	idealLimit := len(relevant)
	if idealLimit > k {
		idealLimit = k
	}
	idealDCG := 0.0
	for i := 0; i < idealLimit; i++ {
		idealDCG += 1.0 / math.Log2(float64(i+2))
	}
	if idealDCG == 0 {
		return 0
	}
	return dcg / idealDCG
}

// QueryResult holds evaluation metrics for one golden query.
type QueryResult struct {
	Query      string
	PrecisionK float64
	MRR        float64
	NDCG       float64
	Retrieved  []string
}
