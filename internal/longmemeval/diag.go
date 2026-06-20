package longmemeval

// diag.go — H-DIAG gold visibility diagnostic fields for the score report.
//
// ComputeDiag takes the full ranked retrieved-memory-ID list, the memory→session
// map, the gold session IDs, and the context window size (contextTopK) and
// returns a DiagFields struct that can be attached to the score report.
//
// These fields enable per-mechanism debugging without requiring a full generation
// re-run:
//   - RetrievedGoldRank     — 1-based rank of first gold chunk; 0 = not found
//   - GoldVisibleInContext  — whether the first gold chunk was within contextTopK
//   - SessionDominanceRatio — fraction of retrieved IDs from the single
//     most-represented session (computed over the full retrieved list)

// DiagFields holds the H-DIAG diagnostic values for one question.
type DiagFields struct {
	// RetrievedGoldRank is the 1-based rank of the first gold session in the
	// full retrieved-IDs list (translated via memoryMap). Zero means no gold
	// session was retrieved at all.
	RetrievedGoldRank int `json:"retrieved_gold_rank"`

	// GoldVisibleInContext is true when the first gold session appears within
	// the top contextTopK retrieved IDs (i.e. was in the generator context
	// window). False when gold was retrieved but ranked below the context
	// cutoff, or was not retrieved at all.
	GoldVisibleInContext bool `json:"gold_visible_in_context"`

	// SessionDominanceRatio is the fraction of the full retrieved-IDs list
	// that maps to the single most-represented session. A ratio of 1.0 means
	// all retrieved chunks came from one session. Computed over the full recall
	// list (not trimmed to contextTopK) so it captures recall-stage diversity.
	SessionDominanceRatio float64 `json:"session_dominance_ratio"`
}

// ComputeDiag computes diagnostic fields for one question.
//
//   - retrievedIDs  — full ranked memory-ID list from recall (recall-topK length)
//   - memoryMap     — memory_id → session_id translation table
//   - goldSessionIDs — gold answer_session_ids from the dataset
//   - contextTopK   — how many of the top retrieved IDs entered the generator
//     context window; 0 means unlimited (all retrieved IDs are in context)
func ComputeDiag(retrievedIDs []string, memoryMap map[string]string, goldSessionIDs []string, contextTopK int) DiagFields {
	if len(retrievedIDs) == 0 || len(memoryMap) == 0 {
		return DiagFields{}
	}

	// --- retrieved_gold_rank ---
	goldSet := make(map[string]bool, len(goldSessionIDs))
	for _, sid := range goldSessionIDs {
		goldSet[sid] = true
	}
	goldRank := 0
	for i, mid := range retrievedIDs {
		if sid, ok := memoryMap[mid]; ok && goldSet[sid] {
			goldRank = i + 1 // 1-based
			break
		}
	}

	// --- gold_visible_in_context ---
	var goldVisible bool
	if goldRank > 0 {
		if contextTopK <= 0 {
			// unlimited context: all retrieved IDs are visible
			goldVisible = true
		} else {
			goldVisible = goldRank <= contextTopK
		}
	}

	// --- session_dominance_ratio ---
	// Count how many retrieved IDs map to each session, over the full list.
	sessionCounts := make(map[string]int)
	var mapped int
	for _, mid := range retrievedIDs {
		if sid, ok := memoryMap[mid]; ok {
			sessionCounts[sid]++
			mapped++
		}
	}
	var dominance float64
	if mapped > 0 {
		var maxCount int
		for _, c := range sessionCounts {
			if c > maxCount {
				maxCount = c
			}
		}
		dominance = float64(maxCount) / float64(mapped)
	}

	return DiagFields{
		RetrievedGoldRank:     goldRank,
		GoldVisibleInContext:  goldVisible,
		SessionDominanceRatio: dominance,
	}
}
