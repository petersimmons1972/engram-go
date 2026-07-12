package mcp

import (
	"context"
	"errors"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/atom"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/stretchr/testify/require"
)

type failingEventWindowBackend struct{ noopBackend }

func (failingEventWindowBackend) GetActiveAtoms(context.Context, string, string) ([]atom.Atom, error) {
	return nil, nil
}

type seededEventWindowBackend struct {
	noopBackend
	atoms    []atom.Atom
	calls    int
	lastOpts db.AtomQueryOpts
}

func (b *seededEventWindowBackend) GetActiveAtoms(context.Context, string, string) ([]atom.Atom, error) {
	return nil, nil
}

func (b *seededEventWindowBackend) GetActiveAtomsFiltered(_ context.Context, _ string, opts db.AtomQueryOpts) ([]atom.Atom, error) {
	b.calls++
	b.lastOpts = opts
	return b.atoms, nil
}

func TestHandleMemoryRecall_EventWindowReturnsContextAndAtomsFromOneQuery(t *testing.T) {
	validFrom := time.Date(2024, 2, 19, 0, 0, 0, 0, time.UTC)
	backend := &seededEventWindowBackend{atoms: []atom.Atom{{
		ID:        "event-1",
		Type:      atom.TypeEvent,
		Statement: "Visited the dentist.",
		ValidFrom: &validFrom,
	}}}
	pool := newPoolWithBackend(t, backend)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test", "query": "What happened yesterday?",
		"question_text": "What happened yesterday?", "question_date": "2024/02/20",
		"event_window_recall":             true,
		"event_window_include_superseded": true,
	}

	res, err := handleMemoryRecall(context.Background(), pool, req, testConfig())

	require.NoError(t, err)
	out := parseRecallResult(t, res)
	require.Contains(t, out, "event_window_context")
	require.Len(t, out["event_window_atoms"], 1)
	require.Equal(t, 1, backend.calls)
	require.True(t, backend.lastOpts.IncludeSuperseded)
}

func (failingEventWindowBackend) GetActiveAtomsFiltered(context.Context, string, db.AtomQueryOpts) ([]atom.Atom, error) {
	return nil, errors.New("event query failed")
}

func TestHandleMemoryRecall_EventWindowFailureDegradesToBaseline(t *testing.T) {
	pool := newPoolWithBackend(t, failingEventWindowBackend{})
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test", "query": "What happened yesterday?",
		"question_text": "What happened yesterday?", "question_date": "2024/02/20",
		"event_window_recall": true,
	}

	res, err := handleMemoryRecall(context.Background(), pool, req, testConfig())
	require.NoError(t, err)
	out := parseRecallResult(t, res)
	require.NotContains(t, out, "event_window_context")
	require.NotContains(t, out, "event_window_atoms")
}
