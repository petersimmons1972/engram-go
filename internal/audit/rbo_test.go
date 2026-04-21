package audit

import (
	"math"
	"testing"
)

func TestRBO_IdenticalLists(t *testing.T) {
	a := []string{"a", "b", "c", "d", "e"}
	got := RBO(a, a, 0.9)
	// Identical lists: RBO lower bound increases monotonically with k.
	// For k=5, p=0.9 the lower bound is ≈0.41. It is never > 1.0.
	// Key property: identical > disjoint, and value is in (0,1].
	if got <= 0 || got > 1.0 {
		t.Errorf("RBO identical: want (0,1], got %f", got)
	}
	// Also verify identical > reversed (ordering matters).
	rev := []string{"e", "d", "c", "b", "a"}
	gotRev := RBO(a, rev, 0.9)
	if got <= gotRev {
		t.Errorf("RBO identical (%f) should exceed reversed (%f)", got, gotRev)
	}
}

func TestRBO_EmptyList(t *testing.T) {
	got := RBO([]string{}, []string{"a", "b"}, 0.9)
	if got != 0 {
		t.Errorf("RBO empty: want 0, got %f", got)
	}
	got2 := RBO([]string{"a"}, []string{}, 0.9)
	if got2 != 0 {
		t.Errorf("RBO empty b: want 0, got %f", got2)
	}
}

func TestRBO_DisjointLists(t *testing.T) {
	a := []string{"a", "b", "c"}
	b := []string{"d", "e", "f"}
	got := RBO(a, b, 0.9)
	if got != 0 {
		t.Errorf("RBO disjoint: want 0, got %f", got)
	}
}

func TestRBO_PartialOverlap(t *testing.T) {
	// First item matches, rest differ.
	a := []string{"x", "a", "b"}
	b := []string{"x", "c", "d"}
	got := RBO(a, b, 0.9)
	// Should be between 0 and 1.
	if got <= 0 || got >= 1 {
		t.Errorf("RBO partial: want (0,1), got %f", got)
	}
}

func TestRBO_ReversedList(t *testing.T) {
	a := []string{"a", "b", "c", "d"}
	b := []string{"d", "c", "b", "a"}
	got := RBO(a, b, 0.9)
	// Reversed should have same items but low RBO (top positions don't match).
	// Jaccard_full = 1.0 but RBO < 1.0.
	if got >= 1.0 {
		t.Errorf("RBO reversed: want < 1.0, got %f", got)
	}
	if got <= 0 {
		t.Errorf("RBO reversed: want > 0 (same items), got %f", got)
	}
}

func TestRBO_UnequalLengths(t *testing.T) {
	// Should use k = min(len(a), len(b)).
	a := []string{"a", "b", "c", "d", "e"}
	b := []string{"a", "b"}
	got := RBO(a, b, 0.9)
	// k=2, both agree at depth 2: RBO = (1-0.9)*(1/1 + 0.9*2/2) = (0.1)*(1+0.9) = 0.19
	want := (1 - 0.9) * (math.Pow(0.9, 0)*1.0/1.0 + math.Pow(0.9, 1)*2.0/2.0)
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("RBO unequal lengths k=2 agree: want %f, got %f", want, got)
	}
	// And verify same items score higher than disjoint.
	c := []string{"x", "y"}
	gotDisjoint := RBO(a, c, 0.9)
	if got <= gotDisjoint {
		t.Errorf("RBO matching items (%f) should exceed disjoint (%f)", got, gotDisjoint)
	}
}

func TestRBO_SingleElement(t *testing.T) {
	a := []string{"a"}
	b := []string{"a"}
	got := RBO(a, b, 0.9)
	// (1-0.9) * (1/1) * 1 = 0.1
	want := 0.1
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("RBO single match: want %f, got %f", want, got)
	}

	b2 := []string{"b"}
	got2 := RBO(a, b2, 0.9)
	if got2 != 0 {
		t.Errorf("RBO single no match: want 0, got %f", got2)
	}
}

func TestJaccardTopK_Identical(t *testing.T) {
	a := []string{"a", "b", "c", "d", "e"}
	got := JaccardTopK(a, a, 5)
	if math.Abs(got-1.0) > 1e-9 {
		t.Errorf("JaccardTopK identical: want 1.0, got %f", got)
	}
}

func TestJaccardTopK_Disjoint(t *testing.T) {
	a := []string{"a", "b", "c"}
	b := []string{"d", "e", "f"}
	got := JaccardTopK(a, b, 3)
	if got != 0 {
		t.Errorf("JaccardTopK disjoint: want 0, got %f", got)
	}
}

func TestJaccardTopK_HalfOverlap(t *testing.T) {
	a := []string{"a", "b", "c", "d"}
	b := []string{"a", "b", "e", "f"}
	got := JaccardTopK(a, b, 4)
	// inter=2, union=6, jaccard=2/6=1/3
	want := 1.0 / 3.0
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("JaccardTopK half: want %f, got %f", want, got)
	}
}

func TestJaccardTopK_KLargerThanList(t *testing.T) {
	a := []string{"a", "b"}
	b := []string{"a", "b", "c"}
	// k=10 but min len is 2, so k=2
	got := JaccardTopK(a, b, 10)
	if math.Abs(got-1.0) > 1e-9 {
		t.Errorf("JaccardTopK k>len: want 1.0, got %f", got)
	}
}

func TestJaccardTopK_ZeroK(t *testing.T) {
	a := []string{"a", "b"}
	b := []string{"a", "b"}
	got := JaccardTopK(a, b, 0)
	if got != 0 {
		t.Errorf("JaccardTopK k=0: want 0, got %f", got)
	}
}

func TestSetDiff(t *testing.T) {
	a := []string{"a", "b", "c", "d"}
	b := []string{"b", "d"}
	diff := setDiff(a, b)
	if len(diff) != 2 {
		t.Fatalf("setDiff: want 2 items, got %d: %v", len(diff), diff)
	}
	diffSet := make(map[string]bool)
	for _, v := range diff {
		diffSet[v] = true
	}
	if !diffSet["a"] || !diffSet["c"] {
		t.Errorf("setDiff: expected {a,c}, got %v", diff)
	}
}

func TestSetDiff_Empty(t *testing.T) {
	diff := setDiff([]string{}, []string{"a"})
	if len(diff) != 0 {
		t.Errorf("setDiff empty a: want [], got %v", diff)
	}
	diff2 := setDiff([]string{"a", "b"}, []string{})
	if len(diff2) != 2 {
		t.Errorf("setDiff empty b: want 2, got %d", len(diff2))
	}
}
