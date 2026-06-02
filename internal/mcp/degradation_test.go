package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
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

// ── #989: handle-mode degradation reason must reflect actual cause ─────────────

// immediateErrEmbedder is an embed.Client that always returns a hard error,
// used to inject a deterministic "embed_error" degradation reason.
type immediateErrEmbedder struct{}

func (immediateErrEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, errors.New("embedder unavailable: hard error")
}
func (e immediateErrEmbedder) EmbedWithModel(ctx context.Context, text string) ([]float32, string, error) {
	v, err := e.Embed(ctx, text)
	return v, e.Name(), err
}
func (immediateErrEmbedder) Name() string    { return "immediate-err-test" }
func (immediateErrEmbedder) Dimensions() int { return 384 }

var _ embed.Client = immediateErrEmbedder{}

// newHandleModePoolWithFailingEmbed builds an EnginePool whose embedder always
// returns a hard error, so every recall call degrades. Suitable for testing
// the handle-mode degradation-reason path.
func newHandleModePoolWithFailingEmbed(t *testing.T) *EnginePool {
	t.Helper()
	return NewEnginePool(func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, noopBackend{}, immediateErrEmbedder{}, project,
			"", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	})
}

// TestHandleMode_DegradedReason_IsEmbedError verifies that when a hard embed
// error triggers degradation in handle mode, the MCP response body contains
// reason="embed_error" — not the previously hardcoded "embed_timeout" (#989).
func TestHandleMode_DegradedReason_IsEmbedError(t *testing.T) {
	pool := newHandleModePoolWithFailingEmbed(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"query":   "anything",
		"mode":    "handle",
	}
	cfg := testConfig()

	res, err := handleMemoryRecall(context.Background(), pool, req, cfg)
	require.NoError(t, err)
	require.NotNil(t, res)

	text, ok := res.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(text.Text), &body))

	degraded, ok := body["degraded"].(map[string]any)
	require.True(t, ok, "degraded field must be a map")
	require.Equal(t, true, degraded["embed"],
		"degraded.embed must be true when embedder fails")
	require.Equal(t, "embed_error", degraded["reason"],
		"handle-mode degraded.reason must be 'embed_error' on hard embed failure, not hardcoded 'embed_timeout' (#989)")
}

// TestHandleMode_DegradedReason_Nil_WhenNoDegrade verifies that when embed
// succeeds in handle mode, the degraded field shows embed=false with no reason.
func TestHandleMode_DegradedReason_Nil_WhenNoDegrade(t *testing.T) {
	pool := newTestNoopPool(t) // noopEmbedder succeeds
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"query":   "anything",
		"mode":    "handle",
	}
	cfg := testConfig()

	res, err := handleMemoryRecall(context.Background(), pool, req, cfg)
	require.NoError(t, err)
	require.NotNil(t, res)

	text, ok := res.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(text.Text), &body))

	degraded, ok := body["degraded"].(map[string]any)
	require.True(t, ok, "degraded field must be present")
	require.Equal(t, false, degraded["embed"],
		"degraded.embed must be false when embed succeeds")
	_, hasReason := degraded["reason"]
	require.False(t, hasReason,
		"degraded.reason must not be present when not degraded (#634/#989)")
}
