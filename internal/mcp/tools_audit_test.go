package mcp

import (
	"context"
	"encoding/json"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// auditHandlerRequest builds a CallToolRequest for audit tool tests.
func auditHandlerRequest(args map[string]any) mcpgo.CallToolRequest {
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = args
	return req
}

// extractAuditResult unwraps a tool result back to a map for assertions.
func extractAuditResult(t *testing.T, result *mcpgo.CallToolResult) map[string]any {
	t.Helper()
	if result == nil || len(result.Content) == 0 {
		t.Fatal("extractAuditResult: nil or empty result")
	}
	text, ok := result.Content[0].(mcpgo.TextContent)
	if !ok {
		t.Fatalf("extractAuditResult: content[0] is not TextContent")
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(text.Text), &m); err != nil {
		t.Fatalf("extractAuditResult: unmarshal failed: %v\ntext: %s", err, text.Text)
	}
	return m
}

// TestAuditTools_NoPgPool verifies all audit tool handlers return a clear error
// when PgPool is nil (not configured).
func TestAuditTools_NoPgPool(t *testing.T) {
	pool := NewEnginePool(func(_ context.Context, project string) (*EngineHandle, error) {
		return nil, nil
	})
	cfg := Config{PgPool: nil}
	ctx := context.Background()

	tests := []struct {
		name    string
		handler func(context.Context, *EnginePool, mcpgo.CallToolRequest, Config) (*mcpgo.CallToolResult, error)
		args    map[string]any
	}{
		{
			name:    "add_query",
			handler: handleMemoryAuditAddQuery,
			args:    map[string]any{"project": "test", "query": "q"},
		},
		{
			name:    "list_queries",
			handler: handleMemoryAuditListQueries,
			args:    map[string]any{"project": "test"},
		},
		{
			name:    "deactivate_query",
			handler: handleMemoryAuditDeactivateQuery,
			args:    map[string]any{"query_id": "qid"},
		},
		{
			name:    "run",
			handler: handleMemoryAuditRun,
			args:    map[string]any{"project": "test"},
		},
		{
			name:    "compare",
			handler: handleMemoryAuditCompare,
			args:    map[string]any{"query_id": "qid"},
		},
		{
			name:    "weight_history",
			handler: handleMemoryWeightHistory,
			args:    map[string]any{"project": "test"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.handler(ctx, pool, auditHandlerRequest(tc.args), cfg)
			if err == nil {
				t.Errorf("handler %q: expected error when PgPool=nil, got nil", tc.name)
			}
		})
	}
}

// TestAuditTools_MissingRequiredArgs verifies that missing required arguments
// return an error rather than a panic.
func TestAuditTools_MissingRequiredArgs(t *testing.T) {
	pool := NewEnginePool(func(_ context.Context, project string) (*EngineHandle, error) {
		return nil, nil
	})
	// PgPool still nil — error path is predictable and doesn't require DB.
	cfg := Config{PgPool: nil}
	ctx := context.Background()

	t.Run("add_query_missing_project", func(t *testing.T) {
		_, err := handleMemoryAuditAddQuery(ctx, pool, auditHandlerRequest(map[string]any{
			"query": "test query",
			// project missing
		}), cfg)
		if err == nil {
			t.Error("expected error for missing project, got nil")
		}
	})

	t.Run("add_query_missing_query", func(t *testing.T) {
		_, err := handleMemoryAuditAddQuery(ctx, pool, auditHandlerRequest(map[string]any{
			"project": "test",
			// query missing
		}), cfg)
		if err == nil {
			t.Error("expected error for missing query, got nil")
		}
	})

	t.Run("deactivate_missing_query_id", func(t *testing.T) {
		_, err := handleMemoryAuditDeactivateQuery(ctx, pool, auditHandlerRequest(map[string]any{}), cfg)
		if err == nil {
			t.Error("expected error for missing query_id, got nil")
		}
	})

	t.Run("compare_missing_query_id", func(t *testing.T) {
		_, err := handleMemoryAuditCompare(ctx, pool, auditHandlerRequest(map[string]any{}), cfg)
		if err == nil {
			t.Error("expected error for missing query_id, got nil")
		}
	})

	t.Run("run_missing_project", func(t *testing.T) {
		_, err := handleMemoryAuditRun(ctx, pool, auditHandlerRequest(map[string]any{}), cfg)
		if err == nil {
			t.Error("expected error for missing project, got nil")
		}
	})

	t.Run("weight_history_missing_project", func(t *testing.T) {
		_, err := handleMemoryWeightHistory(ctx, pool, auditHandlerRequest(map[string]any{}), cfg)
		if err == nil {
			t.Error("expected error for missing project, got nil")
		}
	})
}

// TestEngineRecallerAdapter_ErrorPropagation verifies that pool errors from the
// recaller adapter are wrapped and returned (not swallowed).
func TestEngineRecallerAdapter_ErrorPropagation(t *testing.T) {
	pool := NewEnginePool(func(_ context.Context, project string) (*EngineHandle, error) {
		return nil, nil // returns nil handle — should cause a nil-deref or wrapped error
	})
	adapter := &engineRecallerAdapter{pool: pool}
	_, err := adapter.Recall(context.Background(), "testproject", "some query", 10)
	// We expect an error because the engine is nil.
	if err == nil {
		t.Error("expected error from recaller with nil engine handle, got nil")
	}
}
