package main

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"sync"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// temporalInterrogativeRe strips leading relative-time interrogatives so the
// recall query matches event noun-phrases rather than the question scaffolding.
var temporalInterrogativeRe = regexp.MustCompile(
	`(?i)^(how many (days?|weeks?|months?|years?) (ago |before |after )?(did|have|has|was|were|do|does|is|are) ` +
		`|when did |which (event|thing|one) happened (first|last|earlier|later|more recently) ` +
		`|what (was|is|were|are) the (date|time|day|week|month|year) ` +
		`|on what (date|day) )`,
)

const recallTopK = 100
const contextTopK = 40

func runRun(cfg *Config) {
	items := loadItems(cfg.DataFile)
	itemMap := make(map[string]longmemeval.Item, len(items))
	for _, item := range items {
		itemMap[item.QuestionID] = item
	}

	ingestEntries, err := longmemeval.ReadAllIngest(cfg.OutDir + "/checkpoint-ingest.jsonl")
	if err != nil {
		log.Fatalf("read ingest checkpoint: %v", err)
	}
	ingestMap := make(map[string]longmemeval.IngestEntry, len(ingestEntries))
	for _, e := range ingestEntries {
		if e.Status == "done" {
			ingestMap[e.QuestionID] = e
		}
	}

	ckptPath := cfg.OutDir + "/checkpoint-run.jsonl"
	skip, err := longmemeval.ReadSkipSet(ckptPath)
	if err != nil {
		log.Fatalf("read run checkpoint: %v", err)
	}
	log.Printf("run: %d ingest entries loaded, %d already done", len(ingestMap), len(skip))

	ckptCh := make(chan longmemeval.RunEntry, cfg.Workers*2)
	var wgWriter sync.WaitGroup
	wgWriter.Add(1)
	go func() {
		defer wgWriter.Done()
		longmemeval.WriteCheckpoint(ckptPath, ckptCh)
	}()

	work := make(chan longmemeval.IngestEntry, len(ingestEntries))
	for _, e := range ingestEntries {
		if e.Status == "done" && !skip[e.QuestionID] {
			work <- e
		}
	}
	close(work)

	var wg sync.WaitGroup
	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runWorker(cfg, itemMap, work, ckptCh)
		}()
	}
	wg.Wait()
	close(ckptCh)
	wgWriter.Wait()

	log.Printf("run: complete")
}

// runEntryLogLine formats a RunEntry into the log string emitted by runWorker.
// It is a separate function so the format can be tested independently of a
// live server, and so the error cause (Bug #643) is always visible in logs.
//
// Bug #643 fix: when status=error the hypothesis_len is always 0, which made
// log searches useless for root-cause analysis.  The error field is now always
// included in the log line so ops can grep for the actual failure without
// examining per-item checkpoint files.
func runEntryLogLine(entry longmemeval.RunEntry) string {
	if entry.Status == "error" {
		return fmt.Sprintf("run [%s] status=error hypothesis_len=%d error=%q",
			entry.QuestionID, len(entry.Hypothesis), entry.Error)
	}
	return fmt.Sprintf("run [%s] status=%s hypothesis_len=%d",
		entry.QuestionID, entry.Status, len(entry.Hypothesis))
}

func runWorker(cfg *Config, itemMap map[string]longmemeval.Item, work <-chan longmemeval.IngestEntry, out chan<- longmemeval.RunEntry) {
	for ingestEntry := range work {
		ctx := context.Background()
		item, ok := itemMap[ingestEntry.QuestionID]
		if !ok {
			entry := longmemeval.RunEntry{QuestionID: ingestEntry.QuestionID, Status: "error", Error: "item not found in data file"}
			out <- entry
			log.Print(runEntryLogLine(entry))
			continue
		}

		// Fresh connection per item — SSE sessions expire under long runs.
		mcpClient, err := longmemeval.Connect(ctx, cfg.ServerURL, cfg.APIKey)
		if err != nil {
			entry := longmemeval.RunEntry{QuestionID: ingestEntry.QuestionID, Status: "error", Error: fmt.Sprintf("connect: %v", err)}
			out <- entry
			log.Print(runEntryLogLine(entry))
			continue
		}

		entry := runOne(ctx, cfg, mcpClient, item, ingestEntry)
		out <- entry
		// Bug #643 fix: log.Print the structured line which includes error= on failure.
		log.Print(runEntryLogLine(entry))

		if !cfg.NoCleanup {
			// DeleteProject handles stale-session errors internally (Bug #642):
			// it logs at DEBUG level and returns nil so this WARN is only
			// reached for genuine non-stale errors.
			if err := mcpClient.DeleteProject(ctx, ingestEntry.Project); err != nil {
				log.Printf("WARN run [%s] cleanup failed: %v", item.QuestionID, err)
			}
		}
	}
}

func runOne(ctx context.Context, cfg *Config, mcpClient *longmemeval.Client, item longmemeval.Item, ingest longmemeval.IngestEntry) (entry longmemeval.RunEntry) {
	defer func() {
		if r := recover(); r != nil {
			entry = longmemeval.RunEntry{
				QuestionID: item.QuestionID,
				Status:     "error",
				Error:      fmt.Sprintf("panic: %v", r),
			}
		}
	}()

	// Strip leading interrogative phrases for temporal questions so the recall
	// query matches event noun-phrases rather than "how many weeks ago did...".
	recallQuery := item.Question
	if item.QuestionType == "temporal-reasoning" {
		recallQuery = temporalInterrogativeRe.ReplaceAllString(recallQuery, "")
		if recallQuery == "" {
			recallQuery = item.Question
		}
	}
	retrievedIDs, err := mcpClient.Recall(ctx, ingest.Project, recallQuery, recallTopK)
	if err != nil {
		return longmemeval.RunEntry{
			QuestionID: item.QuestionID,
			Status:     "error",
			Error:      fmt.Sprintf("recall: %v", err),
		}
	}

	// Fetch content for top contextTopK memories.
	contextLimit := contextTopK
	if contextLimit > len(retrievedIDs) {
		contextLimit = len(retrievedIDs)
	}
	contextBlocks := make([]string, 0, contextLimit)
	for _, id := range retrievedIDs[:contextLimit] {
		content, err := mcpClient.FetchContent(ctx, ingest.Project, id)
		if err != nil {
			log.Printf("WARN run [%s] fetch %s: %v", item.QuestionID, id, err)
			continue
		}
		if content != "" {
			contextBlocks = append(contextBlocks, content)
		}
	}

	prompt := longmemeval.GenerationPrompt(item.Question, item.QuestionDate, contextBlocks)
	var hypothesis string
	if cfg.LLMBaseURL != "" {
		hypothesis, err = longmemeval.GenerateOAI(ctx, prompt, cfg.LLMBaseURL, cfg.LLMModel, cfg.Retries)
	} else {
		hypothesis, err = longmemeval.Generate(ctx, prompt, cfg.Retries)
	}
	if err != nil {
		return longmemeval.RunEntry{
			QuestionID:   item.QuestionID,
			RetrievedIDs: retrievedIDs,
			Status:       "error",
			Error:        fmt.Sprintf("generate: %v", err),
		}
	}

	return longmemeval.RunEntry{
		QuestionID:   item.QuestionID,
		Hypothesis:   hypothesis,
		RetrievedIDs: retrievedIDs,
		Status:       "done",
	}
}
