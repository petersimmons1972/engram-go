package mcp

import (
	"context"
	"fmt"
	"log/slog"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/markdown"
)

func handleMemoryExportAll(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest, cfg Config) (*mcpgo.CallToolResult, error) {
	if cfg.DataDir == "" {
		return nil, fmt.Errorf("file operations require --data-dir / ENGRAM_DATA_DIR to be set")
	}
	args := req.GetArguments()
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
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
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	errResult, rawPath := requireString(args, "path")
	if errResult != nil {
		return errResult, nil
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
	project, err := getProject(args, "default")
	if err != nil {
		return nil, err
	}
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	errResult, rawPath := requireString(args, "path")
	if errResult != nil {
		return errResult, nil
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
