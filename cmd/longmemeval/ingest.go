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

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// ttlStamper writes project_ttl rows during ingest (#754). It is a tiny
// interface so the ingest worker can be unit-tested with a fake.
type ttlStamper interface {
	SetProjectTTL(ctx context.Context, project string, createdAt time.Time, expiresAt *time.Time) error
}

func runIngest(cfg *Config) {
	items := loadItems(cfg.DataFile)
	ckptPath := filepath.Join(cfg.OutDir, "checkpoint-ingest.jsonl")

	// #754: if a scratch TTL is in effect and a DSN is configured, open a
	// PostgresBackend so each per-question project gets a project_ttl row
	// stamped at first-store time. Failure to connect downgrades to a WARN
	// rather than failing ingest — a missing TTL row makes the project durable
	// (the operator backfill query is documented in the migration header).
	var stamper ttlStamper
	if cfg.ScratchTTL > 0 && cfg.DatabaseURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		backend, err := db.NewPostgresBackend(ctx, "_lme_ingest", cfg.DatabaseURL)
		if err != nil {
			// Log without the raw DSN/error (which may embed credentials) to
			// satisfy CodeQL CWE-312. Use redactURL on the URL portion only.
			log.Printf("ingest: WARN: cannot open Postgres for TTL stamping (dsn=%s); projects will be durable",
				redactURL(cfg.DatabaseURL))
		} else {
			defer backend.Close()
			stamper = backend
		}
	} else if cfg.ScratchTTL > 0 && cfg.DatabaseURL == "" {
		log.Printf("ingest: WARN: --scratch-ttl set but no --database-url; projects will be durable")
	}

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
			ingestWorker(cfg, stamper, work, ckptCh)
		}()
	}
	wg.Wait()
	close(ckptCh)
	wgWriter.Wait()

	log.Printf("ingest: complete")
}

func ingestWorker(cfg *Config, stamper ttlStamper, work <-chan longmemeval.Item, out chan<- longmemeval.IngestEntry) {
	ctx := context.Background()
	restClient := longmemeval.NewRestClient(cfg.ServerURL, cfg.APIKey)
	for item := range work {
		entry := ingestOne(ctx, cfg, restClient, item)
		// #754: stamp TTL once per successful project so the prune sweep can
		// reclaim it later. Best-effort: stamping failure is logged but does
		// not flip ingest status — the memories landed, only the metadata is
		// missing, and an operator backfill exists.
		if entry.Status == "done" && stamper != nil && cfg.ScratchTTL > 0 {
			now := time.Now().UTC()
			exp := now.Add(cfg.ScratchTTL)
			if err := stamper.SetProjectTTL(ctx, entry.Project, now, &exp); err != nil {
				log.Printf("ingest [%s] WARN: set TTL: %v", item.QuestionID, err)
			}
		}
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
		// Prepend session date so the model can anchor relative-time questions.
		tags := []string{"lme", "sid:" + sessionID}
		if i < len(item.HaystackDates) && item.HaystackDates[i] != "" {
			content = "Session date: " + item.HaystackDates[i] + "\n" + content
			tags = append(tags, "date:"+item.HaystackDates[i])
		}
		sessions = append(sessions, sessionEntry{
			sessionID: sessionID,
			item:      longmemeval.BatchItem{Content: content, Tags: tags},
		})
	}

	memoryMap := make(map[string]string, len(sessions))
	for i, s := range sessions {
		id, err := restClient.QuickStore(ctx, project, s.item.Content, s.item.Tags)
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
