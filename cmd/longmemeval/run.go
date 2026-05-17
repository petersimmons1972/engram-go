package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"regexp"
	"sync"
	"sync/atomic"

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
const contextTopK = 8 // 40 blocks x 10KB avg = 104K tokens; 8 blocks ~21K tokens - 5x faster

// runRun executes the run stage. Returns the process exit code: 0 on success,
// 1 when zero items completed successfully out of any that were attempted
// (#703 — total-failure guard so scripted pipelines don't proceed when every
// recall/generate failed).
func runRun(cfg *Config) int {
	items := loadItems(cfg.DataFile)
	itemMap := make(map[string]longmemeval.Item, len(items))
	for _, item := range items {
		itemMap[item.QuestionID] = item
	}

	ingestEntries, err := longmemeval.ReadAllIngest(filepath.Join(cfg.OutDir, "checkpoint-ingest.jsonl"))
	if err != nil {
		log.Printf("ERROR read ingest checkpoint: %v", err)
		return 1
	}
	ingestMap := make(map[string]longmemeval.IngestEntry, len(ingestEntries))
	for _, e := range ingestEntries {
		if e.Status == "done" {
			ingestMap[e.QuestionID] = e
		}
	}

	ckptPath := filepath.Join(cfg.OutDir, "checkpoint-run.jsonl")
	skip, err := longmemeval.ReadSkipSet(ckptPath)
	if err != nil {
		log.Printf("ERROR read run checkpoint: %v", err)
		return 1
	}
	log.Printf("run: %d ingest entries loaded, %d already done", len(ingestMap), len(skip))

	// #703: track error vs success outcome counts to determine exit code.
	var attempted, errors atomic.Int64

	// Intermediate channel so we can observe outcomes before they reach the
	// checkpoint writer, without losing the existing append semantics.
	innerCh := make(chan longmemeval.RunEntry, cfg.Workers*2)
	ckptCh := make(chan longmemeval.RunEntry, cfg.Workers*2)
	go func() {
		for entry := range innerCh {
			attempted.Add(1)
			if entry.Status == "error" {
				errors.Add(1)
			}
			ckptCh <- entry
		}
		close(ckptCh)
	}()

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
			runWorker(cfg, itemMap, work, innerCh)
		}()
	}
	wg.Wait()
	close(innerCh)
	wgWriter.Wait()

	att := attempted.Load()
	errs := errors.Load()
	successes := att - errs
	log.Printf("run: complete (attempted=%d successes=%d errors=%d)", att, successes, errs)

	// Exit non-zero when at least one item was attempted but zero succeeded.
	// Resume-only invocations where nothing was attempted (everything already
	// in the checkpoint) report exit 0.
	if code := exitCodeForRunOutcome(att, errs); code != 0 {
		log.Printf("ERROR every attempted item failed; exiting non-zero (#703)")
		return code
	}
	return 0
}

// exitCodeForRunOutcome decides the runRun exit code from the attempted+error
// counts. Returns 1 when at least one item was attempted and zero succeeded;
// returns 0 otherwise (including resume-clean runs that attempted nothing).
// Extracted so the decision is unit-testable without spinning up a live MCP
// server. #703.
func exitCodeForRunOutcome(attempted, errors int64) int {
	if attempted == 0 {
		return 0
	}
	if attempted-errors == 0 {
		return 1
	}
	return 0
}

func runWorker(cfg *Config, itemMap map[string]longmemeval.Item, work <-chan longmemeval.IngestEntry, out chan<- longmemeval.RunEntry) {
	for ingestEntry := range work {
		ctx := context.Background()
		item, ok := itemMap[ingestEntry.QuestionID]
		if !ok {
			out <- longmemeval.RunEntry{QuestionID: ingestEntry.QuestionID, Status: "error", Error: "item not found in data file"}
			continue
		}

		// Fresh connection per item — SSE sessions expire under long runs.
		mcpClient, err := longmemeval.Connect(ctx, cfg.ServerURL, cfg.APIKey)
		if err != nil {
			out <- longmemeval.RunEntry{QuestionID: ingestEntry.QuestionID, Status: "error", Error: fmt.Sprintf("connect: %v", err)}
			continue
		}

		entry := runOne(ctx, cfg, mcpClient, item, ingestEntry)
		out <- entry
		if entry.Status == "error" {
			log.Printf("run [%s] status=%s hypothesis_len=%d error=%q", item.QuestionID, entry.Status, len(entry.Hypothesis), entry.Error)
		} else {
			log.Printf("run [%s] status=%s hypothesis_len=%d", item.QuestionID, entry.Status, len(entry.Hypothesis))
		}

		if !cfg.NoCleanup {
			if err := mcpClient.DeleteProject(ctx, ingestEntry.Project); err != nil {
				if !longmemeval.IsStaleSessionError(err) {
					log.Printf("WARN run [%s] cleanup failed: %v", item.QuestionID, err)
				}
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

// runEntryLogLine formats a single RunEntry as a one-line log message.
// Includes the error cause for error entries (#643 regression guard) and
// the hypothesis length for done entries.
func runEntryLogLine(entry longmemeval.RunEntry) string {
	if entry.Status == "error" {
		return fmt.Sprintf("question_id=%s status=%s error=%s",
			entry.QuestionID, entry.Status, entry.Error)
	}
	return fmt.Sprintf("question_id=%s status=%s hypothesis_len=%d",
		entry.QuestionID, entry.Status, len(entry.Hypothesis))
}
