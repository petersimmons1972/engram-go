package mcp

import (
	"context"
	"encoding/json"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

type issue629Backend struct {
	noopBackend
	stats    *types.MemoryStats
	projects []string
	episode  *types.Episode
	memories []*types.Memory
}

func (b issue629Backend) GetStats(_ context.Context, _ string) (*types.MemoryStats, error) {
	return b.stats, nil
}
func (b issue629Backend) ListAllProjects(_ context.Context) ([]string, error) {
	return b.projects, nil
}
func (b issue629Backend) StartEpisode(_ context.Context, _, _ string) (*types.Episode, error) {
	return b.episode, nil
}
func (b issue629Backend) ListEpisodes(_ context.Context, _ string, _ int) ([]*types.Episode, error) {
	return []*types.Episode{b.episode}, nil
}
func (b issue629Backend) RecallEpisode(_ context.Context, _ string) ([]*types.Memory, error) {
	return b.memories, nil
}

func newIssue629Pool(t *testing.T) *EnginePool {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, issue629Backend{
			stats:    &types.MemoryStats{TotalMemories: 3},
			projects: []string{"alpha", "beta"},
			episode:  &types.Episode{ID: "ep-1", Project: project},
			memories: []*types.Memory{{ID: "m-1", Project: project}},
		}, noopEmbedder{}, project, "http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	return NewEnginePool(factory)
}

func decodeToolResult(t *testing.T, res *mcpgo.CallToolResult) map[string]any {
	t.Helper()
	require.NotNil(t, res)
	tc, ok := res.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(tc.Text), &out))
	return out
}

func TestIssue629_EpisodeHandlersAndAdminHandlers(t *testing.T) {
	pool := newIssue629Pool(t)

	res, err := handleMemoryEpisodeStart(context.Background(), pool, mcpgo.CallToolRequest{Params: mcpgo.CallToolParams{Arguments: map[string]any{"project": "proj"}}})
	require.NoError(t, err)
	require.Equal(t, "ep-1", decodeToolResult(t, res)["id"])

	res, err = handleMemoryEpisodeEnd(context.Background(), pool, mcpgo.CallToolRequest{Params: mcpgo.CallToolParams{Arguments: map[string]any{"project": "proj", "episode_id": "ep-1"}}})
	require.NoError(t, err)
	require.Equal(t, true, decodeToolResult(t, res)["ended"])

	res, err = handleMemoryEpisodeList(context.Background(), pool, mcpgo.CallToolRequest{Params: mcpgo.CallToolParams{Arguments: map[string]any{"project": "proj"}}})
	require.NoError(t, err)
	require.Equal(t, float64(1), decodeToolResult(t, res)["count"])

	res, err = handleMemoryEpisodeRecall(context.Background(), pool, mcpgo.CallToolRequest{Params: mcpgo.CallToolParams{Arguments: map[string]any{"project": "proj", "episode_id": "ep-1"}}})
	require.NoError(t, err)
	require.Equal(t, float64(1), decodeToolResult(t, res)["count"])

	res, err = handleMemoryProjects(context.Background(), pool, mcpgo.CallToolRequest{Params: mcpgo.CallToolParams{Arguments: map[string]any{"project": "proj"}}})
	require.NoError(t, err)
	require.Equal(t, float64(2), decodeToolResult(t, res)["count"])

	res, err = handleMemoryStatus(context.Background(), pool, mcpgo.CallToolRequest{Params: mcpgo.CallToolParams{Arguments: map[string]any{"project": "proj"}}})
	require.NoError(t, err)
	require.Equal(t, float64(3), decodeToolResult(t, res)["total_memories"])

	res, err = handleMemoryDiagnose(context.Background(), pool, mcpgo.CallToolRequest{Params: mcpgo.CallToolParams{Arguments: map[string]any{"project": "proj", "question": "what happened?"}}})
	require.NoError(t, err)
	require.Equal(t, float64(0), decodeToolResult(t, res)["memories_used"])
}

var _ db.Backend = issue629Backend{}
