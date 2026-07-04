package mcp

import (
	"context"
	"math"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

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
	handlers := []struct {
		name    string
		handler func(context.Context, *EnginePool, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)
	}{
		{name: "memory_adopt", handler: handleMemoryAdopt},
		{name: "memory_connect", handler: handleMemoryConnect},
	}

	for _, tc := range handlers {
		t.Run(tc.name, func(t *testing.T) {
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
	handlers := []struct {
		name    string
		handler func(context.Context, *EnginePool, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)
	}{
		{name: "memory_adopt", handler: handleMemoryAdopt},
		{name: "memory_connect", handler: handleMemoryConnect},
	}

	badStrengths := []struct {
		name string
		val  any
	}{
		{name: "NaN", val: math.NaN()},
		{name: "Inf", val: math.Inf(1)},
		{name: "-Inf", val: math.Inf(-1)},
	}

	for _, handlerTC := range handlers {
		t.Run(handlerTC.name, func(t *testing.T) {
			for _, strength := range badStrengths {
				t.Run(strength.name, func(t *testing.T) {
					backend := &relationshipCaptureBackend{}
					pool := newRelationshipCapturePool(t, backend)

					req := mcpgo.CallToolRequest{}
					req.Params.Arguments = map[string]any{
						"project":       "test",
						"source_id":     "src-001",
						"target_id":     "dst-001",
						"relation_type": types.RelTypeRelatesTo,
						"strength":      strength.val,
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
