package mcp

import (
	"context"
	"encoding/json"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

func TestGetFloat_CoercesTypedValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		arg  any
		want float64
	}{
		{name: "float64", arg: 0.75, want: 0.75},
		{name: "int", arg: 2, want: 2.0},
		{name: "uint", arg: uint(3), want: 3.0},
		{name: "float32", arg: float32(0.5), want: 0.5},
		{name: "json.Number", arg: json.Number("0.25"), want: 0.25},
		{name: "numeric string", arg: "0.85", want: 0.85},
		{name: "garbage string falls back", arg: "na", want: 1.0},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := getFloat(map[string]any{"strength": tc.arg}, "strength", 1.0)
			require.Equal(t, tc.want, got)
		})
	}
}

type relationshipCaptureBackend struct {
	noopBackend
	relationships []*types.Relationship
}

func (b *relationshipCaptureBackend) StoreRelationship(_ context.Context, rel *types.Relationship) error {
	copyRel := *rel
	b.relationships = append(b.relationships, &copyRel)
	return nil
}

func newRelationshipCapturePool(t *testing.T, backend *relationshipCaptureBackend) *EnginePool {
	t.Helper()

	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, backend, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}

	return NewEnginePool(factory)
}

func TestMemoryRelationshipHandlers_CoerceNumericStringStrength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		handler func(context.Context, *EnginePool, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)
	}{
		{name: "memory_adopt", handler: handleMemoryAdopt},
		{name: "memory_connect", handler: handleMemoryConnect},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backend := &relationshipCaptureBackend{}
			pool := newRelationshipCapturePool(t, backend)

			req := mcpgo.CallToolRequest{}
			req.Params.Arguments = map[string]any{
				"project":       "test",
				"source_id":     "src-001",
				"target_id":     "dst-001",
				"relation_type": types.RelTypeRelatesTo,
				"strength":      "0.85",
			}

			_, err := tc.handler(context.Background(), pool, req)
			require.NoError(t, err)
			require.Len(t, backend.relationships, 1)
			require.InDelta(t, 0.85, backend.relationships[0].Strength, 0.0001)
		})
	}
}

func TestMemoryRelationshipHandlers_RejectNonFiniteStringStrength(t *testing.T) {
	t.Parallel()

	handlers := []struct {
		name    string
		handler func(context.Context, *EnginePool, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)
	}{
		{name: "memory_adopt", handler: handleMemoryAdopt},
		{name: "memory_connect", handler: handleMemoryConnect},
	}

	badStrengths := []string{"NaN", "Inf", "-Inf"}

	for _, handlerTC := range handlers {
		handlerTC := handlerTC
		t.Run(handlerTC.name, func(t *testing.T) {
			t.Parallel()

			for _, strength := range badStrengths {
				strength := strength
				t.Run(strength, func(t *testing.T) {
					t.Parallel()

					backend := &relationshipCaptureBackend{}
					pool := newRelationshipCapturePool(t, backend)

					req := mcpgo.CallToolRequest{}
					req.Params.Arguments = map[string]any{
						"project":       "test",
						"source_id":     "src-001",
						"target_id":     "dst-001",
						"relation_type": types.RelTypeRelatesTo,
						"strength":      strength,
					}

					_, err := handlerTC.handler(context.Background(), pool, req)
					require.Error(t, err)
					require.Contains(t, err.Error(), "strength must be between 0 and 1")
					require.Empty(t, backend.relationships)
				})
			}
		})
	}
}
