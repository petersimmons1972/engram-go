package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/petersimmons1972/engram/internal/rag"
	"github.com/petersimmons1972/engram/internal/types"
)

func buildEvidenceMap(ctx context.Context, backend interface {
	GetRelationships(ctx context.Context, project, memoryID string) ([]types.Relationship, error)
}, project string, memories []*types.Memory) claude.EvidenceMap {
	seen := make(map[string]bool)
	var allRels []types.Relationship
	for _, m := range memories {
		rels, err := backend.GetRelationships(ctx, project, m.ID)
		if err != nil {
			// Log so operators know confidence may be degraded (#132).
			slog.Warn("buildEvidenceMap: GetRelationships failed — conflict confidence may be incomplete",
				"project", project, "memory_id", m.ID, "err", err)
			continue
		}
		for _, r := range rels {
			if !seen[r.ID] {
				seen[r.ID] = true
				allRels = append(allRels, r)
			}
		}
	}
	return claude.DiagnoseMemories(memories, allRels)
}

// handleMemoryDiagnose returns a full evidence map for recalled memories without
// synthesizing an answer — useful for inspecting conflicts before reasoning.

func handleMemoryReason(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	// Cap wall-clock time so the handler cannot run past the HTTP server's ReadTimeout.
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	question := getString(args, "question", "")
	topK := getInt(args, "top_k", 10)
	if topK < 1 {
		topK = 1
	} else if topK > 100 {
		topK = 100
	}
	detail := getString(args, "detail", "full")

	if question == "" {
		return mcpgo.NewToolResultError("question is required"), nil
	}
	if cfg.claudeClient == nil {
		return mcpgo.NewToolResultError("memory_reason requires ANTHROPIC_API_KEY to be set"), nil
	}

	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("get engine for %q: %w", project, err)
	}

	results, err := h.Engine.Recall(ctx, question, topK, detail)
	if err != nil {
		return nil, fmt.Errorf("recall: %w", err)
	}

	memories := make([]*types.Memory, 0, len(results))
	for _, r := range results {
		if r.Memory != nil {
			memories = append(memories, r.Memory)
		}
	}

	ev := buildEvidenceMap(ctx, h.Engine.Backend(), project, memories)

	answer, err := cfg.claudeClient.ReasonWithConflictAwareness(ctx, question, ev)
	if err != nil {
		return nil, fmt.Errorf("reason: %w", err)
	}

	memoryIDs := make([]string, 0, len(memories))
	for _, m := range memories {
		memoryIDs = append(memoryIDs, m.ID)
	}

	out := map[string]any{
		"answer":              answer,
		"memories_used":       len(memories),
		"memory_ids":          memoryIDs,
		"conflicts":           ev.Conflicts,
		"confidence":          ev.Confidence,
		"invalidated_sources": ev.InvalidatedSources,
	}
	data, _ := json.Marshal(out)
	return mcpgo.NewToolResultText(string(data)), nil
}

// exploreScopedRecaller wraps a Recaller and a scope, filtering recalled
// memories to those matching the scope constraints. Filtering is post-recall
// since the underlying search engine does not expose scope-aware APIs.
type exploreScopedRecaller struct {
	inner claude.Recaller
	scope claude.ExploreScope
}

func (s *exploreScopedRecaller) Recall(ctx context.Context, query string, topK int, detail string) ([]types.SearchResult, error) {
	results, err := s.inner.Recall(ctx, query, topK, detail)
	if err != nil {
		return nil, err
	}
	// If no scope constraints are set, return results as-is.
	if len(s.scope.Tags) == 0 && s.scope.EpisodeID == "" && s.scope.Since == nil && s.scope.Until == nil {
		return results, nil
	}
	filtered := results[:0]
	for _, r := range results {
		if r.Memory == nil {
			continue
		}
		m := r.Memory
		// Episode filter.
		if s.scope.EpisodeID != "" && m.EpisodeID != s.scope.EpisodeID {
			continue
		}
		// Time filters.
		if s.scope.Since != nil && m.CreatedAt.Before(*s.scope.Since) {
			continue
		}
		if s.scope.Until != nil && m.CreatedAt.After(*s.scope.Until) {
			continue
		}
		// Tag filter: memory must contain all requested tags.
		if len(s.scope.Tags) > 0 {
			tagSet := make(map[string]bool, len(m.Tags))
			for _, t := range m.Tags {
				tagSet[t] = true
			}
			match := true
			for _, want := range s.scope.Tags {
				if !tagSet[want] {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}
		filtered = append(filtered, r)
	}
	return filtered, nil
}

// exploreMemFetcher implements claude.MemoryFetcher using the engine backend.
type exploreMemFetcher struct {
	backend backendFetcher
}

func (f *exploreMemFetcher) FetchMemory(ctx context.Context, _ string, id string) (*types.Memory, error) {
	return f.backend.GetMemoryByID(ctx, id)
}

// parseExploreScope extracts the optional scope sub-object from MCP args.
func parseExploreScope(args map[string]any) claude.ExploreScope {
	raw, ok := args["scope"]
	if !ok {
		return claude.ExploreScope{}
	}
	scopeMap, ok := raw.(map[string]any)
	if !ok {
		return claude.ExploreScope{}
	}
	var scope claude.ExploreScope
	scope.Tags, _ = toStringSlice(scopeMap["tags"]) // ignore control-char error in optional scope
	scope.EpisodeID = getString(scopeMap, "episode_id", "")
	if since := getString(scopeMap, "since", ""); since != "" {
		if t, err := time.Parse(time.RFC3339, since); err == nil {
			scope.Since = &t
		}
	}
	if until := getString(scopeMap, "until", ""); until != "" {
		if t, err := time.Parse(time.RFC3339, until); err == nil {
			scope.Until = &t
		}
	}
	return scope
}

// handleMemoryExplore runs the iterative recall+score+synthesis loop (A3).
func handleMemoryExplore(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	question := getString(args, "question", "")
	if question == "" {
		return mcpgo.NewToolResultError("question is required"), nil
	}
	if cfg.claudeClient == nil {
		return mcpgo.NewToolResultError("memory_explore requires ANTHROPIC_API_KEY to be set"), nil
	}

	maxIter := getInt(args, "max_iterations", cfg.ExploreMaxIters)
	if maxIter < 1 {
		maxIter = 1
	}
	if maxIter > 10 {
		maxIter = 10
	}
	threshold := getFloat(args, "confidence_threshold", 0.75)
	if threshold < 0 {
		threshold = 0
	}
	if threshold > 1 {
		threshold = 1
	}
	budget := getInt(args, "token_budget", cfg.ExploreTokenBudget)
	if budget <= 0 {
		budget = 20000
	}
	includeTrace := getBool(args, "include_trace", false)
	scope := parseExploreScope(args)

	// Cap wall-clock time so the handler cannot run past the HTTP server's ReadTimeout.
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("get engine for %q: %w", project, err)
	}

	// Wrap the engine in a scope-filtering recaller.
	recaller := claude.Recaller(h.Engine)
	if len(scope.Tags) > 0 || scope.EpisodeID != "" || scope.Since != nil || scope.Until != nil {
		recaller = &exploreScopedRecaller{inner: h.Engine, scope: scope}
	}

	// Wrap the backend as a MemoryFetcher for full-detail corpus upgrade.
	fetcher := &exploreMemFetcher{backend: h.Engine.Backend()}

	result, err := claude.Explore(ctx, cfg.claudeClient, recaller, fetcher, h.Engine.Backend(), claude.ExploreRequest{
		Project:             project,
		Question:            question,
		MaxIterations:       maxIter,
		ConfidenceThreshold: threshold,
		TokenBudget:         budget,
		IncludeTrace:        includeTrace,
		Scope:               scope,
		MaxWorkers:          cfg.ExploreMaxWorkers,
	})
	if err != nil {
		return nil, fmt.Errorf("explore: %w", err)
	}

	data, _ := json.Marshal(result)
	return mcpgo.NewToolResultText(string(data)), nil
}

// handleMemoryAsk performs retrieval-augmented question answering over stored
// memories using the rag.Asker pipeline. Requires a Claude client.
func handleMemoryAsk(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	question := strings.TrimSpace(getString(args, "question", ""))
	if question == "" {
		return mcpgo.NewToolResultError("question: required"), nil
	}
	project, err := getProject(args, "")
	if err != nil {
		return mcpgo.NewToolResultError("project: " + err.Error()), nil
	}
	if project == "" {
		return mcpgo.NewToolResultError("project: required"), nil
	}

	// Guard before any resource allocation.
	if cfg.claudeClient == nil {
		return mcpgo.NewToolResultError("memory_ask requires Claude (set ENGRAM_CLAUDE_KEY)"), nil
	}

	topK := getInt(args, "top_k", 0)
	if topK < 0 {
		return mcpgo.NewToolResultError("top_k: must be >= 0"), nil
	}
	if topK > 100 {
		topK = 100
	}
	// topK == 0 means "use default" — pass 0 to Asker.TopK so Ask applies defaultTopK=10.

	// Cap wall-clock time so the handler cannot run past the HTTP server's ReadTimeout.
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("get engine for %q: %w", project, err)
	}

	maxTokens := cfg.RAGMaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	asker := rag.Asker{
		Engine: h.Engine,
		Client: cfg.claudeClient,
		Budget: rag.ContextBudget{MaxTokens: maxTokens},
		TopK:   topK,
	}

	result, err := asker.Ask(ctx, question)
	if err != nil {
		return nil, fmt.Errorf("ask: %w", err)
	}

	out := map[string]any{
		"answer":              result.Answer,
		"citations":           result.Citations,
		"context_tokens_used": result.ContextTokensUsed,
	}
	data, _ := json.Marshal(out)
	return mcpgo.NewToolResultText(string(data)), nil
}

// ── Simplified front-door tools ───────────────────────────────────────────────
//
// These three handlers are genuine wrappers over the expert-surface tools.
// They exist to reduce the surface area that LLM orchestrators need to know
// about: sensible defaults are injected so callers only supply the minimum.

// handleMemoryQuickStore is a simplified front door for handleMemoryStore.
// It injects defaults for memory_type ("context") and importance (2) when
// those fields are absent from the request, then delegates to handleMemoryStore.
// The original args map is never mutated — a copy is used for the injection.
