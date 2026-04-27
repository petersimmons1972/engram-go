package mcp

import (
	"context"
	"fmt"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/types"
)

// storeTimeout is the maximum time allowed for a single MCP store operation.
// The store path is a pure DB write (Ollama was decoupled in commit 946fea2),
// so it should complete in milliseconds; 10s gives headroom while ensuring
// fast failure when Postgres is unavailable.
//
// Declared as a var (not const) so tests can substitute a shorter duration.
var storeTimeout = 10 * time.Second

func handleMemoryStore(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
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
	if err := validateContent(content); err != nil {
		return nil, fmt.Errorf("content: %w", err)
	}
	memType := getString(args, "memory_type", types.MemoryTypeContext)
	if !types.ValidateMemoryType(memType) {
		return nil, fmt.Errorf("invalid memory_type %q; valid values: decision, pattern, error, context, architecture, preference", memType)
	}
	importance := getInt(args, "importance", 2)
	if importance < 0 || importance > 4 {
		return nil, fmt.Errorf("importance must be 0–4, got %d", importance)
	}
	tags, err := toStringSlice(args["tags"])
	if err != nil {
		return nil, fmt.Errorf("tags: %w", err)
	}
	// Resolve the episode ID: explicit arg wins; fall back to context injection
	// from the auto-episode session hook (#356).
	episodeID := episodeIDFromContextOrArgs(ctx, args)
	m := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     content,
		MemoryType:  memType,
		Project:     project,
		Importance:  importance,
		Tags:        tags,
		Immutable:   getBool(args, "immutable", false),
		StorageMode: "focused",
		EpisodeID:   episodeID,
	}
	storeCtx, storeCancel := context.WithTimeout(ctx, storeTimeout)
	defer storeCancel()
	if err := h.Engine.Store(storeCtx, m); err != nil {
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
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	content, _ := args["content"].(string)
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}
	if err := validateContent(content); err != nil {
		return nil, fmt.Errorf("content: %w", err)
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
	docTags, err := toStringSlice(args["tags"])
	if err != nil {
		return nil, fmt.Errorf("tags: %w", err)
	}
	m := &types.Memory{
		ID:         types.NewMemoryID(),
		MemoryType: memType,
		Project:    project,
		Importance: importance,
		Tags:       docTags,
		Immutable:  getBool(args, "immutable", false),
	}
	engine := h.Engine
	deps := storeDocumentDeps{
		engine:  engineStorerAdapter{store: engine.StoreWithRawBody},
		backend: backendDocumentAdapter{b: engine.Backend()},
	}
	storeCtx, storeCancel := context.WithTimeout(ctx, storeTimeout)
	defer storeCancel()
	out, err := execStoreDocument(storeCtx, deps, m, content, maxDoc, rawMax)
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
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
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

	// Resolve the batch-level episode ID: explicit batch arg wins; fall back to
	// context injection from the auto-episode session hook (#356). Per-item
	// episode_id overrides this default for that item.
	batchEpisodeID := episodeIDFromContextOrArgs(ctx, args)

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
		if contentErr := validateContent(content); contentErr != nil {
			validErrs = append(validErrs, fmt.Sprintf("item %d: content: %v", idx, contentErr))
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
		itemTags, tagErr := toStringSlice(mmap["tags"])
		if tagErr != nil {
			validErrs = append(validErrs, fmt.Sprintf("item %d: tags: %v", idx, tagErr))
			continue
		}
		// Per-item episode_id wins over the batch-level default.
		itemEpisodeID := getString(mmap, "episode_id", batchEpisodeID)
		memories = append(memories, &types.Memory{
			ID:          types.NewMemoryID(),
			Content:     content,
			MemoryType:  memType,
			Project:     project,
			Importance:  importance,
			Tags:        itemTags,
			StorageMode: "focused",
			EpisodeID:   itemEpisodeID,
		})
	}

	if len(validErrs) > 0 {
		return toolResult(map[string]any{
			"ids":    []string{},
			"count":  0,
			"errors": validErrs,
		})
	}

	storeCtx, storeCancel := context.WithTimeout(ctx, storeTimeout)
	defer storeCancel()
	if err := h.Engine.StoreBatch(storeCtx, memories); err != nil {
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

func handleMemoryCorrect(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
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
	correctTags, err := toStringSlice(args["tags"])
	if err != nil {
		return nil, fmt.Errorf("tags: %w", err)
	}
	storeCtx, storeCancel := context.WithTimeout(ctx, storeTimeout)
	defer storeCancel()
	updated, err := h.Engine.Correct(storeCtx, id, content, correctTags, importance)
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
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
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
