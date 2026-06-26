// session_diversity.go — LEVER-9: post-ranking session-diversity sampling.
//
// After composite scoring and SessionNDCGAgg re-ranking, the result set may be
// dominated by chunks from a single LME session — the highest-scoring session
// "floods" the top-K window and buries gold chunks from minority sessions.
//
// applySessionDiversity caps the per-session contribution at N chunks, ensuring
// that no one session crowds out evidence from other sessions. Remaining topK
// slots are filled from the leftover ranked pool.
//
// Flag-gated: RecallOpts.SessionDiversityN (default 0 = off). When 0, the entire
// code path is bypassed and results are byte-identical to the baseline.
//
// Dynamic gate: when all returned chunks belong to a single session (≤1 distinct
// session ID), shouldApplySessionDiversity returns false and results are returned
// unchanged. This eliminates single-session regression risk.
//
// No question_type gating (FM-77): the gate is driven entirely by observable data
// signals (distinct session count, N value, topK).
//
// Reference: LME multi-session category 27.7% → 35.6% target; three-way socialization
// outcome 2026-06-16; plan: ~/.claude/plans/engram-session-diversity.md.
// Experiment tracker: LEVER-9.
package search

import (
	"os"
	"strconv"

	"github.com/petersimmons1972/engram/internal/types"
)

// applySessionDiversity caps the per-session chunk contribution at N and returns
// at most topK results. The input slice must already be sorted by score descending
// (as produced by sortResults + sessionNDCGRerank). The per-session cap is enforced
// by a single linear scan: each chunk is taken if its session has contributed fewer
// than N chunks so far, skipped otherwise. After the scan, excess items beyond
// topK are trimmed. The function is a pure transformation — it does not sort, it
// does not inspect content, and it never returns more items than it received.
//
// When N == 0 or shouldApplySessionDiversity returns false the input slice is
// returned unchanged (no allocation).
//
// Tie-breaking: within the same session, earlier (higher-scoring) chunks are taken
// first because the input is score-sorted before this call. Cross-session order is
// also score-based for the same reason. This is the documented and locked semantic.
func applySessionDiversity(results []types.SearchResult, topK, N int) []types.SearchResult {
	if !shouldApplySessionDiversity(results, N) {
		return results
	}

	sessionCount := make(map[string]int, 8)
	out := make([]types.SearchResult, 0, topK)

	for _, r := range results {
		if len(out) >= topK {
			break
		}
		var sid string
		if r.Memory != nil {
			sid = ExtractSessionID(r.Memory.Tags)
		}
		if sessionCount[sid] < N {
			out = append(out, r)
			sessionCount[sid]++
		}
	}
	return out
}

// shouldApplySessionDiversity returns true only when the diversity pass should run:
// N must be positive and the result set must contain chunks from at least two
// distinct sessions. When all chunks share a session (or N == 0), the pass is a
// no-op by definition and is skipped entirely to preserve baseline identity.
//
// "Distinct session" is determined by ExtractSessionID on each Memory's Tags slice.
// Chunks with no sid: tag (or an empty sid: value) are grouped under the empty-string
// session ID — they count as one distinct session together.
func shouldApplySessionDiversity(results []types.SearchResult, N int) bool {
	if N <= 0 || len(results) == 0 {
		return false
	}
	seen := make(map[string]struct{}, 4)
	for _, r := range results {
		if r.Memory == nil {
			continue
		}
		seen[ExtractSessionID(r.Memory.Tags)] = struct{}{}
		if len(seen) >= 2 {
			return true
		}
	}
	return false
}

// SessionDiversityNFromEnv reads ENGRAM_SESSION_DIVERSITY_N from the environment.
// Returns 0 if the variable is absent, empty, or not a valid non-negative integer.
// 0 means the diversity pass is disabled (baseline-identity contract).
func SessionDiversityNFromEnv() int {
	v := os.Getenv("ENGRAM_SESSION_DIVERSITY_N")
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
