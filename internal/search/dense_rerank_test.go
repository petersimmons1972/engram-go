package search

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/petersimmons1972/engram/internal/types"
)

type denseSpyReranker struct {
	onCall  func(items []RerankItem)
	results []RerankResult
	err     error
}

func (r *denseSpyReranker) RerankResults(_ context.Context, _ string, items []RerankItem) ([]RerankResult, error) {
	if r.onCall != nil {
		r.onCall(items)
	}
	if r.err != nil {
		return nil, r.err
	}
	out := make([]RerankResult, len(r.results))
	copy(out, r.results)
	return out, nil
}

func TestApplyDenseRerank_UpdatesOnlyTopDenseCandidates(t *testing.T) {
	memories := map[string]*types.Memory{
		"a": {ID: "a", Summary: ptr("summary a")},
		"b": {ID: "b", Summary: ptr("summary b")},
		"c": {ID: "c", Summary: ptr("summary c")},
	}
	bestHits := map[string]bestHit{
		"a": {cosine: 0.90, denseScore: 0.90},
		"b": {cosine: 0.80, denseScore: 0.80},
		"c": {cosine: 0.20, denseScore: 0.20},
	}

	var gotIDs []string
	reranker := &denseSpyReranker{
		onCall: func(items []RerankItem) {
			gotIDs = append(gotIDs, items[0].ID, items[1].ID)
		},
		results: []RerankResult{
			{ID: "a", Score: 0.10},
			{ID: "b", Score: 0.90},
		},
	}

	applyDenseRerank(context.Background(), "query", memories, bestHits, reranker, 2)

	if len(gotIDs) != 2 || gotIDs[0] != "a" || gotIDs[1] != "b" {
		t.Fatalf("dense reranker received ids %v, want [a b]", gotIDs)
	}
	if bestHits["a"].cosine != 0.90 || bestHits["b"].cosine != 0.80 {
		t.Fatalf("dense rerank must not overwrite original cosine scores: got a=%v b=%v", bestHits["a"].cosine, bestHits["b"].cosine)
	}
	if bestHits["a"].denseScore != 0.0 {
		t.Fatalf("denseScore(a) = %v, want 0.0 after normalization", bestHits["a"].denseScore)
	}
	if bestHits["b"].denseScore != 1.0 {
		t.Fatalf("denseScore(b) = %v, want 1.0 after normalization", bestHits["b"].denseScore)
	}
	if bestHits["c"].denseScore != 0.20 {
		t.Fatalf("denseScore(c) = %v, want unchanged 0.20 outside top-N", bestHits["c"].denseScore)
	}
}

func TestApplyDenseRerank_DefaultTopNIs20(t *testing.T) {
	memories := make(map[string]*types.Memory, 25)
	bestHits := make(map[string]bestHit, 25)
	for i := 0; i < 25; i++ {
		id := strconv.Itoa(i)
		memories[id] = &types.Memory{ID: id, Summary: ptr("summary " + id)}
		bestHits[id] = bestHit{
			cosine:     1.0 - (float64(i) / 100.0),
			denseScore: 1.0 - (float64(i) / 100.0),
		}
	}

	gotCount := 0
	reranker := &denseSpyReranker{
		onCall: func(items []RerankItem) { gotCount = len(items) },
		results: func() []RerankResult {
			out := make([]RerankResult, 20)
			for i := range out {
				out[i] = RerankResult{ID: strconv.Itoa(i), Score: float64(20 - i)}
			}
			return out
		}(),
	}

	applyDenseRerank(context.Background(), "query", memories, bestHits, reranker, 0)

	if gotCount != defaultDenseRerankTopN {
		t.Fatalf("dense reranker received %d items, want default top-N %d", gotCount, defaultDenseRerankTopN)
	}
}

func TestApplyDenseRerank_ErrorLeavesDenseScoresUnchanged(t *testing.T) {
	memories := map[string]*types.Memory{
		"a": {ID: "a", Summary: ptr("summary a")},
		"b": {ID: "b", Summary: ptr("summary b")},
	}
	bestHits := map[string]bestHit{
		"a": {cosine: 0.90, denseScore: 0.90},
		"b": {cosine: 0.80, denseScore: 0.80},
	}

	applyDenseRerank(context.Background(), "query", memories, bestHits, &denseSpyReranker{
		err: errors.New("boom"),
	}, 2)

	if bestHits["a"].denseScore != 0.90 || bestHits["b"].denseScore != 0.80 {
		t.Fatalf("dense rerank error must leave scores unchanged; got a=%v b=%v", bestHits["a"].denseScore, bestHits["b"].denseScore)
	}
}

func ptr(s string) *string { return &s }
