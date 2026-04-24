package mcp

import (
	"context"
	"fmt"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/audit"
	"github.com/petersimmons1972/engram/internal/weight"
)

// engineRecallerAdapter adapts the engine pool to the audit.Recaller interface.
// It looks up the engine for the given project and delegates to its Recall method,
// returning only memory IDs (the audit system cares about ranking, not content).
type engineRecallerAdapter struct {
	pool *EnginePool
}

func (a *engineRecallerAdapter) Recall(ctx context.Context, project, query string, topK int) ([]string, error) {
	h, err := a.pool.Get(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("get engine for project %q: %w", project, err)
	}
	if h == nil || h.Engine == nil {
		return nil, fmt.Errorf("no engine available for project %q", project)
	}
	results, err := h.Engine.Recall(ctx, query, topK, "id_only")
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.Memory.ID
	}
	return ids, nil
}

// newAuditWorker creates an AuditWorker using cfg, injecting a test DB querier when
// cfg.testAuditDB is set (avoids needing a real pgxpool.Pool in tests).
func newAuditWorker(pool *EnginePool, cfg Config) *audit.AuditWorker {
	recaller := &engineRecallerAdapter{pool: pool}
	if cfg.testAuditDB != nil {
		return audit.NewAuditWorkerWithDB(cfg.PgPool, cfg.testAuditDB, recaller, cfg.EmbedModel, 24*time.Hour)
	}
	return audit.NewAuditWorker(cfg.PgPool, recaller, cfg.EmbedModel, 24*time.Hour)
}

// handleMemoryAuditAddQuery registers a new canonical query for drift monitoring.
//
// Required args: project, query.
// Optional args: description.
func handleMemoryAuditAddQuery(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	if cfg.PgPool == nil && cfg.testAuditDB == nil {
		return nil, fmt.Errorf("audit tools require a database connection (PgPool not configured)")
	}
	args, _ := req.Params.Arguments.(map[string]any)
	project, err := getProject(args, "")
	if err != nil {
		return nil, err
	}
	if project == "" {
		return nil, fmt.Errorf("project is required")
	}
	query := getString(args, "query", "")
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	description := getString(args, "description", "")

	worker := newAuditWorker(pool, cfg)
	id, err := worker.RegisterQuery(ctx, project, query, description)
	if err != nil {
		return nil, fmt.Errorf("register canonical query: %w", err)
	}
	return toolResult(map[string]any{
		"id":          id,
		"project":     project,
		"query":       query,
		"description": description,
		"status":      "registered",
	})
}

// handleMemoryAuditListQueries lists all canonical queries for a project.
//
// Required args: project (empty returns all queries across all projects).
func handleMemoryAuditListQueries(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	if cfg.PgPool == nil && cfg.testAuditDB == nil {
		return nil, fmt.Errorf("audit tools require a database connection (PgPool not configured)")
	}
	args, _ := req.Params.Arguments.(map[string]any)
	project, err := getProject(args, "")
	if err != nil {
		return nil, err
	}
	if project == "" {
		return nil, fmt.Errorf("project is required")
	}

	worker := newAuditWorker(pool, cfg)
	queries, err := worker.ListQueries(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("list canonical queries: %w", err)
	}

	items := make([]map[string]any, len(queries))
	for i, q := range queries {
		items[i] = map[string]any{
			"id":          q.ID,
			"project":     q.Project,
			"query":       q.Query,
			"description": q.Description,
			"active":      q.Active,
			"created_at":  q.CreatedAt.Format(time.RFC3339),
		}
	}
	return toolResult(map[string]any{
		"project": project,
		"queries": items,
		"count":   len(items),
	})
}

// handleMemoryAuditDeactivateQuery deactivates a canonical query (marks active=false).
//
// Required args: query_id.
func handleMemoryAuditDeactivateQuery(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	if cfg.PgPool == nil && cfg.testAuditDB == nil {
		return nil, fmt.Errorf("audit tools require a database connection (PgPool not configured)")
	}
	args, _ := req.Params.Arguments.(map[string]any)
	queryID := getString(args, "query_id", "")
	if queryID == "" {
		return nil, fmt.Errorf("query_id is required")
	}

	worker := newAuditWorker(pool, cfg)
	if err := worker.DeactivateQuery(ctx, queryID); err != nil {
		return nil, fmt.Errorf("deactivate query: %w", err)
	}
	return toolResult(map[string]any{
		"query_id": queryID,
		"status":   "deactivated",
	})
}

// handleMemoryAuditRun runs a full audit pass for a project immediately.
//
// Required args: project.
func handleMemoryAuditRun(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	if cfg.PgPool == nil && cfg.testAuditDB == nil {
		return nil, fmt.Errorf("audit tools require a database connection (PgPool not configured)")
	}
	args, _ := req.Params.Arguments.(map[string]any)
	project, err := getProject(args, "")
	if err != nil {
		return nil, err
	}
	if project == "" {
		return nil, fmt.Errorf("project is required")
	}

	worker := newAuditWorker(pool, cfg)
	summaries, err := worker.RunProjectAudit(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("run audit: %w", err)
	}
	if summaries == nil {
		summaries = []audit.SnapshotSummary{}
	}
	return toolResult(map[string]any{
		"project":   project,
		"snapshots": summaries,
		"count":     len(summaries),
	})
}

// handleMemoryAuditCompare returns the snapshot history for a canonical query,
// enabling comparison of retrieval ranking over time.
//
// Required args: query_id.
// Optional args: limit (default 10).
func handleMemoryAuditCompare(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	if cfg.PgPool == nil && cfg.testAuditDB == nil {
		return nil, fmt.Errorf("audit tools require a database connection (PgPool not configured)")
	}
	args, _ := req.Params.Arguments.(map[string]any)
	queryID := getString(args, "query_id", "")
	if queryID == "" {
		return nil, fmt.Errorf("query_id is required")
	}
	limit := 10
	if v, ok := args["limit"]; ok {
		if n, ok := v.(float64); ok && n > 0 {
			limit = int(n)
		}
	}

	worker := newAuditWorker(pool, cfg)
	snaps, err := worker.GetSnapshots(ctx, queryID, limit)
	if err != nil {
		return nil, fmt.Errorf("get snapshots: %w", err)
	}

	items := make([]map[string]any, len(snaps))
	for i, s := range snaps {
		item := map[string]any{
			"id":              s.ID,
			"run_at":          s.RunAt.Format(time.RFC3339),
			"memory_count":    len(s.MemoryIDs),
			"embedding_model": s.EmbeddingModel,
			"is_baseline":     s.RBOVsPrev == nil,
		}
		if s.RBOVsPrev != nil {
			item["rbo_vs_prev"] = *s.RBOVsPrev
		}
		if s.JaccardAt5 != nil {
			item["jaccard_at_5"] = *s.JaccardAt5
		}
		if s.JaccardAt10 != nil {
			item["jaccard_at_10"] = *s.JaccardAt10
		}
		if s.JaccardFull != nil {
			item["jaccard_full"] = *s.JaccardFull
		}
		if len(s.Additions) > 0 {
			item["additions"] = s.Additions
		}
		if len(s.Removals) > 0 {
			item["removals"] = s.Removals
		}
		items[i] = item
	}
	return toolResult(map[string]any{
		"query_id":  queryID,
		"snapshots": items,
		"count":     len(items),
	})
}

// handleMemoryWeightHistory returns the current weights and tuning history for a project.
//
// Required args: project.
func handleMemoryWeightHistory(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	project, err := getProject(args, "")
	if err != nil {
		return nil, err
	}
	if project == "" {
		return nil, fmt.Errorf("project is required")
	}

	var tuner *weight.TunerWorker
	switch {
	case cfg.testWeightTuner != nil:
		tuner = cfg.testWeightTuner
	case cfg.PgPool != nil:
		tuner = weight.NewTunerWorker(cfg.PgPool, 24*time.Hour)
	default:
		return nil, fmt.Errorf("weight tools require a database connection (PgPool not configured)")
	}
	current, err := tuner.LoadWeights(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("load weights: %w", err)
	}
	history, err := tuner.GetHistory(ctx, project, 20)
	if err != nil {
		return nil, fmt.Errorf("get weight history: %w", err)
	}

	var status string
	if len(history) == 0 {
		status = "no adjustments recorded — tuner fires after 50+ failure events"
	} else {
		status = "active"
	}

	histItems := make([]map[string]any, len(history))
	for i, h := range history {
		item := map[string]any{
			"id":         h.ID,
			"applied_at": h.AppliedAt.Format(time.RFC3339),
			"weights": map[string]any{
				"vector":    h.Weights.Vector,
				"bm25":      h.Weights.BM25,
				"recency":   h.Weights.Recency,
				"precision": h.Weights.Precision,
			},
		}
		if h.Notes != "" {
			item["notes"] = h.Notes
		}
		if len(h.TriggerData) > 0 {
			item["trigger_data"] = string(h.TriggerData)
		}
		histItems[i] = item
	}

	return toolResult(map[string]any{
		"project": project,
		"current_weights": map[string]any{
			"vector":    current.Vector,
			"bm25":      current.BM25,
			"recency":   current.Recency,
			"precision": current.Precision,
		},
		"history": histItems,
		"status":  status,
	})
}
