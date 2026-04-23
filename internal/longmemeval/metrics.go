package longmemeval

import "github.com/petersimmons1972/engram/internal/eval"

// SessionIDs maps a ranked list of memory IDs to session IDs using the
// memory_id → session_id map built during ingestion. IDs not in the map are omitted.
func SessionIDs(memoryIDs []string, memoryMap map[string]string) []string {
	out := make([]string, 0, len(memoryIDs))
	for _, mid := range memoryIDs {
		if sid, ok := memoryMap[mid]; ok {
			out = append(out, sid)
		}
	}
	return out
}

// RecallAny returns 1.0 if at least one relevant session appears in the top-k
// retrieved sessions, 0.0 otherwise.
func RecallAny(retrieved []string, relevant map[string]bool, k int) float64 {
	if len(retrieved) == 0 || len(relevant) == 0 || k <= 0 {
		return 0
	}
	limit := k
	if limit > len(retrieved) {
		limit = len(retrieved)
	}
	for i := 0; i < limit; i++ {
		if relevant[retrieved[i]] {
			return 1.0
		}
	}
	return 0
}

// RecallAll returns 1.0 if all relevant sessions appear in the top-k retrieved
// sessions, 0.0 otherwise.
func RecallAll(retrieved []string, relevant map[string]bool, k int) float64 {
	if len(retrieved) == 0 || len(relevant) == 0 || k <= 0 {
		return 0
	}
	limit := k
	if limit > len(retrieved) {
		limit = len(retrieved)
	}
	inTopK := make(map[string]bool, limit)
	for i := 0; i < limit; i++ {
		inTopK[retrieved[i]] = true
	}
	for sid := range relevant {
		if !inTopK[sid] {
			return 0
		}
	}
	return 1.0
}

// NDCGAny wraps internal/eval.NDCG treating any relevant session as binary relevant.
func NDCGAny(retrieved []string, relevant map[string]bool, k int) float64 {
	return eval.NDCG(retrieved, relevant, k)
}

// BuildRetrievalMetrics computes the four session-level metrics at k=5 and k=10.
func BuildRetrievalMetrics(rankedSessionIDs []string, answerSessionIDs []string) RetrievalMetrics {
	relevant := make(map[string]bool, len(answerSessionIDs))
	for _, sid := range answerSessionIDs {
		relevant[sid] = true
	}
	return RetrievalMetrics{
		RecallAll5:  RecallAll(rankedSessionIDs, relevant, 5),
		NDCGAny5:    NDCGAny(rankedSessionIDs, relevant, 5),
		RecallAll10: RecallAll(rankedSessionIDs, relevant, 10),
		NDCGAny10:   NDCGAny(rankedSessionIDs, relevant, 10),
	}
}
