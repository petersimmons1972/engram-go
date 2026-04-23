package mcp

import (
	"context"
	"encoding/json"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// newTestPool builds an EnginePool backed by a noopBackend + noopEmbedder.
// It is an alias for newTestNoopPool, named to match the test spec.
func newTestPool(t *testing.T) *EnginePool {
	t.Helper()
	return newTestNoopPool(t)
}

// resultText returns the text from the first content item of a non-nil result.
func resultText(t *testing.T, res *mcpgo.CallToolResult) string {
	t.Helper()
	require.NotNil(t, res)
	require.NotEmpty(t, res.Content)
	text, ok := res.Content[0].(mcpgo.TextContent)
	require.True(t, ok, "expected TextContent, got %T", res.Content[0])
	return text.Text
}

func TestHandleMemoryDeleteProject_MissingProject(t *testing.T) {
	pool := newTestPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, err := handleMemoryDeleteProject(context.Background(), pool, req)
	require.NoError(t, err)
	require.True(t, result.IsError, "expected error result for missing project")
}

func TestHandleMemoryDeleteProject_Empty(t *testing.T) {
	pool := newTestPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{"project": "nonexistent-lme-project-xyz"}
	result, err := handleMemoryDeleteProject(context.Background(), pool, req)
	require.NoError(t, err)
	require.False(t, result.IsError)
	text := resultText(t, result)
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &out))
	require.Contains(t, text, `"deleted"`)
}
