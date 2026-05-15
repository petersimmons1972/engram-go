package longmemeval

import "strings"

func RecallAny(retrieved []string, relevant map[string]bool, k int) float64 {
	if len(retrieved) == 0 || len(relevant) == 0 || k <= 0 {
		return 0
	}
	for i, id := range retrieved {
		if i >= k {
			break
		}
		if relevant[id] {
			return 1
		}
	}
	return 0
}

func RecallAll(retrieved []string, relevant map[string]bool, k int) float64 {
	if len(retrieved) == 0 || len(relevant) == 0 || k <= 0 {
		return 0
	}
	for id := range relevant {
		found := false
		for i, rid := range retrieved {
			if i >= k {
				break
			}
			if rid == id {
				found = true
				break
			}
		}
		if !found {
			return 0
		}
	}
	return 1
}

func SessionIDs(retrieved []string, memoryMap map[string]string) []string {
	out := make([]string, 0, len(retrieved))
	for _, id := range retrieved {
		if sid, ok := memoryMap[id]; ok {
			out = append(out, sid)
		}
	}
	return out
}

func BuildRetrievalMetrics(sessionIDs []string, answerSessionIDs []string) RetrievalMetrics {
	relevant := make(map[string]bool, len(answerSessionIDs))
	for _, id := range answerSessionIDs {
		relevant[id] = true
	}
	return RetrievalMetrics{
		RecallAll5:  RecallAll(sessionIDs, relevant, 5),
		NDCGAny5:    ndcgAny(sessionIDs, relevant, 5),
		RecallAll10: RecallAll(sessionIDs, relevant, 10),
		NDCGAny10:   ndcgAny(sessionIDs, relevant, 10),
	}
}

func ndcgAny(retrieved []string, relevant map[string]bool, k int) float64 {
	for i, id := range retrieved {
		if i >= k {
			break
		}
		if relevant[id] {
			return 1 / float64(i+1)
		}
	}
	return 0
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

