package mcp

// LME Phase 3 — MCP integration tests for evidence_first_pack recall argument.
//
// Verifies that:
//   1. evidence_first_pack=true reorders results so exact-signal hits come first.
//   2. evidence_first_pack=false (default) returns results in original score order.
//   3. The ENGRAM_EVIDENCE_FIRST_PACK env var enables the feature server-wide.

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// evidencePackBackend returns two memories:
//   - "mem-miss": generic content, no URL match
//   - "mem-hit":  contains the query URL https://example.com/target
//
// The miss is returned first by the FTS backend (higher mock score) so we can
// verify that evidence_first_pack=true moves the hit in front.
type evidencePackBackend struct {
	noopBackend
}

func (b *evidencePackBackend) FTSSearch(_ context.Context, project, _ string, _ int, _, _ *time.Time) ([]db.FTSResult, error) {
	now := time.Now().UTC()
	miss := db.FTSResult{
		Memory: &types.Memory{
			ID:        "mem-miss",
			Project:   project,
			Content:   "generic memory about groceries and weather",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Score: 2.0, // higher score → comes first without evidence-first ordering
	}
	hit := db.FTSResult{
		Memory: &types.Memory{
			ID:        "mem-hit",
			Project:   project,
			Content:   "I visited https://example.com/target last Tuesday for the demo",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Score: 1.0, // lower score → comes second without evidence-first ordering
	}
	return []db.FTSResult{miss, hit}, nil
}

func newEvidencePackPool(t *testing.T) *EnginePool {
	t.Helper()
	backend := &evidencePackBackend{}
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, backend, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	return NewEnginePool(factory)
}

func parseRecallResults(t *testing.T, res *mcpgo.CallToolResult) []map[string]any {
	t.Helper()
	require.NotNil(t, res)
	require.False(t, res.IsError, "unexpected tool error: %+v", res.Content)
	require.NotEmpty(t, res.Content)
	text, ok := res.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(text.Text), &out))
	rawResults, ok := out["results"].([]any)
	require.True(t, ok, "results field must be a list")
	results := make([]map[string]any, 0, len(rawResults))
	for _, r := range rawResults {
		m, ok := r.(map[string]any)
		require.True(t, ok)
		results = append(results, m)
	}
	return results
}

func firstResultID(t *testing.T, results []map[string]any) string {
	t.Helper()
	require.NotEmpty(t, results, "results must not be empty")
	mem, ok := results[0]["memory"].(map[string]any)
	require.True(t, ok, "first result must have a memory field")
	id, ok := mem["id"].(string)
	require.True(t, ok, "memory must have an id field")
	return id
}

// TestEvidenceFirstPack_FlagOff_OriginalOrder verifies that without
// evidence_first_pack, results are returned in score order (miss first).
func TestEvidenceFirstPack_FlagOff_OriginalOrder(t *testing.T) {
	pool := newEvidencePackPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"query":   "visit https://example.com/target",
		"project": "test",
	}

	res, err := handleMemoryRecall(context.Background(), pool, req, testConfig())
	require.NoError(t, err)
	results := parseRecallResults(t, res)

	// Without evidence_first_pack, FTS score order is preserved: miss (score=2) first.
	id := firstResultID(t, results)
	require.Equal(t, "mem-miss", id,
		"without evidence_first_pack, highest-score result must come first")
}

// TestEvidenceFirstPack_FlagOn_HitMovedFirst verifies that with
// evidence_first_pack=true, the URL-matching result is moved to position 0.
func TestEvidenceFirstPack_FlagOn_HitMovedFirst(t *testing.T) {
	pool := newEvidencePackPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"query":               "visit https://example.com/target",
		"project":             "test",
		"evidence_first_pack": true,
	}

	res, err := handleMemoryRecall(context.Background(), pool, req, testConfig())
	require.NoError(t, err)
	results := parseRecallResults(t, res)

	// With evidence_first_pack=true, the URL-matching memory must come first.
	id := firstResultID(t, results)
	require.Equal(t, "mem-hit", id,
		"with evidence_first_pack=true, URL-matching result must be first")
}

// TestEvidenceFirstPack_EnvVar_EnablesServerWide verifies that
// ENGRAM_EVIDENCE_FIRST_PACK=true enables the feature without a per-call arg.
func TestEvidenceFirstPack_EnvVar_EnablesServerWide(t *testing.T) {
	t.Setenv("ENGRAM_EVIDENCE_FIRST_PACK", "true")

	pool := newEvidencePackPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"query":   "visit https://example.com/target",
		"project": "test",
		// no evidence_first_pack argument — env var should be sufficient
	}

	res, err := handleMemoryRecall(context.Background(), pool, req, testConfig())
	require.NoError(t, err)
	results := parseRecallResults(t, res)

	id := firstResultID(t, results)
	require.Equal(t, "mem-hit", id,
		"ENGRAM_EVIDENCE_FIRST_PACK=true must enable evidence-first ordering server-wide")

	// Cleanup: ensure the env var is actually cleared (t.Setenv does this on cleanup,
	// but verify it is visible within the test scope to validate the test itself).
	require.Equal(t, "true", strings.ToLower(os.Getenv("ENGRAM_EVIDENCE_FIRST_PACK")))
}

// TestEvidenceFirstPack_NoSignalsInQuery_OrderUnchanged verifies that when the
// query has no exact signals, results stay in original order even with the flag on.
func TestEvidenceFirstPack_NoSignalsInQuery_OrderUnchanged(t *testing.T) {
	pool := newEvidencePackPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"query":               "what did I do last week",
		"project":             "test",
		"evidence_first_pack": true,
	}

	res, err := handleMemoryRecall(context.Background(), pool, req, testConfig())
	require.NoError(t, err)
	results := parseRecallResults(t, res)

	// No signals → stable sort, miss (higher FTS score) stays first.
	id := firstResultID(t, results)
	require.Equal(t, "mem-miss", id,
		"with no query signals, evidence_first_pack must not change order")
}
