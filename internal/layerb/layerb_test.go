package layerb_test

import (
	"context"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/layerb"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

type stubStore struct {
	atoms  map[string]layerb.Atom
	events map[string]layerb.Event
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
	require.NoError(t, err)
	require.NotNil(t, summary)
	require.Equal(t, "visit doctor", summary.Anchor)
	require.Len(t, summary.Evidence, 1)
	require.NotNil(t, summary.Evidence[0].EventTime)
	require.True(t, summary.Evidence[0].EventTime.Equal(validFrom), "valid_from must win over created_at for temporal inversion replay")
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
	require.NoError(t, err)
	require.Nil(t, summary)
	require.Empty(t, store.atoms)
	require.Empty(t, store.events)
}
