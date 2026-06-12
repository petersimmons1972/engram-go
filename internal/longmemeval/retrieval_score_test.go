package longmemeval_test

// TDD suite for the generator-free retrieval scoring logic (Deliverable A).
// These tests must pass before any production caller uses the functions.
// The measurement instrument must be correct — hence TDD rather than casual tests.

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// ---------------------------------------------------------------------------
// GoldSessionInContext
// ---------------------------------------------------------------------------

// GoldSessionInContext: at least one gold session appears among the retrieved
// memory IDs when translated through memoryMap.
func TestGoldSessionInContext_Hit(t *testing.T) {
	memoryMap := map[string]string{
		"mem-1": "sess-a",
		"mem-2": "sess-b",
		"mem-3": "sess-c",
	}
	retrieved := []string{"mem-2", "mem-3", "mem-1"}
	gold := []string{"sess-b"}

	if !longmemeval.GoldSessionInContext(retrieved, memoryMap, gold) {
		t.Error("expected gold session in context, got false")
	}
}

func TestGoldSessionInContext_Miss(t *testing.T) {
	memoryMap := map[string]string{
		"mem-1": "sess-a",
		"mem-2": "sess-b",
	}
	retrieved := []string{"mem-1"}
	gold := []string{"sess-z"}

	if longmemeval.GoldSessionInContext(retrieved, memoryMap, gold) {
		t.Error("expected gold session NOT in context, got true")
	}
}

func TestGoldSessionInContext_Empty(t *testing.T) {
	if longmemeval.GoldSessionInContext(nil, nil, []string{"sess-a"}) {
		t.Error("nil retrieved should return false")
	}
	if longmemeval.GoldSessionInContext([]string{"mem-1"}, map[string]string{"mem-1": "sess-a"}, nil) {
		t.Error("nil gold should return false")
	}
}

func TestGoldSessionInContext_MultipleGold(t *testing.T) {
	// Multi-session: ALL gold sessions must be present for the stricter check.
	// GoldSessionInContext checks ANY gold — use GoldAllSessionsInContext for ALL.
	memoryMap := map[string]string{
		"mem-1": "sess-a",
		"mem-2": "sess-b",
		"mem-3": "sess-c",
	}
	retrieved := []string{"mem-1", "mem-3"}
	gold := []string{"sess-a", "sess-b", "sess-c"} // sess-b NOT in retrieved

	// GoldSessionInContext = ANY (should be true because sess-a + sess-c are there)
	if !longmemeval.GoldSessionInContext(retrieved, memoryMap, gold) {
		t.Error("expected ANY gold session in context")
	}
}

// ---------------------------------------------------------------------------
// GoldAllSessionsInContext
// ---------------------------------------------------------------------------

func TestGoldAllSessionsInContext_AllPresent(t *testing.T) {
	memoryMap := map[string]string{"m1": "s1", "m2": "s2", "m3": "s3"}
	retrieved := []string{"m1", "m2", "m3"}
	gold := []string{"s1", "s2"}
	if !longmemeval.GoldAllSessionsInContext(retrieved, memoryMap, gold) {
		t.Error("expected all gold sessions in context")
	}
}

func TestGoldAllSessionsInContext_Partial(t *testing.T) {
	memoryMap := map[string]string{"m1": "s1", "m2": "s2"}
	retrieved := []string{"m1"}
	gold := []string{"s1", "s2"}
	if longmemeval.GoldAllSessionsInContext(retrieved, memoryMap, gold) {
		t.Error("expected NOT all gold sessions in context")
	}
}

// ---------------------------------------------------------------------------
// GoldSessionRank (1-based rank of first gold hit, 0 if not found)
// ---------------------------------------------------------------------------

func TestGoldSessionRank_FoundEarly(t *testing.T) {
	memoryMap := map[string]string{"m1": "s1", "m2": "s2", "m3": "s3"}
	retrieved := []string{"m2", "m1", "m3"}
	gold := []string{"s1"}
	got := longmemeval.GoldSessionRank(retrieved, memoryMap, gold)
	if got != 2 {
		t.Errorf("GoldSessionRank = %d, want 2", got)
	}
}

func TestGoldSessionRank_FirstPosition(t *testing.T) {
	memoryMap := map[string]string{"m1": "s1", "m2": "s2"}
	retrieved := []string{"m1", "m2"}
	gold := []string{"s1"}
	if got := longmemeval.GoldSessionRank(retrieved, memoryMap, gold); got != 1 {
		t.Errorf("GoldSessionRank = %d, want 1", got)
	}
}

func TestGoldSessionRank_NotFound(t *testing.T) {
	memoryMap := map[string]string{"m1": "s1"}
	retrieved := []string{"m1"}
	gold := []string{"s-missing"}
	if got := longmemeval.GoldSessionRank(retrieved, memoryMap, gold); got != 0 {
		t.Errorf("GoldSessionRank = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// ScoreRetrievalForItem
// ---------------------------------------------------------------------------

func TestScoreRetrievalForItem_Basic(t *testing.T) {
	memoryMap := map[string]string{
		"m1": "s1", "m2": "s2", "m3": "s3", "m4": "s4", "m5": "s5",
		"m6": "s6", "m7": "s7", "m8": "s8", "m9": "s9", "m10": "s10",
	}
	// Gold session s3 is at rank 3
	retrieved := []string{"m1", "m2", "m3", "m4", "m5", "m6", "m7", "m8", "m9", "m10"}
	gold := []string{"s3"}

	result := longmemeval.ScoreRetrievalForItem(retrieved, memoryMap, gold)

	// GoldInContext (any) = true (s3 is at rank 3, within top 10)
	if !result.GoldSessionInContext {
		t.Error("GoldSessionInContext should be true")
	}
	// Recall@5 = 1.0 (s3 is at rank 3)
	if result.RecallAt5 != 1.0 {
		t.Errorf("RecallAt5 = %.2f, want 1.0", result.RecallAt5)
	}
	// Recall@10 = 1.0 (s3 is at rank 3)
	if result.RecallAt10 != 1.0 {
		t.Errorf("RecallAt10 = %.2f, want 1.0", result.RecallAt10)
	}
	// GoldRank = 3
	if result.GoldRank != 3 {
		t.Errorf("GoldRank = %d, want 3", result.GoldRank)
	}
}

func TestScoreRetrievalForItem_GoldAtRank6(t *testing.T) {
	memoryMap := map[string]string{}
	retrieved := []string{}
	gold := []string{"s6"}
	for i := 1; i <= 15; i++ {
		key := mkKey(i)
		val := mkSess(i)
		memoryMap[key] = val
		retrieved = append(retrieved, key)
	}

	// gold s6 is at rank 6 (m6)
	result := longmemeval.ScoreRetrievalForItem(retrieved, memoryMap, gold)

	if result.RecallAt5 != 0.0 {
		t.Errorf("RecallAt5 = %.2f, want 0.0 (gold at rank 6)", result.RecallAt5)
	}
	if result.RecallAt10 != 1.0 {
		t.Errorf("RecallAt10 = %.2f, want 1.0 (gold at rank 6)", result.RecallAt10)
	}
	if result.GoldRank != 6 {
		t.Errorf("GoldRank = %d, want 6", result.GoldRank)
	}
}

func TestScoreRetrievalForItem_NotFound(t *testing.T) {
	memoryMap := map[string]string{"m1": "s1", "m2": "s2"}
	retrieved := []string{"m1", "m2"}
	gold := []string{"s-missing"}

	result := longmemeval.ScoreRetrievalForItem(retrieved, memoryMap, gold)

	if result.GoldSessionInContext {
		t.Error("GoldSessionInContext should be false")
	}
	if result.RecallAt5 != 0.0 {
		t.Errorf("RecallAt5 = %.2f, want 0.0", result.RecallAt5)
	}
	if result.GoldRank != 0 {
		t.Errorf("GoldRank = %d, want 0", result.GoldRank)
	}
}

// ---------------------------------------------------------------------------
// AggregateRetrievalReport
// ---------------------------------------------------------------------------

func TestAggregateRetrievalReport_Basic(t *testing.T) {
	results := []longmemeval.ItemRetrievalResult{
		{QuestionType: "single-session-preference", GoldSessionInContext: true, RecallAt5: 1.0, RecallAt10: 1.0, NDCGAt5: 0.8, GoldRank: 2},
		{QuestionType: "single-session-preference", GoldSessionInContext: false, RecallAt5: 0.0, RecallAt10: 0.0, NDCGAt5: 0.0, GoldRank: 0},
		{QuestionType: "temporal-reasoning", GoldSessionInContext: true, RecallAt5: 1.0, RecallAt10: 1.0, NDCGAt5: 1.0, GoldRank: 1},
	}

	report := longmemeval.AggregateRetrievalReport(results)

	if report.Overall.N != 3 {
		t.Errorf("Overall.N = %d, want 3", report.Overall.N)
	}
	// 2/3 gold in context
	wantOverallGIC := 2.0 / 3.0
	if abs(report.Overall.GoldInContextRate-wantOverallGIC) > 1e-6 {
		t.Errorf("Overall.GoldInContextRate = %.4f, want %.4f", report.Overall.GoldInContextRate, wantOverallGIC)
	}

	// Preference type: 2 items, 1 hit
	pref, ok := report.ByType["single-session-preference"]
	if !ok {
		t.Fatal("missing single-session-preference in ByType")
	}
	if pref.N != 2 {
		t.Errorf("pref.N = %d, want 2", pref.N)
	}
	if abs(pref.GoldInContextRate-0.5) > 1e-6 {
		t.Errorf("pref.GoldInContextRate = %.4f, want 0.5", pref.GoldInContextRate)
	}

	// Temporal: 1 item, 1 hit
	temporal, ok := report.ByType["temporal-reasoning"]
	if !ok {
		t.Fatal("missing temporal-reasoning in ByType")
	}
	if abs(temporal.GoldInContextRate-1.0) > 1e-6 {
		t.Errorf("temporal.GoldInContextRate = %.4f, want 1.0", temporal.GoldInContextRate)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func mkKey(i int) string {
	return "m" + itoa(i)
}

func mkSess(i int) string {
	return "s" + itoa(i)
}

func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
