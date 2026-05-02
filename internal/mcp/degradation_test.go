package mcp

import (
	"context"
	"errors"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

type failingFeedbackBackend struct{ noopBackend }

func (failingFeedbackBackend) RecordFeedbackWithClass(_ context.Context, _ string, _ []string, _ string) error {
	return errors.New("feedback db stalled")
}

func (failingFeedbackBackend) RecordFeedback(_ context.Context, _ string, _ []string) error {
	return errors.New("feedback db stalled")
}

type failingAggregateBackend struct{ noopBackend }

func (failingAggregateBackend) AggregateMemories(_ context.Context, _, _, _ string, _ int) ([]types.AggregateRow, error) {
	return nil, errors.New("aggregate db stalled")
}

func (failingAggregateBackend) AggregateFailureClasses(_ context.Context, _ string, _ int) ([]types.AggregateRow, error) {
	return nil, errors.New("aggregate db stalled")
}

type failingQueryDocBackend struct{ noopBackend }

func (failingQueryDocBackend) GetMemory(_ context.Context, _ string) (*types.Memory, error) {
	return nil, errors.New("document db stalled")
}

func newFailingPool(t *testing.T, backend db.Backend) *EnginePool {
	t.Helper()
	return NewEnginePool(func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, backend, noopEmbedder{}, project, "", "", false, nil, 0)
		return &EngineHandle{Engine: engine}, nil
	})
}

func TestHandleMemoryFeedback_DBErrorRaisesReviewPath(t *testing.T) {
	pool := newFailingPool(t, failingFeedbackBackend{})
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":       "demo",
		"memory_ids":    []any{"mem-1"},
		"event_id":      "123e4567-e89b-12d3-a456-426614174000",
		"failure_class": "missing_content",
	}

	res, err := handleMemoryFeedback(context.Background(), pool, req)
	require.Error(t, err)
	require.Nil(t, res)
	require.Contains(t, err.Error(), "feedback db stalled")
}

func TestHandleMemoryAggregate_DBErrorRaisesReviewPath(t *testing.T) {
	pool := newFailingPool(t, failingAggregateBackend{})
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "demo",
		"by":      "tag",
	}

	res, err := handleMemoryAggregate(context.Background(), pool, req)
	require.Error(t, err)
	require.Nil(t, res)
	require.Contains(t, err.Error(), "aggregate db stalled")
}

func TestHandleMemoryQueryDocument_DBErrorRaisesReviewPath(t *testing.T) {
	_, err := execQueryDocument(context.Background(), queryDocumentDeps{
		getMemory: func(context.Context, string) (*types.Memory, error) {
			return nil, errors.New("document db stalled")
		},
		claudeClient: &claude.Client{},
	}, claude.DocumentQuery{
		Project:     "demo",
		MemoryID:    "mem-1",
		Question:    "what happened?",
		WindowChars: 100,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "document db stalled")
}
