package mcp

import (
	"context"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestMemorySleepLoadBearingParamsWrongTypeReturnLoudError(t *testing.T) {
	tests := []string{
		"limit",
		"llm_max_calls",
		"contradiction_limit",
		"llm_contradiction_detection",
		"auto_supersede",
	}

	for _, key := range tests {
		t.Run(key, func(t *testing.T) {
			req := mcpgo.CallToolRequest{}
			req.Params.Arguments = map[string]any{
				"project": "default",
				key:       []any{"wrong-type"},
			}

			_, err := handleMemorySleep(context.Background(), nil, req, Config{})
			require.Error(t, err)
			require.Contains(t, err.Error(), key)
		})
	}
}
