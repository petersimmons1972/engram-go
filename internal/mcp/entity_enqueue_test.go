package mcp

// Tests for extractResultID.
//
// extractResultID is unexported so these tests live in package mcp (same
// package, no _test suffix in the declaration).

import (
	"encoding/json"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// textResult builds a CallToolResult whose first Content element is
// TextContent with the given JSON payload.
func textResult(t *testing.T, payload map[string]any) *mcpgo.CallToolResult {
	t.Helper()
	b, err := json.Marshal(payload)
	require.NoError(t, err)
	return mcpgo.NewToolResultText(string(b))
}

// ── extractResultID unit tests ────────────────────────────────────────────────

func TestExtractResultID_NilResult(t *testing.T) {
	id, ok := extractResultID(nil)
	require.False(t, ok, "nil result must return false")
	require.Empty(t, id)
}

func TestExtractResultID_EmptyContent(t *testing.T) {
	result := &mcpgo.CallToolResult{}
	id, ok := extractResultID(result)
	require.False(t, ok, "empty Content must return false")
	require.Empty(t, id)
}

func TestExtractResultID_ContentNotTextContent(t *testing.T) {
	// ImageContent satisfies the Content interface but is not TextContent.
	result := &mcpgo.CallToolResult{
		Content: []mcpgo.Content{
			mcpgo.ImageContent{Type: "image", Data: "abc", MIMEType: "image/png"},
		},
	}
	id, ok := extractResultID(result)
	require.False(t, ok, "non-TextContent must return false")
	require.Empty(t, id)
}

func TestExtractResultID_InvalidJSON(t *testing.T) {
	result := &mcpgo.CallToolResult{
		Content: []mcpgo.Content{
			mcpgo.TextContent{Type: "text", Text: "not-json"},
		},
	}
	id, ok := extractResultID(result)
	require.False(t, ok, "invalid JSON must return false")
	require.Empty(t, id)
}

func TestExtractResultID_NoIDField(t *testing.T) {
	result := textResult(t, map[string]any{"status": "ok"})
	id, ok := extractResultID(result)
	require.False(t, ok, "JSON without id field must return false")
	require.Empty(t, id)
}

func TestExtractResultID_EmptyStringID(t *testing.T) {
	result := textResult(t, map[string]any{"id": ""})
	id, ok := extractResultID(result)
	require.False(t, ok, `id="" must return false`)
	require.Empty(t, id)
}

func TestExtractResultID_NonStringID(t *testing.T) {
	// id is a number — type assertion to string should fail.
	result := textResult(t, map[string]any{"id": 42})
	id, ok := extractResultID(result)
	require.False(t, ok, "non-string id must return false")
	require.Empty(t, id)
}

func TestExtractResultID_HappyPath(t *testing.T) {
	const want = "mem_abc123"
	result := textResult(t, map[string]any{"id": want, "status": "stored"})
	id, ok := extractResultID(result)
	require.True(t, ok, "valid id must return true")
	require.Equal(t, want, id)
}
