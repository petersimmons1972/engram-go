package search

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/petersimmons1972/engram/internal/types"
)

func TestSessionDiversity_NZeroIsBaseline(t *testing.T) {
	results := []types.SearchResult{
		sessionDiversityResult("s1-1", []string{"sid:s1"}, 0.99),
		sessionDiversityResult("s1-2", []string{"sid:s1"}, 0.98),
		sessionDiversityResult("s2-1", []string{"sid:s2"}, 0.97),
		sessionDiversityResult("s1-3", []string{"sid:s1"}, 0.96),
	}

	before, err := json.Marshal(results)
	if err != nil {
		t.Fatalf("marshal baseline: %v", err)
	}

	got := applySessionDiversity(results, 4, 0)

	after, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal diversified: %v", err)
	}

	if string(after) != string(before) {
		t.Fatalf("N=0 must be byte-identical to baseline\nbefore=%s\nafter=%s", string(before), string(after))
	}
}

func TestSessionDiversity_HappyPath_GoldInMinoritySession(t *testing.T) {
	results := []types.SearchResult{
		sessionDiversityResult("s1-1", []string{"sid:s1"}, 0.99),
		sessionDiversityResult("s1-2", []string{"sid:s1"}, 0.98),
		sessionDiversityResult("s1-3", []string{"sid:s1"}, 0.97),
		sessionDiversityResult("s1-4", []string{"sid:s1"}, 0.96),
		sessionDiversityResult("gold-s2", []string{"sid:s2"}, 0.95),
	}

	got := applySessionDiversity(results, 4, 2)

	if len(got) < 4 {
		t.Fatalf("len(got)=%d, want at least 4", len(got))
	}
	if !containsID(got[:4], "gold-s2") {
		t.Fatalf("minority-session gold must appear in top-4, got ids=%v", resultIDs(got[:4]))
	}
}

func TestSessionDiversity_AllChunksSingleSession(t *testing.T) {
	results := []types.SearchResult{
		sessionDiversityResult("s1-1", []string{"sid:s1"}, 0.99),
		sessionDiversityResult("s1-2", []string{"sid:s1"}, 0.98),
		sessionDiversityResult("s1-3", []string{"sid:s1"}, 0.97),
		sessionDiversityResult("s1-4", []string{"sid:s1"}, 0.96),
		sessionDiversityResult("s1-5", []string{"sid:s1"}, 0.95),
	}

	got := applySessionDiversity(results, 5, 2)
	if !reflect.DeepEqual(got, results) {
		t.Fatalf("single-session input must be returned unchanged\ngot=%v\nwant=%v", resultIDs(got), resultIDs(results))
	}
}

func TestSessionDiversity_MissingOrEmptySessionID(t *testing.T) {
	results := []types.SearchResult{
		sessionDiversityResult("s1-1", []string{"sid:s1"}, 0.99),
		sessionDiversityResult("s1-2", []string{"sid:s1"}, 0.98),
		sessionDiversityResult("untagged", nil, 0.97),
		sessionDiversityResult("empty-sid", []string{"sid:"}, 0.96),
		sessionDiversityResult("s2-1", []string{"sid:s2"}, 0.95),
	}

	got1 := applySessionDiversity(results, 4, 1)
	got2 := applySessionDiversity(results, 4, 1)

	wantTop4 := []string{"s1-1", "untagged", "empty-sid", "s2-1"}
	if !reflect.DeepEqual(resultIDs(got1[:4]), wantTop4) {
		t.Fatalf("unexpected top-4 order: got=%v want=%v", resultIDs(got1[:4]), wantTop4)
	}
	if !reflect.DeepEqual(resultIDs(got1), resultIDs(got2)) {
		t.Fatalf("output must be deterministic across runs: got1=%v got2=%v", resultIDs(got1), resultIDs(got2))
	}
}

func TestSessionDiversity_TopK3_N2_TwoSessions(t *testing.T) {
	// Tie semantics: preserve session encounter order from the baseline ranking,
	// and preserve within-session order from the baseline ranking.
	results := []types.SearchResult{
		sessionDiversityResult("s1-1", []string{"sid:s1"}, 0.99),
		sessionDiversityResult("s1-2", []string{"sid:s1"}, 0.98),
		sessionDiversityResult("s1-3", []string{"sid:s1"}, 0.97),
		sessionDiversityResult("s1-4", []string{"sid:s1"}, 0.96),
		sessionDiversityResult("s2-1", []string{"sid:s2"}, 0.95),
		sessionDiversityResult("s2-2", []string{"sid:s2"}, 0.94),
	}

	got := applySessionDiversity(results, 3, 2)
	wantTop3 := []string{"s1-1", "s1-2", "s2-1"}
	if !reflect.DeepEqual(resultIDs(got[:3]), wantTop3) {
		t.Fatalf("unexpected top-3 order: got=%v want=%v", resultIDs(got[:3]), wantTop3)
	}
	if countSessionIDs(got[:3], "s1") > 2 {
		t.Fatalf("top-3 must contain at most 2 results from one session, got ids=%v", resultIDs(got[:3]))
	}
}

func TestSessionDiversity_AlreadyDiverseOrShortInput(t *testing.T) {
	results := []types.SearchResult{
		sessionDiversityResult("s1-1", []string{"sid:s1"}, 0.99),
		sessionDiversityResult("s2-1", []string{"sid:s2"}, 0.98),
		sessionDiversityResult("s1-2", []string{"sid:s1"}, 0.97),
	}

	got := applySessionDiversity(results, 5, 2)
	if len(got) != len(results) {
		t.Fatalf("short input length must be preserved: got=%d want=%d", len(got), len(results))
	}
	if !reflect.DeepEqual(resultIDs(got), resultIDs(results)) {
		t.Fatalf("already-diverse short input should keep order: got=%v want=%v", resultIDs(got), resultIDs(results))
	}
}

func TestSessionDiversity_DynamicGateSkipsWhenSingleSession(t *testing.T) {
	results := []types.SearchResult{
		sessionDiversityResult("s1-1", []string{"sid:s1"}, 0.99),
		sessionDiversityResult("s1-2", []string{"sid:s1"}, 0.98),
		sessionDiversityResult("s1-3", []string{"sid:s1"}, 0.97),
	}

	if shouldApplySessionDiversity(results, 3, 2) {
		t.Fatal("dynamic gate must return false when all results share one session")
	}

	got := applySessionDiversity(results, 3, 2)
	if !reflect.DeepEqual(got, results) {
		t.Fatalf("single-session result slice must be returned unchanged: got=%v want=%v", resultIDs(got), resultIDs(results))
	}
}

func sessionDiversityResult(id string, tags []string, score float64) types.SearchResult {
	return types.SearchResult{
		Memory: &types.Memory{
			ID:      id,
			Tags:    tags,
			Content: id,
		},
		Score: score,
	}
}

func resultIDs(results []types.SearchResult) []string {
	ids := make([]string, 0, len(results))
	for _, r := range results {
		if r.Memory == nil {
			ids = append(ids, "")
			continue
		}
		ids = append(ids, r.Memory.ID)
	}
	return ids
}

func containsID(results []types.SearchResult, want string) bool {
	for _, r := range results {
		if r.Memory != nil && r.Memory.ID == want {
			return true
		}
	}
	return false
}

func countSessionIDs(results []types.SearchResult, want string) int {
	count := 0
	for _, r := range results {
		if r.Memory == nil {
			continue
		}
		if extractSessionID(r.Memory.Tags) == want {
			count++
		}
	}
	return count
}
