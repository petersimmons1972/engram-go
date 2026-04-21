package weight

import (
	"math"
	"testing"
)

func TestNormalizeWeights_AlreadyNormalized(t *testing.T) {
	w := DefaultWeights()
	got, ok := normalizeWeights(w)
	if !ok {
		t.Fatal("normalizeWeights: expected ok=true for defaults")
	}
	sum := got.Vector + got.BM25 + got.Recency + got.Precision
	if math.Abs(sum-1.0) > 0.01 {
		t.Errorf("sum want 1.0, got %f", sum)
	}
}

func TestNormalizeWeights_ClampAndNormalize(t *testing.T) {
	// Weights that need clamping.
	w := Weights{Vector: 0.80, BM25: 0.80, Recency: 0.80, Precision: 0.80}
	got, ok := normalizeWeights(w)
	if !ok {
		t.Fatal("normalizeWeights: expected ok=true")
	}
	sum := got.Vector + got.BM25 + got.Recency + got.Precision
	if math.Abs(sum-1.0) > 0.02 {
		t.Errorf("sum want ~1.0, got %f", sum)
	}
	// Each weight should be within bounds.
	if got.Vector < minVector || got.Vector > maxVector {
		t.Errorf("vector %f out of [%f,%f]", got.Vector, minVector, maxVector)
	}
	if got.BM25 < minBM25 || got.BM25 > maxBM25 {
		t.Errorf("bm25 %f out of [%f,%f]", got.BM25, minBM25, maxBM25)
	}
}

func TestNormalizeWeights_BelowMin(t *testing.T) {
	// Very small weights — should be clamped up.
	w := Weights{Vector: 0.01, BM25: 0.01, Recency: 0.01, Precision: 0.01}
	got, ok := normalizeWeights(w)
	if !ok {
		t.Fatal("normalizeWeights: expected ok=true")
	}
	if got.Vector < minVector {
		t.Errorf("vector below min: %f < %f", got.Vector, minVector)
	}
}

func TestComputeDelta_StaleRanking(t *testing.T) {
	d := computeDelta("stale_ranking")
	if d == nil {
		t.Fatal("expected non-nil delta for stale_ranking")
	}
	if d.Recency <= 0 {
		t.Errorf("stale_ranking: expected positive recency delta, got %f", d.Recency)
	}
	if d.Vector >= 0 {
		t.Errorf("stale_ranking: expected negative vector delta, got %f", d.Vector)
	}
}

func TestComputeDelta_VocabularyMismatch(t *testing.T) {
	d := computeDelta("vocabulary_mismatch")
	if d == nil {
		t.Fatal("expected non-nil delta for vocabulary_mismatch")
	}
	if d.Vector <= 0 {
		t.Errorf("vocabulary_mismatch: expected positive vector delta, got %f", d.Vector)
	}
	if d.BM25 >= 0 {
		t.Errorf("vocabulary_mismatch: expected negative BM25 delta, got %f", d.BM25)
	}
}

func TestComputeDelta_ScopeMismatch(t *testing.T) {
	d := computeDelta("scope_mismatch")
	if d == nil {
		t.Fatal("expected non-nil delta for scope_mismatch")
	}
	if d.Precision <= 0 {
		t.Errorf("scope_mismatch: expected positive precision delta, got %f", d.Precision)
	}
}

func TestComputeDelta_NoChange(t *testing.T) {
	for _, class := range []string{"aggregation_failure", "missing_content", "other", ""} {
		if computeDelta(class) != nil {
			t.Errorf("class %q: expected nil delta (no weight change), got non-nil", class)
		}
	}
}

func TestApplyDelta(t *testing.T) {
	base := DefaultWeights()
	delta := Weights{Vector: 0.05, Recency: -0.05}
	got := applyDelta(base, delta)
	wantVector := base.Vector + 0.05
	wantRecency := base.Recency - 0.05
	if math.Abs(got.Vector-wantVector) > 1e-9 {
		t.Errorf("vector: want %f, got %f", wantVector, got.Vector)
	}
	if math.Abs(got.Recency-wantRecency) > 1e-9 {
		t.Errorf("recency: want %f, got %f", wantRecency, got.Recency)
	}
}

func TestDefaultWeights_SumToOne(t *testing.T) {
	d := DefaultWeights()
	sum := d.Vector + d.BM25 + d.Recency + d.Precision
	if math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("default weights sum want 1.0, got %f", sum)
	}
}
