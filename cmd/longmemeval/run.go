package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// preservedLog is a mutex-protected accumulator for project names that were
// preserved (not cleaned up) during a run. Using a slice rather than a buffered
// channel avoids the R2-B2 deadlock: a channel blocks senders when full, which
// causes runWorker goroutines to hang while wg.Wait() waits for them to finish.
// See R2-B2, #807.
type preservedLog struct {
	mu    sync.Mutex
	items []string
}

// add appends name to the log. Safe for concurrent use.
func (pl *preservedLog) add(name string) {
	pl.mu.Lock()
	pl.items = append(pl.items, name)
	pl.mu.Unlock()
}

// names returns a copy of the collected names. Safe to call after all writers
// have finished (e.g., after wg.Wait()).
func (pl *preservedLog) names() []string {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	if len(pl.items) == 0 {
		return nil
	}
	out := make([]string, len(pl.items))
	copy(out, pl.items)
	return out
}

// temporalInterrogativeRe strips leading relative-time interrogatives so the
// recall query matches event noun-phrases rather than the question scaffolding.
var temporalInterrogativeRe = regexp.MustCompile(
	`(?i)^(how many (days?|weeks?|months?|years?) (ago |before |after )?(did|have|has|was|were|do|does|is|are) ` +
		`|when did |which (event|thing|one) happened (first|last|earlier|later|more recently) ` +
		`|what (was|is|were|are) the (date|time|day|week|month|year) ` +
		`|on what (date|day) )`,
)

// buildRecallQuery derives the Engram recall query from a raw LME question.
// For temporal-reasoning questions it strips the interrogative scaffold so the
// query matches event noun-phrases. For preference questions it delegates to
// longmemeval.PreferenceRecallQuery. When disableRewrite is true the raw
// question is returned unchanged.
func buildRecallQuery(question, questionType string, disableRewrite bool) string {
	if disableRewrite {
		return question
	}
	switch questionType {
	case "temporal-reasoning":
		q := temporalInterrogativeRe.ReplaceAllString(question, "")
		if q == "" {
			return question
		}
		// Preserve temporal classifier signal so isTemporalQuery() returns true
		// on the Engram server and TemporalWeights are applied. The interrogative
		// strip removes all temporal words (e.g. "ago", "days") — prepending
		// "recent " (present in temporalQuerySignals) restores the signal while
		// keeping the semantic noun-phrase clean. (F2)
		return "recent " + strings.TrimSpace(q)
	case "single-session-preference":
		return longmemeval.PreferenceRecallQuery(question)
	default:
		return question
	}
}

// runRun executes the run stage. Returns the process exit code: 0 on success,
// 1 when zero items completed successfully out of any that were attempted
// (#703 — total-failure guard so scripted pipelines don't proceed when every
// recall/generate failed).
func runRun(cfg *Config) int {
	// #749: acquire per-backend lock before doing any work so parallel lme
	// invocations against the same vLLM endpoint are detected and rejected.
	if cfg.LLMBaseURL != "" {
		lockCfg := &BackendLockConfig{
			ExclusiveBackend: cfg.ExclusiveBackend,
			BackendLockDir:   cfg.BackendLockDir,
		}
		release, lockErr := acquireBackendLock(lockCfg, cfg.LLMBaseURL)
		if lockErr != nil {
			log.Print(lockErr)
			return ExitCodeLockContention
		}
		defer release()
	}

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

	// S9 (#807 R3): collect preserved-project names via a mutex-protected slice.
	// A buffered channel was used in R2 but deadlocks when the number of
	// preserved projects exceeds the buffer size (cfg.Workers*2) — workers block
	// on send while wg.Wait() blocks waiting for workers to finish. The slice
	// approach has no capacity limit and never blocks. (R2-B2)
	pl := &preservedLog{}

	var wg sync.WaitGroup
	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runWorker(cfg, itemMap, work, innerCh, pl)
		}()
	}
	wg.Wait()
	close(innerCh)
	wgWriter.Wait()

	// S9 (#807): emit one end-of-run summary line (token: cleanup-summary).
	preserved := pl.names()
	if len(preserved) > 0 {
		sample := preserved
		if len(sample) > 5 {
			sample = sample[:5]
		}
		// TODO(#807-S3): migrate to slog once structured logging is wired into
		// cmd/longmemeval (currently uses stdlib log; slog lives in internal/ only).
		log.Printf("cleanup-summary cleanup-policy=auto preserved=%d sample=%v",
			len(preserved), sample)
	}

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

// runWorker processes items from work, writing RunEntry results to out and
// recording preserved-project names in pl (for S9 end-of-run summary logging).
func runWorker(cfg *Config, itemMap map[string]longmemeval.Item, work <-chan longmemeval.IngestEntry, out chan<- longmemeval.RunEntry, pl *preservedLog) {
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

		// #669: close the per-item SSE client so its background goroutine + HTTP
		// connection don't accumulate over hundreds of items. Wrapped in a closure
		// because `defer mcpClient.Close()` inside a for-range loop fires only
		// when the function returns — we need it on each iteration. Hence the IIFE.
		func() {
			defer func() {
				if cerr := mcpClient.Close(); cerr != nil {
					log.Printf("WARN run [%s] mcpClient close: %v", item.QuestionID, cerr)
				}
			}()
			entry := runOne(ctx, cfg, mcpClient, item, ingestEntry)
			out <- entry
			if entry.Status == "error" {
				log.Printf("run [%s] status=%s hypothesis_len=%d error=%q", item.QuestionID, entry.Status, len(entry.Hypothesis), entry.Error)
			} else {
				log.Printf("run [%s] status=%s hypothesis_len=%d", item.QuestionID, entry.Status, len(entry.Hypothesis))
			}

			if shouldCleanupProject(cfg, ingestEntry.Project) {
				if err := mcpClient.DeleteProject(ctx, ingestEntry.Project); err != nil {
					if !longmemeval.IsStaleSessionError(err) {
						log.Printf("WARN run [%s] cleanup failed: %v", item.QuestionID, err)
					}
				}
			} else {
				// S9 (#807): record in preservedLog for end-of-run summary;
				// avoids per-question log noise (~N lines for N questions).
				pl.add(ingestEntry.Project)
			}
		}()
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
	// When --disable-query-rewrite is set, use the raw question unchanged.
	recallQuery := buildRecallQuery(item.Question, item.QuestionType, cfg.DisableQueryRewrite)
	retrievedIDs, err := mcpClient.Recall(ctx, ingest.Project, recallQuery, cfg.RecallTopK)
	if err != nil {
		return longmemeval.RunEntry{
			QuestionID: item.QuestionID,
			Status:     "error",
			Error:      fmt.Sprintf("recall: %v", err),
		}
	}

	// H8 (lme-h8h12h15): exhaustive aggregation recall — run a topK=500 sweep on
	// the object noun-phrase for count-shaped questions and union with primary.
	if cfg.ExhaustiveAggregation && longmemeval.IsAggregationQuestion(item.Question) {
		const aggregationSweepTopK = 500
		sweepQuery := longmemeval.ExtractAggregationAnchor(item.Question)
		sweepIDs, sweepErr := mcpClient.Recall(ctx, ingest.Project, sweepQuery, aggregationSweepTopK)
		if sweepErr == nil {
			retrievedIDs = longmemeval.UnionMemoryIDs(retrievedIDs, sweepIDs)
		} else {
			log.Printf("WARN run [%s] H8 aggregation sweep failed: %v", item.QuestionID, sweepErr)
		}
	}

	// H15 (lme-h8h12h15): dual-query preference recall — run a second recall
	// using the subject-anchor query for preference questions and union results.
	if cfg.DualPreferenceRecall && item.QuestionType == "single-session-preference" {
		anchorTopK := cfg.RecallTopK / 2
		if anchorTopK < 1 {
			anchorTopK = 1
		}
		anchor := longmemeval.ExtractSubjectAnchor(item.Question)
		anchorIDs, anchorErr := mcpClient.Recall(ctx, ingest.Project, anchor, anchorTopK)
		if anchorErr == nil {
			retrievedIDs = longmemeval.UnionMemoryIDs(retrievedIDs, anchorIDs)
		} else {
			log.Printf("WARN run [%s] H15 anchor recall failed: %v", item.QuestionID, anchorErr)
		}
	}

	// H15: paraphrased multi-pass BM25 union.
	// When --query-paraphrase-passes N > 0, ask Haiku to generate N paraphrase
	// variants of the recall query, run a separate Recall for each variant, then
	// union all retrieved IDs (deduped, primary-pass order preserved first).
	// On paraphrase or recall errors we log a warning and continue with the IDs
	// collected so far — a partial union is strictly better than no union.
	if cfg.QueryParaphrasePasses > 0 {
		paraphrases, pErr := longmemeval.GenerateParaphrases(ctx, recallQuery, cfg.QueryParaphrasePasses, cfg.Retries)
		if pErr != nil {
			log.Printf("WARN run [%s] paraphrase: %v — falling back to single-pass recall", item.QuestionID, pErr)
		} else {
			for _, pq := range paraphrases {
				pIDs, pErr := mcpClient.Recall(ctx, ingest.Project, pq, cfg.RecallTopK)
				if pErr != nil {
					log.Printf("WARN run [%s] paraphrase recall (%q): %v", item.QuestionID, pq, pErr)
					continue
				}
				retrievedIDs = append(retrievedIDs, pIDs...)
			}
			retrievedIDs = longmemeval.DeduplicateIDs(retrievedIDs)
		}
	}

	// Fetch content for top contextLimit memories.
	// --context-topk overrides per-type default; 0 means use per-type default.
	var contextLimit int
	if cfg.ContextTopKOverride > 0 {
		contextLimit = cfg.ContextTopKOverride
	} else {
		contextLimit = longmemeval.ContextTopKForTypeWithBump(item.QuestionType, cfg.ContextTopKBump)
	}
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

	// --max-block-chars: truncate each block so the assembled prompt stays within
	// the model's max_model_len. Applied before chrono-sort so truncation is
	// independent of ordering. Required when --context-topk is large (e.g. 100)
	// and the vLLM endpoint has a modest context window (e.g. 131072 tokens).
	if cfg.MaxBlockChars > 0 {
		for i, block := range contextBlocks {
			if len(block) > cfg.MaxBlockChars {
				contextBlocks[i] = block[:cfg.MaxBlockChars]
			}
		}
	}

	// --chrono-sort: sort blocks by Session date ascending before prompt assembly.
	if cfg.ChronoSort {
		contextBlocks = sortBlocksChronologically(contextBlocks)
	}

	// Exp-14: --temporal-prompt-aug takes priority over --inject-question-date;
	// the two are mutually exclusive. When both are set, aug wins.
	// H12 (--enumerate-first) is orthogonal — it only fires for aggregation
	// questions, which the temporal/preference branches above never match.
	var prompt string
	switch {
	case cfg.TemporalPromptAug:
		prompt = longmemeval.GenerationPromptForTypeWithTemporalAug(item.Question, item.QuestionType, item.QuestionDate, contextBlocks, true)
	case cfg.InjectQuestionDate:
		prompt = longmemeval.GenerationPromptForTypeWithDateInjection(item.Question, item.QuestionType, item.QuestionDate, contextBlocks, true)
	case cfg.EnumerateFirst:
		prompt = longmemeval.GenerationPromptForTypeEnumerate(item.Question, item.QuestionType, item.QuestionDate, contextBlocks, true)
	default:
		prompt = longmemeval.GenerationPromptForType(item.Question, item.QuestionType, item.QuestionDate, contextBlocks)
	}
	var hypothesis string
	if cfg.LLMBaseURL != "" {
		maxTok := cfg.LLMMaxTokens
		if maxTok == 0 && cfg.EnableThinking {
			maxTok = 8192 // thinking mode default: room for reasoning chain + answer
		}
		opts := longmemeval.OAIOptions{EnableThinking: cfg.EnableThinking, MaxTokens: maxTok}
		hypothesis, err = longmemeval.GenerateOAIWithOpts(ctx, prompt, cfg.LLMBaseURL, cfg.LLMModel, cfg.Retries, opts)
	} else {
		hypothesis, err = longmemeval.GenerateForModel(ctx, prompt, cfg.GenerationModel, cfg.Retries)
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

// sessionDateRe matches the first "Session date: YYYY-MM-DD" line in a block.
var sessionDateRe = regexp.MustCompile(`(?m)^Session date:\s*(\d{4}-\d{2}-\d{2})`)

// blockDate extracts the Session date from the first matching line of a memory
// block. Returns time.Time{} (zero value / 1970) if no date is found.
func blockDate(block string) time.Time {
	m := sessionDateRe.FindStringSubmatch(block)
	if m == nil {
		return time.Time{}
	}
	t, err := time.Parse("2006-01-02", strings.TrimSpace(m[1]))
	if err != nil {
		return time.Time{}
	}
	return t
}

// sortBlocksChronologically returns a copy of blocks sorted by Session date
// ascending. Blocks with no parseable date sort first (treated as 1970-01-01).
func sortBlocksChronologically(blocks []string) []string {
	out := make([]string, len(blocks))
	copy(out, blocks)
	sort.SliceStable(out, func(i, j int) bool {
		return blockDate(out[i]).Before(blockDate(out[j]))
	})
	return out
}
