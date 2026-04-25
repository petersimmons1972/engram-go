package mcp

import (
	"context"
	"fmt"
	"os"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/ingest/router"
	"github.com/petersimmons1972/engram/internal/ingest/slack"
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

	format := router.DetectFromPath(safePath)
	if format == router.FormatUnknown {
		return nil, fmt.Errorf("memory_ingest_export: unrecognised format for %q — supported: slack (.zip), claudeai, chatgpt", path)
	}

	var memories []*types.Memory
	switch format {
	case router.FormatSlack:
		memories, err = slack.ParseFile(safePath)
		if err != nil {
			return nil, fmt.Errorf("memory_ingest_export: slack parse: %w", err)
		}
	default:
		f, openErr := os.Open(safePath)
		if openErr != nil {
			return nil, fmt.Errorf("memory_ingest_export: open: %w", openErr)
		}
		defer f.Close()
		_, memories, err = router.ParseAuto(f)
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
	return runExportFanout(ctx, deps, project, format, memories)
}
