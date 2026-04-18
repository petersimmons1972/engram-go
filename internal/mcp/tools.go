package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/claude"
	consolidatepkg "github.com/petersimmons1972/engram/internal/consolidate"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/markdown"
	"github.com/petersimmons1972/engram/internal/rag"
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
	DataDir string
	// RecallDefaultMode controls the default recall response format.
	// "" or "full" returns complete SearchResults; "handle" returns lightweight
	// Handle references. Set via ENGRAM_RECALL_DEFAULT_MODE env var.
	RecallDefaultMode string
	// FetchMaxBytes caps the content returned by memory_fetch detail=full.
	// Defaults to 65536 (64 KB). Set via ENGRAM_FETCH_MAX_BYTES env var.
	FetchMaxBytes int
	// ExploreMaxIters caps memory_explore loop iterations (default 5).
	ExploreMaxIters int
	// ExploreMaxWorkers bounds FanOutReason concurrency (default 8).
	ExploreMaxWorkers int
	// ExploreTokenBudget caps cumulative scoring-call tokens (default 20000).
	ExploreTokenBudget int
	// MaxDocumentBytes is the Tier-1 streaming cap: content up to this size is
	// chunked+embedded inline. Defaults to 8 MiB. Set via
	// ENGRAM_MAX_DOCUMENT_BYTES env var.
	MaxDocumentBytes int
	// RawDocumentMaxBytes is the Tier-2 raw-storage cap: content up to this
	// size is stored in the documents table as a handle-referenced blob.
	// Above this size, ingestion is refused. Defaults to 50 MiB. Set via
	// ENGRAM_RAW_DOCUMENT_MAX_BYTES env var.
	RawDocumentMaxBytes int
	// RAGMaxTokens caps the context window assembled for memory_ask prompt
	// synthesis. Defaults to 4096. Set via ENGRAM_RAG_MAX_TOKENS env var.
	RAGMaxTokens int
	// AllowRFC1918SetupToken extends /setup-token access to RFC1918 private
	// addresses (10.x, 172.16-31.x, 192.168.x) in addition to loopback.
	// Required for Docker setups where the host appears as a bridge IP.
	// Set via ENGRAM_SETUP_TOKEN_ALLOW_RFC1918=1.
	AllowRFC1918SetupToken bool
	claudeClient *claude.Client // set via Server.SetClaudeClient
}

// backendFetcher is the narrow interface required by execFetch.
// Satisfied by db.Backend; declared separately so tests can inject a stub.
type backendFetcher interface {
	GetMemory(ctx context.Context, id string) (*types.Memory, error)
	GetChunksForMemory(ctx context.Context, id string) ([]*types.Chunk, error)
}

// execFetch is the testable core of handleMemoryFetch.
// detail: "summary" | "chunk" | "full"
// requestedChunkIDs: when non-empty, only those chunk IDs are returned (chunk mode only).
// maxBytes: byte cap applied to content in full mode; 0 means no cap.
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
	project := getString(args, "project", "default")
	id := getString(args, "id", "")
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	detail := getString(args, "detail", "summary")
	chunkIDs := toStringSlice(args["chunk_ids"])
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

// toolResult marshals v to JSON and wraps it in an MCP text result.
func toolResult(v any) (*mcpgo.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return mcpgo.NewToolResultText(string(b)), nil
}

// extractResultID pulls the "id" field from a toolResult JSON payload.
// Returns ("", false) if the result is nil or the id field is absent/non-string.
func extractResultID(result *mcpgo.CallToolResult) (string, bool) {
	if result == nil || len(result.Content) == 0 {
		return "", false
	}
	// Content[0] is a TextContent whose Text is the JSON payload.
	text, ok := result.Content[0].(mcpgo.TextContent)
	if !ok {
		return "", false
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(text.Text), &m); err != nil {
		return "", false
	}
	id, ok := m["id"].(string)
	return id, ok && id != ""
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
			return int(math.Round(n))
		case int:
			return n
		}
	}
	return def
}

// getFloat extracts a float64 arg with a fallback.
func getFloat(args map[string]any, key string, def float64) float64 {
	if v, ok := args[key]; ok {
		if f, ok := v.(float64); ok {
			return f
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
// Applies per-tag and count limits to prevent tag injection attacks (#149).
const (
	maxTagCount  = 50
	maxTagLength = 256
)

func toStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		s, ok := item.(string)
		if !ok {
			continue
		}
		if len(result) >= maxTagCount {
			break // silently drop excess tags
		}
		if len(s) > maxTagLength {
			s = s[:maxTagLength] // truncate oversized tag
		}
		result = append(result, s)
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
	if len(content) > types.MaxContentLength {
		return nil, fmt.Errorf("content exceeds max length %d bytes", types.MaxContentLength)
	}
	memType := getString(args, "memory_type", types.MemoryTypeContext)
	if !types.ValidateMemoryType(memType) {
		return nil, fmt.Errorf("invalid memory_type %q; valid values: decision, pattern, error, context, architecture, preference", memType)
	}
	importance := getInt(args, "importance", 2)
	if importance < 0 || importance > 4 {
		return nil, fmt.Errorf("importance must be 0–4, got %d", importance)
	}
	m := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     content,
		MemoryType:  memType,
		Project:     project,
		Importance:  importance,
		Tags:        toStringSlice(args["tags"]),
		Immutable:   getBool(args, "immutable", false),
		StorageMode: "focused",
	}
	if err := h.Engine.Store(ctx, m); err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"id": m.ID, "status": "stored"})
}

// handleMemoryStoreDocument stores a document-mode memory, auto-selecting a
// storage tier based on content size:
//
//   - ≤ 500 KB (types.MaxContentLength): content is stored verbatim as
//     Memory.Content and chunked inline (legacy behaviour).
//   - ≤ MaxDocumentBytes (default 8 MiB): a synopsis is stored as
//     Memory.Content and chunks are produced from the full body.
//   - ≤ RawDocumentMaxBytes (default 50 MiB): the full body lands in the
//     documents table; Memory.Content is a synopsis referencing document_id.
//   - > RawDocumentMaxBytes: refused.
func handleMemoryStoreDocument(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
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
	memType := getString(args, "memory_type", types.MemoryTypeContext)
	if !types.ValidateMemoryType(memType) {
		return nil, fmt.Errorf("invalid memory_type %q; valid values: decision, pattern, error, context, architecture, preference", memType)
	}
	importance := getInt(args, "importance", 2)
	if importance < 0 || importance > 4 {
		return nil, fmt.Errorf("importance must be 0–4, got %d", importance)
	}
	maxDoc, rawMax := configOrDefaults(cfg)
	m := &types.Memory{
		ID:         types.NewMemoryID(),
		MemoryType: memType,
		Project:    project,
		Importance: importance,
		Tags:       toStringSlice(args["tags"]),
		Immutable:  getBool(args, "immutable", false),
	}
	engine := h.Engine
	deps := storeDocumentDeps{
		engine:  engineStorerAdapter{store: engine.StoreWithRawBody},
		backend: backendDocumentAdapter{b: engine.Backend()},
	}
	out, err := execStoreDocument(ctx, deps, m, content, maxDoc, rawMax)
	if err != nil {
		return nil, err
	}
	return toolResult(out)
}

// handleMemoryStoreBatch stores multiple memories in a single atomic call (#115).
// All items are validated first; then embeddings are computed; then all writes
// are committed in one transaction — either all succeed or none do.
func handleMemoryStoreBatch(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	const maxBatchItems = 100 // guard against CPU/DB overload (#144)
	items, _ := args["memories"].([]any)
	if len(items) == 0 {
		return toolResult(map[string]any{"ids": []string{}, "count": 0, "warning": "no memories provided"})
	}
	if len(items) > maxBatchItems {
		return nil, fmt.Errorf("memory_store_batch: too many items (%d > max %d)", len(items), maxBatchItems)
	}

	// Validate all items before touching the database.
	var validErrs []string
	var memories []*types.Memory
	for idx, item := range items {
		mmap, ok := item.(map[string]any)
		if !ok {
			validErrs = append(validErrs, fmt.Sprintf("item %d: expected object, got %T", idx, item))
			continue
		}
		content := getString(mmap, "content", "")
		if content == "" {
			validErrs = append(validErrs, fmt.Sprintf("item %d: content is empty", idx))
			continue
		}
		if len(content) > types.MaxContentLength {
			validErrs = append(validErrs, fmt.Sprintf("item %d: content exceeds max length %d bytes", idx, types.MaxContentLength))
			continue
		}
		memType := getString(mmap, "memory_type", types.MemoryTypeContext)
		if !types.ValidateMemoryType(memType) {
			validErrs = append(validErrs, fmt.Sprintf("item %d: invalid memory_type %q", idx, memType))
			continue
		}
		importance := getInt(mmap, "importance", 2)
		if importance < 0 || importance > 4 {
			validErrs = append(validErrs, fmt.Sprintf("item %d: importance must be 0–4, got %d", idx, importance))
			continue
		}
		memories = append(memories, &types.Memory{
			ID:          types.NewMemoryID(),
			Content:     content,
			MemoryType:  memType,
			Project:     project,
			Importance:  importance,
			Tags:        toStringSlice(mmap["tags"]),
			StorageMode: "focused",
		})
	}

	if len(validErrs) > 0 {
		return toolResult(map[string]any{
			"ids":    []string{},
			"count":  0,
			"errors": validErrs,
		})
	}

	if err := h.Engine.StoreBatch(ctx, memories); err != nil {
		return nil, err
	}

	ids := make([]string, len(memories))
	for i, m := range memories {
		ids[i] = m.ID
	}
	return toolResult(map[string]any{"ids": ids, "count": len(ids)})
}

// handleMemoryRecall performs semantic recall against one or more project engines.
// When the optional "projects" argument is a non-empty slice, a federated fan-out
// is performed across all named projects and results are merged by score.
// Pass cfg to enable optional Claude re-ranking (single-project only).
func handleMemoryRecall(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
	query := getString(args, "query", "")
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	topK := getInt(args, "top_k", 10)
	if topK < 1 || topK > 100 {
		topK = 10
	}
	detail := getString(args, "detail", "summary")
	includeConflicts := getBool(args, "include_conflicts", false)
	mode := getString(args, "mode", cfg.RecallDefaultMode)

	// Federated path: "projects" overrides the single-project recall.
	projectNames := toStringSlice(args["projects"])
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
func handleMemoryProjects(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	// Use any project to get a backend connection for the cross-project query.
	anchorProject := getString(args, "project", "default")
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
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	srcID := getString(args, "source_id", "")
	dstID := getString(args, "target_id", "")
	if srcID == "" {
		return nil, fmt.Errorf("source_id is required")
	}
	if dstID == "" {
		return nil, fmt.Errorf("target_id is required")
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
func handleMemoryList(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
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
	if math.IsNaN(strength) || math.IsInf(strength, 0) || strength < 0 || strength > 1.0 {
		return nil, fmt.Errorf("strength must be between 0 and 1, got %v", strength)
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
		n := types.ValidateImportance(int(v))
		importance = &n
	}
	updated, err := h.Engine.Correct(ctx, id, content, toStringSlice(args["tags"]), importance)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, fmt.Errorf("memory %q not found or has been soft-deleted", id)
	}
	return toolResult(updated)
}

// handleMemoryForget soft-deletes a memory by ID (sets valid_to=NOW(), preserves history).
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
	reason := getString(args, "reason", "")
	deleted, err := h.Engine.Forget(ctx, id, reason)
	if err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"deleted": deleted, "memory_id": id})
}

// handleMemoryHistory returns the version chain for a memory.
func handleMemoryHistory(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
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
	history, err := h.Engine.MemoryHistory(ctx, id)
	if err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"memory_id": id, "versions": history, "count": len(history)})
}

// handleMemoryTimeline recalls memories that were active at a given point in time.
func handleMemoryTimeline(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	asOfStr := getString(args, "as_of", "")
	if asOfStr == "" {
		return nil, fmt.Errorf("as_of is required (RFC3339 timestamp)")
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
		return nil, fmt.Errorf("%w (project=%q — did you mean a different project?)", err, project)
	}
	return toolResult(map[string]any{"status": "summarized", "memory_id": id})
}

// handleMemoryResummarize clears all summaries for a project so the background
// worker regenerates them with the current model on its next tick (within 60s).
func handleMemoryResummarize(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
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
// When event_id is provided (returned by memory_recall), retrieval outcome
// tracking is updated in addition to the standard edge boost.
func handleMemoryFeedback(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
	ids := toStringSlice(args["memory_ids"])
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
	project := getString(args, "project", "default")
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

// handleMemorySleep runs the full sleep-consolidation cycle (Feature 3).
// cfg is passed so the handler can read OllamaURL for the LLM second pass.
func handleMemorySleep(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
	minSim := getFloat(args, "min_similarity", 0.7)
	limit := getInt(args, "limit", 500)
	if limit < 1 || limit > 5000 {
		limit = 500
	}
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	// Optional LLM contradiction detection params (opt-in, default off).
	// OllamaURL comes from server config; model and call cap are per-request.
	llmDetect := getBool(args, "llm_contradiction_detection", false)
	llmModel := getString(args, "llm_model", "llama3.2:3b")
	llmMaxCalls := getInt(args, "llm_max_calls", 10)
	autoSupersede := getBool(args, "auto_supersede", false)

	contradictionLimit := getInt(args, "contradiction_limit", 0) // 0 → falls back to limit

	runner := consolidatepkg.NewRunner(h.Engine.Backend(), project, h.Engine.Embedder())
	stats, err := runner.RunAll(ctx, consolidatepkg.RunOptions{
		InferRelationshipsMinSimilarity: minSim,
		InferRelationshipsLimit:         limit,
		ContradictionDetectionLimit:     contradictionLimit,
		LLMContradictionDetection:       llmDetect,
		OllamaURL:                       cfg.OllamaURL,
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
	skipped := 0
	for _, m := range memories {
		if isOperationalConfig(m.Content) {
			skipped++
			continue
		}
		m.Project = project
		if err := h.Engine.Store(ctx, m); err != nil {
			return nil, err
		}
		ids = append(ids, m.ID)
	}
	return toolResult(map[string]any{"imported": len(ids), "skipped": skipped, "ids": ids})
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
	var ingested, skipped int
	for _, m := range memories {
		if isOperationalConfig(m.Content) {
			skipped++
			continue
		}
		m.Project = project
		contentHash := db.ContentHash(m.Content)
		exists, err := h.Engine.Backend().ExistsWithContentHash(ctx, project, contentHash)
		if err != nil {
			return nil, fmt.Errorf("dedup check: %w", err)
		}
		if exists {
			skipped++
			slog.Debug("handleMemoryIngest: skipping duplicate", "hash", contentHash[:8], "project", project)
			continue
		}
		if err := h.Engine.Store(ctx, m); err != nil {
			return nil, err
		}
		ids = append(ids, m.ID)
		ingested++
	}
	return toolResult(map[string]any{"ingested": ingested, "skipped": skipped, "ids": ids})
}

// handleMemoryEpisodeStart creates a new episode for a project.
func handleMemoryEpisodeStart(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
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
	project := getString(args, "project", "default")
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
	project := getString(args, "project", "default")
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
	project := getString(args, "project", "default")
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

// buildEvidenceMap gathers the EvidenceMap for a set of recalled memories by
// fetching their relationships from the backend (best-effort: errors are ignored
// so the caller still gets an answer even if relationship queries fail).
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
func handleMemoryDiagnose(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
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
		"conflicts":          ev.Conflicts,
		"invalidated_sources": ev.InvalidatedSources,
		"memories_used":      len(ev.Memories),
	})
}

// handleMemoryReason recalls memories relevant to a question and uses Claude to
// synthesize a grounded, conflict-aware answer.
func handleMemoryReason(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "default")
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
	return f.backend.GetMemory(ctx, id)
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
	scope.Tags = toStringSlice(scopeMap["tags"])
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
	project := getString(args, "project", "default")
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
	project := getString(args, "project", "")
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
		"answer":               result.Answer,
		"citations":            result.Citations,
		"context_tokens_used":  result.ContextTokensUsed,
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
func handleMemoryQuickStore(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	// Build a merged copy so we never mutate the caller's map.
	merged := make(map[string]any, len(args)+2)
	for k, v := range args {
		merged[k] = v
	}
	// Defaults are pinned here independently of handleMemoryStore's own defaults
	// so this wrapper's contract stays stable even if upstream defaults drift.
	if _, ok := merged["memory_type"]; !ok {
		merged["memory_type"] = "context"
	}
	if _, ok := merged["importance"]; !ok {
		merged["importance"] = 2
	}

	req2 := req
	req2.Params.Arguments = merged
	return handleMemoryStore(ctx, pool, req2)
}

// handleMemoryQuery is a simplified front door for handleMemoryRecall.
// It maps the caller-friendly "limit" parameter to "top_k", defaulting to 5
// when neither is provided, then delegates to handleMemoryRecall.
// The original args map is never mutated — a copy is used.
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

	project := getString(args, "project", "default")
	memoryID := getString(args, "memory_id", "")
	if memoryID == "" {
		return nil, fmt.Errorf("memory_id is required")
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
