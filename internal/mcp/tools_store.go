package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/parse"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
)

// storeTimeout is the maximum time allowed for a single MCP store operation.
// The store path is a pure DB write (Ollama was decoupled in commit 946fea2),
// so it should complete in milliseconds; 10s gives headroom while ensuring
// fast failure when Postgres is unavailable.
//
// Declared as a var (not const) so tests can substitute a shorter duration.
var storeTimeout = 10 * time.Second

func handleMemoryStore(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
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
		return mcpgo.NewToolResultError("content is required"), nil
	}
	if len(content) > types.MaxContentLength {
		return mcpgo.NewToolResultError(fmt.Sprintf("content exceeds max length %d bytes", types.MaxContentLength)), nil
	}
	if err := validateContent(content); err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("content: %v", err)), nil
	}
	memType := getString(args, "memory_type", types.MemoryTypeContext)
	if !types.ValidateMemoryType(memType) {
		return mcpgo.NewToolResultError(fmt.Sprintf("invalid memory_type %q; valid values: decision, pattern, error, context, architecture, preference", memType)), nil
	}
	// Auto-tag: when memory_type was not explicitly provided by the caller,
	// detect preference-expressing content and override to "preference" (#364).
	if _, hasExplicitType := args["memory_type"]; !hasExplicitType {
		if search.IsPreferenceContent(content) {
			memType = types.MemoryTypePreference
		}
	}
	importance := getInt(args, "importance", 2)
	if importance < 0 || importance > 4 {
		return mcpgo.NewToolResultError(fmt.Sprintf("importance must be 0–4, got %d", importance)), nil
	}
	tags, err := toStringSlice(args["tags"])
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("tags: %v", err)), nil
	}
	// Validate optional pattern_confidence field.
	var patternConfidence *float64
	if raw, exists := args["pattern_confidence"]; exists && raw != nil {
		v, ok := raw.(float64)
		if !ok {
			return mcpgo.NewToolResultError(fmt.Sprintf("pattern_confidence must be a number, got %T", raw)), nil
		}
		// Error (not clamp) is intentional — see ValidatePatternConfidence godoc.
		validated, validErr := types.ValidatePatternConfidence(v)
		if validErr != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("pattern_confidence: %v", validErr)), nil
		}
		patternConfidence = &validated
	}
	// Resolve the episode ID: explicit arg wins; fall back to context injection
	// from the auto-episode session hook (#356).
	episodeID := episodeIDFromContextOrArgs(ctx, args)
	m := &types.Memory{
		ID:                types.NewMemoryID(),
		Content:           content,
		MemoryType:        memType,
		Project:           project,
		Importance:        importance,
		Tags:              tags,
		Immutable:         getBool(args, "immutable", false),
		StorageMode:       "focused",
		EpisodeID:         episodeID,
		ValidFrom:         parse.ParseDateTag(tags),
		PatternConfidence: patternConfidence,
	}
	storeCtx, storeCancel := context.WithTimeout(ctx, storeTimeout)
	defer storeCancel()
	if err := h.Engine.Store(storeCtx, m); err != nil {
		// Fast-fail on permanent embedder mismatch without propagating as Go error.
		var pe *embed.PermanentError
		if errors.As(err, &pe) {
			body, _ := json.Marshal(pe)
			return mcpgo.NewToolResultError(string(body)), nil
		}
		return nil, err
	}

	// Preference extraction: when the memory is not already typed as a preference,
	// run the extractor and store each discovered fact as a separate short preference
	// memory. This gives the preference boost a correctly-typed target to fire on.
	// Non-fatal: extraction errors are logged and swallowed so the primary store succeeds.
	if extractor := h.Engine.PreferenceExtractor; extractor != nil && memType != types.MemoryTypePreference {
		if facts, extractErr := extractor.Extract(ctx, content); extractErr == nil {
			for _, fact := range facts {
				if strings.TrimSpace(fact) == "" {
					continue
				}
				factMem := &types.Memory{
					ID:          types.NewMemoryID(),
					Content:     fact,
					MemoryType:  types.MemoryTypePreference,
					Project:     project,
					Importance:  1,
					Tags:        []string{"extracted-preference"},
					StorageMode: "focused",
				}
				if storeErr := h.Engine.Store(ctx, factMem); storeErr != nil {
					slog.Debug("preference extraction: store fact failed",
						"parent_id", m.ID, "fact", fact, "err", storeErr)
				}
			}
		}
	}

	// Probe embedder health and add degraded field to response.
	ok, reason := cfg.EmbedderHealth.Snapshot(ctx)
	degraded := map[string]any{"embed": !ok, "reason": reason}

	return toolResult(map[string]any{
		"id":        m.ID,
		"status":    "stored",
		"degraded":  degraded,
	})
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
		return mcpgo.NewToolResultError("content is required"), nil
	}
	if err := validateContent(content); err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("content: %v", err)), nil
	}
	memType := getString(args, "memory_type", types.MemoryTypeContext)
	if !types.ValidateMemoryType(memType) {
		return mcpgo.NewToolResultError(fmt.Sprintf("invalid memory_type %q; valid values: decision, pattern, error, context, architecture, preference", memType)), nil
	}
	importance := getInt(args, "importance", 2)
	if importance < 0 || importance > 4 {
		return mcpgo.NewToolResultError(fmt.Sprintf("importance must be 0–4, got %d", importance)), nil
	}
	maxDoc, rawMax := configOrDefaults(cfg)
	docTags, err := toStringSlice(args["tags"])
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("tags: %v", err)), nil
	}
	m := &types.Memory{
		ID:         types.NewMemoryID(),
		MemoryType: memType,
		Project:    project,
		Importance: importance,
		Tags:       docTags,
		Immutable:  getBool(args, "immutable", false),
		EpisodeID:  episodeIDFromContextOrArgs(ctx, args),
		ValidFrom:  parse.ParseDateTag(docTags),
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
		// Fast-fail on permanent embedder mismatch without propagating as Go error.
		var pe *embed.PermanentError
		if errors.As(err, &pe) {
			body, _ := json.Marshal(pe)
			return mcpgo.NewToolResultError(string(body)), nil
		}
		return nil, err
	}

	// Probe embedder health and add degraded field to response.
	ok, reason := cfg.EmbedderHealth.Snapshot(ctx)
	degraded := map[string]any{"embed": !ok, "reason": reason}
	out["degraded"] = degraded

	return toolResult(out)
}

// handleMemoryStoreBatch stores multiple memories in a single atomic call (#115).
// All items are validated first; then embeddings are computed; then all writes
// are committed in one transaction — either all succeed or none do.
func handleMemoryStoreBatch(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
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
		// Validate optional per-item pattern_confidence field — mirrors handleMemoryStore.
		var itemPatternConfidence *float64
		if raw, exists := mmap["pattern_confidence"]; exists && raw != nil {
			v, ok := raw.(float64)
			if !ok {
				validErrs = append(validErrs, fmt.Sprintf("item %d: pattern_confidence must be a number, got %T", idx, raw))
				continue
			}
			validated, validErr := types.ValidatePatternConfidence(v)
			if validErr != nil {
				validErrs = append(validErrs, fmt.Sprintf("item %d: pattern_confidence: %v", idx, validErr))
				continue
			}
			itemPatternConfidence = &validated
		}
		// Per-item episode_id wins over the batch-level default.
		itemEpisodeID := getString(mmap, "episode_id", batchEpisodeID)
		memories = append(memories, &types.Memory{
			ID:                types.NewMemoryID(),
			Content:           content,
			MemoryType:        memType,
			Project:           project,
			Importance:        importance,
			Tags:              itemTags,
			Immutable:         getBool(mmap, "immutable", false),
			StorageMode:       "focused",
			EpisodeID:         itemEpisodeID,
			ValidFrom:         parse.ParseDateTag(itemTags),
			PatternConfidence: itemPatternConfidence,
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
		// Fast-fail on permanent embedder mismatch without propagating as Go error.
		var pe *embed.PermanentError
		if errors.As(err, &pe) {
			body, _ := json.Marshal(pe)
			return mcpgo.NewToolResultError(string(body)), nil
		}
		return nil, err
	}

	ids := make([]string, len(memories))
	for i, m := range memories {
		ids[i] = m.ID
	}

	// Probe embedder health and add degraded field to response.
	ok, reason := cfg.EmbedderHealth.Snapshot(ctx)
	degraded := map[string]any{"embed": !ok, "reason": reason}

	return toolResult(map[string]any{
		"ids":       ids,
		"count":     len(ids),
		"degraded":  degraded,
	})
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
	errResult, id := requireString(args, "memory_id")
	if errResult != nil {
		return errResult, nil
	}
	var content *string
	if c := getString(args, "content", ""); c != "" {
		content = &c
	}
	var importance *int
	if v, ok := args["importance"].(float64); ok {
		n := int(v)
		if n < minImportanceValue || n > maxImportanceValue {
			return mcpgo.NewToolResultError(fmt.Sprintf("importance must be %d–%d, got %d", minImportanceValue, maxImportanceValue, n)), nil
		}
		importance = &n
	}
	var patternConfidence *float64
	if raw, exists := args["pattern_confidence"]; exists && raw != nil {
		v, ok := raw.(float64)
		if !ok {
			return mcpgo.NewToolResultError(fmt.Sprintf("pattern_confidence must be a number, got %T", raw)), nil
		}
		// Error (not clamp) is intentional — see ValidatePatternConfidence godoc.
		validated, validErr := types.ValidatePatternConfidence(v)
		if validErr != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("pattern_confidence: %v", validErr)), nil
		}
		patternConfidence = &validated
	}
	correctTags, err := toStringSlice(args["tags"])
	if err != nil {
		return nil, fmt.Errorf("tags: %w", err)
	}
	storeCtx, storeCancel := context.WithTimeout(ctx, storeTimeout)
	defer storeCancel()
	updated, err := h.Engine.Correct(storeCtx, id, content, correctTags, importance, patternConfidence)
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
	errResult, id := requireString(args, "memory_id")
	if errResult != nil {
		return errResult, nil
	}
	reason := getString(args, "reason", "")
	deleted, err := h.Engine.Forget(ctx, id, reason)
	if err != nil {
		return nil, err
	}
	return toolResult(map[string]any{"deleted": deleted, "memory_id": id})
}

// handleMemoryHistory returns the version chain for a memory.

// maxQuickStoreContentSize is the maximum content size for quick-store (1 MiB).
const maxQuickStoreContentSize = 1024 * 1024

// maxQuickStoreTags is the maximum number of tags allowed.
const maxQuickStoreTags = 64

// maxQuickStoreTagLength is the maximum length of a single tag.
const maxQuickStoreTagLength = 256

// maxImportanceValue is the maximum allowed importance value.
// Must match the 0–4 range enforced by handleMemoryStore.
const maxImportanceValue = 4

// minImportanceValue is the minimum allowed importance value.
const minImportanceValue = 0

// projectNamePattern validates project names: ^[a-z0-9_-]{1,64}$.
var projectNamePattern = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)

// validateQuickStoreInput validates all fields for quick-store:
// - content: required, max 1 MiB
// - project: must match ^[a-z0-9_-]{1,64}$
// - tags: max 64, each max 256 chars
// - importance: 0–4.
func validateQuickStoreInput(content string, project string, tags []string, importance int) error {
	if len(content) > maxQuickStoreContentSize {
		return fmt.Errorf("content exceeds max size %d bytes", maxQuickStoreContentSize)
	}
	if !projectNamePattern.MatchString(project) {
		return fmt.Errorf("project name must match ^[a-z0-9_-]{1,64}$, got %q", project)
	}
	if len(tags) > maxQuickStoreTags {
		return fmt.Errorf("too many tags (%d > max %d)", len(tags), maxQuickStoreTags)
	}
	for i, tag := range tags {
		if len(tag) > maxQuickStoreTagLength {
			return fmt.Errorf("tag %d exceeds max length %d chars", i, maxQuickStoreTagLength)
		}
	}
	if importance < minImportanceValue || importance > maxImportanceValue {
		return fmt.Errorf("importance must be %d–%d, got %d", minImportanceValue, maxImportanceValue, importance)
	}
	return nil
}

// validateQuickRecallInput validates project and query for quick-recall.
func validateQuickRecallInput(project string, query string) error {
	if !projectNamePattern.MatchString(project) {
		return fmt.Errorf("project name must match ^[a-z0-9_-]{1,64}$, got %q", project)
	}
	if len(query) == 0 {
		return fmt.Errorf("query is required")
	}
	return nil
}

func handleMemoryQuickStore(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
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
	return handleMemoryStore(ctx, pool, req2, cfg)
}

// handleMemoryQuery is a simplified front door for handleMemoryRecall.
// It maps the caller-friendly "limit" parameter to "top_k", defaulting to 5
// when neither is provided, then delegates to handleMemoryRecall.
// The original args map is never mutated — a copy is used.
