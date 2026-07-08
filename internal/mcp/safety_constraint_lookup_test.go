package mcp

// Handler-level tests for the shared constraintLookupParams helper (#1282),
// exercised through handleGetConstraints, handleCheckConstraints, and
// handleVerifyBeforeActing. These three tools gate which constraints the
// safety pipeline surfaces and whether one is treated as stale (i.e. no
// longer authoritative) — a caller who deliberately widens limit or disables
// staleness via stale_after_days must not have that intent silently
// discarded by a mistyped value falling back to the (narrower) default.

import (
	"context"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

type constraintLookupHandler struct {
	name           string
	requiresAction bool
	handler        func(context.Context, *EnginePool, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)
}

func constraintLookupHandlers() []constraintLookupHandler {
	return []constraintLookupHandler{
		{name: "get_constraints", requiresAction: false, handler: handleGetConstraints},
		{name: "check_constraints", requiresAction: true, handler: handleCheckConstraints},
		{name: "verify_before_acting", requiresAction: true, handler: handleVerifyBeforeActing},
	}
}

func constraintLookupRequest(tc constraintLookupHandler, extras map[string]any) mcpgo.CallToolRequest {
	args := map[string]any{"project": "test"}
	if tc.requiresAction {
		args["proposed_action"] = "select 1"
	} else {
		args["query"] = "select 1"
	}
	for k, v := range extras {
		args[k] = v
	}
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = args
	return req
}

// TestConstraintLookupHandlers_LimitWrongType_LoudError covers #1282: a
// present-but-uncoercible "limit" must be a loud tool error, not a silent
// fallback to defaultConstraintLimit — an analogous case to
// memory_audit_compare's limit, which #1281/#1282 explicitly called out as
// load-bearing.
func TestConstraintLookupHandlers_LimitWrongType_LoudError(t *testing.T) {
	for _, tc := range constraintLookupHandlers() {
		t.Run(tc.name, func(t *testing.T) {
			pool := newTestNoopPool(t)
			result, err := tc.handler(context.Background(), pool, constraintLookupRequest(tc, map[string]any{
				"limit": []any{1, 2},
			}))
			require.NoError(t, err)
			require.NotNil(t, result)
			require.True(t, result.IsError, "expected tool error for wrong-typed limit")
			text, ok := result.Content[0].(mcpgo.TextContent)
			require.True(t, ok)
			require.Contains(t, text.Text, "limit")
		})
	}
}

// TestConstraintLookupHandlers_StaleAfterDaysWrongType_LoudError covers
// #1282: a present-but-uncoercible "stale_after_days" must be a loud tool
// error, not a silent fallback to defaultStaleAfterDays — silently defaulting
// here could mask a caller's deliberate attempt to widen or narrow the
// staleness window on a constraint-verification call.
func TestConstraintLookupHandlers_StaleAfterDaysWrongType_LoudError(t *testing.T) {
	for _, tc := range constraintLookupHandlers() {
		t.Run(tc.name, func(t *testing.T) {
			pool := newTestNoopPool(t)
			result, err := tc.handler(context.Background(), pool, constraintLookupRequest(tc, map[string]any{
				"stale_after_days": map[string]any{"a": 1},
			}))
			require.NoError(t, err)
			require.NotNil(t, result)
			require.True(t, result.IsError, "expected tool error for wrong-typed stale_after_days")
			text, ok := result.Content[0].(mcpgo.TextContent)
			require.True(t, ok)
			require.Contains(t, text.Text, "stale_after_days")
		})
	}
}

// TestConstraintLookupHandlers_EmptyArgs_HappyPath verifies the zero-value
// case: no limit/stale_after_days supplied at all applies the defaults and
// returns a non-error result (empty constraint set, since noopBackend has no
// stored memories).
func TestConstraintLookupHandlers_EmptyArgs_HappyPath(t *testing.T) {
	for _, tc := range constraintLookupHandlers() {
		t.Run(tc.name, func(t *testing.T) {
			pool := newTestNoopPool(t)
			result, err := tc.handler(context.Background(), pool, constraintLookupRequest(tc, nil))
			require.NoError(t, err)
			require.NotNil(t, result)
			require.False(t, result.IsError, "unexpected tool error: %+v", result.Content)
		})
	}
}

// TestConstraintLookupHandlers_ValidLimitAndStaleAfterDays_Honored covers the
// boundary case: valid caller-supplied limit/stale_after_days values within
// range are accepted and produce a non-error result.
func TestConstraintLookupHandlers_ValidLimitAndStaleAfterDays_Honored(t *testing.T) {
	for _, tc := range constraintLookupHandlers() {
		t.Run(tc.name, func(t *testing.T) {
			pool := newTestNoopPool(t)
			result, err := tc.handler(context.Background(), pool, constraintLookupRequest(tc, map[string]any{
				"limit":            5,
				"stale_after_days": 30,
			}))
			require.NoError(t, err)
			require.NotNil(t, result)
			require.False(t, result.IsError, "unexpected tool error: %+v", result.Content)
		})
	}
}
