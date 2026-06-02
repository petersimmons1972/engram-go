// engine_pure_test.go — tests for pure (no-DB) functions in the search package:
// bigramJaccard, sortResults, toConnectedMemories.
// This file is test-only; it does NOT modify engine.go.
package search

import (
	"context"
	"math"
	"sync/atomic"
	"testing"

	"github.com/petersimmons1972/engram/internal/types"
)

// ---------------------------------------------------------------------------
// bigramJaccard
// ---------------------------------------------------------------------------

func TestBigramJaccard_BothEmpty(t *testing.T) {
	// Both empty strings → both bigram sets are empty → defined as 1.0 (identical).
	got := bigramJaccard("", "")
	if got != 1.0 {
		t.Errorf("bigramJaccard('','') = %v, want 1.0", got)
	}
}

func TestBigramJaccard_SingleCharEach(t *testing.T) {
	// Single-character strings produce no bigrams → same as both-empty treatment.
	got := bigramJaccard("a", "b")
	if got != 1.0 {
		t.Errorf("bigramJaccard('a','b') = %v, want 1.0 (both produce empty bigram sets)", got)
	}
}

func TestBigramJaccard_OneEmptyOneNonEmpty(t *testing.T) {
	// One side empty → intersection is 0; union is non-zero → result is 0.
	got := bigramJaccard("", "hello")
	if got != 0.0 {
		t.Errorf("bigramJaccard('','hello') = %v, want 0.0", got)
	}
	got2 := bigramJaccard("world", "")
	if got2 != 0.0 {
		t.Errorf("bigramJaccard('world','') = %v, want 0.0", got2)
	}
}

func TestBigramJaccard_Identical(t *testing.T) {
	// Identical strings → perfect overlap → 1.0.
	got := bigramJaccard("hello", "hello")
	if got != 1.0 {
		t.Errorf("bigramJaccard('hello','hello') = %v, want 1.0", got)
	}
}

func TestBigramJaccard_Disjoint(t *testing.T) {
	// Completely disjoint character sets → no common bigrams → 0.0.
	got := bigramJaccard("abcd", "efgh")
	if got != 0.0 {
		t.Errorf("bigramJaccard('abcd','efgh') = %v, want 0.0", got)
	}
}

func TestBigramJaccard_PartialOverlap(t *testing.T) {
	// "abc" has bigrams {ab, bc}; "bcd" has bigrams {bc, cd}.
	// intersection = {bc} = 1; union = {ab, bc, cd} = 3 → 1/3 ≈ 0.333...
	got := bigramJaccard("abc", "bcd")
	want := 1.0 / 3.0
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("bigramJaccard('abc','bcd') = %v, want %v", got, want)
	}
}

func TestBigramJaccard_UnicodeRunes(t *testing.T) {
	// Unicode: "αβγ" has bigrams {αβ, βγ}; "βγδ" has bigrams {βγ, γδ}.
	// intersection = {βγ}; union = {αβ, βγ, γδ} → 1/3.
	got := bigramJaccard("αβγ", "βγδ")
	want := 1.0 / 3.0
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("bigramJaccard unicode = %v, want %v", got, want)
	}
}

func TestBigramJaccard_TwoCharSame(t *testing.T) {
	// Two-char strings: "ab" and "ab" → bigram sets each contain {ab} → 1.0.
	got := bigramJaccard("ab", "ab")
	if got != 1.0 {
		t.Errorf("bigramJaccard('ab','ab') = %v, want 1.0", got)
	}
}

func TestBigramJaccard_TwoCharDifferent(t *testing.T) {
	// "ab" vs "cd" → sets {ab} and {cd} → intersection 0, union 2 → 0.0.
	got := bigramJaccard("ab", "cd")
	if got != 0.0 {
		t.Errorf("bigramJaccard('ab','cd') = %v, want 0.0", got)
	}
}

// ---------------------------------------------------------------------------
// sortResults
// ---------------------------------------------------------------------------

func TestSortResults_DescendingByScore(t *testing.T) {
	results := []types.SearchResult{
		{Memory: &types.Memory{ID: "low"}, Score: 0.1},
		{Memory: &types.Memory{ID: "high"}, Score: 0.9},
		{Memory: &types.Memory{ID: "mid"}, Score: 0.5},
	}

	sortResults(results)

	if results[0].Memory.ID != "high" {
		t.Errorf("first result should be 'high', got %q", results[0].Memory.ID)
	}
	if results[1].Memory.ID != "mid" {
		t.Errorf("second result should be 'mid', got %q", results[1].Memory.ID)
	}
	if results[2].Memory.ID != "low" {
		t.Errorf("third result should be 'low', got %q", results[2].Memory.ID)
	}
}

func TestSortResults_EmptySlice(t *testing.T) {
	// Must not panic on nil or empty input.
	sortResults(nil)
	sortResults([]types.SearchResult{})
}

func TestSortResults_SingleElement(t *testing.T) {
	results := []types.SearchResult{
		{Memory: &types.Memory{ID: "only"}, Score: 0.7},
	}
	sortResults(results)
	if results[0].Memory.ID != "only" {
		t.Errorf("single element: expected 'only', got %q", results[0].Memory.ID)
	}
}

func TestSortResults_TiedScores(t *testing.T) {
	// Tied scores — sort must not panic; order among ties is unspecified.
	results := []types.SearchResult{
		{Memory: &types.Memory{ID: "a"}, Score: 0.5},
		{Memory: &types.Memory{ID: "b"}, Score: 0.5},
	}
	sortResults(results)
	// Both scores are equal; just verify both IDs still present.
	ids := map[string]bool{results[0].Memory.ID: true, results[1].Memory.ID: true}
	if !ids["a"] || !ids["b"] {
		t.Errorf("tied sort lost an element: got %v", results)
	}
}

// ---------------------------------------------------------------------------
// toConnectedMemories
// ---------------------------------------------------------------------------

func TestToConnectedMemories_Empty(t *testing.T) {
	out := toConnectedMemories(nil, "mem-1", nil)
	if len(out) != 0 {
		t.Errorf("expected empty result for nil rels, got %d", len(out))
	}
}

func TestToConnectedMemories_OutgoingRelationship(t *testing.T) {
	// Relationship where TargetID != memID → direction = "outgoing", neighborID = TargetID.
	neighborMem := &types.Memory{ID: "neighbor-1", Content: "neighbor content"}
	rels := []types.Relationship{
		{SourceID: "mem-1", TargetID: "neighbor-1", RelType: "relates_to", Strength: 0.8},
	}
	neighborMap := map[string]*types.Memory{"neighbor-1": neighborMem}

	out := toConnectedMemories(rels, "mem-1", neighborMap)
	if len(out) != 1 {
		t.Fatalf("expected 1 connected memory, got %d", len(out))
	}
	if out[0].Direction != "outgoing" {
		t.Errorf("direction = %q, want 'outgoing'", out[0].Direction)
	}
	if out[0].RelType != "relates_to" {
		t.Errorf("rel_type = %q, want 'relates_to'", out[0].RelType)
	}
	if out[0].Memory == nil || out[0].Memory.ID != "neighbor-1" {
		t.Errorf("expected neighbor memory ID 'neighbor-1', got %v", out[0].Memory)
	}
	if out[0].Strength != 0.8 {
		t.Errorf("strength = %v, want 0.8", out[0].Strength)
	}
}

func TestToConnectedMemories_IncomingRelationship(t *testing.T) {
	// Relationship where TargetID == memID → direction = "incoming", neighborID = SourceID.
	neighborMem := &types.Memory{ID: "source-1"}
	rels := []types.Relationship{
		{SourceID: "source-1", TargetID: "mem-1", RelType: "supports", Strength: 0.5},
	}
	neighborMap := map[string]*types.Memory{"source-1": neighborMem}

	out := toConnectedMemories(rels, "mem-1", neighborMap)
	if len(out) != 1 {
		t.Fatalf("expected 1 connected memory, got %d", len(out))
	}
	if out[0].Direction != "incoming" {
		t.Errorf("direction = %q, want 'incoming'", out[0].Direction)
	}
	if out[0].Memory == nil || out[0].Memory.ID != "source-1" {
		t.Errorf("expected source memory ID 'source-1', got %v", out[0].Memory)
	}
}

func TestToConnectedMemories_NeighborMissing(t *testing.T) {
	// Neighbor not in neighborMap → Memory field is nil (best-effort).
	rels := []types.Relationship{
		{SourceID: "mem-1", TargetID: "missing-1", RelType: "relates_to", Strength: 1.0},
	}
	neighborMap := map[string]*types.Memory{} // empty

	out := toConnectedMemories(rels, "mem-1", neighborMap)
	if len(out) != 1 {
		t.Fatalf("expected 1 result even for missing neighbor, got %d", len(out))
	}
	if out[0].Memory != nil {
		t.Errorf("expected nil Memory for missing neighbor, got %v", out[0].Memory)
	}
}

func TestToConnectedMemories_MixedDirections(t *testing.T) {
	m1 := &types.Memory{ID: "out-neighbor"}
	m2 := &types.Memory{ID: "in-source"}
	rels := []types.Relationship{
		{SourceID: "mem-1", TargetID: "out-neighbor", RelType: "r1", Strength: 1.0},
		{SourceID: "in-source", TargetID: "mem-1", RelType: "r2", Strength: 0.9},
	}
	neighborMap := map[string]*types.Memory{
		"out-neighbor": m1,
		"in-source":    m2,
	}

	out := toConnectedMemories(rels, "mem-1", neighborMap)
	if len(out) != 2 {
		t.Fatalf("expected 2 connected memories, got %d", len(out))
	}

	dirs := map[string]string{}
	for _, c := range out {
		if c.Memory != nil {
			dirs[c.Memory.ID] = c.Direction
		}
	}
	if dirs["out-neighbor"] != "outgoing" {
		t.Errorf("out-neighbor direction = %q, want 'outgoing'", dirs["out-neighbor"])
	}
	if dirs["in-source"] != "incoming" {
		t.Errorf("in-source direction = %q, want 'incoming'", dirs["in-source"])
	}
}


// ---------------------------------------------------------------------------
// maybeRerank
// ---------------------------------------------------------------------------

// countingReranker is a stub ResultReranker that records how many times
// RerankResults has been invoked. It returns the input items unchanged (same IDs,
// same scores) so callers can observe call count without altering result ordering.
type countingReranker struct {
	calls atomic.Int64
}

func (r *countingReranker) RerankResults(_ context.Context, _ string, items []RerankItem) ([]RerankResult, error) {
	r.calls.Add(1)
	out := make([]RerankResult, len(items))
	for i, it := range items {
		out[i] = RerankResult{ID: it.ID, Score: it.Score}
	}
	return out, nil
}

// TestMaybeRerank_NilRerankerIsNoop verifies that maybeRerank returns the
// input slice unchanged when the reranker is nil.
func TestMaybeRerank_NilRerankerIsNoop(t *testing.T) {
	e := &SearchEngine{}
	results := []types.SearchResult{
		{Memory: &types.Memory{ID: "a"}, Score: 0.9},
		{Memory: &types.Memory{ID: "b"}, Score: 0.5},
	}
	out := e.maybeRerank(context.Background(), "query", 10, results, nil)
	if len(out) != 2 {
		t.Fatalf("expected 2 results, got %d", len(out))
	}
	if out[0].Memory.ID != "a" || out[1].Memory.ID != "b" {
		t.Errorf("nil reranker must not reorder results; got %v %v", out[0].Memory.ID, out[1].Memory.ID)
	}
}

// TestMaybeRerank_EmptyResultsIsNoop verifies that maybeRerank returns nil/empty
// without calling the reranker when results is empty.
func TestMaybeRerank_EmptyResultsIsNoop(t *testing.T) {
	e := &SearchEngine{}
	cr := &countingReranker{}
	out := e.maybeRerank(context.Background(), "query", 10, nil, cr)
	if out != nil {
		t.Errorf("expected nil out for nil input, got %v", out)
	}
	if cr.calls.Load() != 0 {
		t.Errorf("reranker called %d times on empty input, want 0", cr.calls.Load())
	}
}

// TestMaybeRerank_InvokesRerankerExactlyOnce verifies that maybeRerank calls
// the reranker exactly once and applies the returned scores.
func TestMaybeRerank_InvokesRerankerExactlyOnce(t *testing.T) {
	e := &SearchEngine{}
	cr := &countingReranker{}
	results := []types.SearchResult{
		{Memory: &types.Memory{ID: "x"}, Score: 0.3},
		{Memory: &types.Memory{ID: "y"}, Score: 0.7},
	}
	out := e.maybeRerank(context.Background(), "query", 10, results, cr)
	if cr.calls.Load() != 1 {
		t.Errorf("reranker called %d times, want exactly 1", cr.calls.Load())
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 results, got %d", len(out))
	}
}

// TestMaybeRerank_TopKCapLimitsRerankCandidates verifies that when len(results)
// exceeds topK the reranker only receives topK candidates (not the full slice).
func TestMaybeRerank_TopKCapLimitsRerankCandidates(t *testing.T) {
	e := &SearchEngine{}
	var capturedItemCount int
	spy := &spyReranker{onCall: func(items []RerankItem) {
		capturedItemCount = len(items)
	}}
	results := make([]types.SearchResult, 5)
	for i := range results {
		results[i] = types.SearchResult{Memory: &types.Memory{ID: string(rune('a' + i))}, Score: float64(i) * 0.1}
	}
	e.maybeRerank(context.Background(), "q", 3, results, spy)
	if capturedItemCount != 3 {
		t.Errorf("reranker received %d items, want 3 (topK cap)", capturedItemCount)
	}
}

// spyReranker invokes an onCall hook on each RerankResults call.
type spyReranker struct {
	onCall func(items []RerankItem)
}

func (s *spyReranker) RerankResults(_ context.Context, _ string, items []RerankItem) ([]RerankResult, error) {
	if s.onCall != nil {
		s.onCall(items)
	}
	out := make([]RerankResult, len(items))
	for i, it := range items {
		out[i] = RerankResult{ID: it.ID, Score: it.Score}
	}
	return out, nil
}
