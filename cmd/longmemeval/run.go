package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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

var relativeAgoRe = regexp.MustCompile(`(?i)\b(\d+|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve)\s+(day|days|week|weeks|month|months|year|years)\s+ago\b`)

func parseAgoAmount(raw string) (int, bool) {
	if n, err := strconv.Atoi(raw); err == nil {
		return n, n > 0
	}
	words := map[string]int{
		"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6,
		"seven": 7, "eight": 8, "nine": 9, "ten": 10, "eleven": 11, "twelve": 12,
	}
	n := words[strings.ToLower(raw)]
	return n, n > 0
}

func targetDateFromQuestion(question, questionType, questionDate string) (time.Time, bool) {
	if questionType != "temporal-reasoning" {
		return time.Time{}, false
	}
	anchor, ok := parseLongMemEvalQuestionDate(questionDate)
	if !ok {
		return time.Time{}, false
	}
	lower := strings.ToLower(strings.TrimSpace(question))
	if strings.Contains(lower, "yesterday") {
		return dateOnly(anchor).AddDate(0, 0, -1), true
	}
	match := relativeAgoRe.FindStringSubmatch(lower)
	if len(match) != 3 {
		return time.Time{}, false
	}
	n, ok := parseAgoAmount(match[1])
	if !ok {
		return time.Time{}, false
	}
	target := dateOnly(anchor)
	switch match[2] {
	case "day", "days":
		return target.AddDate(0, 0, -n), true
	case "week", "weeks":
		return target.AddDate(0, 0, -7*n), true
	case "month", "months":
		return target.AddDate(0, -n, 0), true
	case "year", "years":
		return target.AddDate(-n, 0, 0), true
	default:
		return time.Time{}, false
	}
}

func temporalRecallWindow(question, questionType, questionDate string) (*time.Time, *time.Time) {
	if questionType != "temporal-reasoning" {
		return nil, nil
	}
	anchor, ok := parseLongMemEvalQuestionDate(questionDate)
	if !ok {
		return nil, nil
	}
	lower := strings.ToLower(strings.TrimSpace(question))
	// Questions like "How many weeks ago..." require finding the target date
	// first; treating "weeks ago" as a filter would discard the answer.
	if strings.HasPrefix(lower, "how many") {
		return nil, nil
	}
	if strings.Contains(lower, "yesterday") {
		target := dateOnly(anchor).AddDate(0, 0, -1)
		before := target.AddDate(0, 0, 1)
		return &target, &before
	}
	match := relativeAgoRe.FindStringSubmatch(lower)
	if len(match) != 3 {
		return nil, nil
	}
	n, ok := parseAgoAmount(match[1])
	if !ok {
		return nil, nil
	}
	target := dateOnly(anchor)
	padDays := 0
	switch match[2] {
	case "day", "days":
		target = target.AddDate(0, 0, -n)
		padDays = 1
	case "week", "weeks":
		target = target.AddDate(0, 0, -7*n)
		padDays = 3
	case "month", "months":
		target = target.AddDate(0, -n, 0)
		padDays = 7
	case "year", "years":
		target = target.AddDate(-n, 0, 0)
		padDays = 30
	default:
		return nil, nil
	}
	since := target.AddDate(0, 0, -padDays)
	before := target.AddDate(0, 0, padDays+1)
	return &since, &before
}

func parseLongMemEvalQuestionDate(questionDate string) (time.Time, bool) {
	fields := strings.Fields(questionDate)
	if len(fields) == 0 {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006/01/02", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, fields[0], time.UTC); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func dateOnly(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
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
	writerErr := make(chan error, 1)
	wgWriter.Add(1)
	go func() {
		defer wgWriter.Done()
		writerErr <- longmemeval.WriteCheckpoint(ckptPath, ckptCh)
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
	if err := <-writerErr; err != nil {
		log.Printf("ERROR run checkpoint write failed: %v", err)
		return 1
	}

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

type dateRangeRecallFunc func(query string, topK int, since, before *time.Time) ([]string, error)

func recallWithTemporalFallback(query string, topK int, since, before *time.Time, recall dateRangeRecallFunc) ([]string, []string, error) {
	primary, err := recall(query, topK, since, before)
	if err != nil {
		return nil, nil, err
	}
	if since == nil && before == nil {
		return primary, nil, nil
	}
	fallback, err := recall(query, topK, nil, nil)
	if err != nil {
		log.Printf("WARN temporal fallback recall failed: %v", err)
		return primary, nil, nil
	}
	return longmemeval.UnionMemoryIDs(primary, fallback), fallback, nil
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
	since, before := temporalRecallWindow(item.Question, item.QuestionType, item.QuestionDate)
	recall := func(query string, topK int, callSince, callBefore *time.Time) ([]string, error) {
		return mcpClient.RecallWithDateRange(ctx, ingest.Project, query, topK, callSince, callBefore)
	}
	recallDefault := func(query string, topK int) ([]string, error) {
		return mcpClient.RecallWithDateRange(ctx, ingest.Project, query, topK, since, before)
	}
	retrievedIDs, temporalFallbackIDs, err := recallWithTemporalFallback(recallQuery, cfg.RecallTopK, since, before, recall)
	if err != nil {
		return longmemeval.RunEntry{
			QuestionID: item.QuestionID,
			Status:     "error",
			Error:      fmt.Sprintf("recall: %v", err),
		}
	}
	secondaryContextIDs := temporalFallbackIDs

	// H8 (lme-h8h12h15): exhaustive aggregation recall — run a topK=500 sweep on
	// the object noun-phrase for count-shaped questions and union with primary.
	if cfg.ExhaustiveAggregation && longmemeval.IsAggregationQuestion(item.Question) {
		const aggregationSweepTopK = 500
		sweepQuery := longmemeval.ExtractAggregationAnchor(item.Question)
		sweepIDs, sweepErr := recallDefault(sweepQuery, aggregationSweepTopK)
		if sweepErr == nil {
			secondaryContextIDs = append(secondaryContextIDs, sweepIDs...)
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
		anchor := longmemeval.PreferenceSubjectAnchorQuery(item.Question)
		anchorIDs, anchorErr := recallDefault(anchor, anchorTopK)
		if anchorErr == nil {
			secondaryContextIDs = append(secondaryContextIDs, anchorIDs...)
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
				pIDs, pErr := recallDefault(pq, cfg.RecallTopK)
				if pErr != nil {
					log.Printf("WARN run [%s] paraphrase recall (%q): %v", item.QuestionID, pq, pErr)
					continue
				}
				secondaryContextIDs = append(secondaryContextIDs, pIDs...)
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
	contextIDs := selectContextIDs(retrievedIDs, secondaryContextIDs, contextLimit)
	contextBlocks := make([]string, 0, contextLimit)
	for _, id := range contextIDs {
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

	// For relative N-ago temporal questions, rank context by proximity to the
	// requested historical target date. This is more precise than generic
	// chronological order and prevents nearby sessions from being displaced.
	if _, ok := targetDateFromQuestion(item.Question, item.QuestionType, item.QuestionDate); ok {
		contextBlocks = sortBlocksByTargetDate(contextBlocks, item.Question, item.QuestionType, item.QuestionDate)
	} else if cfg.ChronoSort {
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
	case cfg.PreferenceAnchor:
		// PA (#938): inject anchoring instructions for single-session-preference
		// questions to prevent the generator from averaging across sessions.
		// GenerationPromptForTypeWithPreferenceAnchor is a no-op for other types.
		prompt = longmemeval.GenerationPromptForTypeWithPreferenceAnchor(item.Question, item.QuestionType, item.QuestionDate, contextBlocks, true)
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

func selectContextIDs(retrievedIDs, secondaryIDs []string, limit int) []string {
	if limit <= 0 || len(retrievedIDs) == 0 {
		return nil
	}
	if limit > len(retrievedIDs) {
		limit = len(retrievedIDs)
	}
	out := make([]string, 0, limit)
	seen := make(map[string]bool, limit)
	for _, id := range retrievedIDs {
		if id == "" || seen[id] {
			continue
		}
		out = append(out, id)
		seen[id] = true
		if len(out) == limit {
			break
		}
	}
	if len(secondaryIDs) == 0 || len(out) == 0 {
		return out
	}
	reserve := (limit + 3) / 4
	if reserve < 1 {
		reserve = 1
	}
	if reserve > 3 {
		reserve = 3
	}
	filled := 0
	countedSecondary := make(map[string]bool, len(secondaryIDs))
	for _, id := range secondaryIDs {
		if id != "" && seen[id] && !countedSecondary[id] {
			countedSecondary[id] = true
			filled++
			if filled == reserve {
				return out
			}
		}
	}
	replaced := 0
	for _, id := range secondaryIDs {
		if id == "" || seen[id] || countedSecondary[id] {
			continue
		}
		slot := len(out) - 1 - replaced
		if slot < 0 {
			break
		}
		delete(seen, out[slot])
		out[slot] = id
		seen[id] = true
		replaced++
		if filled+replaced == reserve {
			break
		}
	}
	return out
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

func sortBlocksByTargetDate(blocks []string, question, questionType, questionDate string) []string {
	target, ok := targetDateFromQuestion(question, questionType, questionDate)
	if !ok {
		out := make([]string, len(blocks))
		copy(out, blocks)
		return out
	}
	out := make([]string, len(blocks))
	copy(out, blocks)
	sort.SliceStable(out, func(i, j int) bool {
		di := blockDistanceToTarget(out[i], target)
		dj := blockDistanceToTarget(out[j], target)
		return di < dj
	})
	return out
}

func blockDistanceToTarget(block string, target time.Time) time.Duration {
	d := blockDate(block)
	if d.IsZero() {
		return time.Duration(1<<63 - 1)
	}
	delta := dateOnly(d).Sub(dateOnly(target))
	if delta < 0 {
		return -delta
	}
	return delta
}
