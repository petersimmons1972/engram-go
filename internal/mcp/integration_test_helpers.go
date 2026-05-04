package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// CallHandleMemoryStoreForTest invokes handleMemoryStore and returns the
// raw CallToolResult. Used by integration tests.
func CallHandleMemoryStoreForTest(
	ctx context.Context,
	pool *EnginePool,
	req mcpgo.CallToolRequest,
	cfg Config,
) (*mcpgo.CallToolResult, error) {
	return handleMemoryStore(ctx, pool, req, cfg)
}

// CallHandleMemoryRecallFullForTest invokes handleMemoryRecall with full
// argument control and returns the decoded output map. Used by integration tests.
func CallHandleMemoryRecallFullForTest(
	ctx context.Context,
	pool *EnginePool,
	project string,
	query string,
	args map[string]any,
	cfg Config,
) (map[string]any, error) {
	req := mcpgo.CallToolRequest{}
	merged := map[string]any{
		"project": project,
		"query":   query,
		"top_k":   float64(10),
		"detail":  "full",
	}
	for k, v := range args {
		merged[k] = v
	}
	req.Params.Arguments = merged

	result, err := handleMemoryRecall(ctx, pool, req, cfg)
	if err != nil {
		return nil, err
	}

	if len(result.Content) == 0 {
		return nil, fmt.Errorf("tool result has no content items")
	}
	tc, ok := result.Content[0].(mcpgo.TextContent)
	if !ok {
		return nil, fmt.Errorf("expected TextContent, got %T", result.Content[0])
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		return nil, fmt.Errorf("decode tool result JSON: %w", err)
	}
	return out, nil
}
