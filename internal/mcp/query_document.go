package mcp

import (
	"context"
	"fmt"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/petersimmons1972/engram/internal/types"
)

// queryDocumentDeps bundles the collaborators execQueryDocument needs. Kept
// as a narrow interface pair so tests can inject stubs without a live
// PostgresBackend or SearchEngine.
type queryDocumentDeps struct {
	// getMemory returns the memory row for id or (nil, nil) if not found.
	getMemory func(ctx context.Context, id string) (*types.Memory, error)
	// getDocument returns raw document content for docID, or "" if not found.
	getDocument func(ctx context.Context, docID string) (string, error)
	// recallWithinMemory is used to reconstruct content from chunks when the
	// parent memory has no DocumentID (Tier-1 path) or when Semantic is set.
	// Returned memories carry chunk text as Content.
	recallWithinMemory func(ctx context.Context, query, memoryID string, topK int, detail string) ([]*types.Memory, error)
	// claudeClient is the LLM client used for answer synthesis. Must be non-nil.
	claudeClient *claude.Client
}

// execQueryDocument is the testable core of handleMemoryQueryDocument. It
// resolves the content source (Tier-2 document blob vs Tier-1 chunk
// reconstruction vs explicit semantic recall), then delegates to
// claude.QueryDocument for span extraction + answer synthesis.
func execQueryDocument(ctx context.Context, deps queryDocumentDeps, q claude.DocumentQuery) (*claude.DocumentQueryResult, error) {
	if q.Project == "" {
		return nil, fmt.Errorf("project is required")
	}
	if q.MemoryID == "" {
		return nil, fmt.Errorf("memory_id is required")
	}
	if q.Question == "" {
		return nil, fmt.Errorf("question is required")
	}
	if deps.claudeClient == nil {
		return nil, fmt.Errorf("memory_query_document requires a Claude API key — set ANTHROPIC_API_KEY")
	}

	mem, err := deps.getMemory(ctx, q.MemoryID)
	if err != nil {
		return nil, fmt.Errorf("get memory: %w", err)
	}
	if mem == nil {
		return nil, fmt.Errorf("memory %q not found", q.MemoryID)
	}

	content, err := resolveDocumentContent(ctx, deps, mem, q)
	if err != nil {
		return nil, err
	}

	return claude.QueryDocument(ctx, deps.claudeClient, content, q)
}

// resolveDocumentContent selects the content source based on tier + options.
// Priority:
//  1. Tier-2 raw document (mem.DocumentID set) — load from documents table.
//  2. Semantic recall (q.Semantic=true) — vector search within memory chunks.
//  3. Tier-1 fallback — reuse RecallWithinMemory with the question as the
//     implicit query. If the caller supplied filter terms, use those to
//     bias the recall toward the right chunks.
func resolveDocumentContent(ctx context.Context, deps queryDocumentDeps, mem *types.Memory, q claude.DocumentQuery) (string, error) {
	if mem.DocumentID != "" {
		doc, err := deps.getDocument(ctx, mem.DocumentID)
		if err != nil {
			return "", fmt.Errorf("get document: %w", err)
		}
		if doc == "" {
			// Document row missing — fall through to chunks below.
			return recallChunks(ctx, deps, q)
		}
		return doc, nil
	}
	// For Tier-0/Tier-1: prefer stored content unless caller explicitly
	// wants semantic chunk recall.
	if !q.Semantic {
		return mem.Content, nil
	}
	return recallChunks(ctx, deps, q)
}

func recallChunks(ctx context.Context, deps queryDocumentDeps, q claude.DocumentQuery) (string, error) {
	if deps.recallWithinMemory == nil {
		return "", fmt.Errorf("no chunk recall available for memory %q", q.MemoryID)
	}
	chunks, err := deps.recallWithinMemory(ctx, q.Question, q.MemoryID, 20, "full")
	if err != nil {
		return "", fmt.Errorf("recall within memory: %w", err)
	}
	if len(chunks) == 0 {
		return "", nil
	}
	var buf []byte
	for i, c := range chunks {
		if i > 0 {
			buf = append(buf, '\n')
		}
		buf = append(buf, c.Content...)
	}
	return string(buf), nil
}

// handleMemoryQueryDocument implements the memory_query_document MCP tool.
// See spec A5: query a large document stored in memory using regex/substring
// matching or semantic search. Returns relevant spans and an AI-synthesized
// answer grounded in those spans.
func handleMemoryQueryDocument(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project := getString(args, "project", "")
	memoryID := getString(args, "memory_id", "")
	question := getString(args, "question", "")

	if cfg.claudeClient == nil {
		return nil, fmt.Errorf("memory_query_document requires a Claude API key — set ANTHROPIC_API_KEY")
	}

	// filter is a nested object { regex: string, substrings: []string }.
	var filterRegex string
	var filterSubs []string
	if rawFilter, ok := args["filter"]; ok {
		if fmap, ok := rawFilter.(map[string]any); ok {
			filterRegex = getString(fmap, "regex", "")
			filterSubs = toStringSlice(fmap["substrings"])
		}
	}

	q := claude.DocumentQuery{
		Project:     project,
		MemoryID:    memoryID,
		Question:    question,
		FilterRegex: filterRegex,
		FilterSubs:  filterSubs,
		WindowChars: getInt(args, "window_chars", 4000),
		Semantic:    getBool(args, "semantic", false),
		TokenBudget: getInt(args, "token_budget", 6000),
	}

	// Early validation so we don't even hit the engine pool for obviously-bad input.
	if project == "" {
		return nil, fmt.Errorf("project is required")
	}
	if memoryID == "" {
		return nil, fmt.Errorf("memory_id is required")
	}
	if question == "" {
		return nil, fmt.Errorf("question is required")
	}

	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	engine := h.Engine
	backend := engine.Backend()

	deps := queryDocumentDeps{
		getMemory:          backend.GetMemory,
		getDocument:        backend.GetDocument,
		recallWithinMemory: engine.RecallWithinMemory,
		claudeClient:       cfg.claudeClient,
	}

	res, err := execQueryDocument(ctx, deps, q)
	if err != nil {
		return nil, err
	}
	return toolResult(res)
}
