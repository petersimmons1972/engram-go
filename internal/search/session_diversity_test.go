package search

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/types"
)

// makeDivResult builds a minimal SearchResult with a session tag and a score.
// sid="" produces a result with no sid: tag (missing-tag case).
// Named makeDivResult to avoid collision with makeResult in session_ndcg_agg_test.go.
func makeDivResult(sid string, score float64) types.SearchResult {
	m := &types.Memory{
		ID:      "mem-" + sid,
		Content: "content for " + sid,
	}
	if sid != "" {
		m.Tags = []string{"sid:" + sid}
	}
	return types.SearchResult{Memory: m, Score: score}
}

// makeDivResultWithTag builds a result where the raw tag is provided verbatim (for
// edge cases like "sid:" with no value, or an unrelated tag prefix).
func makeDivResultWithTag(tag string, score float64) types.SearchResult {
	m := &types.Memory{
		ID:      "mem-rawtag",
		Content: "content",
		Tags:    []string{tag},
	}
	return types.SearchResult{Memory: m, Score: score}
}

// divSessionIDs extracts the session IDs from a result slice for assertions.
func divSessionIDs(results []types.SearchResult) []string {
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = ExtractSessionID(r.Memory.Tags)
	}
	return out
}

// TestSessionDiversity_NZeroIsBaseline is the FIRST test written per TDD discipline.
// When N=0 (env var absent/zero), applySessionDiversity must return the input slice
// unchanged — no allocation, no reordering (regression guard).
func TestSessionDiversity_NZeroIsBaseline(t *testing.T) {
	input := []types.SearchResult{
		makeDivResult("s1", 0.9),
		makeDivResult("s2", 0.8),
		makeDivResult("s1", 0.7),
	}
	got := applySessionDiversity(input, 5, 0)
	// Must be the SAME slice (pointer equality) — no copy made.
	if &got[0] != &input[0] {
		t.Fatal("N=0: applySessionDiversity must return the input slice unchanged (same pointer)")
	}
	if len(got) != len(input) {
		t.Fatalf("N=0: got len %d, want %d", len(got), len(input))
	}
}

// TestSessionDiversity_HappyPath_GoldInMinoritySession verifies the core use case:
// a gold chunk from a minority session (sid:s2) is promoted into the top-K window
// when the majority session (sid:s1) is capped at N chunks.
//
// Setup: s1 floods with 4 high-scoring chunks; s2 has one gold chunk at lower score;
// topK=4, N=2. Without diversity, all 4 slots go to s1. With diversity, s1 is capped
// at 2, freeing a slot for s2:gold.
func TestSessionDiversity_HappyPath_GoldInMinoritySession(t *testing.T) {
	input := []types.SearchResult{
		makeDivResult("s1", 0.95),
		makeDivResult("s1", 0.90),
		makeDivResult("s1", 0.85),
		makeDivResult("s1", 0.80),
		makeDivResult("s2", 0.70), // gold — buried without diversity
	}
	got := applySessionDiversity(input, 4, 2)
	// s2:gold must appear in the result.
	found := false
	for _, r := range got {
		if ExtractSessionID(r.Memory.Tags) == "s2" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("sid:s2 gold chunk missing from top-%d with N=2; got sessions %v", 4, divSessionIDs(got))
	}
	// No session contributes more than N=2 chunks.
	count := map[string]int{}
	for _, r := range got {
		count[ExtractSessionID(r.Memory.Tags)]++
	}
	for sid, n := range count {
		if n > 2 {
			t.Errorf("session %q contributes %d chunks; max is N=2", sid, n)
		}
	}
}

// TestSessionDiversity_AllChunksSingleSession verifies the dynamic gate:
// when all chunks belong to one session, shouldApplySessionDiversity returns false
// and applySessionDiversity returns the input slice unmodified.
func TestSessionDiversity_AllChunksSingleSession(t *testing.T) {
	input := []types.SearchResult{
		makeDivResult("s1", 0.9),
		makeDivResult("s1", 0.8),
		makeDivResult("s1", 0.7),
		makeDivResult("s1", 0.6),
		makeDivResult("s1", 0.5),
	}
	// Gate must fire: 1 distinct session → no-op.
	if shouldApplySessionDiversity(input, 2) {
		t.Fatal("shouldApplySessionDiversity must return false when only 1 session present")
	}
	got := applySessionDiversity(input, 5, 2)
	if &got[0] != &input[0] {
		t.Fatal("single-session: applySessionDiversity must return input unchanged (same pointer)")
	}
}

// TestSessionDiversity_MissingOrEmptySessionID verifies graceful handling of
// memories with no sid: tag, an empty sid: value, or an unrelated tag.
// Requirements: no crash; missing/empty tags receive the empty-string session ID
// as their fallback; output is deterministic.
func TestSessionDiversity_MissingOrEmptySessionID(t *testing.T) {
	input := []types.SearchResult{
		makeDivResult("s1", 0.9),
		makeDivResultWithTag("sid:", 0.8),                                   // empty value after prefix
		{Memory: &types.Memory{ID: "no-tag", Content: "x"}, Score: 0.7},    // no tags at all
		makeDivResultWithTag("category:foo", 0.65),                          // no sid: tag at all
		makeDivResult("s2", 0.6),
	}
	// Must not panic.
	got := applySessionDiversity(input, 5, 3)
	// Output must have a deterministic, positive length.
	if len(got) == 0 {
		t.Fatal("expected non-empty output")
	}
	// All results must have non-nil Memory.
	for i, r := range got {
		if r.Memory == nil {
			t.Errorf("result[%d] has nil Memory", i)
		}
	}
	// Run twice: must produce the same order (determinism guard).
	got2 := applySessionDiversity(input, 5, 3)
	if len(got) != len(got2) {
		t.Errorf("non-deterministic: first run len=%d, second run len=%d", len(got), len(got2))
	}
	for i := range got {
		if got[i].Memory.ID != got2[i].Memory.ID {
			t.Errorf("non-deterministic at index %d: %s vs %s", i, got[i].Memory.ID, got2[i].Memory.ID)
		}
	}
}

// TestSessionDiversity_TopK3_N2_TwoSessions verifies the boundary case:
// s1 has 4 chunks (scores 0.9, 0.8, 0.7, 0.6), s2 has 2 chunks (scores 0.5, 0.4),
// topK=3, N=2. At most 2 chunks from any one session may appear in the result.
func TestSessionDiversity_TopK3_N2_TwoSessions(t *testing.T) {
	input := []types.SearchResult{
		makeDivResult("s1", 0.9),
		makeDivResult("s1", 0.8),
		makeDivResult("s1", 0.7),
		makeDivResult("s1", 0.6),
		makeDivResult("s2", 0.5),
		makeDivResult("s2", 0.4),
	}
	got := applySessionDiversity(input, 3, 2)
	if len(got) != 3 {
		t.Fatalf("expected 3 results, got %d", len(got))
	}
	count := map[string]int{}
	for _, r := range got {
		count[ExtractSessionID(r.Memory.Tags)]++
	}
	for sid, n := range count {
		if n > 2 {
			t.Errorf("session %q contributes %d chunks in topK=3 with N=2; max is 2", sid, n)
		}
	}
}

// TestSessionDiversity_AlreadyDiverseOrShortInput verifies that when the input is
// shorter than topK, the function does not panic and returns a slice whose length
// equals the input length (not topK).
func TestSessionDiversity_AlreadyDiverseOrShortInput(t *testing.T) {
	input := []types.SearchResult{
		makeDivResult("s1", 0.9),
		makeDivResult("s2", 0.8),
		makeDivResult("s1", 0.7),
	}
	// topK=10 >> input length=3; N=2 caps each session.
	got := applySessionDiversity(input, 10, 2)
	if len(got) != len(input) {
		t.Fatalf("short input: got len %d, want %d (== input length)", len(got), len(input))
	}
}

// TestSessionDiversity_DynamicGateSkipsWhenSingleSession verifies that
// shouldApplySessionDiversity returns false and the result slice is returned
// unmodified when all results share the same session ID. This test exercises the
// gate contract independently of applySessionDiversity to keep the invariant
// sharply documented.
func TestSessionDiversity_DynamicGateSkipsWhenSingleSession(t *testing.T) {
	input := []types.SearchResult{
		makeDivResult("solo", 0.9),
		makeDivResult("solo", 0.7),
		makeDivResult("solo", 0.5),
	}
	if shouldApplySessionDiversity(input, 2) {
		t.Error("shouldApplySessionDiversity must return false for single-session input")
	}
	out := applySessionDiversity(input, 3, 2)
	if len(out) != len(input) {
		t.Fatalf("dynamic gate: expected output len %d, got %d", len(input), len(out))
	}
	// Pointer equality: same backing slice returned.
	if len(out) > 0 && &out[0] != &input[0] {
		t.Error("dynamic gate: must return the same slice (no allocation when gate fires)")
	}
}
