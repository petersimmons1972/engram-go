package mcp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	consolidatepkg "github.com/petersimmons1972/engram/internal/consolidate"
	"github.com/petersimmons1972/engram/internal/summarize"
)

func handleMemorySummarize(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	errResult, id := requireString(args, "memory_id")
	if errResult != nil {
		return errResult, nil
	}
	if err := summarize.SummarizeOne(ctx, h.Engine.Backend(), id, cfg.RouterURL, cfg.SummarizeModel); err != nil {
		return nil, fmt.Errorf("%w (project=%q — did you mean a different project?)", err, project)
	}
	return toolResult(map[string]any{"status": "summarized", "memory_id": id})
}

// handleMemoryResummarize clears all summaries for a project so the background
// worker regenerates them with the current model on its next tick (within 60s).
func handleMemoryResummarize(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	cleared, err := h.Engine.Backend().ClearSummaries(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("clear summaries: %w", err)
	}
	return toolResult(map[string]any{
		"cleared": cleared,
		"message": fmt.Sprintf("Cleared %d summaries for project %q — they will regenerate within 60s", cleared, project),
	})
}

// handleMemoryStatus returns aggregate statistics for a project's memory store.

func handleMemoryConsolidate(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	// Cap wall-clock time so the handler cannot run past the HTTP server's ReadTimeout.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	claudeConsolidateEnabled := cfg.ClaudeConsolidateEnabled
	if cfg.RuntimeConfig != nil {
		claudeConsolidateEnabled = cfg.RuntimeConfig.ClaudeConsolidate.Load()
	}
	if claudeConsolidateEnabled && cfg.claudeClient != nil {
		result, err = h.Engine.ConsolidateWithClaude(ctx, &claudeMergeAdapter{client: cfg.claudeClient})
	} else {
		result, err = h.Engine.Consolidate(ctx)
	}
	if err != nil {
		return nil, err
	}
	return toolResult(result)
}

// handleMemorySleep runs the full sleep-consolidation cycle (Feature 3).
// cfg is passed so the handler can read RouterURL for the LLM second pass.
func handleMemorySleep(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	// Cap wall-clock time so the handler cannot run past the HTTP server's ReadTimeout.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	minSim, minSimPresent, minSimErr := requireOptionalFloat(args, "min_similarity")
	if minSimErr != nil {
		return mcpgo.NewToolResultError(minSimErr.Error()), nil
	}
	if !minSimPresent {
		minSim = 0.7
	}
	limit, limitPresent, limitErr := requireOptionalInt(args, "limit")
	if limitErr != nil {
		return mcpgo.NewToolResultError(limitErr.Error()), nil
	}
	if !limitPresent {
		limit = 500
	}
	if limit < 1 || limit > 5000 {
		limit = 500
	}
	llmDetect, llmDetectPresent, llmDetectErr := requireOptionalBool(args, "llm_contradiction_detection")
	if llmDetectErr != nil {
		return mcpgo.NewToolResultError(llmDetectErr.Error()), nil
	}
	if !llmDetectPresent {
		llmDetect = false
	}
	llmModel, llmModelPresent, llmModelErr := requireOptionalString(args, "llm_model")
	if llmModelErr != nil {
		return mcpgo.NewToolResultError(llmModelErr.Error()), nil
	}
	if !llmModelPresent {
		llmModel = "llama3.2:3b"
	}
	llmMaxCalls, llmMaxCallsPresent, llmMaxCallsErr := requireOptionalInt(args, "llm_max_calls")
	if llmMaxCallsErr != nil {
		return mcpgo.NewToolResultError(llmMaxCallsErr.Error()), nil
	}
	if !llmMaxCallsPresent {
		llmMaxCalls = 10
	}
	autoSupersede, autoSupersedePresent, autoSupersedeErr := requireOptionalBool(args, "auto_supersede")
	if autoSupersedeErr != nil {
		return mcpgo.NewToolResultError(autoSupersedeErr.Error()), nil
	}
	if !autoSupersedePresent {
		autoSupersede = false
	}
	contradictionLimit, contradictionLimitPresent, contradictionLimitErr := requireOptionalInt(args, "contradiction_limit")
	if contradictionLimitErr != nil {
		return mcpgo.NewToolResultError(contradictionLimitErr.Error()), nil
	}
	if !contradictionLimitPresent {
		contradictionLimit = 0
	}

	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}

	runner := consolidatepkg.NewRunner(h.Engine.Backend(), project, h.Engine.Embedder())
	stats, err := runner.RunAll(ctx, consolidatepkg.RunOptions{
		InferRelationshipsMinSimilarity: minSim,
		InferRelationshipsLimit:         limit,
		ContradictionDetectionLimit:     contradictionLimit,
		LLMContradictionDetection:       llmDetect,
		RouterURL:                       cfg.RouterURL,
		OllamaModel:                     llmModel,
		LLMMaxCalls:                     llmMaxCalls,
		AutoSupersede:                   autoSupersede,
	})
	if err != nil {
		return nil, err
	}
	return toolResult(stats)
}

// handleMemoryVerify checks integrity of the memory store.
func handleMemoryVerify(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	result, err := h.Engine.Verify(ctx)
	if err != nil {
		return nil, err
	}
	return toolResult(result)
}

// handleMemoryDeleteProject hard-deletes all memories, chunks, relationships,
// episodes, and metadata for a project. Irreversible. Used for eval isolation
// project cleanup (#384).
//
// A-4 (#689) authorization gate: requires BOTH
//  1. Server started with ENGRAM_ALLOW_PROJECT_DELETE=1 (out-of-band gate
//     that an LLM cannot flip).
//  2. confirm argument exactly matches the project argument (typo guard).
//
// Both failures return a tool-level error (isError=true), not a Go error,
// so the calling LLM sees structured guidance — not a transport failure.
func handleMemoryDeleteProject(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "")
	if err != nil {
		return nil, err
	}
	if project == "" {
		return nil, fmt.Errorf("project is required for memory_delete_project")
	}

	// A-4 gate 1: server-level env var must be set.
	gate := os.Getenv("ENGRAM_ALLOW_PROJECT_DELETE")
	if gate != "1" && !strings.EqualFold(gate, "true") {
		return mcpgo.NewToolResultError("memory_delete_project is disabled. This server was started without ENGRAM_ALLOW_PROJECT_DELETE=1. To delete a project, the operator must restart the server with that env var set. This is intentional — project deletion is irreversible. Do not retry; ask the human operator."), nil
	}

	// A-4 gate 2: confirm argument must match project (typo guard).
	confirm, _ := args["confirm"].(string)
	if confirm != project {
		return mcpgo.NewToolResultError(fmt.Sprintf("memory_delete_project: confirm argument must exactly match project. Got project=%q, confirm=%q. This is a typo guard — supply confirm=%q to proceed.", project, confirm, project)), nil
	}

	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}

	if err := h.Engine.Backend().DeleteProject(ctx, project); err != nil {
		return nil, fmt.Errorf("delete project %q: %w", project, err)
	}

	return toolResult(map[string]any{
		"deleted": true,
		"project": project,
		"message": fmt.Sprintf("Permanently deleted all data for project %q", project),
	})
}

// handleMemoryMigrateEmbedder re-embeds all chunks using a new Ollama model.
// Performs a dimension pre-flight before nulling existing embeddings: if the
// new model outputs a different vector width than the current stored dimension,
// migration is refused to prevent silent pgvector corruption (#251).
