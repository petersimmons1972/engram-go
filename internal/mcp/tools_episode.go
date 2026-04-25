package mcp

import (
	"context"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/types"
)

func handleMemoryEpisodeStart(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	description := getString(args, "description", "")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	ep, err := h.Engine.Backend().StartEpisode(ctx, project, description)
	if err != nil {
		return nil, err
	}
	return toolResult(ep)
}

// handleMemoryEpisodeEnd marks an episode as ended with an optional summary.
func handleMemoryEpisodeEnd(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	id := getString(args, "episode_id", "")
	summary := getString(args, "summary", "")
	if id == "" {
		return mcpgo.NewToolResultError("episode_id is required"), nil
	}
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	if err := h.Engine.Backend().EndEpisode(ctx, id, summary); err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"episode_id": id, "ended": true})
}

// handleMemoryEpisodeList returns recent episodes for a project.
func handleMemoryEpisodeList(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	limit := getInt(args, "limit", 20)
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	eps, err := h.Engine.Backend().ListEpisodes(ctx, project, limit)
	if err != nil {
		return nil, err
	}
	if eps == nil {
		eps = []*types.Episode{}
	}
	return toolResult(map[string]any{"episodes": eps, "count": len(eps)})
}

// handleMemoryEpisodeRecall returns all memories from a specific episode in
// chronological order.
func handleMemoryEpisodeRecall(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	episodeID := getString(args, "episode_id", "")
	if episodeID == "" {
		return mcpgo.NewToolResultError("episode_id is required"), nil
	}
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	memories, err := h.Engine.Backend().RecallEpisode(ctx, episodeID)
	if err != nil {
		return nil, err
	}
	if memories == nil {
		memories = []*types.Memory{}
	}
	return toolResult(map[string]any{"memories": memories, "count": len(memories), "episode_id": episodeID})
}
