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

const ingestBatchSize = 100

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

	// Collect non-empty sessions with their IDs.
	type sessionEntry struct {
		sessionID string
		item      longmemeval.BatchItem
	}
	var sessions []sessionEntry
	for i, session := range item.HaystackSessions {
		if i >= len(item.HaystackSessionIDs) {
			break
		}
		sessionID := item.HaystackSessionIDs[i]
		content := longmemeval.SessionContent(session)
		if strings.TrimSpace(content) == "" {
			continue
		}
		sessions = append(sessions, sessionEntry{
			sessionID: sessionID,
			item:      longmemeval.BatchItem{Content: content, Tags: []string{"lme", "sid:" + sessionID}},
		})
	}

	memoryMap := make(map[string]string, len(sessions))
	for start := 0; start < len(sessions); start += ingestBatchSize {
		end := start + ingestBatchSize
		if end > len(sessions) {
			end = len(sessions)
		}
		batch := sessions[start:end]
		batchItems := make([]longmemeval.BatchItem, len(batch))
		for i, s := range batch {
			batchItems[i] = s.item
		}
		ids, err := mcpClient.StoreBatch(ctx, project, batchItems)
		if err != nil {
			return longmemeval.IngestEntry{
				QuestionID: item.QuestionID,
				Project:    project,
				Status:     "error",
				Error:      fmt.Sprintf("store_batch offset %d: %v", start, err),
			}
		}
		for i, id := range ids {
			memoryMap[id] = batch[i].sessionID
		}
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
