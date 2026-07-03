package mcp

import (
	"context"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/layerb"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

type layerBRecallBackend struct {
	noopBackend
	mem      *types.Memory
	atoms    map[string]layerb.Atom
	events   map[string]layerb.Event
	evidence map[string]layerb.EventRecord
}

func newLayerBRecallBackend(project string) *layerBRecallBackend {
	now := time.Date(2026, 7, 3, 18, 0, 0, 0, time.UTC)
	return &layerBRecallBackend{
		mem: &types.Memory{
			ID:         "mem-layerb-1",
			Project:    project,
			Content:    "I visited the doctor on Monday. I visited the doctor again on Friday.",
			CreatedAt:  now,
			UpdatedAt:  now,
			MemoryType: types.MemoryTypeContext,
		},
		atoms:    map[string]layerb.Atom{},
		events:   map[string]layerb.Event{},
		evidence: map[string]layerb.EventRecord{},
	}
}

func (b *layerBRecallBackend) FTSSearch(_ context.Context, project, _ string, _ int, _, _ *time.Time) ([]db.FTSResult, error) {
	mem := *b.mem
	mem.Project = project
	return []db.FTSResult{{
		Memory: &mem,
		Score:  1,
	}}, nil
}

func (b *layerBRecallBackend) UpsertLayerBAtom(_ context.Context, atom layerb.Atom) error {
	key := atom.MemoryID + "|" + atom.ProvenanceSpan + "|" + atom.Statement
	b.atoms[key] = atom
	return nil
}

func (b *layerBRecallBackend) UpsertLayerBEvent(_ context.Context, event layerb.Event) error {
	key := event.MemoryID + "|" + event.ProvenanceSpan + "|" + event.Anchor
	b.events[key] = event
	b.evidence[key] = layerb.EventRecord{
		MemoryID:       event.MemoryID,
		ProvenanceSpan: event.ProvenanceSpan,
		SpanText:       event.SpanText,
		Anchor:         event.Anchor,
		EventTime:      event.EventTime,
	}
	return nil
}

func (b *layerBRecallBackend) ListLayerBEvents(_ context.Context, _ string, memoryIDs []string) ([]layerb.EventRecord, error) {
	allow := make(map[string]bool, len(memoryIDs))
	for _, id := range memoryIDs {
		allow[id] = true
	}
	out := make([]layerb.EventRecord, 0, len(b.evidence))
	for _, rec := range b.evidence {
		if allow[rec.MemoryID] {
			out = append(out, rec)
		}
	}
	return out, nil
}

func newLayerBRecallPool(t *testing.T, backend *layerBRecallBackend) *EnginePool {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, backend, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	return NewEnginePool(factory)
}

func TestHandleMemoryRecall_AggregationQuestionAddsLayerBCount(t *testing.T) {
	backend := newLayerBRecallBackend("test")
	pool := newLayerBRecallPool(t, backend)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"query":   "How many times did I visit the doctor?",
	}

	res, err := handleMemoryRecall(context.Background(), pool, req, testConfig())
	require.NoError(t, err)

	out := parseRecallResult(t, res)
	rawLayerB, ok := out["layer_b"]
	require.True(t, ok, "aggregation-shaped recall must include layer_b metadata")

	layerBMap, ok := rawLayerB.(map[string]any)
	require.True(t, ok, "layer_b must marshal as an object")
	require.Equal(t, "count", layerBMap["mode"])
	require.Equal(t, "visit doctor", layerBMap["anchor"])
	require.Equal(t, float64(2), layerBMap["count"])

	rawEvidence, ok := layerBMap["evidence"].([]any)
	require.True(t, ok, "layer_b evidence must be a JSON array")
	require.Len(t, rawEvidence, 2)
}

func TestHandleMemoryRecall_LayerBIndexingIsIdempotent(t *testing.T) {
	backend := newLayerBRecallBackend("test")
	pool := newLayerBRecallPool(t, backend)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"query":   "How many times did I visit the doctor?",
	}

	_, err := handleMemoryRecall(context.Background(), pool, req, testConfig())
	require.NoError(t, err)
	_, err = handleMemoryRecall(context.Background(), pool, req, testConfig())
	require.NoError(t, err)

	require.Len(t, backend.atoms, 2, "re-indexing the same recalled memory must not duplicate atoms")
	require.Len(t, backend.events, 2, "re-indexing the same recalled memory must not duplicate events")
}
