package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
)

func execFetch(ctx context.Context, f backendFetcher, id, detail string, maxBytes int, requestedChunkIDs []string) (map[string]any, error) {
	m, err := f.GetMemory(ctx, id)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, fmt.Errorf("memory %q not found", id)
	}

	switch detail {
	case "chunk":
		chunks, err := f.GetChunksForMemory(ctx, m.ID)
		if err != nil {
			return nil, err
		}
		if len(requestedChunkIDs) > 0 {
			want := make(map[string]bool, len(requestedChunkIDs))
			for _, cid := range requestedChunkIDs {
				want[cid] = true
			}
			filtered := chunks[:0]
			for _, c := range chunks {
				if want[c.ID] {
					filtered = append(filtered, c)
				}
			}
			chunks = filtered
		}
		return map[string]any{
			"id":          m.ID,
			"memory_type": m.MemoryType,
			"project":     m.Project,
			"chunks":      chunks,
			"chunk_count": len(chunks),
		}, nil

	case "full":
		content := m.Content
		truncated := false
		if maxBytes > 0 && len(content) > maxBytes {
			content = content[:maxBytes]
			truncated = true
		}
		out := map[string]any{
			"id":          m.ID,
			"memory_type": m.MemoryType,
			"project":     m.Project,
			"content":     content,
			"truncated":   truncated,
		}
		if m.Summary != nil {
			out["summary"] = *m.Summary
		}
		return out, nil

	default: // "summary" and anything unrecognised
		sum := ""
		if m.Summary != nil {
			sum = *m.Summary
		}
		return map[string]any{
			"id":          m.ID,
			"memory_type": m.MemoryType,
			"project":     m.Project,
			"summary":     sum,
		}, nil
	}
}

// handleMemoryFetch implements the memory_fetch MCP tool.
func handleMemoryFetch(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	errResult, id := requireString(args, "id")
	if errResult != nil {
		return errResult, nil
	}
	detail := getString(args, "detail", "summary")
	chunkIDs, err := toStringSlice(args["chunk_ids"])
	if err != nil {
		return nil, fmt.Errorf("chunk_ids: %w", err)
	}
	maxBytes := cfg.FetchMaxBytes
	if maxBytes == 0 {
		maxBytes = 65536
	}

	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	result, err := execFetch(ctx, h.Engine.Backend(), id, detail, maxBytes, chunkIDs)
	if err != nil {
		return nil, err
	}
	return toolResult(result)
}

// claudeMergeAdapter adapts *claude.Client to search.MergeReviewer by converting
// between the search-package and claude-package candidate/decision types.
type claudeMergeAdapter struct {
	client *claude.Client
}

func (a *claudeMergeAdapter) ReviewMergeCandidates(ctx context.Context, candidates []search.MergeCandidate) ([]search.MergeDecision, error) {
	// Convert search.MergeCandidate → claude.MergeCandidate.
	claudeCandidates := make([]claude.MergeCandidate, len(candidates))
	for i, c := range candidates {
		claudeCandidates[i] = claude.MergeCandidate{
			MemoryA:    c.MemoryA,
			MemoryB:    c.MemoryB,
			Similarity: c.Similarity,
		}
	}
	claudeDecisions, err := a.client.ReviewMergeCandidates(ctx, claudeCandidates)
	if err != nil {
		return nil, err
	}
	// Convert claude.MergeDecision → search.MergeDecision.
	decisions := make([]search.MergeDecision, len(claudeDecisions))
	for i, d := range claudeDecisions {
		decisions[i] = search.MergeDecision{
			MemoryAID:     d.MemoryAID,
			MemoryBID:     d.MemoryBID,
			ShouldMerge:   d.ShouldMerge,
			Reason:        d.Reason,
			MergedContent: d.MergedContent,
		}
	}
	return decisions, nil
}

// claudeRerankAdapter bridges search.ResultReranker to claude.Client.
type claudeRerankAdapter struct {
	client *claude.Client
}

func (a *claudeRerankAdapter) RerankResults(ctx context.Context, query string, items []search.RerankItem) ([]search.RerankResult, error) {
	// Convert search.RerankItem → claude.RerankItem.
	claudeItems := make([]claude.RerankItem, len(items))
	for i, item := range items {
		claudeItems[i] = claude.RerankItem{
			ID:      item.ID,
			Summary: item.Summary,
			Score:   item.Score,
		}
	}
	claudeResults, err := a.client.RerankResults(ctx, query, claudeItems)
	if err != nil {
		return nil, err
	}
	results := make([]search.RerankResult, len(claudeResults))
	for i, r := range claudeResults {
		results[i] = search.RerankResult{
			ID:    r.ID,
			Score: r.Score,
		}
	}
	return results, nil
}

func handleMemoryRecall(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	query := getString(args, "query", "")
	if query == "" {
		return mcpgo.NewToolResultError("query: required"), nil
	}
	topK := getInt(args, "top_k", 10)
	if topK < 1 || topK > 100 {
		topK = 10
	}
	detail := getString(args, "detail", "summary")
	includeConflicts := getBool(args, "include_conflicts", false)
	mode := getString(args, "mode", cfg.RecallDefaultMode)

	// Federated path: "projects" overrides the single-project recall.
	projectNames, err := toStringSlice(args["projects"])
	if err != nil {
		return nil, fmt.Errorf("projects: %w", err)
	}
	if len(projectNames) > 0 {
		// Expand wildcard "*" to all known projects.
		if len(projectNames) == 1 && projectNames[0] == "*" {
			h0, err := pool.Get(ctx, project)
			if err != nil {
				return nil, err
			}
			all, err := h0.Engine.Backend().ListAllProjects(ctx)
			if err != nil {
				return nil, err
			}
			projectNames = all
		}
		engines := make([]*search.SearchEngine, 0, len(projectNames))
		var firstHandle *EngineHandle // retained for conflict enrichment
		var firstProject string       // project name that firstHandle was initialized for
		for _, p := range projectNames {
			h, err := pool.Get(ctx, p)
			if err != nil {
				// Log so callers know results may be partial (#130).
				slog.Warn("handleMemoryRecall: skipping project — init failed",
					"project", p, "err", err)
				continue
			}
			if firstHandle == nil {
				firstHandle = h
				firstProject = p
			}
			engines = append(engines, h.Engine)
		}
		results, err := search.RecallAcrossEngines(ctx, engines, query, topK, detail)
		if err != nil {
			return nil, err
		}
		if mode == "handle" {
			return toolResult(map[string]any{
				"handles":    search.ToHandles(results),
				"count":      len(results),
				"fetch_hint": "call memory_fetch with id and detail=summary|chunk|full",
			})
		}
		out := map[string]any{"results": results, "count": len(results)}
		if includeConflicts && firstHandle != nil {
			// All projects share the same Postgres instance, so the backend from
			// the first successfully-initialized engine can serve cross-project
			// GetRelationships and GetMemory calls (#154).
			// EnrichWithConflicts uses each result's Memory.Project for the
			// per-memory lookup; firstProject is the fallback for the rare empty case.
			conflicts := EnrichWithConflicts(ctx, firstHandle.Engine.Backend(), firstProject, results)
			out["conflicting_results"] = conflicts
			out["conflict_count"] = len(conflicts)
		}
		return toolResult(out)
	}

	// Single-project path.
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	rerank := getBool(args, "rerank", false)
	var opts search.RecallOpts
	if cfg.ClaudeRerankEnabled && rerank && cfg.claudeClient != nil {
		opts.Reranker = &claudeRerankAdapter{client: cfg.claudeClient}
	}
	// Inject current session episode for same-session score boosting (Phase 3).
	if id, ok := episodeIDFromContext(ctx); ok {
		opts.CurrentEpisodeID = id
	}

	// Use RecallWithEvent to log the retrieval; on the rerank path we call
	// RecallWithOpts (which supports a custom Reranker) and then manually
	// record the retrieval event + warm times_retrieved so the feedback loop
	// and precision signal work regardless of which path was taken.
	var results []types.SearchResult
	var eventID string
	if opts.Reranker != nil {
		results, err = h.Engine.RecallWithOpts(ctx, query, topK, detail, opts)
		if err != nil {
			return nil, err
		}
		// Post-hoc event recording — mirrors RecallWithEvent internals.
		rerankIDs := make([]string, 0, len(results))
		for _, r := range results {
			if r.Memory != nil {
				rerankIDs = append(rerankIDs, r.Memory.ID)
			}
		}
		if len(rerankIDs) > 0 {
			event := &types.RetrievalEvent{
				ID:        types.NewMemoryID(),
				Project:   project,
				Query:     query,
				ResultIDs: rerankIDs,
				CreatedAt: time.Now().UTC(),
			}
			if storeErr := h.Engine.Backend().StoreRetrievalEvent(ctx, event); storeErr != nil {
				slog.Warn("store retrieval event (rerank path) failed", "project", project, "err", storeErr)
			} else {
				eventID = event.ID
				if incErr := h.Engine.Backend().IncrementTimesRetrieved(ctx, rerankIDs); incErr != nil {
					slog.Warn("auto-increment times_retrieved (rerank path) failed", "project", project, "err", incErr)
				}
			}
		}
	} else if opts.CurrentEpisodeID != "" {
		// Episode context present: use RecallWithOpts so the 1.15× episode
		// boost applies, then record the retrieval event manually (mirrors
		// RecallWithEvent internals so the feedback loop and precision signal
		// continue to work on this path).
		results, err = h.Engine.RecallWithOpts(ctx, query, topK, detail, opts)
		if err != nil {
			return nil, err
		}
		resultIDs := make([]string, 0, len(results))
		for _, r := range results {
			if r.Memory != nil {
				resultIDs = append(resultIDs, r.Memory.ID)
			}
		}
		if len(resultIDs) > 0 {
			event := &types.RetrievalEvent{
				ID:        types.NewMemoryID(),
				Project:   project,
				Query:     query,
				ResultIDs: resultIDs,
				CreatedAt: time.Now().UTC(),
			}
			if storeErr := h.Engine.Backend().StoreRetrievalEvent(ctx, event); storeErr != nil {
				slog.Warn("store retrieval event (episode path) failed", "project", project, "err", storeErr)
			} else {
				eventID = event.ID
				if incErr := h.Engine.Backend().IncrementTimesRetrieved(ctx, resultIDs); incErr != nil {
					slog.Warn("auto-increment times_retrieved (episode path) failed", "project", project, "err", incErr)
				}
			}
		}
	} else {
		results, eventID, err = h.Engine.RecallWithEvent(ctx, query, topK, detail)
		if err != nil {
			return nil, err
		}
	}

	if mode == "handle" {
		return toolResult(map[string]any{
			"handles":    search.ToHandles(results),
			"count":      len(results),
			"fetch_hint": "call memory_fetch with id and detail=summary|chunk|full",
		})
	}
	out := map[string]any{"results": results, "count": len(results)}
	if eventID != "" {
		out["event_id"] = eventID
		out["feedback_hint"] = "Call memory_feedback with this event_id and the memory_ids you used"
	}
	if includeConflicts {
		conflicts := EnrichWithConflicts(ctx, h.Engine.Backend(), project, results)
		out["conflicting_results"] = conflicts
		out["conflict_count"] = len(conflicts)
	}
	return toolResult(out)
}

// handleMemoryProjects lists all projects with their memory counts and last-active timestamps.

func handleMemoryList(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	limit := getInt(args, "limit", 50)
	if limit < 1 || limit > 500 {
		limit = 50
	}
	offset := getInt(args, "offset", 0)
	var memType *string
	if s := getString(args, "memory_type", ""); s != "" {
		memType = &s
	}
	listTags, err := toStringSlice(args["tags"])
	if err != nil {
		return nil, fmt.Errorf("tags: %w", err)
	}
	memories, err := h.Engine.List(ctx, memType, listTags, nil, limit, offset)
	if err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"memories": memories, "count": len(memories)})
}

// handleMemoryConnect creates a directional relationship between two memories.

func handleMemoryHistory(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
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
	history, err := h.Engine.MemoryHistory(ctx, id)
	if err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"memory_id": id, "versions": history, "count": len(history)})
}

// handleMemoryTimeline recalls memories that were active at a given point in time.
func handleMemoryTimeline(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	errResult, asOfStr := requireString(args, "as_of")
	if errResult != nil {
		return errResult, nil
	}
	asOf, err := time.Parse(time.RFC3339, asOfStr)
	if err != nil {
		return nil, fmt.Errorf("as_of must be RFC3339 (e.g. 2025-03-05T14:00:00Z): %w", err)
	}
	limit := getInt(args, "limit", 20)
	memories, err := h.Engine.MemoryAsOf(ctx, asOf, limit)
	if err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"as_of": asOfStr, "memories": memories, "count": len(memories)})
}

// handleMemorySummarize requests Ollama to summarize a memory's content.

func handleMemoryQuery(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	// Build a merged copy so we never mutate the caller's map.
	merged := make(map[string]any, len(args)+1)
	for k, v := range args {
		merged[k] = v
	}

	// Map "limit" → "top_k". If both are supplied, top_k wins.
	if limit, ok := merged["limit"]; ok {
		if _, hasTopK := merged["top_k"]; !hasTopK {
			merged["top_k"] = limit
		}
		delete(merged, "limit")
	} else if _, hasTopK := merged["top_k"]; !hasTopK {
		merged["top_k"] = 5
	}

	req2 := req
	req2.Params.Arguments = merged
	return handleMemoryRecall(ctx, pool, req2, cfg)
}

// handleMemoryExpand explores the relationship-graph neighbourhood of a known
// memory. It calls GetConnected on the engine's backend and returns all
// reachable nodes within depth hops.
func handleMemoryExpand(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	errResult, memoryID := requireString(args, "memory_id")
	if errResult != nil {
		return errResult, nil
	}
	requestedDepth := getInt(args, "depth", 2)
	depth := requestedDepth
	if depth < 1 || depth > 5 {
		depth = 2
	}

	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}

	connected, err := h.Engine.Backend().GetConnected(ctx, memoryID, depth)
	if err != nil {
		return nil, err
	}

	type connItem struct {
		ID         string   `json:"id"`
		Content    string   `json:"content"`
		MemoryType string   `json:"memory_type"`
		Project    string   `json:"project"`
		Tags       []string `json:"tags"`
		CreatedAt  string   `json:"created_at,omitempty"`
		RelType    string   `json:"rel_type"`
		Direction  string   `json:"direction"`
		Strength   float64  `json:"strength"`
	}
	items := make([]connItem, 0, len(connected))
	for _, c := range connected {
		if c.Memory == nil {
			continue
		}
		createdAt := ""
		if !c.Memory.CreatedAt.IsZero() {
			createdAt = c.Memory.CreatedAt.Format(time.RFC3339)
		}
		items = append(items, connItem{
			ID:         c.Memory.ID,
			Content:    c.Memory.Content,
			MemoryType: c.Memory.MemoryType,
			Project:    c.Memory.Project,
			Tags:       c.Memory.Tags,
			CreatedAt:  createdAt,
			RelType:    c.RelType,
			Direction:  c.Direction,
			Strength:   c.Strength,
		})
	}

	out := map[string]any{
		"memory_id": memoryID,
		"depth":     depth,
		"connected": items,
	}
	if requestedDepth != depth {
		out["requested_depth"] = requestedDepth
	}
	return toolResult(out)
}

// handleMemoryModels returns installed Ollama embedding models merged with
// the curated SuggestedModels registry.
