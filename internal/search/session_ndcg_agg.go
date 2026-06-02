// session_ndcg_agg.go — LEVER-8: session-level DCG aggregation for recall re-ranking.
//
// EmergenceMem (emergence.ai) achieves SOTA on LongMemEval-S by retrieving at
// turn/chunk granularity but ranking returned *sessions* by the aggregate DCG of
// their constituent turn scores, then packing the session's turns together before
// returning to the caller. This gives credit to sessions where multiple evidence
// turns each score mid-pack — a pattern the current max-cosine approach misses.
//
// Flag-gated: RecallOpts.SessionNDCGAgg must be true (default false). When false,
// the entire code path is bypassed and results are identical to the baseline.
// No schema change, no re-ingest required.
//
// Reference: emergence.ai SOTA on LongMemEval-S, June 2026.
// Experiment tracker: LEVER-8.
package search

import (
	"math"
	"sort"
	"strings"

	"github.com/petersimmons1972/engram/internal/types"
)

// extractSessionID scans tags for a "sid:<value>" entry and returns the value.
// Returns "" if no sid: tag is present or the value after the prefix is empty.
// Used by sessionNDCGRerank to group memories by their originating LME session.
func extractSessionID(tags []string) string {
	const prefix = "sid:"
	for _, tag := range tags {
		if strings.HasPrefix(tag, prefix) {
			v := tag[len(prefix):]
			return v // empty string if tag is exactly "sid:"
		}
	}
	return ""
}

// sessionDCG computes the Discounted Cumulative Gain (DCG) of a set of chunk
// cosine similarity scores for a single session. Scores are sorted descending
// before applying positional discounts, so the strongest chunk occupies rank 1.
//
// Formula: DCG = Σ score_i / log2(rank_i + 1), rank starting at 1.
//
// DCG is chosen over NDCG because:
//  1. In LME each session is stored as one Memory; chunk count is a proxy for
//     session information density. DCG's additive nature rewards richer sessions.
//  2. Normalization (NDCG) requires knowing the ideal ordering per-session, which
//     adds complexity with no measurable benefit when session sizes are similar.
//
// An empty slice returns 0.0.
func sessionDCG(scores []float64) float64 {
	if len(scores) == 0 {
		return 0.0
	}
	// Sort a copy descending so we don't mutate the caller's slice.
	sorted := make([]float64, len(scores))
	copy(sorted, scores)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] > sorted[j] })

	var dcg float64
	for i, s := range sorted {
		rank := i + 1 // 1-based
		dcg += s / math.Log2(float64(rank)+1)
	}
	return dcg
}

// sessionGroup is one LME session group produced during re-ranking.
type sessionGroup struct {
	sessionID  string              // "" for untagged (no sid: tag) memories
	members    []types.SearchResult // results belonging to this session
	dcgScore   float64             // aggregate DCG of chunk cosines
}

// sessionNDCGRerank re-ranks results by session-DCG aggregation (LEVER-8).
//
// When enabled is false, results are returned unchanged (baseline identity).
// When enabled is true:
//  1. Group results by their sid: tag (untagged memories form singleton groups).
//  2. Compute each group's DCG from allChunkCosines (map[memoryID][]float64).
//     Memories absent from allChunkCosines are treated as having dcg=0.
//  3. Sort groups by DCG descending; break DCG ties by the group's best
//     individual composite score (deterministic).
//  4. Within each group, sort members by composite score descending (P1 policy).
//  5. Emit the final flat list: group 1's members, then group 2's, etc.
//
// The returned slice has the same length as results and contains no duplicates.
// allChunkCosines may be nil when enabled is false.
func sessionNDCGRerank(results []types.SearchResult, allChunkCosines map[string][]float64, enabled bool) []types.SearchResult {
	if !enabled || len(results) == 0 {
		return results
	}

	// Step 1: group by session_id.
	// Use a slice of groups to preserve deterministic insertion order as the
	// tiebreak for equal-DCG groups (maps have non-deterministic iteration).
	groupIndex := make(map[string]int) // sessionID → index in groups
	var groups []sessionGroup
	addToGroup := func(sid string, r types.SearchResult) {
		idx, ok := groupIndex[sid]
		if !ok {
			idx = len(groups)
			groupIndex[sid] = idx
			groups = append(groups, sessionGroup{sessionID: sid})
		}
		groups[idx].members = append(groups[idx].members, r)
	}

	for _, r := range results {
		var sid string
		if r.Memory != nil {
			sid = extractSessionID(r.Memory.Tags)
		}
		// Use the memory's ID as a unique singleton key when no sid: tag is present.
		// This ensures untagged memories each get their own group (no cross-memory
		// grouping) and prevents a nil-memory entry from silently merging into an
		// unrelated untagged group.
		if sid == "" {
			if r.Memory != nil {
				sid = "\x00no-sid:" + r.Memory.ID // unique key; never matches a real sid
			} else {
				sid = "\x00no-sid:nil"
			}
		}
		addToGroup(sid, r)
	}

	// Step 2: compute DCG per group.
	for i := range groups {
		var cosines []float64
		for _, r := range groups[i].members {
			if r.Memory == nil {
				continue
			}
			cosines = append(cosines, allChunkCosines[r.Memory.ID]...)
		}
		groups[i].dcgScore = sessionDCG(cosines)
	}

	// Step 3: sort groups by DCG descending; break ties by best composite score.
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].dcgScore != groups[j].dcgScore {
			return groups[i].dcgScore > groups[j].dcgScore
		}
		// Tiebreak: group with higher best composite score ranks first.
		return bestScore(groups[i].members) > bestScore(groups[j].members)
	})

	// Step 4: within each group, sort by composite score descending (P1).
	for i := range groups {
		sort.SliceStable(groups[i].members, func(a, b int) bool {
			return groups[i].members[a].Score > groups[i].members[b].Score
		})
	}

	// Step 5: flatten into output slice.
	out := make([]types.SearchResult, 0, len(results))
	for _, g := range groups {
		out = append(out, g.members...)
	}
	return out
}

// bestScore returns the highest composite Score in a slice of results.
// Returns 0.0 for an empty slice.
func bestScore(results []types.SearchResult) float64 {
	best := 0.0
	for _, r := range results {
		if r.Score > best {
			best = r.Score
		}
	}
	return best
}
