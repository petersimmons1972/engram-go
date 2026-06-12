package longmemeval

// retrieval_score.go — generator-free retrieval metrics for the Phase 0
// retrieval-metrics-only mode (Deliverable A).
//
// These functions score retrieved memory IDs against gold session IDs without
// calling the generator. They are the measurement instrument for the Phase 0
// BEFORE/AFTER retrieval-metric table.

// ItemRetrievalResult holds per-question retrieval metrics.
type ItemRetrievalResult struct {
	QuestionID          string  `json:"question_id"`
	QuestionType        string  `json:"question_type"`
	GoldSessionInContext bool   `json:"gold_session_in_context"` // ANY gold session present in retrieved
	GoldAllInContext     bool   `json:"gold_all_in_context"`     // ALL gold sessions present in retrieved
	RecallAt5           float64 `json:"recall@5"`
	RecallAt10          float64 `json:"recall@10"`
	NDCGAt5             float64 `json:"ndcg@5"`
	NDCGAt10            float64 `json:"ndcg@10"`
	GoldRank            int     `json:"gold_rank"` // 1-based rank of first gold hit; 0 = not found
}

// TypeRetrievalStats aggregates ItemRetrievalResults for one question type.
type TypeRetrievalStats struct {
	N                  int     `json:"n"`
	GoldInContextRate  float64 `json:"gold_in_context_rate"`
	GoldAllRate        float64 `json:"gold_all_rate"`
	AvgRecallAt5       float64 `json:"avg_recall@5"`
	AvgRecallAt10      float64 `json:"avg_recall@10"`
	AvgNDCGAt5         float64 `json:"avg_ndcg@5"`
	AvgNDCGAt10        float64 `json:"avg_ndcg@10"`
	AvgGoldRank        float64 `json:"avg_gold_rank"` // mean rank among found items (0 = none found)
}

// RetrievalReport is the full BEFORE/AFTER comparison output.
type RetrievalReport struct {
	Overall TypeRetrievalStats            `json:"overall"`
	ByType  map[string]TypeRetrievalStats `json:"by_type"`
}

// GoldSessionInContext returns true if at least one gold session ID is present
// in the retrieved memory IDs (translated via memoryMap).
func GoldSessionInContext(retrievedIDs []string, memoryMap map[string]string, goldSessionIDs []string) bool {
	if len(retrievedIDs) == 0 || len(memoryMap) == 0 || len(goldSessionIDs) == 0 {
		return false
	}
	gold := make(map[string]bool, len(goldSessionIDs))
	for _, sid := range goldSessionIDs {
		gold[sid] = true
	}
	for _, mid := range retrievedIDs {
		if sid, ok := memoryMap[mid]; ok && gold[sid] {
			return true
		}
	}
	return false
}

// GoldAllSessionsInContext returns true if ALL gold session IDs appear in the
// retrieved memory IDs (translated via memoryMap).
func GoldAllSessionsInContext(retrievedIDs []string, memoryMap map[string]string, goldSessionIDs []string) bool {
	if len(retrievedIDs) == 0 || len(memoryMap) == 0 || len(goldSessionIDs) == 0 {
		return false
	}
	// Build set of retrieved session IDs
	retrievedSessions := make(map[string]bool, len(retrievedIDs))
	for _, mid := range retrievedIDs {
		if sid, ok := memoryMap[mid]; ok {
			retrievedSessions[sid] = true
		}
	}
	for _, sid := range goldSessionIDs {
		if !retrievedSessions[sid] {
			return false
		}
	}
	return true
}

// GoldSessionRank returns the 1-based rank of the first gold session in the
// retrieved list. Returns 0 if no gold session is found.
func GoldSessionRank(retrievedIDs []string, memoryMap map[string]string, goldSessionIDs []string) int {
	if len(retrievedIDs) == 0 || len(memoryMap) == 0 || len(goldSessionIDs) == 0 {
		return 0
	}
	gold := make(map[string]bool, len(goldSessionIDs))
	for _, sid := range goldSessionIDs {
		gold[sid] = true
	}
	for i, mid := range retrievedIDs {
		if sid, ok := memoryMap[mid]; ok && gold[sid] {
			return i + 1 // 1-based
		}
	}
	return 0
}

// ScoreRetrievalForItem computes all retrieval metrics for a single question.
// retrievedIDs is the ranked list of memory IDs from recall (full recall list,
// not context-trimmed). memoryMap translates memory IDs to session IDs.
// goldSessionIDs is the gold answer_session_ids from the dataset.
func ScoreRetrievalForItem(retrievedIDs []string, memoryMap map[string]string, goldSessionIDs []string) ItemRetrievalResult {
	rankedSessions := SessionIDs(retrievedIDs, memoryMap)
	relevant := make(map[string]bool, len(goldSessionIDs))
	for _, sid := range goldSessionIDs {
		relevant[sid] = true
	}
	return ItemRetrievalResult{
		GoldSessionInContext: GoldSessionInContext(retrievedIDs, memoryMap, goldSessionIDs),
		GoldAllInContext:     GoldAllSessionsInContext(retrievedIDs, memoryMap, goldSessionIDs),
		RecallAt5:            RecallAny(rankedSessions, relevant, 5),
		RecallAt10:           RecallAny(rankedSessions, relevant, 10),
		NDCGAt5:              NDCGAny(rankedSessions, relevant, 5),
		NDCGAt10:             NDCGAny(rankedSessions, relevant, 10),
		GoldRank:             GoldSessionRank(retrievedIDs, memoryMap, goldSessionIDs),
	}
}

// AggregateRetrievalReport computes per-type and overall stats from a slice of
// ItemRetrievalResults. Items with no question type go into a "_unknown" bucket.
func AggregateRetrievalReport(results []ItemRetrievalResult) RetrievalReport {
	type accumulator struct {
		n                int
		goldInContext    int
		goldAll          int
		sumRecallAt5     float64
		sumRecallAt10    float64
		sumNDCGAt5       float64
		sumNDCGAt10      float64
		sumGoldRankFound float64
		goldRankFound    int
	}
	overall := &accumulator{}
	byType := map[string]*accumulator{}

	for _, r := range results {
		qt := r.QuestionType
		if qt == "" {
			qt = "_unknown"
		}
		if byType[qt] == nil {
			byType[qt] = &accumulator{}
		}
		acc := byType[qt]
		for _, a := range []*accumulator{overall, acc} {
			a.n++
			if r.GoldSessionInContext {
				a.goldInContext++
			}
			if r.GoldAllInContext {
				a.goldAll++
			}
			a.sumRecallAt5 += r.RecallAt5
			a.sumRecallAt10 += r.RecallAt10
			a.sumNDCGAt5 += r.NDCGAt5
			a.sumNDCGAt10 += r.NDCGAt10
			if r.GoldRank > 0 {
				a.sumGoldRankFound += float64(r.GoldRank)
				a.goldRankFound++
			}
		}
	}

	toStats := func(a *accumulator) TypeRetrievalStats {
		if a.n == 0 {
			return TypeRetrievalStats{}
		}
		s := TypeRetrievalStats{
			N:                 a.n,
			GoldInContextRate: float64(a.goldInContext) / float64(a.n),
			GoldAllRate:       float64(a.goldAll) / float64(a.n),
			AvgRecallAt5:      a.sumRecallAt5 / float64(a.n),
			AvgRecallAt10:     a.sumRecallAt10 / float64(a.n),
			AvgNDCGAt5:        a.sumNDCGAt5 / float64(a.n),
			AvgNDCGAt10:       a.sumNDCGAt10 / float64(a.n),
		}
		if a.goldRankFound > 0 {
			s.AvgGoldRank = a.sumGoldRankFound / float64(a.goldRankFound)
		}
		return s
	}

	report := RetrievalReport{
		Overall: toStats(overall),
		ByType:  make(map[string]TypeRetrievalStats, len(byType)),
	}
	for qt, acc := range byType {
		report.ByType[qt] = toStats(acc)
	}
	return report
}
