package mcp

import (
	"context"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestHandleMemoryProjects_ScopeToCallerOnly(t *testing.T) {
	pool := newIssue629Pool(t)

	res, err := handleMemoryProjects(context.Background(), pool, mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{Arguments: map[string]any{"project": "proj"}},
	})
	require.NoError(t, err)

	out := decodeToolResult(t, res)
	require.Equal(t, float64(1), out["count"])

	projects, ok := out["projects"].([]any)
	require.True(t, ok)
	require.Len(t, projects, 1)

	projectInfo, ok := projects[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "proj", projectInfo["project"])
	require.Equal(t, float64(3), projectInfo["count"])
}
