package layerb_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/layerb"
	"github.com/petersimmons1972/engram/internal/types"
)

type stubStore struct {
	atoms  map[string]layerb.Atom
	events map[string]layerb.Event
}

type listErrorStore struct {
	stubStore
	err error
}

func (s *stubStore) UpsertLayerBAtom(_ context.Context, atom layerb.Atom) error {
	if s.atoms == nil {
		s.atoms = map[string]layerb.Atom{}
	}
	s.atoms[atom.MemoryID+"|"+atom.ProvenanceSpan+"|"+atom.NormalizedText] = atom
	return nil
}

func (s *stubStore) UpsertLayerBEvent(_ context.Context, event layerb.Event) error {
	if s.events == nil {
		s.events = map[string]layerb.Event{}
	}
	s.events[event.MemoryID+"|"+event.ProvenanceSpan+"|"+event.Anchor] = event
	return nil
}

func (s *stubStore) ListLayerBEvents(_ context.Context, _ string, memoryIDs []string) ([]layerb.EventRecord, error) {
	allow := make(map[string]bool, len(memoryIDs))
	for _, id := range memoryIDs {
		allow[id] = true
	}
	out := make([]layerb.EventRecord, 0, len(s.events))
	for _, event := range s.events {
		if !allow[event.MemoryID] {
			continue
		}
		out = append(out, layerb.EventRecord{
			MemoryID:       event.MemoryID,
			ProvenanceSpan: event.ProvenanceSpan,
			SpanText:       event.SpanText,
			Anchor:         event.Anchor,
			NormalizedText: event.NormalizedText,
			EventTime:      event.EventTime,
		})
	}
	return out, nil
}

func (s *listErrorStore) ListLayerBEvents(_ context.Context, _ string, _ []string) ([]layerb.EventRecord, error) {
	return nil, s.err
}

func TestBuildSummary_UsesValidFromForTemporalInversion(t *testing.T) {
	store := &stubStore{}
	validFrom := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 7, 3, 18, 0, 0, 0, time.UTC)
	results := []types.SearchResult{{
		Memory: &types.Memory{
			ID:        "mem-1",
			Project:   "proj",
			Content:   "I visited the doctor on Tuesday.",
			ValidFrom: &validFrom,
			CreatedAt: createdAt,
		},
		Score: 1,
	}}

	summary, err := layerb.BuildSummary(context.Background(), store, "How many times did I visit the doctor?", results)
	if err != nil {
		t.Fatalf("BuildSummary: %v", err)
	}
	if summary == nil {
		t.Fatal("BuildSummary returned nil summary")
	}
	if summary.Anchor != "visit doctor" {
		t.Fatalf("anchor = %q, want %q", summary.Anchor, "visit doctor")
	}
	if len(summary.Evidence) != 1 {
		t.Fatalf("evidence len = %d, want 1", len(summary.Evidence))
	}
	if summary.Evidence[0].EventTime == nil {
		t.Fatal("event_time is nil")
	}
	if !summary.Evidence[0].EventTime.Equal(validFrom) {
		t.Fatalf("event_time = %v, want %v", summary.Evidence[0].EventTime, validFrom)
	}
}

func TestBuildSummary_NonAggregationQuestionIsNoOp(t *testing.T) {
	store := &stubStore{}
	results := []types.SearchResult{{
		Memory: &types.Memory{
			ID:        "mem-1",
			Project:   "proj",
			Content:   "I visited the doctor on Tuesday.",
			CreatedAt: time.Now().UTC(),
		},
		Score: 1,
	}}

	summary, err := layerb.BuildSummary(context.Background(), store, "When did I visit the doctor?", results)
	if err != nil {
		t.Fatalf("BuildSummary: %v", err)
	}
	if summary != nil {
		t.Fatalf("summary = %#v, want nil", summary)
	}
	if len(store.atoms) != 0 {
		t.Fatalf("atoms len = %d, want 0", len(store.atoms))
	}
	if len(store.events) != 0 {
		t.Fatalf("events len = %d, want 0", len(store.events))
	}
}

func TestBuildSummary_PreservesProvenanceSpanAgainstSourceWhitespace(t *testing.T) {
	store := &stubStore{}
	content := "\n  I visited the doctor on Tuesday.  \n"
	results := []types.SearchResult{{
		Memory: &types.Memory{
			ID:        "mem-1",
			Project:   "proj",
			Content:   content,
			CreatedAt: time.Date(2026, 7, 3, 18, 0, 0, 0, time.UTC),
		},
		Score: 1,
	}}

	summary, err := layerb.BuildSummary(context.Background(), store, "How many times did I visit the doctor?", results)
	if err != nil {
		t.Fatalf("BuildSummary: %v", err)
	}
	if summary == nil {
		t.Fatal("BuildSummary returned nil summary")
	}
	if len(summary.Evidence) != 1 {
		t.Fatalf("evidence len = %d, want 1", len(summary.Evidence))
	}

	start, end := mustParseSpan(t, summary.Evidence[0].ProvenanceSpan)
	if got := content[start:end]; got != "I visited the doctor on Tuesday." {
		t.Fatalf("content[%d:%d] = %q, want exact sentence", start, end, got)
	}
	if got := summary.Evidence[0].SpanText; got != "I visited the doctor on Tuesday." {
		t.Fatalf("span_text = %q, want exact sentence", got)
	}
}

func TestBuildSummary_EmptyAnchorTermsReturnNil(t *testing.T) {
	store := &stubStore{}
	results := []types.SearchResult{{
		Memory: &types.Memory{
			ID:        "mem-1",
			Project:   "proj",
			Content:   "I did it yesterday.",
			CreatedAt: time.Date(2026, 7, 3, 18, 0, 0, 0, time.UTC),
		},
		Score: 1,
	}}

	summary, err := layerb.BuildSummary(context.Background(), store, "How many times did I do?", results)
	if err != nil {
		t.Fatalf("BuildSummary: %v", err)
	}
	if summary != nil {
		t.Fatalf("summary = %#v, want nil", summary)
	}
	if len(store.atoms) != 0 || len(store.events) != 0 {
		t.Fatalf("store writes = atoms:%d events:%d, want none", len(store.atoms), len(store.events))
	}
}

func TestBuildSummary_IgnoresNilAndBlankMemories(t *testing.T) {
	store := &stubStore{}
	results := []types.SearchResult{
		{Memory: nil},
		{Memory: &types.Memory{ID: "mem-blank", Project: "proj", Content: " \n\t "}},
	}

	summary, err := layerb.BuildSummary(context.Background(), store, "How many times did I visit the doctor?", results)
	if err != nil {
		t.Fatalf("BuildSummary: %v", err)
	}
	if summary != nil {
		t.Fatalf("summary = %#v, want nil", summary)
	}
}

func TestBuildSummary_ReturnsNilWhenNoEvidenceMatchesAnchor(t *testing.T) {
	store := &stubStore{}
	results := []types.SearchResult{{
		Memory: &types.Memory{
			ID:        "mem-1",
			Project:   "proj",
			Content:   "I cooked dinner at home.",
			CreatedAt: time.Date(2026, 7, 3, 18, 0, 0, 0, time.UTC),
		},
		Score: 1,
	}}

	summary, err := layerb.BuildSummary(context.Background(), store, "How many times did I visit the doctor?", results)
	if err != nil {
		t.Fatalf("BuildSummary: %v", err)
	}
	if summary != nil {
		t.Fatalf("summary = %#v, want nil", summary)
	}
}

func TestBuildSummary_SortsEvidenceWithNilEventTimesLast(t *testing.T) {
	store := &stubStore{}
	validFrom := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	results := []types.SearchResult{
		{
			Memory: &types.Memory{
				ID:        "mem-no-time",
				Project:   "proj",
				Content:   "I visited the doctor during intake.",
				CreatedAt: time.Time{},
			},
			Score: 1,
		},
		{
			Memory: &types.Memory{
				ID:        "mem-with-time",
				Project:   "proj",
				Content:   "I visited the doctor during follow-up.",
				ValidFrom: &validFrom,
				CreatedAt: time.Date(2026, 7, 3, 18, 0, 0, 0, time.UTC),
			},
			Score: 1,
		},
	}

	summary, err := layerb.BuildSummary(context.Background(), store, "How many times did I visit the doctor?", results)
	if err != nil {
		t.Fatalf("BuildSummary: %v", err)
	}
	if summary == nil {
		t.Fatal("BuildSummary returned nil summary")
	}
	if len(summary.Evidence) != 2 {
		t.Fatalf("evidence len = %d, want 2", len(summary.Evidence))
	}
	if got := summary.Evidence[0].MemoryID; got != "mem-with-time" {
		t.Fatalf("first evidence memory_id = %q, want timed record first", got)
	}
	if got := summary.Evidence[1].MemoryID; got != "mem-no-time" {
		t.Fatalf("second evidence memory_id = %q, want nil-time record last", got)
	}
}

func TestBuildSummary_PropagatesListErrors(t *testing.T) {
	store := &listErrorStore{err: fmt.Errorf("boom")}
	results := []types.SearchResult{{
		Memory: &types.Memory{
			ID:        "mem-1",
			Project:   "proj",
			Content:   "I visited the doctor on Tuesday.",
			CreatedAt: time.Date(2026, 7, 3, 18, 0, 0, 0, time.UTC),
		},
		Score: 1,
	}}

	_, err := layerb.BuildSummary(context.Background(), store, "How many times did I visit the doctor?", results)
	if err == nil {
		t.Fatal("BuildSummary error = nil, want wrapped list failure")
	}
	if !strings.Contains(err.Error(), "layerb list events: boom") {
		t.Fatalf("error = %v, want wrapped list failure", err)
	}
}

func TestBuildSummary_MatchesScatteredAggregationEvidenceAcrossSessions(t *testing.T) {
	store := &stubStore{}
	results := []types.SearchResult{
		{
			Memory: &types.Memory{
				ID:        "mem-1",
				Project:   "proj",
				Content:   "I serviced my neighbor's mountain bike as a favor.",
				CreatedAt: time.Date(2026, 3, 3, 18, 0, 0, 0, time.UTC),
			},
			Score: 1,
		},
		{
			Memory: &types.Memory{
				ID:        "mem-2",
				Project:   "proj",
				Content:   "I still plan to service my own work queue.",
				CreatedAt: time.Date(2026, 3, 10, 18, 0, 0, 0, time.UTC),
			},
			Score: 1,
		},
		{
			Memory: &types.Memory{
				ID:        "mem-3",
				Project:   "proj",
				Content:   "Later in March I inspected my sister's bike.",
				CreatedAt: time.Date(2026, 3, 20, 18, 0, 0, 0, time.UTC),
			},
			Score: 1,
		},
	}

	summary, err := layerb.BuildSummary(context.Background(), store, "How many bikes did I service or plan to service in March?", results)
	if err != nil {
		t.Fatalf("BuildSummary: %v", err)
	}
	if summary == nil {
		t.Fatal("BuildSummary returned nil summary")
	}
	if summary.Anchor != "bike service plan march" {
		t.Fatalf("anchor = %q, want %q", summary.Anchor, "bike service plan march")
	}
	if summary.Count != 3 {
		t.Fatalf("count = %d, want 3", summary.Count)
	}
}

func mustParseSpan(t *testing.T, provenance string) (int, int) {
	t.Helper()
	var start, end int
	if _, err := fmt.Sscanf(strings.TrimSpace(provenance), "chars:%d-%d", &start, &end); err != nil {
		t.Fatalf("parse provenance span %q: %v", provenance, err)
	}
	return start, end
}

func TestBuildSummary_WholeMemoryFallbackFiresWhenNoSingleSentenceMatches(t *testing.T) {
	store := &stubStore{}
	results := []types.SearchResult{
		{
			Memory: &types.Memory{
				ID:        "mem-scattered-1",
				Project:   "proj",
				Content:   "I have a bike. I need to service it. I plan to do that.",
				CreatedAt: time.Date(2026, 3, 5, 12, 0, 0, 0, time.UTC),
			},
			Score: 1,
		},
	}

	summary, err := layerb.BuildSummary(context.Background(), store, "How many bikes did I service or plan to service in March?", results)
	if err != nil {
		t.Fatalf("BuildSummary: %v", err)
	}
	if summary == nil {
		t.Fatal("BuildSummary returned nil summary — expected the whole-memory fallback to fire")
	}
	if summary.Count != 1 {
		t.Fatalf("count = %d, want 1 (whole-memory fallback should produce exactly one Atom for this memory)", summary.Count)
	}
}

func TestBuildSummary_LoneMemoryWithPartialAnchorOverlapDoesNotFire(t *testing.T) {
	store := &stubStore{}
	results := []types.SearchResult{
		{
			Memory: &types.Memory{
				ID:        "mem-unrelated",
				Project:   "proj",
				Content:   "I rode my bike to plan a surprise party for my coworker.",
				CreatedAt: time.Date(2026, 3, 12, 12, 0, 0, 0, time.UTC),
			},
			Score: 1,
		},
	}

	summary, err := layerb.BuildSummary(context.Background(), store, "How many bikes did I service or plan to service in March?", results)
	if err != nil {
		t.Fatalf("BuildSummary: %v", err)
	}
	if summary != nil {
		t.Fatalf("BuildSummary returned a summary (count=%d) for a single memory covering only 2 of 4 anchor terms in unrelated content — expected nil (collective near-complete gate should reject this)", summary.Count)
	}
}

func TestBuildSummary_DisjointFragmentUnionAcrossUnrelatedMemoriesDoesNotFire(t *testing.T) {
	store := &stubStore{}
	results := []types.SearchResult{
		{
			Memory: &types.Memory{
				ID:        "mem-unrelated-a",
				Project:   "proj",
				Content:   "I serviced the bike rack in January.",
				CreatedAt: time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC),
			},
			Score: 1,
		},
		{
			Memory: &types.Memory{
				ID:        "mem-unrelated-b",
				Project:   "proj",
				Content:   "I plan the March budget next week.",
				CreatedAt: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
			},
			Score: 1,
		},
	}

	summary, err := layerb.BuildSummary(context.Background(), store, "How many bikes did I service or plan to service in March?", results)
	if err != nil {
		t.Fatalf("BuildSummary: %v", err)
	}
	if summary != nil {
		t.Fatalf("BuildSummary returned a summary (count=%d) for two memories with disjoint matched-term sets (bike/service vs plan/march, zero shared terms) — expected nil (connectivity gate should reject a coincidental disjoint-fragment union)", summary.Count)
	}
}
