// session_ndcg_agg_test.go — unit tests for LEVER-8 session-DCG aggregation.
//
// Tests are pure (no DB, no network) because sessionDCG, extractSessionID,
// and sessionNDCGRerank are all pure functions declared in the same package.
package search

import (
	"math"
	"testing"

	"github.com/petersimmons1972/engram/internal/types"
)

// ---------------------------------------------------------------------------
// extractSessionID
// ---------------------------------------------------------------------------

func TestExtractSessionID_Present(t *testing.T) {
	tags := []string{"lme", "sid:abc123", "date:2024-01-01"}
	got := extractSessionID(tags)
	if got != "abc123" {
		t.Errorf("extractSessionID = %q, want %q", got, "abc123")
	}
}

func TestExtractSessionID_Missing(t *testing.T) {
	tags := []string{"lme", "date:2024-01-01"}
	got := extractSessionID(tags)
	if got != "" {
		t.Errorf("extractSessionID = %q, want empty string", got)
	}
}

func TestExtractSessionID_Empty(t *testing.T) {
	got := extractSessionID(nil)
	if got != "" {
		t.Errorf("extractSessionID(nil) = %q, want empty string", got)
	}
}

func TestExtractSessionID_SidEmptyValue(t *testing.T) {
	// "sid:" with empty value returns ""
	tags := []string{"sid:"}
	got := extractSessionID(tags)
	if got != "" {
		t.Errorf("extractSessionID([sid:]) = %q, want empty string", got)
	}
}

// ---------------------------------------------------------------------------
// sessionDCG
// ---------------------------------------------------------------------------

func TestSessionDCG_Empty(t *testing.T) {
	got := sessionDCG(nil)
	if got != 0.0 {
		t.Errorf("sessionDCG(nil) = %v, want 0.0", got)
	}
	got2 := sessionDCG([]float64{})
	if got2 != 0.0 {
		t.Errorf("sessionDCG([]) = %v, want 0.0", got2)
	}
}

func TestSessionDCG_SingleChunk(t *testing.T) {
	// 1 chunk at rank 1: score / log2(2) = score / 1.0 = score
	scores := []float64{0.8}
	got := sessionDCG(scores)
	want := 0.8
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("sessionDCG([0.8]) = %v, want %v", got, want)
	}
}

func TestSessionDCG_TwoChunks(t *testing.T) {
	// Rank 1: 0.9 / log2(2) = 0.9 / 1.0 = 0.9
	// Rank 2: 0.6 / log2(3) ≈ 0.6 / 1.585 ≈ 0.3785
	scores := []float64{0.9, 0.6}
	got := sessionDCG(scores)
	want := 0.9/math.Log2(2) + 0.6/math.Log2(3)
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("sessionDCG([0.9,0.6]) = %v, want %v", got, want)
	}
}

func TestSessionDCG_SortedDescending(t *testing.T) {
	// Scores must be sorted descending before DCG; [0.3, 0.9] should give same as [0.9, 0.3]
	ascending := sessionDCG([]float64{0.3, 0.9})
	descending := sessionDCG([]float64{0.9, 0.3})
	// sessionDCG sorts internally, so both should be equal
	if math.Abs(ascending-descending) > 1e-9 {
		t.Errorf("sessionDCG should sort internally: ascending=%v descending=%v", ascending, descending)
	}
}

func TestSessionDCG_MultipleChunks_HigherIsMore(t *testing.T) {
	// A session with 3 good chunks should score higher than a session with 1 good chunk
	sessionA := sessionDCG([]float64{0.8, 0.7, 0.6})
	sessionB := sessionDCG([]float64{0.9})
	// sessionA has more evidence: three mid-pack turns collectively beat a single top hit?
	// This is intentional: DCG gives credit to multi-turn evidence.
	// 0.9/log2(2) = 0.9; sessionA = 0.8 + 0.7/log2(3) + 0.6/log2(4) ≈ 0.8 + 0.441 + 0.3 = 1.541
	if sessionA <= sessionB {
		t.Errorf("3-chunk session (%v) should beat 1-chunk session (%v) via DCG multi-evidence", sessionA, sessionB)
	}
}

// ---------------------------------------------------------------------------
// sessionNDCGRerank — the core LEVER-8 property
// ---------------------------------------------------------------------------

// makeResult is a helper to create a SearchResult with given memory ID, tags, and score.
func makeResult(id string, tags []string, score float64) types.SearchResult {
	return types.SearchResult{
		Memory: &types.Memory{ID: id, Tags: tags},
		Score:  score,
	}
}

// TestSessionNDCGRerank_FlagOff — when flag is off, output is unchanged baseline.
func TestSessionNDCGRerank_FlagOff(t *testing.T) {
	results := []types.SearchResult{
		makeResult("m1", []string{"sid:s1"}, 0.9),
		makeResult("m2", []string{"sid:s2"}, 0.8),
		makeResult("m3", []string{"sid:s3"}, 0.7),
	}
	// allChunkCosines not needed when flag is off
	got := sessionNDCGRerank(results, nil, false)
	// should be identical to input order
	if len(got) != 3 {
		t.Fatalf("flag-off: len=%d, want 3", len(got))
	}
	for i, want := range []string{"m1", "m2", "m3"} {
		if got[i].Memory.ID != want {
			t.Errorf("flag-off: results[%d].ID = %q, want %q", i, got[i].Memory.ID, want)
		}
	}
}

// TestSessionNDCGRerank_Empty — empty input returns empty output.
func TestSessionNDCGRerank_Empty(t *testing.T) {
	got := sessionNDCGRerank(nil, nil, true)
	if len(got) != 0 {
		t.Errorf("empty: len=%d, want 0", len(got))
	}
}

// TestSessionNDCGRerank_CoreProperty — a session with mid-pack turns that aggregate
// high is surfaced over a single high-scoring isolated turn.
//
// Setup:
//   - Session "multi" has 3 chunks with cosines [0.65, 0.60, 0.55]; composite scores [0.4, 0.35, 0.3]
//   - Session "single" has 1 chunk with cosine 0.80; composite score 0.7
//
// Without LEVER-8: single (0.7) ranks first, then multi-1 (0.4), multi-2 (0.35), multi-3 (0.3).
// With LEVER-8:
//   - sessionDCG("single") = 0.80/log2(2) = 0.80
//   - sessionDCG("multi") = 0.65/log2(2) + 0.60/log2(3) + 0.55/log2(4) ≈ 0.65 + 0.378 + 0.275 = 1.303
//   - "multi" session ranks first → its 3 memories emitted first (sorted by composite score desc)
func TestSessionNDCGRerank_CoreProperty(t *testing.T) {
	results := []types.SearchResult{
		makeResult("single-1", []string{"sid:single"}, 0.70),
		makeResult("multi-1", []string{"sid:multi"}, 0.40),
		makeResult("multi-2", []string{"sid:multi"}, 0.35),
		makeResult("multi-3", []string{"sid:multi"}, 0.30),
	}
	allChunkCosines := map[string][]float64{
		"single-1": {0.80},
		"multi-1":  {0.65},
		"multi-2":  {0.60},
		"multi-3":  {0.55},
	}

	got := sessionNDCGRerank(results, allChunkCosines, true)

	if len(got) != 4 {
		t.Fatalf("core property: len=%d, want 4", len(got))
	}
	// First 3 results should be the multi-session memories
	multiIDs := map[string]bool{"multi-1": true, "multi-2": true, "multi-3": true}
	for i := 0; i < 3; i++ {
		if !multiIDs[got[i].Memory.ID] {
			t.Errorf("core property: results[%d].ID = %q, expected a multi-session memory", i, got[i].Memory.ID)
		}
	}
	// Last result should be the single-session memory
	if got[3].Memory.ID != "single-1" {
		t.Errorf("core property: results[3].ID = %q, want single-1", got[3].Memory.ID)
	}
}

// TestSessionNDCGRerank_WithinSessionOrder — within a session group, memories are
// sorted by composite score descending (P1 policy).
func TestSessionNDCGRerank_WithinSessionOrder(t *testing.T) {
	results := []types.SearchResult{
		// Deliberately out of composite-score order within session "s1"
		makeResult("s1-low", []string{"sid:s1"}, 0.30),
		makeResult("s1-high", []string{"sid:s1"}, 0.70),
		makeResult("s1-mid", []string{"sid:s1"}, 0.50),
		makeResult("s2-only", []string{"sid:s2"}, 0.60),
	}
	allChunkCosines := map[string][]float64{
		"s1-low":  {0.80},
		"s1-high": {0.75},
		"s1-mid":  {0.70},
		"s2-only": {0.40},
	}

	got := sessionNDCGRerank(results, allChunkCosines, true)

	if len(got) != 4 {
		t.Fatalf("within-session order: len=%d, want 4", len(got))
	}
	// s1 should rank first (DCG: 0.80 + 0.75/log2(3) + 0.70/log2(4) > 0.40)
	// Within s1, order should be high, mid, low (by composite score desc)
	if got[0].Memory.ID != "s1-high" {
		t.Errorf("within-session: results[0].ID = %q, want s1-high", got[0].Memory.ID)
	}
	if got[1].Memory.ID != "s1-mid" {
		t.Errorf("within-session: results[1].ID = %q, want s1-mid", got[1].Memory.ID)
	}
	if got[2].Memory.ID != "s1-low" {
		t.Errorf("within-session: results[2].ID = %q, want s1-low", got[2].Memory.ID)
	}
	if got[3].Memory.ID != "s2-only" {
		t.Errorf("within-session: results[3].ID = %q, want s2-only", got[3].Memory.ID)
	}
}

// TestSessionNDCGRerank_NoSidTag — memories without a sid: tag are treated as
// singletons (no-session group) and ranked by original composite score.
func TestSessionNDCGRerank_NoSidTag(t *testing.T) {
	results := []types.SearchResult{
		makeResult("tagged", []string{"sid:s1"}, 0.50),
		makeResult("untagged", []string{"lme"}, 0.90),
	}
	allChunkCosines := map[string][]float64{
		"tagged":   {0.60},
		"untagged": {0.85},
	}

	got := sessionNDCGRerank(results, allChunkCosines, true)

	if len(got) != 2 {
		t.Fatalf("no-sid: len=%d, want 2", len(got))
	}
	// untagged has higher chunk cosine (0.85 > 0.60), so it ranks first
	if got[0].Memory.ID != "untagged" {
		t.Errorf("no-sid: results[0].ID = %q, want untagged", got[0].Memory.ID)
	}
}

// TestSessionNDCGRerank_Deterministic — same input always produces same output.
func TestSessionNDCGRerank_Deterministic(t *testing.T) {
	results := []types.SearchResult{
		makeResult("m-a", []string{"sid:sA"}, 0.6),
		makeResult("m-b", []string{"sid:sB"}, 0.7),
		makeResult("m-c", []string{"sid:sA"}, 0.5),
	}
	cosines := map[string][]float64{
		"m-a": {0.65},
		"m-b": {0.70},
		"m-c": {0.55},
	}

	first := sessionNDCGRerank(results, cosines, true)
	second := sessionNDCGRerank(results, cosines, true)

	if len(first) != len(second) {
		t.Fatalf("deterministic: lengths differ %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Memory.ID != second[i].Memory.ID {
			t.Errorf("deterministic: results[%d] differs: %q vs %q",
				i, first[i].Memory.ID, second[i].Memory.ID)
		}
	}
}

// TestSessionNDCGRerank_NilMemory — nil Memory entries are passed through without panic.
func TestSessionNDCGRerank_NilMemory(t *testing.T) {
	results := []types.SearchResult{
		{Memory: nil, Score: 0.9},
		makeResult("m1", []string{"sid:s1"}, 0.5),
	}
	got := sessionNDCGRerank(results, nil, true)
	// Should not panic; nil-memory items are treated as no-session
	if len(got) != 2 {
		t.Fatalf("nil-memory: len=%d, want 2", len(got))
	}
}
