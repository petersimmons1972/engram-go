package mcp

import (
	"context"
	"errors"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/atom"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/stretchr/testify/require"
)

type failingEventWindowBackend struct{ noopBackend }

func (failingEventWindowBackend) GetActiveAtoms(context.Context, string, string) ([]atom.Atom, error) {
	return nil, nil
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
}
