package wp05retrofit

import (
	"context"
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/layerb"
	"github.com/petersimmons1972/engram/internal/longmemeval"
	"github.com/petersimmons1972/engram/internal/types"
)

func memoryResult(id, content string, score float64) types.SearchResult {
	return types.SearchResult{
		Memory: &types.Memory{ID: id, Content: content, Project: "proj"},
		Score:  score,
	}
}

func TestMergeSearchResults_UnionsByIDAndKeepsMaxScore(t *testing.T) {
	primary := []types.SearchResult{
		memoryResult("a", "alpha", 0.9),
		memoryResult("b", "beta", 0.5),
	}
	secondary := []types.SearchResult{
		memoryResult("b", "beta", 0.8),
		memoryResult("c", "gamma", 0.1),
	}
	got := mergeSearchResults(primary, secondary)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3; got %#v", len(got), got)
	}
	if got[0].Memory.ID != "a" || got[1].Memory.ID != "b" || got[2].Memory.ID != "c" {
		t.Fatalf("order = [%s %s %s], want [a b c]", got[0].Memory.ID, got[1].Memory.ID, got[2].Memory.ID)
	}
	if got[1].Score != 0.8 {
		t.Fatalf("merged score for b = %v, want 0.8", got[1].Score)
	}
}

func TestRecallItem_BaselineUsesSingleRecall(t *testing.T) {
	client := &fakeClient{
		recallResult: longmemeval.RecallResult{
			LayerB: &layerb.Summary{Count: 1},
		},
	}
	item := Item{QuestionID: "q1", Question: "How many times did I bake bread?"}
	got, err := RecallItem(context.Background(), client, "proj-q1", item, Config{Limit: 200})
	if err != nil {
		t.Fatalf("RecallItem: %v", err)
	}
	if len(client.recallCalls) != 1 {
		t.Fatalf("recallCalls = %d, want 1", len(client.recallCalls))
	}
	if client.recallCalls[0].topK != 200 {
		t.Fatalf("topK = %d, want 200", client.recallCalls[0].topK)
	}
	if len(client.listCalls) != 0 {
		t.Fatalf("listCalls = %d, want 0", len(client.listCalls))
	}
	if got.LayerB == nil || got.LayerB.Count != 1 {
		t.Fatalf("LayerB = %+v, want server summary", got.LayerB)
	}
}

func TestRecallItem_ExhaustiveAggregationUnionsPasses(t *testing.T) {
	question := "How many times did I visit the doctor?"
	anchor := longmemeval.ExtractAggregationAnchor(question)
	client := &fakeClient{
		recallByQuery: map[string]longmemeval.RecallResult{
			question: {
				Results: []types.SearchResult{memoryResult("primary", "visited doctor once", 0.9)},
			},
			anchor: {
				Results: []types.SearchResult{memoryResult("anchor", "doctor appointment follow-up", 0.7)},
			},
		},
		listResult: []types.SearchResult{
			memoryResult("listed", "Session date: 2024-01-01\nuser: saw doctor again", 0),
		},
	}
	item := Item{QuestionID: "q1", Question: question}
	got, err := RecallItem(context.Background(), client, "proj-q1", item, Config{
		Limit:                 200,
		ExhaustiveAggregation: true,
	})
	if err != nil {
		t.Fatalf("RecallItem: %v", err)
	}
	if len(client.recallCalls) != 2 {
		t.Fatalf("recallCalls = %d, want 2 (primary + anchor)", len(client.recallCalls))
	}
	if client.recallCalls[0].topK != 500 || client.recallCalls[1].topK != 500 {
		t.Fatalf("recall topK = [%d %d], want [500 500]", client.recallCalls[0].topK, client.recallCalls[1].topK)
	}
	if len(client.listCalls) != 1 || client.listCalls[0] != projectSweepLimit {
		t.Fatalf("listCalls = %v, want [%d]", client.listCalls, projectSweepLimit)
	}
	if len(got.Results) != 3 {
		t.Fatalf("len(Results) = %d, want 3", len(got.Results))
	}
	if got.LayerB == nil {
		t.Fatal("LayerB = nil, want client-side summary over merged set")
	}
}

func TestRecallItem_ExhaustiveSkipsNonAggregation(t *testing.T) {
	client := &fakeClient{
		recallResult: longmemeval.RecallResult{},
	}
	item := Item{QuestionID: "q1", Question: "What is Alex's favorite color?"}
	_, err := RecallItem(context.Background(), client, "proj-q1", item, Config{
		Limit:                 200,
		ExhaustiveAggregation: true,
	})
	if err != nil {
		t.Fatalf("RecallItem: %v", err)
	}
	if len(client.recallCalls) != 1 || client.recallCalls[0].topK != 200 {
		t.Fatalf("recallCalls = %+v, want single baseline recall at 200", client.recallCalls)
	}
	if len(client.listCalls) != 0 {
		t.Fatalf("listCalls = %d, want 0", len(client.listCalls))
	}
}

func TestRecallItem_ExhaustivePropagatesListError(t *testing.T) {
	client := &fakeClient{
		recallByQuery: map[string]longmemeval.RecallResult{
			"How many times did I visit the doctor?": {Results: nil},
		},
		listErr: context.Canceled,
	}
	item := Item{QuestionID: "q1", Question: "How many times did I visit the doctor?"}
	_, err := RecallItem(context.Background(), client, "proj-q1", item, Config{
		Limit:                 200,
		ExhaustiveAggregation: true,
	})
	if err == nil || !strings.Contains(err.Error(), "project sweep") {
		t.Fatalf("err = %v, want project sweep failure", err)
	}
}