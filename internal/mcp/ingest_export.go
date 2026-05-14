package mcp

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/ingest/router"
	"github.com/petersimmons1972/engram/internal/ingest/slack"
	"github.com/petersimmons1972/engram/internal/ingestqueue"
	"github.com/petersimmons1972/engram/internal/types"
)

func handleMemoryIngestExport(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}

	path := getString(args, "path", "")
	if path == "" {
		return mcpgo.NewToolResultError("path is required"), nil
	}

	safePath, err := SafePath(cfg.DataDir, path)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}
	info, err := os.Stat(safePath)
	if err != nil {
		return nil, fmt.Errorf("memory_ingest_export: stat: %w", err)
	}
	importMax := cfg.ImportMaxBytes
	if importMax <= 0 {
		importMax = 50 * 1024 * 1024
	}
	expandedMax := cfg.ImportExpandedMaxBytes
	if expandedMax <= 0 {
		expandedMax = 100 * 1024 * 1024
	}
	if info.Size() > int64(importMax) {
		return nil, fmt.Errorf("memory_ingest_export: file exceeds maximum size (%d bytes > %d)", info.Size(), importMax)
	}

	format := router.DetectFromPath(safePath)
	if format == router.FormatUnknown {
		return nil, fmt.Errorf("memory_ingest_export: unrecognised format for %q — supported: slack (.zip), claudeai, chatgpt", path)
	}

	var memories []*types.Memory
	switch format {
	case router.FormatSlack:
		memories, err = slack.ParseFileWithLimits(safePath, int64(importMax), int64(expandedMax))
		if err != nil {
			return nil, fmt.Errorf("memory_ingest_export: slack parse: %w", err)
		}
	default:
		f, openErr := os.Open(safePath)
		if openErr != nil {
			return nil, fmt.Errorf("memory_ingest_export: open: %w", openErr)
		}
		defer func() { _ = f.Close() }()
		_, memories, err = router.ParseAutoWithLimit(f, int64(importMax))
		if err != nil {
			return nil, fmt.Errorf("memory_ingest_export: parse: %w", err)
		}
	}

	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	deps := storeDocumentDeps{
		engine:  engineStorerAdapter{store: h.Engine.StoreWithRawBody},
		backend: backendDocumentAdapter{b: h.Engine.Backend()},
	}

	if cfg.IngestQueue != nil {
		jobID := uuid.New().String()
		memoriesCopy := memories
		job := &ingestqueue.Job{
			ID: jobID, Project: project,
			Work: func(bgCtx context.Context) error {
				_, err := runExportFanout(bgCtx, deps, project, format, memoriesCopy)
				return err
			},
		}
		if err := cfg.IngestQueue.Enqueue(job); err != nil {
			return toolResult(map[string]any{
				"status":      "queue_full",
				"retry_after": "30s",
				"message":     err.Error(),
			})
		}
		return toolResult(map[string]any{
			"status":          "queued",
			"job_id":          jobID,
			"memories_parsed": len(memories),
			"message":         fmt.Sprintf("%d memories queued. Poll memory_ingest_status for completion.", len(memories)),
		})
	}

	return runExportFanout(ctx, deps, project, format, memories)
}
