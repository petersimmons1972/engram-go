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

func TestMemoryRelationshipHandlers_AcceptBoundaryStrength(t *testing.T) {
	handlers := []struct {
		name    string
		handler func(context.Context, *EnginePool, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)
	}{
		{name: "memory_adopt", handler: handleMemoryAdopt},
		{name: "memory_connect", handler: handleMemoryConnect},
	}

	for _, handlerTC := range handlers {
		t.Run(handlerTC.name, func(t *testing.T) {
			for _, strength := range []float64{0.0, 1.0} {
				t.Run("strength_boundary", func(t *testing.T) {
					backend := &relationshipCaptureBackend{}
					pool := newRelationshipCapturePool(t, backend)

					req := relationshipRequest(strength)

					_, err := handlerTC.handler(context.Background(), pool, req)
					require.NoError(t, err)
					require.Len(t, backend.relationships, 1)
					require.InDelta(t, strength, backend.relationships[0].Strength, 0.0001)
				})
			}
		})
	}
}

func TestMemoryRelationshipHandlers_RejectOutOfRangeFiniteStrength(t *testing.T) {
	handlers := []struct {
		name    string
		handler func(context.Context, *EnginePool, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)
	}{
		{name: "memory_adopt", handler: handleMemoryAdopt},
		{name: "memory_connect", handler: handleMemoryConnect},
	}

	badStrengths := []float64{1.5, -0.5, 1.0000001, -1e-9}
	for _, handlerTC := range handlers {
		t.Run(handlerTC.name, func(t *testing.T) {
			for _, strength := range badStrengths {
				t.Run("out_of_range", func(t *testing.T) {
					backend := &relationshipCaptureBackend{}
					pool := newRelationshipCapturePool(t, backend)

					_, err := handlerTC.handler(context.Background(), pool, relationshipRequest(strength))
					require.Error(t, err)
					require.Contains(t, err.Error(), "strength must be between 0 and 1")
					require.Empty(t, backend.relationships)
				})
			}
		})
	}
}

func TestMemoryRelationshipHandlers_RejectNonFiniteNumericStrength(t *testing.T) {
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

					_, err := handlerTC.handler(context.Background(), pool, relationshipRequest(strength.val))
					require.Error(t, err)
					require.Contains(t, err.Error(), "strength must be between 0 and 1")
					require.Empty(t, backend.relationships)
				})
			}
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

	for _, handlerTC := range handlers {
		t.Run(handlerTC.name, func(t *testing.T) {
			for _, strength := range []string{"NaN", "Inf", "+Inf", "-Inf"} {
				t.Run(strength, func(t *testing.T) {
					backend := &relationshipCaptureBackend{}
					pool := newRelationshipCapturePool(t, backend)

					_, err := handlerTC.handler(context.Background(), pool, relationshipRequest(strength))
					require.Error(t, err)
					require.Contains(t, err.Error(), "strength must be between 0 and 1")
					require.Empty(t, backend.relationships)
				})
			}
		})
	}
}

// TestMemoryRelationshipHandlers_RejectUncoercibleNonStringStrength covers
// #1282: a present-but-uncoercible non-string "strength" (bool, array,
// object) must be a loud tool error, not a silent fallback to the default
// 1.0 — before this fix, relationshipStrength routed non-string values
// through getFloat, which discarded the caller's bad value and stored 1.0
// as if the caller had asked for it.
func TestMemoryRelationshipHandlers_RejectUncoercibleNonStringStrength(t *testing.T) {
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
		{name: "bool", val: true},
		{name: "array", val: []any{1, 2}},
		{name: "object", val: map[string]any{"a": 1}},
	}

	for _, handlerTC := range handlers {
		t.Run(handlerTC.name, func(t *testing.T) {
			for _, strength := range badStrengths {
				t.Run(strength.name, func(t *testing.T) {
					backend := &relationshipCaptureBackend{}
					pool := newRelationshipCapturePool(t, backend)

					_, err := handlerTC.handler(context.Background(), pool, relationshipRequest(strength.val))
					require.Error(t, err)
					require.Contains(t, err.Error(), "strength must be numeric")
					require.Empty(t, backend.relationships, "a bad strength must not silently default to 1.0 and persist")
				})
			}
		})
	}
}

func relationshipRequest(strength any) mcpgo.CallToolRequest {
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":       "test",
		"source_id":     "src-001",
		"target_id":     "dst-001",
		"relation_type": types.RelTypeRelatesTo,
		"strength":      strength,
	}
	return req
}
