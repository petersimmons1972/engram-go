package mcp

import (
	"context"
	"fmt"
	"math"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/types"
)

func handleMemoryProjects(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	// Use any project to get a backend connection for the cross-project query.
	anchorProject, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	h, err := pool.Get(ctx, anchorProject)
	if err != nil {
		return nil, err
	}
	projects, err := h.Engine.Backend().ListAllProjects(ctx)
	if err != nil {
		return nil, err
	}
	// Enrich with per-project stats.
	type projectInfo struct {
		Project string `json:"project"`
		Count   int    `json:"count"`
	}
	out := make([]projectInfo, 0, len(projects))
	for _, p := range projects {
		ph, err := pool.Get(ctx, p)
		if err != nil {
			out = append(out, projectInfo{Project: p})
			continue
		}
		stats, err := ph.Engine.Status(ctx)
		if err != nil {
			out = append(out, projectInfo{Project: p})
			continue
		}
		out = append(out, projectInfo{Project: p, Count: stats.TotalMemories})
	}
	return toolResult(map[string]any{"projects": out, "count": len(out)})
}

// handleMemoryAdopt creates a cross-project reference relationship from a memory in
// the calling project to a memory in another project. The relationship is stored in
// the calling project's edge table.
func handleMemoryAdopt(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	errResult, srcID := requireString(args, "source_id")
	if errResult != nil {
		return errResult, nil
	}
	errResult, dstID := requireString(args, "target_id")
	if errResult != nil {
		return errResult, nil
	}
	relType := getString(args, "relation_type", types.RelTypeRelatesTo)
	strength := 1.0
	if v, ok := args["strength"].(float64); ok {
		strength = v
	}
	if err := h.Engine.Connect(ctx, srcID, dstID, relType, strength); err != nil {
		return nil, err
	}
	return toolResult(map[string]any{
		"status":    "adopted",
		"source_id": srcID,
		"target_id": dstID,
	})
}

// handleMemoryList lists memories with optional type and tag filters.

func handleMemoryConnect(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	errResult, src := requireString(args, "source_id")
	if errResult != nil {
		return errResult, nil
	}
	errResult, dst := requireString(args, "target_id")
	if errResult != nil {
		return errResult, nil
	}
	relType := getString(args, "relation_type", types.RelTypeRelatesTo)
	strength := 1.0
	if v, ok := args["strength"].(float64); ok {
		strength = v
	}
	if math.IsNaN(strength) || math.IsInf(strength, 0) || strength < 0 || strength > 1.0 {
		return nil, fmt.Errorf("strength must be between 0 and 1, got %v", strength)
	}
	if err := h.Engine.Connect(ctx, src, dst, relType, strength); err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"status": "connected", "source_id": src, "target_id": dst})
}

// handleMemoryCorrect updates the content, tags, or importance of an existing memory.

func handleMemoryStatus(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	stats, err := h.Engine.Status(ctx)
	if err != nil {
		return nil, err
	}
	return toolResult(stats)
}

// handleMemoryFeedback records positive reinforcement for recalled memory IDs.
// When event_id is provided (returned by memory_recall), retrieval outcome
// tracking is updated in addition to the standard edge boost.
func handleMemoryFeedback(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	ids, err := toStringSlice(args["memory_ids"])
	if err != nil {
		return nil, fmt.Errorf("memory_ids: %w", err)
	}
	if len(ids) > 100 {
		return nil, fmt.Errorf("memory_ids: too many IDs (%d), max 100", len(ids))
	}
	eventID := getString(args, "event_id", "")
	if eventID != "" {
		if _, err := uuid.Parse(eventID); err != nil {
			return nil, fmt.Errorf("event_id: must be a valid UUID, got %q", eventID)
		}
	}
	failureClass := getString(args, "failure_class", "")
	if failureClass != "" {
		if !types.ValidateFailureClass(failureClass) {
			return nil, fmt.Errorf("failure_class: invalid value %q", failureClass)
		}
		if eventID == "" {
			return nil, fmt.Errorf("event_id is required when failure_class is set")
		}
	}
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	if failureClass != "" {
		if err := h.Engine.FeedbackWithEventAndClass(ctx, eventID, ids, failureClass); err != nil {
			return nil, err
		}
		return toolResult(map[string]any{"status": "recorded", "count": len(ids)})
	}
	if eventID != "" {
		if err := h.Engine.FeedbackWithEvent(ctx, eventID, ids); err != nil {
			return nil, err
		}
	} else {
		if err := h.Engine.Feedback(ctx, ids); err != nil {
			return nil, err
		}
	}
	return toolResult(map[string]any{"status": "recorded", "count": len(ids)})
}

// handleMemoryAggregate groups memories by a given dimension (tag, type, or
// failure_class) and returns counts with oldest/newest timestamps per label.
func handleMemoryAggregate(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	by := getString(args, "by", "")
	if by == "" {
		return nil, fmt.Errorf("by: required (tag, type, or failure_class)")
	}
	filter := getString(args, "filter", "")
	limit := getInt(args, "limit", 20)
	if limit < 1 || limit > 1000 {
		limit = 20
	}
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	rows, err := h.Engine.Aggregate(ctx, by, filter, limit)
	if err != nil {
		return nil, err
	}
	rowsAny := make([]any, len(rows))
	for i, r := range rows {
		rowsAny[i] = map[string]any{
			"label":  r.Label,
			"count":  r.Count,
			"oldest": r.Oldest,
			"newest": r.Newest,
		}
	}
	return toolResult(map[string]any{
		"by":      by,
		"project": project,
		"rows":    rowsAny,
	})
}

// handleMemoryConsolidate merges near-duplicate memories to reduce redundancy.
// When cfg.ClaudeConsolidateEnabled is true and a claude client is available,
// it uses bigramJaccard similarity + Claude review for near-duplicate merging.

func handleMemoryDiagnose(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	question := getString(args, "question", "")
	topK := getInt(args, "top_k", 10)
	detail := getString(args, "detail", "full")
	if question == "" {
		return mcpgo.NewToolResultError("question is required"), nil
	}
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	results, err := h.Engine.Recall(ctx, question, topK, detail)
	if err != nil {
		return nil, err
	}
	memories := make([]*types.Memory, 0, len(results))
	for _, r := range results {
		if r.Memory != nil {
			memories = append(memories, r.Memory)
		}
	}
	ev := buildEvidenceMap(ctx, h.Engine.Backend(), project, memories)
	return toolResult(map[string]any{
		"confidence":          ev.Confidence,
		"conflicts":           ev.Conflicts,
		"invalidated_sources": ev.InvalidatedSources,
		"memories_used":       len(ev.Memories),
	})
}

// handleMemoryReason recalls memories relevant to a question and uses Claude to
// synthesize a grounded, conflict-aware answer.
