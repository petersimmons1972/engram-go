package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

func runIngest(cfg *Config) {
	items := loadItems(cfg.DataFile)
	ckptPath := filepath.Join(cfg.OutDir, "checkpoint-ingest.jsonl")

	skip, err := longmemeval.ReadSkipSet(ckptPath)
	if err != nil {
		log.Fatalf("read ingest checkpoint: %v", err)
	}
	log.Printf("ingest: %d items loaded, %d already done", len(items), len(skip))

	ckptCh := make(chan longmemeval.IngestEntry, cfg.Workers*2)
	var wgWriter sync.WaitGroup
	wgWriter.Add(1)
	go func() {
		defer wgWriter.Done()
		longmemeval.WriteCheckpoint(ckptPath, ckptCh)
	}()

	work := make(chan longmemeval.Item, len(items))
	for _, item := range items {
		if !skip[item.QuestionID] {
			work <- item
		}
	}
	close(work)

	var wg sync.WaitGroup
	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ingestWorker(cfg, work, ckptCh)
		}()
	}
	wg.Wait()
	close(ckptCh)
	wgWriter.Wait()

	log.Printf("ingest: complete")
}

func ingestWorker(cfg *Config, work <-chan longmemeval.Item, out chan<- longmemeval.IngestEntry) {
	ctx := context.Background()
	mcpClient, err := longmemeval.Connect(ctx, cfg.ServerURL, cfg.APIKey)
	if err != nil {
		log.Printf("WARN ingest worker: connect failed: %v", err)
		for item := range work {
			out <- longmemeval.IngestEntry{QuestionID: item.QuestionID, Status: "error", Error: err.Error()}
		}
		return
	}

	for item := range work {
		entry := ingestOne(ctx, cfg, mcpClient, item)
		out <- entry
		log.Printf("ingest [%s] project=%s sessions=%d status=%s",
			item.QuestionID, entry.Project, entry.SessionCount, entry.Status)
	}
}

func ingestOne(ctx context.Context, cfg *Config, mcpClient *longmemeval.Client, item longmemeval.Item) (entry longmemeval.IngestEntry) {
	defer func() {
		if r := recover(); r != nil {
			entry = longmemeval.IngestEntry{
				QuestionID: item.QuestionID,
				Status:     "error",
				Error:      fmt.Sprintf("panic: %v", r),
			}
		}
	}()

	project := projectName(cfg.RunID, item.QuestionID)
	memoryMap := make(map[string]string, len(item.HaystackSessions))

	for i, session := range item.HaystackSessions {
		if i >= len(item.HaystackSessionIDs) {
			break
		}
		sessionID := item.HaystackSessionIDs[i]
		content := longmemeval.SessionContent(session)
		if strings.TrimSpace(content) == "" {
			continue
		}
		tags := []string{"lme", "sid:" + sessionID}
		memID, err := mcpClient.Store(ctx, project, content, tags)
		if err != nil {
			return longmemeval.IngestEntry{
				QuestionID: item.QuestionID,
				Project:    project,
				Status:     "error",
				Error:      fmt.Sprintf("store session %s: %v", sessionID, err),
			}
		}
		memoryMap[memID] = sessionID
	}

	return longmemeval.IngestEntry{
		QuestionID:   item.QuestionID,
		Project:      project,
		SessionCount: len(memoryMap),
		MemoryMap:    memoryMap,
		Status:       "done",
	}
}

// loadItems parses the LongMemEval JSON file.
func loadItems(path string) []longmemeval.Item {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("read data file %q: %v", path, err)
	}
	var items []longmemeval.Item
	if err := json.Unmarshal(data, &items); err != nil {
		log.Fatalf("parse data file: %v", err)
	}
	if len(items) == 0 {
		log.Fatal("data file is empty")
	}
	return items
}
