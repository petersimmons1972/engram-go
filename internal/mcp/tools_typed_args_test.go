package mcp

// Tests for float-typed optional args on admin MCP tools.

import (
	"context"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// memoryConnectionCaptureBackend captures the last relationship passed to StoreRelationship.
type memoryConnectionCaptureBackend struct {
	noopBackend
	capturedRel types.Relationship
	captured    bool
}

func (b *memoryConnectionCaptureBackend) StoreRelationship(_ context.Context, rel *types.Relationship) error {
	b.capturedRel = *rel
	b.captured = true
	return nil
}

type memoryConnectionHandler func(context.Context, *EnginePool, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)

func TestHandleMemoryConnectStrengthSupportsCoercion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		handler memoryConnectionHandler
	}{
		{"memory_connect", handleMemoryConnect},
		{"memory_adopt", handleMemoryAdopt},
	}
	values := []struct {
		name  string
		value any
		want  float64
	}{
		{"native float64", 0.73, 0.73},
		{"JSON string", "0.73", 0.73},
		{"uncoercible garbage", "na", 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, v := range values {
				t.Run(v.name, func(t *testing.T) {
					backend := &memoryConnectionCaptureBackend{}
					pool := newPoolWithBackend(t, backend)
					req := mcpgo.CallToolRequest{}
					req.Params.Arguments = map[string]any{
						"source_id": "src-a",
						"target_id": "dst-a",
						"strength":  v.value,
					}

					result, err := tt.handler(context.Background(), pool, req)
					require.NoError(t, err)
					require.NotNil(t, result)
					require.False(t, result.IsError)
					require.True(t, backend.captured)
					require.InDelta(t, v.want, backend.capturedRel.Strength, 0.0001)
				})
			}
		})
	}
}
