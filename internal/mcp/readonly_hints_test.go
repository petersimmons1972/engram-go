package mcp

import (
	"context"
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
