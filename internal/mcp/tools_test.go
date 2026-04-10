package mcp_test

import (
	"context"
	"testing"

	internalmcp "github.com/petersimmons1972/engram/internal/mcp"
	"github.com/stretchr/testify/require"
)

func TestEnginePool_GetOrCreate_SameProject_SameInstance(t *testing.T) {
	calls := 0
	pool := internalmcp.NewEnginePool(func(ctx context.Context, project string) (*internalmcp.EngineHandle, error) {
		calls++
		return &internalmcp.EngineHandle{}, nil
	})

	ctx := context.Background()
	h1, err := pool.Get(ctx, "proj-a")
	require.NoError(t, err)
	h2, err := pool.Get(ctx, "proj-a")
	require.NoError(t, err)

	require.Same(t, h1, h2)
	require.Equal(t, 1, calls)
}
