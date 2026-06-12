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
	"time"

	"github.com/petersimmons1972/engram/internal/chunk"
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
	writerErr := make(chan error, 1)
	var wgWriter sync.WaitGroup
	wgWriter.Add(1)
	go func() {
		defer wgWriter.Done()
		writerErr <- longmemeval.WriteCheckpoint(ckptPath, ckptCh)
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
	if err := <-writerErr; err != nil {
		log.Fatalf("write ingest checkpoint: %v", err)
	}

	log.Printf("ingest: complete")
}

func ingestWorker(cfg *Config, work <-chan longmemeval.Item, out chan<- longmemeval.IngestEntry) {
	ctx := context.Background()
	restClient := longmemeval.NewRestClient(cfg.ServerURL, cfg.APIKey)
	for item := range work {
		entry := ingestOne(ctx, cfg, restClient, item)
		out <- entry
		log.Printf("ingest [%s] project=%s sessions=%d status=%s error=%q", item.QuestionID, entry.Project, entry.SessionCount, entry.Status, entry.Error)
	}
}

func ingestOne(ctx context.Context, cfg *Config, restClient *longmemeval.RestClient, item longmemeval.Item) (entry longmemeval.IngestEntry) {
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

	// #837: compute expiresAt once per question. Passed to every QuickStore call;
	// SetProjectTTL is idempotent (ON CONFLICT DO UPDATE) so repeated upserts are safe.
	var expiresAt *time.Time
	if cfg.ScratchTTL > 0 {
		t := time.Now().UTC().Add(cfg.ScratchTTL)
		expiresAt = &t
	}

	// Collect non-empty sessions with their IDs.
	type sessionEntry struct {
		sessionID string
		item      longmemeval.BatchItem
	}
	mode := chunk.ParseChunkMode(cfg.ChunkMode)
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
		tags := []string{"lme", "sid:" + sessionID}
		if i < len(item.HaystackDates) && item.HaystackDates[i] != "" {
			tags = append(tags, "date:"+item.HaystackDates[i])
		}
		sessions = append(sessions, sessionEntry{
			sessionID: sessionID,
			item:      longmemeval.BatchItem{Content: content, Tags: tags},
		})
	}

	// If no non-empty sessions were found, return error instead of fake "done"
	if len(sessions) == 0 {
		return longmemeval.IngestEntry{
			QuestionID: item.QuestionID,
			Project:    project,
			Status:     "error",
			Error:      "no non-empty sessions found in haystack",
		}
	}

	memoryMap := make(map[string]string, len(sessions))
	memoryProvenance := make(map[string]longmemeval.TurnChunkProvenance, len(sessions))
	for i, s := range sessions {
		if mode == chunk.ChunkModeTurn {
			for _, tc := range chunk.ChunkTurns(s.item.Content, chunk.DefaultTurnChunkChars) {
				tags := append([]string{}, s.item.Tags...)
				tags = append(tags, "speaker:"+tc.Speaker, fmt.Sprintf("turn:%d", tc.TurnIndex))
				chunkContent := tc.Text
				if sessionDate := extractDateTag(s.item.Tags); sessionDate != "" {
					chunkContent = "Session date: " + sessionDate + "\n" + chunkContent
				}
				id, err := restClient.QuickStore(ctx, project, chunkContent, tags, expiresAt)
				if err != nil {
					return longmemeval.IngestEntry{
						QuestionID: item.QuestionID,
						Project:    project,
						Status:     "error",
						Error:      fmt.Sprintf("quick-store offset %d: %v", i, err),
					}
				}
				memoryMap[id] = s.sessionID
				memoryProvenance[id] = longmemeval.TurnChunkProvenance{
					SessionID: s.sessionID,
					TurnIndex: tc.TurnIndex,
					Speaker:   tc.Speaker,
				}
			}
			continue
		}

		id, err := restClient.QuickStore(ctx, project, s.item.Content, s.item.Tags, expiresAt)
		if err != nil {
			return longmemeval.IngestEntry{
				QuestionID: item.QuestionID,
				Project:    project,
				Status:     "error",
				Error:      fmt.Sprintf("quick-store offset %d: %v", i, err),
			}
		}
		memoryMap[id] = s.sessionID
	}

	return longmemeval.IngestEntry{
		QuestionID:       item.QuestionID,
		Project:          project,
		SessionCount:     len(memoryMap),
		MemoryMap:        memoryMap,
		MemoryProvenance: memoryProvenance,
		Status:           "done",
	}
}

func extractDateTag(tags []string) string {
	const prefix = "date:"
	for _, tag := range tags {
		if strings.HasPrefix(tag, prefix) {
			return strings.TrimPrefix(tag, prefix)
		}
	}
	return ""
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
