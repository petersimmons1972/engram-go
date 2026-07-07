package mcp

// Tests for handleMemoryMigrateEmbedder safety guards (#spurious-reembed).
//
// Guards tested here at the MCP-handler level:
//   G1: Same-canonical-identity no-op response surfaces correctly (status unchanged)
//   G2: Same-dimension soft refusal surfaced as MCP tool error
//   G3: dry_run arg passthrough; large volume refusal surfaced as MCP tool error
//   G4: force/confirm arg passthrough verified via MigrateParams in migrateFunc

import (
	"context"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/stretchr/testify/require"
)

func requireToolErrorText(t *testing.T, result *mcpgo.CallToolResult) string {
	t.Helper()
	require.NotNil(t, result)
	require.True(t, result.IsError, "expected MCP tool error result")
	require.NotEmpty(t, result.Content)
	text, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok, "expected TextContent, got %T", result.Content[0])
	return text.Text
}

// makeMigrateRequestFull builds a CallToolRequest with the full set of new args.
func makeMigrateRequestFull(project, newModel string, extras map[string]any) mcpgo.CallToolRequest {
	var req mcpgo.CallToolRequest
	args := map[string]any{
		"project":   project,
		"new_model": newModel,
	}
	for k, v := range extras {
		args[k] = v
	}
	req.Params.Arguments = args
	return req
}

// noopProbe is a test embedProbe that returns a dimEmbedder without hitting Ollama.
// Used in tests that have stored dims and need to bypass the real probe.
func noopProbeWithDims(dims int) func(ctx context.Context, baseURL, model string) (embed.Client, error) {
	return func(_ context.Context, _, _ string) (embed.Client, error) {
		return dimEmbedder{dims: dims}, nil
	}
}

// ── G1: MCP no-op on same-canonical-identity ─────────────────────────────────

// TestMCPMigrateSameCanonicalIdentityIsNoop verifies that when migrateFunc
// returns status="identity unchanged" and chunks_nulled=0, the handler surfaces
// that response as a non-error result and does NOT fire onPostMigrate.
func TestMCPMigrateSameCanonicalIdentityIsNoop(t *testing.T) {
	// newDimPool with stored=384 so embedProbe is needed to bypass real Ollama.
	pool := newDimPool(t, "1024", 1024)
	req := makeMigrateRequestFull("proj", "bge-m3", nil)

	postMigrateFired := false
	cfg := Config{
		RouterURL: "http://ollama-test:11434",
		testHooks: &testHooks{
			embedProbe: noopProbeWithDims(1024),
			migrateFunc: func(_ context.Context, p search.MigrateParams) (map[string]any, error) {
				// Simulate identity no-op.
				return map[string]any{
					"chunks_nulled": 0,
					"status":        "identity unchanged",
					"new_model":     p.NewModel,
				}, nil
			},
			onPostMigrate: func(_ context.Context, _ string) {
				postMigrateFired = true
			},
		},
	}

	result, err := handleMemoryMigrateEmbedder(context.Background(), pool, req, cfg)
	require.NoError(t, err)
	require.NotNil(t, result)
	// status="identity unchanged" has no "error" key — must NOT be an MCP tool error.
	require.False(t, result.IsError, "identity no-op must not be an MCP tool error")
	// onPostMigrate must NOT fire on identity no-op.
	require.False(t, postMigrateFired, "onPostMigrate must not fire on identity unchanged")
}

// ── G2: MCP same-dim refusal passthrough ─────────────────────────────────────

// TestMCPMigrateSameDimRefusalPassedThrough verifies that a same-dim refusal
// from the engine (returned as result["error"]) is surfaced as a tool error,
// not a Go error.
func TestMCPMigrateSameDimRefusalPassedThrough(t *testing.T) {
	pool := newDimPool(t, "1024", 1024)
	req := makeMigrateRequestFull("proj", "other-1024-model", map[string]any{
		"force": false,
	})

	cfg := Config{
		RouterURL: "http://ollama-test:11434",
		testHooks: &testHooks{
			embedProbe: noopProbeWithDims(1024),
			migrateFunc: func(_ context.Context, _ search.MigrateParams) (map[string]any, error) {
				return map[string]any{
					"error":  "same-dim migration requires force=true",
					"status": "refused",
				}, nil
			},
		},
	}

	result, err := handleMemoryMigrateEmbedder(context.Background(), pool, req, cfg)
	require.NoError(t, err)
	require.NotNil(t, result)
	// The handler must surface the engine's soft refusal as an MCP tool error.
	require.True(t, result.IsError, "same-dim refusal must be surfaced as MCP tool error")
}

// ── G3: MCP dry_run arg passthrough ──────────────────────────────────────────

// TestMCPMigrateDryRunArgPassedToEngine verifies that dry_run=true in the MCP
// request is forwarded to MigrateEmbedder as MigrateParams.DryRun=true.
func TestMCPMigrateDryRunArgPassedToEngine(t *testing.T) {
	pool := newDimPool(t, "1024", 1024)
	req := makeMigrateRequestFull("proj", "new-model-768", map[string]any{
		"dry_run": true,
	})

	var dryRunSeen bool
	cfg := Config{
		RouterURL: "http://ollama-test:11434",
		testHooks: &testHooks{
			embedProbe: noopProbeWithDims(768),
			migrateFunc: func(_ context.Context, p search.MigrateParams) (map[string]any, error) {
				dryRunSeen = p.DryRun
				return map[string]any{
					"chunks_would_null": 999,
					"status":            "dry_run",
					"new_model":         p.NewModel,
				}, nil
			},
		},
	}

	result, err := handleMemoryMigrateEmbedder(context.Background(), pool, req, cfg)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)
	require.True(t, dryRunSeen, "dry_run=true must reach migrateFunc as DryRun=true")
}

// TestMCPMigrateLargeVolumeRefusalPassedThrough: large volume refusal from
// the engine is surfaced as a tool error.
func TestMCPMigrateLargeVolumeRefusalPassedThrough(t *testing.T) {
	// No stored dims — pre-flight skips; this test is about volume-guard passthrough.
	pool := newDimPool(t, "", 1024)
	req := makeMigrateRequestFull("proj", "new-model-different", map[string]any{
		"confirm": false,
	})

	cfg := Config{
		RouterURL: "http://ollama-test:11434",
		testHooks: &testHooks{
			embedProbe: noopProbeWithDims(1024),
			migrateFunc: func(_ context.Context, _ search.MigrateParams) (map[string]any, error) {
				return map[string]any{
					"error":             "1001 chunks would be nulled; pass confirm=true to proceed",
					"chunks_would_null": 1001,
					"status":            "refused",
				}, nil
			},
		},
	}

	result, err := handleMemoryMigrateEmbedder(context.Background(), pool, req, cfg)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.IsError, "volume refusal must be surfaced as MCP tool error")
}

// ── G3: confirm arg passthrough ───────────────────────────────────────────────

// TestMCPMigrateConfirmArgPassedToEngine verifies confirm=true is forwarded.
func TestMCPMigrateConfirmArgPassedToEngine(t *testing.T) {
	// No stored dims — pre-flight skips; this test is about confirm-arg passthrough.
	pool := newDimPool(t, "", 1024)
	req := makeMigrateRequestFull("proj", "new-model-different", map[string]any{
		"confirm": true,
	})

	var confirmSeen bool
	cfg := Config{
		RouterURL: "http://ollama-test:11434",
		testHooks: &testHooks{
			embedProbe: noopProbeWithDims(1024),
			migrateFunc: func(_ context.Context, p search.MigrateParams) (map[string]any, error) {
				confirmSeen = p.Confirm
				return map[string]any{
					"chunks_nulled": 5,
					"status":        "migration queued",
					"new_model":     p.NewModel,
				}, nil
			},
			onPostMigrate: func(_ context.Context, _ string) {},
		},
	}

	_, err := handleMemoryMigrateEmbedder(context.Background(), pool, req, cfg)
	require.NoError(t, err)
	require.True(t, confirmSeen, "confirm=true must reach migrateFunc as Confirm=true")
}

// ── G4: force arg passthrough ─────────────────────────────────────────────────

// TestMCPMigrateForceArgPassedToEngine verifies force=true is forwarded.
func TestMCPMigrateForceArgPassedToEngine(t *testing.T) {
	// No stored dims — pre-flight skips; this test is about force-arg passthrough.
	pool := newDimPool(t, "", 1024)
	req := makeMigrateRequestFull("proj", "new-model-different", map[string]any{
		"force":   true,
		"confirm": true,
	})

	var forceSeen bool
	cfg := Config{
		RouterURL: "http://ollama-test:11434",
		testHooks: &testHooks{
			embedProbe: noopProbeWithDims(1024),
			migrateFunc: func(_ context.Context, p search.MigrateParams) (map[string]any, error) {
				forceSeen = p.Force
				return map[string]any{
					"chunks_nulled": 5,
					"status":        "migration queued",
					"new_model":     p.NewModel,
				}, nil
			},
			onPostMigrate: func(_ context.Context, _ string) {},
		},
	}

	_, err := handleMemoryMigrateEmbedder(context.Background(), pool, req, cfg)
	require.NoError(t, err)
	require.True(t, forceSeen, "force=true must reach migrateFunc as Force=true")
}

func TestMCPMigrateLoadBearingBoolWrongTypeReturnsLoudError(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{name: "force", key: "force"},
		{name: "dry_run", key: "dry_run"},
		{name: "confirm", key: "confirm"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var poolGets int
			pool := NewEnginePool(func(_ context.Context, _ string) (*EngineHandle, error) {
				poolGets++
				return nil, context.Canceled
			})
			req := makeMigrateRequestFull("proj", "new-model", map[string]any{
				tc.key: []any{"oops"},
			})

			result, err := handleMemoryMigrateEmbedder(context.Background(), pool, req, Config{
				RouterURL: "http://ollama-test:11434",
			})
			require.NoError(t, err)
			require.Zero(t, poolGets, "wrong-type %s must be rejected before pool access", tc.key)
			require.Contains(t, requireToolErrorText(t, result), tc.key)
		})
	}
}
