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

type contextBlock struct {
	Content   string
	SessionID string
	Date      string
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
var urlRe = regexp.MustCompile(`https?://[^\s)]+`)
var phoneRe = regexp.MustCompile(`\b(?:\+?1[-.\s]?)?(?:\(?\d{3}\)?[-.\s]?){2}\d{4}\b`)
var quotedRe = regexp.MustCompile(`"([^"]+)"`)

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

func haystackDateBySessionID(item longmemeval.Item) map[string]string {
	limit := len(item.HaystackSessionIDs)
	if len(item.HaystackDates) < limit {
		limit = len(item.HaystackDates)
	}
	out := make(map[string]string, limit)
	for i := 0; i < limit; i++ {
		sessionID := strings.TrimSpace(item.HaystackSessionIDs[i])
		date := strings.TrimSpace(item.HaystackDates[i])
		if sessionID == "" || date == "" {
			continue
		}
		out[sessionID] = date
	}
	return out
}

func formatContextBlock(content, sessionID, date string) string {
	parts := make([]string, 0, 2)
	if sessionID != "" {
		parts = append(parts, "Session: "+sessionID)
	}
	if date != "" {
		parts = append(parts, "Date: "+date)
	}
	if len(parts) == 0 {
		return content
	}
	return "[" + strings.Join(parts, " | ") + "]\n" + content
}

func formatContextBlocks(blocks []contextBlock) []string {
	out := make([]string, 0, len(blocks))
	for _, block := range blocks {
		out = append(out, formatContextBlock(block.Content, block.SessionID, block.Date))
	}
	return out
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

	// #1079: oracle mode bypasses Engram entirely — no ingest checkpoint exists
	// or is needed. Synthesize a minimal IngestEntry per item so the run loop
	// processes all items without requiring a prior ingest stage.
	var ingestEntries []longmemeval.IngestEntry
	if cfg.AtomOracle {
		ingestEntries = make([]longmemeval.IngestEntry, 0, len(items))
		for _, item := range items {
			ingestEntries = append(ingestEntries, longmemeval.IngestEntry{
				QuestionID: item.QuestionID,
				Project:    "", // no Engram project for oracle mode
				Status:     "done",
			})
		}
		log.Printf("run: oracle mode — synthesized %d ingest entries (no checkpoint required)", len(ingestEntries))
	} else {
		var err error
		ingestEntries, err = longmemeval.ReadAllIngest(filepath.Join(cfg.OutDir, "checkpoint-ingest.jsonl"))
		if err != nil {
			log.Printf("ERROR read ingest checkpoint: %v", err)
			return 1
		}
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

type dateRangeRecallFunc func(query string, topK int, since, before *time.Time) (longmemeval.RecallResult, error)

func recallWithTemporalFallback(query string, topK int, since, before *time.Time, recall dateRangeRecallFunc) (longmemeval.RecallResult, []string, error) {
	primary, err := recall(query, topK, since, before)
	if err != nil {
		return longmemeval.RecallResult{}, nil, err
	}
	if since == nil && before == nil {
		return primary, nil, nil
	}
	fallback, err := recall(query, topK, nil, nil)
	if err != nil {
		log.Printf("WARN temporal fallback recall failed: %v", err)
		return primary, nil, nil
	}
	primary.IDs = longmemeval.UnionMemoryIDs(primary.IDs, fallback.IDs)
	return primary, fallback.IDs, nil
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

		// #1079: oracle mode bypasses Engram entirely — no MCP connection needed.
		if cfg.AtomOracle {
			entry := runOneOracle(ctx, cfg, item, ingestEntry)
			out <- entry
			if entry.Status == "error" {
				log.Printf("run [%s] status=%s hypothesis_len=%d error=%q", item.QuestionID, entry.Status, len(entry.Hypothesis), entry.Error)
			} else {
				log.Printf("run [%s] status=%s hypothesis_len=%d", item.QuestionID, entry.Status, len(entry.Hypothesis))
			}
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

	// Phase 2A (#1079): oracle mode bypasses all Engram recall. Atoms are
	// extracted locally from gold sessions and injected as the context.
	if cfg.AtomOracle {
		return runOneOracle(ctx, cfg, item, ingest)
	}

	// Strip leading interrogative phrases for temporal questions so the recall
	// query matches event noun-phrases rather than "how many weeks ago did...".
	// When --disable-query-rewrite is set, use the raw question unchanged.
	runOpts := longmemeval.RunOpts{
		ExhaustiveAggregation: cfg.ExhaustiveAggregation,
		EnumerateFirst:        cfg.EnumerateFirst,
	}
	recallQuery := buildRecallQuery(item.Question, item.QuestionType, cfg.DisableQueryRewrite)
	effectiveRecallTopK := runOpts.EffectiveRecallTopK(item.Question, cfg.RecallTopK)
	since, before := temporalRecallWindow(item.Question, item.QuestionType, item.QuestionDate)
	var atomRecallAsOf *time.Time
	if cfg.AtomMode {
		if asOf, ok := parseLongMemEvalQuestionDate(item.QuestionDate); ok {
			atomRecallAsOf = &asOf
		}
	}

	recall := func(query string, topK int, callSince, callBefore *time.Time) (longmemeval.RecallResult, error) {
		if cfg.AtomMode {
			return mcpClient.RecallWithAtomRecall(ctx, ingest.Project, query, topK, callSince, callBefore, cfg.TopicAnchorBoost, cfg.ExactSignalBoost, atomRecallAsOf)
		}
		if cfg.ExactSignalBoost {
			ids, err := mcpClient.RecallWithExactBoost(ctx, ingest.Project, query, topK, callSince, callBefore)
			return longmemeval.RecallResult{IDs: ids}, err
		}
		ids, err := mcpClient.RecallWithOpts(ctx, ingest.Project, query, topK, callSince, callBefore, cfg.TopicAnchorBoost)
		return longmemeval.RecallResult{IDs: ids}, err
	}
	recallScoredDefault := func(query string, topK int) ([]longmemeval.ScoredMemoryID, error) {
		if cfg.ExactSignalBoost {
			return mcpClient.RecallScoredWithExactBoost(ctx, ingest.Project, query, topK, since, before)
		}
		return mcpClient.RecallScoredWithOpts(ctx, ingest.Project, query, topK, since, before, cfg.TopicAnchorBoost)
	}
	recallDefault := func(query string, topK int) (longmemeval.RecallResult, error) {
		if cfg.AtomMode {
			return mcpClient.RecallWithAtomRecall(ctx, ingest.Project, query, topK, since, before, cfg.TopicAnchorBoost, cfg.ExactSignalBoost, atomRecallAsOf)
		}
		if cfg.ExactSignalBoost {
			ids, err := mcpClient.RecallWithExactBoost(ctx, ingest.Project, query, topK, since, before)
			return longmemeval.RecallResult{IDs: ids}, err
		}
		ids, err := mcpClient.RecallWithOpts(ctx, ingest.Project, query, topK, since, before, cfg.TopicAnchorBoost)
		return longmemeval.RecallResult{IDs: ids}, err
	}

	// H-NEW-1: when --temporal-window-recall is set, hand temporal anchoring to the
	// server for temporal-reasoning questions. The server runs a two-pass
	// date-windowed recall (unfiltered + valid_from-filtered, unioned) using the raw
	// question and question_date, and falls back to baseline single-pass recall
	// server-side for non-windowable questions ("how many X ago"). This path is
	// exclusive of the client-side temporal fallback and the recall-augmentation
	// passes below (fusion/paraphrase/aggregation) so the lever is measured cleanly.
	var (
		retrievedIDs        []string
		temporalFallbackIDs []string
		atomPreamble        string
		primaryScoredHits   []longmemeval.ScoredMemoryID
		err                 error
	)
	serverTemporalWindow := cfg.TemporalWindowRecall && item.QuestionType == "temporal-reasoning"
	dualPreferenceRecall := cfg.DualPreferenceRecall && !serverTemporalWindow && longmemeval.IsInferredPreferenceQuestion(item.Question)
	var sessionDominanceRatio float64
	var contextSessionCount int
	if serverTemporalWindow {
		retrievedIDs, err = mcpClient.RecallWithTemporalWindow(ctx, ingest.Project, recallQuery, effectiveRecallTopK, item.Question, item.QuestionDate)
		if err != nil {
			return longmemeval.RunEntry{
				QuestionID: item.QuestionID,
				Status:     "error",
				Error:      fmt.Sprintf("temporal-window recall: %v", err),
			}
		}
	} else if dualPreferenceRecall {
		primaryScoredHits, err = recallScoredDefault(recallQuery, cfg.RecallTopK)
		if err != nil {
			return longmemeval.RunEntry{
				QuestionID: item.QuestionID,
				Status:     "error",
				Error:      fmt.Sprintf("recall: %v", err),
			}
		}
		retrievedIDs = longmemeval.IDsFromScoredRecall(primaryScoredHits)
	} else {
		recallResult, fallbackIDs, recallErr := recallWithTemporalFallback(recallQuery, effectiveRecallTopK, since, before, recall)
		retrievedIDs, temporalFallbackIDs, atomPreamble, err = recallResult.IDs, fallbackIDs, recallResult.AtomPreamble, recallErr
		if err != nil {
			return longmemeval.RunEntry{
				QuestionID: item.QuestionID,
				Status:     "error",
				Error:      fmt.Sprintf("recall: %v", err),
			}
		}
		sessionDominanceRatio, contextSessionCount = computeSessionDiagnostics(recallResult.Results)
	}
	secondaryContextIDs := temporalFallbackIDs
	if cfg.RetrievalFusion && !serverTemporalWindow {
		// Fix #938: fusion candidates flow through retrievedIDs (primary) only —
		// NOT secondaryContextIDs — to avoid the reserve≤3 secondary-slot cap in
		// selectContextIDs. UnionMemoryIDs deduplicates and preserves primary order,
		// so fusion hits appear after the primary batch and are included when
		// contextLimit allows. Adding them to secondaryContextIDs would throttle all
		// fusion candidates to at most 3 swapped-in slots, defeating the lever.
		variants := buildRecallVariants(item.Question, recallQuery, cfg.DisableQueryRewrite, true)
		for _, q := range variants[1:] {
			recallResult, qErr := recallDefault(q, effectiveRecallTopK)
			if qErr != nil {
				log.Printf("WARN run [%s] fusion recall (%q): %v", item.QuestionID, q, qErr)
				continue
			}
			retrievedIDs = longmemeval.UnionMemoryIDs(retrievedIDs, recallResult.IDs)
		}
	}

	// H8 (lme-h8h12h15): exhaustive aggregation recall is now handled by
	// RunOpts.EffectiveRecallTopK (deep primary recall for count-shaped questions)
	// and RunOpts.UseFullAggregationContext (full result set into context), rather
	// than a separate anchor sweep — see internal/longmemeval/claude_additions.go.

	// H15 (lme-h8h12h15): dual-query preference recall — run a second recall
	// using the subject-anchor query for inferred preference questions only.
	if dualPreferenceRecall {
		anchorTopK := cfg.RecallTopK / 2
		if anchorTopK < 1 {
			anchorTopK = 1
		}
		anchor := longmemeval.PreferenceSubjectAnchorQuery(item.Question)
		anchorHits, anchorErr := recallScoredDefault(anchor, anchorTopK)
		if anchorErr == nil {
			anchorIDs := longmemeval.IDsFromScoredRecall(anchorHits)
			secondaryContextIDs = append(secondaryContextIDs, anchorIDs...)
			retrievedIDs = longmemeval.IDsFromScoredRecall(longmemeval.MergeScoredRecall(primaryScoredHits, anchorHits))
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
	if cfg.QueryParaphrasePasses > 0 && !serverTemporalWindow {
		paraphrases, pErr := longmemeval.GenerateParaphrases(ctx, recallQuery, cfg.QueryParaphrasePasses, cfg.Retries)
		if pErr != nil {
			log.Printf("WARN run [%s] paraphrase: %v — falling back to single-pass recall", item.QuestionID, pErr)
		} else {
			for _, pq := range paraphrases {
				pResult, pErr := recallDefault(pq, effectiveRecallTopK)
				if pErr != nil {
					log.Printf("WARN run [%s] paraphrase recall (%q): %v", item.QuestionID, pq, pErr)
					continue
				}
				secondaryContextIDs = append(secondaryContextIDs, pResult.IDs...)
				retrievedIDs = append(retrievedIDs, pResult.IDs...)
			}
			retrievedIDs = longmemeval.DeduplicateIDs(retrievedIDs)
		}
	}

	// Fetch content for top contextLimit memories.
	// --context-topk overrides per-type default; 0 means use per-type default.
	var contextLimit int
	if cfg.ContextTopKOverride > 0 {
		contextLimit = cfg.ContextTopKOverride
	} else if runOpts.UseFullAggregationContext(item.Question) {
		contextLimit = len(retrievedIDs)
	} else {
		contextLimit = longmemeval.ContextTopKForTypeWithBump(item.QuestionType, cfg.ContextTopKBump)
	}
	if contextLimit > len(retrievedIDs) {
		contextLimit = len(retrievedIDs)
	}
	contextIDs := selectContextIDs(retrievedIDs, secondaryContextIDs, contextLimit)
	sessionDateByID := haystackDateBySessionID(item)
	contextBlocks := make([]string, 0, contextLimit)
	contentByID := make(map[string]string, contextLimit)
	contextBlockByID := make(map[string]string, contextLimit)
	for _, id := range contextIDs {
		content, err := mcpClient.FetchContent(ctx, ingest.Project, id)
		if err != nil {
			log.Printf("WARN run [%s] fetch %s: %v", item.QuestionID, id, err)
			continue
		}
		if content != "" {
			// Truncate at storage time so contentByID and contextBlocks stay
			// consistent. If --exact-signal-boost later rebuilds contextBlocks
			// from contentByID, it gets already-truncated content — preventing
			// full-length blocks from exceeding the model's context window.
			if cfg.MaxBlockChars > 0 && len(content) > cfg.MaxBlockChars {
				content = content[:cfg.MaxBlockChars]
			}
			sessionID := ingest.MemoryMap[id]
			block := formatContextBlock(content, sessionID, sessionDateByID[sessionID])
			contentByID[id] = content
			contextBlockByID[id] = block
			contextBlocks = append(contextBlocks, block)
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

	// Fix #938: exact-signal reorders run AFTER chrono-sort so temporal ordering
	// is not overwritten. --evidence-first-pack is suppressed for temporal-reasoning
	// questions where chrono order is load-bearing (mirrors --temporal-prompt-aug
	// precedence: the temporal branch takes priority over other reordering passes).
	if cfg.ExactSignalBoost {
		ranked := rankIDsByExactSignals(contextIDs, item.Question, contentByID)
		reordered := make([]string, 0, len(ranked))
		for _, id := range ranked {
			if block := contextBlockByID[id]; block != "" {
				reordered = append(reordered, block)
			}
		}
		if len(reordered) > 0 {
			contextBlocks = reordered
		}
	}
	if cfg.EvidenceFirstPacked && item.QuestionType != "temporal-reasoning" {
		// Skip for temporal-reasoning: chrono order is load-bearing there and
		// evidence-first packing would displace temporally-proximate blocks.
		contextBlocks = orderContextEvidenceFirst(contextBlocks, item.Question)
	}

	var atomContextBlock string
	if cfg.AtomMode {
		atomContextBlock = atomPreamble
		if atomContextBlock == "" {
			atomContextBlock = fetchAtomContextBlock(ctx, mcpClient, ingest.Project, item.QuestionID, cfg.AtomCacheDir)
		}
	}
	// Exp-14: --temporal-prompt-aug takes priority over --inject-question-date;
	// the two are mutually exclusive. When both are set, aug wins.
	// H12 (--enumerate-first) is applied after the per-type prompt is chosen
	// (via runOpts.ApplyEnumerateFirst below) so it prefixes the baseline prompt
	// instead of replacing it.
	// H-PE (--preference-enumerate) is orthogonal — it only fires for
	// single-session-preference questions and is independent of the other flags.
	var prompt string
	switch {
	case cfg.TemporalPromptAug:
		prompt = longmemeval.GenerationPromptForTypeWithTemporalAug(item.Question, item.QuestionType, item.QuestionDate, contextBlocks, true)
	case cfg.InjectQuestionDate:
		prompt = longmemeval.GenerationPromptForTypeWithDateInjection(item.Question, item.QuestionType, item.QuestionDate, contextBlocks, true)
	case cfg.PreferenceEnumerate:
		prompt = longmemeval.GenerationPromptForTypePreferenceEnumerate(item.Question, item.QuestionType, item.QuestionDate, contextBlocks, true)
	case cfg.PreferenceGround:
		prompt = longmemeval.GenerationPromptForTypePreferenceGround(item.Question, item.QuestionType, item.QuestionDate, contextBlocks, true)
	case cfg.PreferenceQuoteFirst:
		prompt = longmemeval.GenerationPromptForTypePreferenceQuoteFirst(item.Question, item.QuestionType, item.QuestionDate, contextBlocks, true)
	case cfg.KURecencyPrompt:
		prompt = longmemeval.GenerationPromptForTypeWithKURecency(item.Question, item.QuestionType, item.QuestionDate, contextBlocks, true)
	default:
		prompt = longmemeval.GenerationPromptForType(item.Question, item.QuestionType, item.QuestionDate, contextBlocks)
	}
	prompt = runOpts.ApplyEnumerateFirst(prompt, item.Question, item.QuestionType)

	// #938: prepend atom context block before the memory context when --atom-mode is set.
	if atomContextBlock != "" {
		prompt = atomContextBlock + "\n" + prompt
	}
	var hypothesis string
	if cfg.LLMBaseURL != "" {
		maxTok := cfg.LLMMaxTokens
		if maxTok == 0 && cfg.EnableThinking {
			maxTok = 8192 // thinking mode default: room for reasoning chain + answer
		}
		opts := longmemeval.OAIOptions{EnableThinking: cfg.EnableThinking, MaxTokens: maxTok, APIKey: cfg.LLMApiKey}
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
		QuestionID:            item.QuestionID,
		Hypothesis:            hypothesis,
		RetrievedIDs:          retrievedIDs,
		SessionDominanceRatio: sessionDominanceRatio,
		ContextSessionCount:   contextSessionCount,
		Status:                "done",
		AtomRetrieved:         atomPreamble != "",
		AtomInContext:         atomContextBlock != "",
	}
}


func buildRecallVariants(question, primary string, disableRewrite, includeIdentifiers bool) []string {
	seen := map[string]bool{}
	add := func(dst []string, q string) []string {
		q = strings.TrimSpace(q)
		if q == "" || seen[q] {
			return dst
		}
		seen[q] = true
		return append(dst, q)
	}
	out := add(nil, primary)
	if !disableRewrite {
		out = add(out, question)
	}
	if includeIdentifiers {
		for _, idq := range extractExactSignals(question) {
			out = add(out, idq)
		}
	}
	return out
}

func extractExactSignals(question string) []string {
	var out []string
	out = append(out, urlRe.FindAllString(question, -1)...)
	out = append(out, phoneRe.FindAllString(question, -1)...)
	for _, m := range quotedRe.FindAllStringSubmatch(question, -1) {
		if len(m) > 1 {
			out = append(out, m[1])
		}
	}
	return longmemeval.DeduplicateIDs(out)
}

func scoreExactSignals(text, question string) int {
	score := 0
	lower := strings.ToLower(text)
	for _, sig := range extractExactSignals(question) {
		if strings.Contains(lower, strings.ToLower(sig)) {
			score += 3
		}
	}
	return score
}

func rankIDsByExactSignals(ids []string, question string, contentByID map[string]string) []string {
	out := make([]string, len(ids))
	copy(out, ids)
	sort.SliceStable(out, func(i, j int) bool {
		return scoreExactSignals(contentByID[out[i]], question) > scoreExactSignals(contentByID[out[j]], question)
	})
	return out
}

func orderContextEvidenceFirst(blocks []string, question string) []string {
	out := make([]string, len(blocks))
	copy(out, blocks)
	sort.SliceStable(out, func(i, j int) bool {
		return scoreExactSignals(stripContextMetadataHeader(out[i]), question) > scoreExactSignals(stripContextMetadataHeader(out[j]), question)
	})
	return out
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
var contextMetadataDateRe = regexp.MustCompile(`(?m)^\[(?:Session:\s*[^|\]]+\s*\|\s*)?Date:\s*(\d{4}-\d{2}-\d{2})\]`)

func stripContextMetadataHeader(block string) string {
	header, rest, ok := strings.Cut(block, "\n")
	if !ok {
		return block
	}
	if strings.HasPrefix(header, "[") && strings.HasSuffix(header, "]") &&
		(strings.Contains(header, "Session:") || strings.Contains(header, "Date:")) {
		return rest
	}
	return block
}

// blockDate extracts the Session date from the first matching line of a memory
// block. Returns time.Time{} (zero value / 1970) if no date is found.
func blockDate(block string) time.Time {
	m := contextMetadataDateRe.FindStringSubmatch(block)
	if m == nil {
		m = sessionDateRe.FindStringSubmatch(block)
	}
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
