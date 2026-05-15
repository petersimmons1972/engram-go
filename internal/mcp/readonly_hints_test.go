package mcp

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// TestReadOnlyToolAnnotations verifies that every tool listed in
// readOnlyToolNames() carries ReadOnlyHint=true on its MCP annotation, and
// that representative mutating tools carry ReadOnlyHint=false.
//
// Why this exists: clients (notably Claude Code's plan mode) gate tool calls
// on the ReadOnlyHint annotation. If a recall tool ships without the hint,
// the call is silently rejected client-side with no permission prompt — the
// failure mode that motivated this work. This test prevents the regression
// recurring when new tools are added.
func TestReadOnlyToolAnnotations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := &Server{
		cfg:             Config{},
		uploads:         make(map[string]*uploadSession),
		toolAnnotations: make(map[string]mcpgo.ToolAnnotation),
	}
	srv.mcp = server.NewMCPServer("engram-test", "0.0.0",
		server.WithToolCapabilities(true),
		server.WithHooks(&server.Hooks{}),
	)
	srv.registerTools()

	annotations := srv.RegisteredToolAnnotations()
	expectedReadOnly := readOnlyToolNames()

	// Every read-only tool must have been registered AND carry ReadOnlyHint=true.
	for name := range expectedReadOnly {
		ann, ok := annotations[name]
		if !ok {
			t.Errorf("tool %q is in readOnlyToolNames() but was not registered (typo or removed?)", name)
			continue
		}
		if ann.ReadOnlyHint == nil || !*ann.ReadOnlyHint {
			t.Errorf("tool %q: expected ReadOnlyHint=true, got %v", name, ann.ReadOnlyHint)
		}
	}

	// Representative mutating tools must NOT have ReadOnlyHint=true. Asserting on
	// a small representative set rather than the inverse of readOnlyToolNames so
	// the test stays useful even if new mutating tools are added without updating
	// the read-only set.
	mutators := []string{
		"memory_store",
		"memory_correct",
		"memory_forget",
		// These mutating tools are hidden from tools/list but remain registered.
		// The hidden status doesn't affect the ReadOnlyHint check.
		"memory_delete_project",
		"memory_consolidate",
		"memory_feedback",
	}
	for _, name := range mutators {
		ann, ok := annotations[name]
		if !ok {
			t.Errorf("expected mutator tool %q to be registered", name)
			continue
		}
		if ann.ReadOnlyHint != nil && *ann.ReadOnlyHint {
			t.Errorf("tool %q is a mutator but carries ReadOnlyHint=true — it will bypass plan-mode gating", name)
		}
	}

	// Unused ctx cancel guard (linter quiet).
	_ = ctx
}

// TestHiddenToolsAbsentFromList verifies that tools in hiddenToolNames() are
// registered (callable via tools/call) but do not appear in the tools/list
// response served to MCP clients. Hidden tools must stay absent so AI clients
// don't load their descriptions into context unnecessarily.
//
// Correctness guarantee: this test fires a real tools/list JSON-RPC request
// through srv.mcp.HandleMessage so the registered AfterListTools hook runs.
// If the AddAfterListTools call in registerTools() is deleted, the hidden tools
// will reappear in the response and this test will fail.
func TestHiddenToolsAbsentFromList(t *testing.T) {
	srv := &Server{
		cfg:             Config{},
		uploads:         make(map[string]*uploadSession),
		toolAnnotations: make(map[string]mcpgo.ToolAnnotation),
	}
	srv.mcp = server.NewMCPServer("engram-test", "0.0.0",
		server.WithToolCapabilities(true),
		server.WithHooks(&server.Hooks{}),
	)
	srv.registerTools() // also wires the AfterListTools hook

	// All hidden tools must still be registered (callable via tools/call).
	annotations := srv.RegisteredToolAnnotations()
	for name := range hiddenToolNames() {
		if _, ok := annotations[name]; !ok {
			t.Errorf("hidden tool %q is not registered — it must remain callable via tools/call", name)
		}
	}

	// Fire a real tools/list request through the mcp-go machinery so the
	// AfterListTools hook executes on the actual result set. This is the
	// load-bearing assertion: if AddAfterListTools were removed from
	// registerTools(), the hidden tools would appear here and the test fails.
	resp := srv.mcp.HandleMessage(context.Background(), json.RawMessage(`{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "tools/list"
	}`))

	jsonResp, ok := resp.(mcpgo.JSONRPCResponse)
	if !ok {
		t.Fatalf("tools/list response is not a JSONRPCResponse: %T", resp)
	}
	result, ok := jsonResp.Result.(mcpgo.ListToolsResult)
	if !ok {
		t.Fatalf("tools/list result is not a ListToolsResult: %T", jsonResp.Result)
	}

	// Build the set of visible tool names from the hook-filtered response.
	visibleNames := make(map[string]bool, len(result.Tools))
	for _, tool := range result.Tools {
		visibleNames[tool.Name] = true
	}

	// No hidden tool may appear in the hook-filtered visible set.
	for name := range hiddenToolNames() {
		if visibleNames[name] {
			t.Errorf("hidden tool %q appears in tools/list response — AfterListTools hook failed to suppress it", name)
		}
	}
}

// TestReadOnlyHiddenOverlapIsStable guards the known intersection of
// readOnlyToolNames() and hiddenToolNames(). Both maps are correct in
// isolation; this test catches accidental divergence in the overlap and
// forces a conscious decision when the intersection changes.
func TestReadOnlyHiddenOverlapIsStable(t *testing.T) {
	readOnly := readOnlyToolNames()
	hidden := hiddenToolNames()

	var overlap []string
	for name := range readOnly {
		if hidden[name] {
			overlap = append(overlap, name)
		}
	}
	sort.Strings(overlap)

	// Known stable overlap as of feat/mcp-slim-profile.
	// When this list changes, update it deliberately — don't just fix the test.
	want := []string{
		"memory_aggregate",
		"memory_audit_compare",
		"memory_audit_list_queries",
		"memory_audit_run",
		"memory_diagnose",
		"memory_embedding_eval",
		"memory_episode_list",
		"memory_episode_recall",
		"memory_expand",
		"memory_export_all",
		"memory_ingest_status",
		"memory_models",
		"memory_verify",
		"memory_weight_history",
	}

	if !reflect.DeepEqual(overlap, want) {
		t.Errorf("readOnly∩hidden overlap changed.\ngot:  %v\nwant: %v\n\nIf intentional, update the want list above.", overlap, want)
	}
}
