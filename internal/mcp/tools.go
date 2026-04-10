package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/petersimmons1972/engram/internal/markdown"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/summarize"
	"github.com/petersimmons1972/engram/internal/types"
)

// Config holds server-wide configuration passed to tool handlers.
type Config struct {
	OllamaURL                string
	SummarizeModel           string
	SummarizeEnabled         bool
	ClaudeEnabled            bool // true when a claude client is present
	ClaudeConsolidateEnabled bool
	ClaudeRerankEnabled      bool
	// DataDir is the base directory for all file-system operations (export,
	// import, ingest). Paths provided by callers are validated to stay within
	// this directory. Must be set; file-operation tools return an error if empty.
	DataDir      string
	claudeClient *claude.Client // set via Server.SetClaudeClient
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

// toolResult marshals v to JSON and wraps it in an MCP text result.
func toolResult(v any) (*mcpgo.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return mcpgo.NewToolResultText(string(b)), nil
}

// getString extracts a string arg with a fallback default.
func getString(args map[string]any, key, def string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return def
}

// getInt extracts an int arg (JSON numbers arrive as float64) with a fallback.
func getInt(args map[string]any, key string, def int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return def
}

// getBool extracts a bool arg with a fallback default.
func getBool(args map[string]any, key string, def bool) bool {
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return def
}

// toStringSlice converts []any to []string, skipping non-string entries.
func toStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// handleMemoryStore stores a single focused memory.
func handleMemoryStore(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	content, _ := args["content"].(string)
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}
	m := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     content,
		MemoryType:  getString(args, "memory_type", types.MemoryTypeContext),
		Project:     project,
		Importance:  getInt(args, "importance", 2),
		Tags:        toStringSlice(args["tags"]),
		Immutable:   getBool(args, "immutable", false),
		StorageMode: "focused",
	}
	if err := h.Engine.Store(ctx, m); err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"id": m.ID, "status": "stored"})
}

// handleMemoryStoreDocument stores a document-mode memory (whole document as one chunk).
func handleMemoryStoreDocument(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	content, _ := args["content"].(string)
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}
	m := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     content,
		MemoryType:  getString(args, "memory_type", types.MemoryTypeContext),
		Project:     project,
		Importance:  getInt(args, "importance", 2),
		Tags:        toStringSlice(args["tags"]),
		StorageMode: "document",
	}
	if err := h.Engine.Store(ctx, m); err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"id": m.ID, "status": "stored", "mode": "document"})
}

// handleMemoryStoreBatch stores multiple memories in a single call.
func handleMemoryStoreBatch(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	items, _ := args["memories"].([]any)
	if len(items) == 0 {
		return toolResult(map[string]any{"ids": []string{}, "count": 0, "warning": "no memories provided"})
	}
	var ids []string
	for _, item := range items {
		mmap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		m := &types.Memory{
			ID:          types.NewMemoryID(),
			Content:     getString(mmap, "content", ""),
			MemoryType:  getString(mmap, "memory_type", types.MemoryTypeContext),
			Project:     project,
			Importance:  getInt(mmap, "importance", 2),
			Tags:        toStringSlice(mmap["tags"]),
			StorageMode: "focused",
		}
		if m.Content == "" {
			continue
		}
		if err := h.Engine.Store(ctx, m); err != nil {
			return nil, err
		}
		ids = append(ids, m.ID)
	}
	return toolResult(map[string]any{"ids": ids, "count": len(ids)})
}

// handleMemoryRecall performs semantic recall against a project's memories.
// Pass cfg to enable optional Claude re-ranking via the rerank argument.
func handleMemoryRecall(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	query := getString(args, "query", "")
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	topK := getInt(args, "top_k", 10)
	detail := getString(args, "detail", "summary")
	rerank := getBool(args, "rerank", false)

	var opts search.RecallOpts
	if cfg.ClaudeRerankEnabled && rerank && cfg.claudeClient != nil {
		opts.Reranker = &claudeRerankAdapter{client: cfg.claudeClient}
	}
	results, err := h.Engine.RecallWithOpts(ctx, query, topK, detail, opts)
	if err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"results": results, "count": len(results)})
}

// handleMemoryList lists memories with optional type and tag filters.
func handleMemoryList(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	limit := getInt(args, "limit", 50)
	offset := getInt(args, "offset", 0)
	var memType *string
	if s := getString(args, "memory_type", ""); s != "" {
		memType = &s
	}
	memories, err := h.Engine.List(ctx, memType, toStringSlice(args["tags"]), nil, limit, offset)
	if err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"memories": memories, "count": len(memories)})
}

// handleMemoryConnect creates a directional relationship between two memories.
func handleMemoryConnect(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	src := getString(args, "source_id", "")
	dst := getString(args, "target_id", "")
	if src == "" {
		return nil, fmt.Errorf("source_id is required")
	}
	if dst == "" {
		return nil, fmt.Errorf("target_id is required")
	}
	relType := getString(args, "relation_type", types.RelTypeRelatesTo)
	strength := 1.0
	if v, ok := args["strength"].(float64); ok {
		strength = v
	}
	if err := h.Engine.Connect(ctx, src, dst, relType, strength); err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"status": "connected", "source_id": src, "target_id": dst})
}

// handleMemoryCorrect updates the content, tags, or importance of an existing memory.
func handleMemoryCorrect(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	id := getString(args, "memory_id", "")
	if id == "" {
		return nil, fmt.Errorf("memory_id is required")
	}
	var content *string
	if c := getString(args, "content", ""); c != "" {
		content = &c
	}
	var importance *int
	if v, ok := args["importance"].(float64); ok {
		n := int(v)
		importance = &n
	}
	updated, err := h.Engine.Correct(ctx, id, content, toStringSlice(args["tags"]), importance)
	if err != nil {
		return nil, err
	}
	return toolResult(updated)
}

// handleMemoryForget deletes a memory by ID.
func handleMemoryForget(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	id := getString(args, "memory_id", "")
	if id == "" {
		return nil, fmt.Errorf("memory_id is required")
	}
	deleted, err := h.Engine.Forget(ctx, id)
	if err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"deleted": deleted, "memory_id": id})
}

// handleMemorySummarize requests Ollama to summarize a memory's content.
func handleMemorySummarize(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	id := getString(args, "memory_id", "")
	if id == "" {
		return nil, fmt.Errorf("memory_id is required")
	}
	if err := summarize.SummarizeOne(ctx, h.Engine.Backend(), id, cfg.OllamaURL, cfg.SummarizeModel); err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"status": "summarized", "memory_id": id})
}

// handleMemoryStatus returns aggregate statistics for a project's memory store.
func handleMemoryStatus(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
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
func handleMemoryFeedback(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	ids := toStringSlice(args["memory_ids"])
	if err := h.Engine.Feedback(ctx, ids); err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"status": "recorded", "count": len(ids)})
}

// handleMemoryConsolidate merges near-duplicate memories to reduce redundancy.
// When cfg.ClaudeConsolidateEnabled is true and a claude client is available,
// it uses bigramJaccard similarity + Claude review for near-duplicate merging.
func handleMemoryConsolidate(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if cfg.ClaudeConsolidateEnabled && cfg.claudeClient != nil {
		result, err = h.Engine.ConsolidateWithClaude(ctx, &claudeMergeAdapter{client: cfg.claudeClient})
	} else {
		result, err = h.Engine.Consolidate(ctx)
	}
	if err != nil {
		return nil, err
	}
	return toolResult(result)
}

// handleMemoryVerify checks integrity of the memory store.
func handleMemoryVerify(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
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

// handleMemoryMigrateEmbedder re-embeds all chunks using a new Ollama model.
func handleMemoryMigrateEmbedder(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	newModel := getString(args, "new_model", "")
	if newModel == "" {
		return nil, fmt.Errorf("new_model is required")
	}
	result, err := h.Engine.MigrateEmbedder(ctx, newModel)
	if err != nil {
		return nil, err
	}
	return toolResult(result)
}

// handleMemoryExportAll exports all memories to markdown files in output_path.
func handleMemoryExportAll(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	if cfg.DataDir == "" {
		return nil, fmt.Errorf("file operations require --data-dir / ENGRAM_DATA_DIR to be set")
	}
	args := req.GetArguments()
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	rawPath := getString(args, "output_path", "./memory-export")
	outputPath, err := SafePath(cfg.DataDir, rawPath)
	if err != nil {
		return nil, fmt.Errorf("invalid output_path: %w", err)
	}
	memories, err := h.Engine.List(ctx, nil, nil, nil, 10_000, 0)
	if err != nil {
		return nil, err
	}
	if err := markdown.Export(memories, outputPath); err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"exported": len(memories), "path": outputPath})
}

// handleMemoryImportClaudeMD imports a CLAUDE.md file as sectioned memories.
func handleMemoryImportClaudeMD(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	if cfg.DataDir == "" {
		return nil, fmt.Errorf("file operations require --data-dir / ENGRAM_DATA_DIR to be set")
	}
	args := req.GetArguments()
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	rawPath := getString(args, "path", "")
	if rawPath == "" {
		return nil, fmt.Errorf("path is required")
	}
	safePath, err := SafePath(cfg.DataDir, rawPath)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}
	memories, err := markdown.ImportClaudeMD(safePath)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, m := range memories {
		m.Project = project
		if err := h.Engine.Store(ctx, m); err != nil {
			return nil, err
		}
		ids = append(ids, m.ID)
	}
	return toolResult(map[string]any{"imported": len(ids), "ids": ids})
}

// handleMemoryDump is an alias for handleMemoryExportAll.
func handleMemoryDump(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	return handleMemoryExportAll(ctx, pool, req, cfg)
}

// handleMemoryIngest reads markdown files from a directory and stores each as a memory.
func handleMemoryIngest(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	if cfg.DataDir == "" {
		return nil, fmt.Errorf("file operations require --data-dir / ENGRAM_DATA_DIR to be set")
	}
	args := req.GetArguments()
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	rawPath := getString(args, "path", "")
	if rawPath == "" {
		return nil, fmt.Errorf("path is required")
	}
	safePath, err := SafePath(cfg.DataDir, rawPath)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}
	memories, err := markdown.Ingest(safePath)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, m := range memories {
		m.Project = project
		if err := h.Engine.Store(ctx, m); err != nil {
			return nil, err
		}
		ids = append(ids, m.ID)
	}
	return toolResult(map[string]any{"ingested": len(ids), "ids": ids})
}

// handleMemoryReason recalls memories relevant to a question and uses Claude to
// synthesize a grounded answer from those memories.
func handleMemoryReason(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
	question := getString(args, "question", "")
	topK := getInt(args, "top_k", 10)
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

	// Extract the Memory pointer from each search result.
	memories := make([]*types.Memory, 0, len(results))
	for _, r := range results {
		if r.Memory != nil {
			memories = append(memories, r.Memory)
		}
	}

	answer, err := cfg.claudeClient.ReasonOverMemories(ctx, question, memories)
	if err != nil {
		return nil, fmt.Errorf("reason: %w", err)
	}

	memoryIDs := make([]string, 0, len(memories))
	for _, m := range memories {
		memoryIDs = append(memoryIDs, m.ID)
	}

	out := map[string]any{
		"answer":        answer,
		"memories_used": len(memories),
		"memory_ids":    memoryIDs,
	}
	data, _ := json.Marshal(out)
	return mcpgo.NewToolResultText(string(data)), nil
}
