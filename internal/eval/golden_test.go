package eval_test

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/eval"
)

func TestPrecisionAtK(t *testing.T) {
	retrieved := []string{"a", "b", "c", "d", "e"}
	relevant := map[string]bool{"a": true, "c": true, "e": true}

	got := eval.PrecisionAtK(retrieved, relevant, 5)
	if got < 0.59 || got > 0.61 {
		t.Errorf("PrecisionAtK(k=5) = %f, want ~0.6", got)
	}
	got1 := eval.PrecisionAtK(retrieved, relevant, 1)
	if got1 < 0.99 {
		t.Errorf("PrecisionAtK(k=1) = %f, want 1.0 (a is relevant)", got1)
	}
	got0 := eval.PrecisionAtK(nil, relevant, 5)
	if got0 != 0 {
		t.Errorf("PrecisionAtK(nil) = %f, want 0", got0)
	}
	gotNegK := eval.PrecisionAtK(retrieved, relevant, 0)
	if gotNegK != 0 {
		t.Errorf("PrecisionAtK(k=0) = %f, want 0", gotNegK)
	}
}

func TestMRR(t *testing.T) {
	retrieved := []string{"x", "a", "b"}
	relevant := map[string]bool{"a": true}
	got := eval.MRR(retrieved, relevant)
	if got < 0.49 || got > 0.51 {
		t.Errorf("MRR = %f, want ~0.5 (first hit at rank 2)", got)
	}
	got0 := eval.MRR(nil, relevant)
	if got0 != 0 {
		t.Errorf("MRR(nil) = %f, want 0", got0)
	}
	gotNoHit := eval.MRR(retrieved, map[string]bool{})
	if gotNoHit != 0 {
		t.Errorf("MRR(no relevant) = %f, want 0", gotNoHit)
	}
}

func TestNDCG(t *testing.T) {
	retrieved := []string{"a", "b", "c"}
	relevant := map[string]bool{"a": true, "c": true}
	score := eval.NDCG(retrieved, relevant, 3)
	if score <= 0 || score > 1 {
		t.Errorf("NDCG = %f, want (0,1]", score)
	}
	score0 := eval.NDCG(nil, relevant, 3)
	if score0 != 0 {
		t.Errorf("NDCG(nil) = %f, want 0", score0)
	}
	scoreEmpty := eval.NDCG(retrieved, map[string]bool{}, 3)
	if scoreEmpty != 0 {
		t.Errorf("NDCG(no relevant) = %f, want 0", scoreEmpty)
	}
}
