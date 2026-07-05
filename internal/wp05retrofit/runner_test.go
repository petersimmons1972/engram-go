package wp05retrofit

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/layerb"
	"github.com/petersimmons1972/engram/internal/longmemeval"
	"github.com/petersimmons1972/engram/internal/types"
)

type fakeClient struct {
	storeCalls      []storeCall
	storeBatchCalls []storeBatchCall
	recallCalls     []recallCall
	recallByQuery   map[string]longmemeval.RecallResult
	listCalls       []int
	listResult      []types.SearchResult
	recallResult    longmemeval.RecallResult
	recallErr       error
	listErr         error
}

type storeCall struct {
	project string
	content string
	tags    []string
}

type storeBatchCall struct {
	project string
	items   []longmemeval.BatchItem
}

func (f *fakeClient) Store(ctx context.Context, project, content string, tags []string) (string, error) {
	f.storeCalls = append(f.storeCalls, storeCall{project: project, content: content, tags: tags})
	return "mem-1", nil
}

func (f *fakeClient) StoreBatch(ctx context.Context, project string, items []longmemeval.BatchItem) ([]string, error) {
	copied := append([]longmemeval.BatchItem(nil), items...)
	f.storeBatchCalls = append(f.storeBatchCalls, storeBatchCall{project: project, items: copied})
	return []string{"mem-1", "mem-2"}, nil
}

func (f *fakeClient) RecallFullResult(ctx context.Context, project, query string, topK int) (longmemeval.RecallResult, error) {
	f.recallCalls = append(f.recallCalls, recallCall{query: query, topK: topK})
	if f.recallErr != nil {
		return longmemeval.RecallResult{}, f.recallErr
	}
	if f.recallByQuery != nil {
		if result, ok := f.recallByQuery[query]; ok {
			return result, nil
		}
	}
	return f.recallResult, nil
}

func (f *fakeClient) ListProjectMemories(ctx context.Context, project string, limit int) ([]types.SearchResult, error) {
	f.listCalls = append(f.listCalls, limit)
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.listResult, nil
}

func TestScoreItem_HappyPath(t *testing.T) {
	item := longmemeval.Item{
		QuestionID:   "q1",
		QuestionType: "multi-session",
		Question:     "How many times did I bake something?",
		Answer:       "3",
	}
	result := longmemeval.RecallResult{
		LayerB: &layerb.Summary{
			Mode:  "count",
			Count: 3,
			Evidence: []layerb.EventRecord{
				{ProvenanceSpan: "chars:0-10"},
				{ProvenanceSpan: "chars:11-20"},
				{ProvenanceSpan: "chars:21-30"},
			},
		},
	}

	got := ScoreItem(item, result, Provenance{
		GoldVersion:   "gold",
		ScorerVersion: "scorer",
		FeatureFlags:  []string{"layer_b_retrofit"},
		System:        "engram-go-retrofit",
		ItemSet:       "lme-s-multi-session-develop",
		RunID:         "run-1",
		HarnessSHA:    "abc1234",
	})

	if !got.AggregationExpected {
		t.Fatal("AggregationExpected = false, want true")
	}
	if !got.Fired {
		t.Fatal("Fired = false, want true")
	}
	if !got.SolvedType {
		t.Fatal("SolvedType = false, want true")
	}
	if got.ArithmeticCorrect != 1.0 {
		t.Fatalf("ArithmeticCorrect = %v, want 1.0", got.ArithmeticCorrect)
	}
	if got.DedupAccuracy != 1.0 {
		t.Fatalf("DedupAccuracy = %v, want 1.0", got.DedupAccuracy)
	}
	if got.ScopeAccuracy != 1.0 {
		t.Fatalf("ScopeAccuracy = %v, want 1.0", got.ScopeAccuracy)
	}
	if got.MeasuredPath != "layer_b_only" {
		t.Fatalf("MeasuredPath = %q, want layer_b_only", got.MeasuredPath)
	}
	if got.ConstituentRecall != 0.0 || got.ConstituentPrecision != 0.0 {
		t.Fatalf("constituent metrics = (%v, %v), want (0.0, 0.0)", got.ConstituentRecall, got.ConstituentPrecision)
	}
	if len(got.Notes) == 0 || !strings.Contains(got.Notes[0], "unmeasured") {
		t.Fatalf("Notes = %v, want unmeasured-floor note", got.Notes)
	}
}

func TestScoreItem_ZeroEvidenceDedupAccuracyIsVacuouslyOne(t *testing.T) {
	item := longmemeval.Item{
		QuestionID: "q1",
		Question:   "How many times did I bake something?",
		Answer:     "3",
	}
	got := ScoreItem(item, longmemeval.RecallResult{
		LayerB: &layerb.Summary{Count: 3},
	}, Provenance{})
	if got.DedupAccuracy != 1.0 {
		t.Fatalf("DedupAccuracy = %v, want 1.0", got.DedupAccuracy)
	}
}

func TestScoreItem_NonIntegerGoldAnswerForcedZero(t *testing.T) {
	item := longmemeval.Item{
		QuestionID: "q1",
		Question:   "How many pets do I have?",
		Answer:     "three",
	}
	got := ScoreItem(item, longmemeval.RecallResult{
		LayerB: &layerb.Summary{Count: 3},
	}, Provenance{})
	if got.SolvedType {
		t.Fatal("SolvedType = true, want false")
	}
	if got.ArithmeticCorrect != 0.0 {
		t.Fatalf("ArithmeticCorrect = %v, want 0.0", got.ArithmeticCorrect)
	}
	found := false
	for _, note := range got.Notes {
		if strings.Contains(note, "clean integer") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Notes = %v, want integer-parse note", got.Notes)
	}
}

func TestScoreItem_AggregationExpectedFalse(t *testing.T) {
	item := longmemeval.Item{
		QuestionID: "q1",
		Question:   "What is Alex's favorite color?",
		Answer:     "blue",
	}
	got := ScoreItem(item, longmemeval.RecallResult{}, Provenance{})
	if got.AggregationExpected {
		t.Fatal("AggregationExpected = true, want false")
	}
	if got.Fired {
		t.Fatal("Fired = true, want false")
	}
}

func TestIngestItem_UsesStoreBatchAndSessionTags(t *testing.T) {
	item := longmemeval.Item{
		QuestionID:         "q1",
		HaystackDates:      []string{"2026/07/01", "2026/07/02"},
		HaystackSessionIDs: []string{"s1", "s2"},
		HaystackSessions: [][]longmemeval.Turn{
			{{Role: "user", Content: "first"}},
			{{Role: "assistant", Content: "second"}},
		},
	}
	client := &fakeClient{}

	if err := IngestItem(context.Background(), client, "proj-q1", item); err != nil {
		t.Fatalf("IngestItem: %v", err)
	}
	if len(client.storeCalls) != 0 {
		t.Fatalf("storeCalls = %d, want 0 for multi-session batch ingest", len(client.storeCalls))
	}
	if len(client.storeBatchCalls) != 1 {
		t.Fatalf("storeBatchCalls = %d, want 1", len(client.storeBatchCalls))
	}
	got := client.storeBatchCalls[0]
	if got.project != "proj-q1" {
		t.Fatalf("project = %q, want proj-q1", got.project)
	}
	if len(got.items) != 2 {
		t.Fatalf("batch items = %d, want 2", len(got.items))
	}
	if got.items[0].Tags[0] != "session:s1" || got.items[1].Tags[0] != "session:s2" {
		t.Fatalf("tags = %#v, want session tags", got.items)
	}
	if !strings.Contains(got.items[0].Content, "Session date: 2026/07/01") || !strings.Contains(got.items[1].Content, "assistant: second") {
		t.Fatalf("content = %#v, want rendered session content", got.items)
	}
}

func TestRun_WritesDevelopBundle(t *testing.T) {
	items := []longmemeval.Item{
		{
			QuestionID:         "q1",
			QuestionType:       "multi-session",
			Question:           "How many times did I bake something?",
			Answer:             "1",
			HaystackSessionIDs: []string{"s1"},
			HaystackDates:      []string{"2026/07/01"},
			HaystackSessions:   [][]longmemeval.Turn{{{Role: "user", Content: "I baked bread."}}},
		},
	}
	client := &fakeClient{
		recallResult: longmemeval.RecallResult{
			LayerB: &layerb.Summary{
				Count:    1,
				Evidence: []layerb.EventRecord{{ProvenanceSpan: "chars:0-14"}},
			},
		},
	}

	bundle, err := Run(context.Background(), client, items, Config{
		ProjectPrefix: "wp05",
		Limit:         10,
		ProvenanceTemplate: Provenance{
			GoldVersion:   "gold",
			ScorerVersion: "scorer",
			FeatureFlags:  []string{"layer_b_retrofit"},
			System:        SystemName,
			ItemSet:       "set",
			RunID:         "run",
			HarnessSHA:    "sha",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if bundle.System != SystemName || bundle.Substrate != SubstrateName || bundle.SubstrateAssessment != SubstrateAssessment {
		t.Fatalf("bundle header = %+v", bundle)
	}
	if len(bundle.Items) != 1 || bundle.Items[0].Split != "develop" {
		t.Fatalf("bundle items = %+v, want one develop item", bundle.Items)
	}
}

func TestWriteBundle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out", "bundle.json")
	bundle := Bundle{System: SystemName, Substrate: SubstrateName, SubstrateAssessment: SubstrateAssessment}

	if err := WriteBundle(path, bundle); err != nil {
		t.Fatalf("WriteBundle: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "\"system\": \"engram-go-retrofit\"") {
		t.Fatalf("bundle JSON = %s, want system field", data)
	}
}

func TestParseIntAnswer(t *testing.T) {
	cases := []struct {
		name   string
		input  any
		want   int
		wantOK bool
	}{
		{name: "string integer", input: "42", want: 42, wantOK: true},
		{name: "json number integer", input: json.Number("7"), want: 7, wantOK: true},
		{name: "float integer", input: 3.0, want: 3, wantOK: true},
		{name: "float not integer", input: 3.5, wantOK: false},
		{name: "bad string", input: "three", wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseIntAnswer(tc.input)
			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("ParseIntAnswer(%v) = (%d, %t), want (%d, %t)", tc.input, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestRun_PropagatesRecallError(t *testing.T) {
	items := []longmemeval.Item{
		{
			QuestionID:       "q1",
			Question:         "How many times did I bake something?",
			Answer:           "1",
			HaystackSessions: [][]longmemeval.Turn{{{Role: "user", Content: "x"}}},
		},
	}
	client := &fakeClient{recallErr: errors.New("boom")}
	_, err := Run(context.Background(), client, items, Config{
		ProjectPrefix: "wp05",
		Limit:         10,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "recall item q1") {
		t.Fatalf("Run error = %v, want recall item context", err)
	}
}
