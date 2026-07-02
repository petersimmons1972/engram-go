package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

func runScore(cfg *Config) int {
	items := loadItems(cfg.DataFile)
	itemMap := make(map[string]longmemeval.Item, len(items))
	for _, item := range items {
		itemMap[item.QuestionID] = item
	}

	ingestMap, err := loadIngestMap(cfg.OutDir)
	if err != nil {
		log.Fatalf("read ingest checkpoint: %v", err)
	}

	runEntries, err := longmemeval.ReadAllRun(filepath.Join(cfg.OutDir, "checkpoint-run.jsonl"))
	if err != nil {
		log.Fatalf("read run checkpoint: %v", err)
	}
	runMap := make(map[string]longmemeval.RunEntry, len(runEntries))
	for _, r := range runEntries {
		runMap[r.QuestionID] = r
	}

	ckptPath := filepath.Join(cfg.OutDir, "checkpoint-score.jsonl")
	skip, err := longmemeval.ReadSkipSet(ckptPath)
	if err != nil {
		log.Fatalf("read score checkpoint: %v", err)
	}
	log.Printf("score: %d run entries loaded, %d already done", len(runEntries), len(skip))

	ckptCh := make(chan longmemeval.ScoreEntry, cfg.Workers*2)
	scoreStats := make(chan longmemeval.ScoreEntry, len(runEntries))
	writerErr := make(chan error, 1)
	var wgWriter sync.WaitGroup
	wgWriter.Add(1)
	go func() {
		defer wgWriter.Done()
		writerErr <- longmemeval.WriteCheckpoint(ckptPath, ckptCh)
	}()

	// Buffer sized to full input so pre-load before workers start cannot block.
	work := make(chan longmemeval.RunEntry, len(runEntries))
	for _, e := range runEntries {
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
			scoreWorker(cfg, itemMap, work, ckptCh, scoreStats)
		}()
	}
	wg.Wait()
	close(scoreStats)
	close(ckptCh)
	wgWriter.Wait()
	checkpointErr := <-writerErr

	var attempted, failed int
	for entry := range scoreStats {
		attempted++
		if entry.Status != "done" {
			failed++
		}
	}

	// Load all score entries and write output files.
	allScores, err := longmemeval.ReadAllScore(ckptPath)
	if err != nil {
		log.Fatalf("read final scores: %v", err)
	}
	writeOutputs(cfg, itemMap, ingestMap, runMap, allScores)
	writeRunManifest(cfg, "score", itemMap, ingestMap, runMap, allScores)
	if checkpointErr != nil {
		log.Printf("ERROR score checkpoint write failed: %v", checkpointErr)
		return 1
	}
	if attempted > 0 && failed == attempted {
		log.Printf("ERROR score: all attempted rows failed (attempted=%d failed=%d)", attempted, failed)
		return 1
	}
	log.Printf("score: complete")
	return 0
}

func loadIngestMap(outDir string) (map[string]longmemeval.IngestEntry, error) {
	ingestEntries, err := longmemeval.ReadAllIngest(filepath.Join(outDir, "checkpoint-ingest.jsonl"))
	if err != nil {
		return nil, err
	}
	ingestMap := make(map[string]longmemeval.IngestEntry, len(ingestEntries))
	for _, e := range ingestEntries {
		if e.Status == "done" {
			ingestMap[e.QuestionID] = e
		}
	}
	return ingestMap, nil
}

func scoreWorker(cfg *Config, itemMap map[string]longmemeval.Item, work <-chan longmemeval.RunEntry, out chan<- longmemeval.ScoreEntry, stats chan<- longmemeval.ScoreEntry) {
	ctx := context.Background()
	for runEntry := range work {
		item, ok := itemMap[runEntry.QuestionID]
		if !ok {
			entry := longmemeval.ScoreEntry{QuestionID: runEntry.QuestionID, Status: "error", Error: "item not in data file"}
			out <- entry
			stats <- entry
			continue
		}
		entry := scoreOne(ctx, cfg, item, runEntry)
		out <- entry
		stats <- entry
		log.Printf("score [%s] label=%s", runEntry.QuestionID, entry.ScoreLabel)
	}
}

func scoreOne(ctx context.Context, cfg *Config, item longmemeval.Item, run longmemeval.RunEntry) (entry longmemeval.ScoreEntry) {
	defer func() {
		if r := recover(); r != nil {
			entry = longmemeval.ScoreEntry{
				QuestionID: item.QuestionID,
				Status:     "error",
				Error:      fmt.Sprintf("panic: %v", r),
			}
		}
	}()

	var result longmemeval.ScoreResult
	var err error
	if cfg.LLMBaseURL != "" {
		result, err = longmemeval.ScoreOAI(ctx, item.Question, string(item.Answer), run.Hypothesis, cfg.LLMBaseURL, cfg.LLMModel, cfg.Retries)
	} else {
		result, err = longmemeval.Score(ctx, item.Question, string(item.Answer), run.Hypothesis, cfg.Retries)
	}
	if err != nil {
		return longmemeval.ScoreEntry{
			QuestionID:   item.QuestionID,
			QuestionType: item.QuestionType,
			Hypothesis:   run.Hypothesis,
			Status:       "error",
			Error:        err.Error(),
		}
	}
	return longmemeval.ScoreEntry{
		QuestionID:   item.QuestionID,
		QuestionType: item.QuestionType,
		Hypothesis:   run.Hypothesis,
		ScoreLabel:   result.Label,
		Explanation:  result.Explanation,
		Status:       "done",
	}
}

func writeOutputs(cfg *Config, itemMap map[string]longmemeval.Item, ingestMap map[string]longmemeval.IngestEntry, runMap map[string]longmemeval.RunEntry, scores []longmemeval.ScoreEntry) {
	writeHypotheses(cfg, scores)
	writeRetrievalLog(cfg, itemMap, ingestMap, runMap, scores)
	writeScoreReportWithCompleteness(
		cfg,
		scores,
		scoreCompletenessFromMaps(itemMap, ingestMap, runMap, scores),
	)
}

type scoreReportCounts struct {
	Correct          int `json:"correct"`
	PartiallyCorrect int `json:"partially_correct"`
	Incorrect        int `json:"incorrect"`
	Total            int `json:"total"`
}

func writeHypotheses(cfg *Config, scores []longmemeval.ScoreEntry) {
	f, err := createPrivateArtifact(filepath.Join(cfg.OutDir, "hypotheses.jsonl"))
	if err != nil {
		log.Printf("WARN write hypotheses.jsonl: %v", err)
		return
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	for _, s := range scores {
		if err := enc.Encode(longmemeval.HypothesisLine{QuestionID: s.QuestionID, Hypothesis: s.Hypothesis}); err != nil {
			log.Printf("WARN writeHypotheses encode [%s]: %v", s.QuestionID, err)
			break
		}
	}
	log.Printf("wrote %s", filepath.Join(cfg.OutDir, "hypotheses.jsonl"))
}

func writeRetrievalLog(cfg *Config, itemMap map[string]longmemeval.Item, ingestMap map[string]longmemeval.IngestEntry, runMap map[string]longmemeval.RunEntry, scores []longmemeval.ScoreEntry) {
	f, err := createPrivateArtifact(filepath.Join(cfg.OutDir, "retrieval_log.jsonl"))
	if err != nil {
		log.Printf("WARN write retrieval_log.jsonl: %v", err)
		return
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)

	for _, s := range scores {
		item, ok := itemMap[s.QuestionID]
		if !ok {
			continue
		}
		// Skip abstention questions per LongMemEval convention.
		if strings.HasSuffix(s.QuestionID, "_abs") {
			continue
		}
		ingest, ok := ingestMap[s.QuestionID]
		if !ok {
			continue
		}
		run, ok := runMap[s.QuestionID]
		if !ok {
			continue
		}
		sessionIDs := longmemeval.SessionIDs(run.RetrievedIDs, ingest.MemoryMap)
		var metrics longmemeval.RetrievalMetrics
		if cfg.SessionNDCGAgg {
			metrics = longmemeval.BuildRetrievalMetricsWithNDCGAgg(sessionIDs, item.AnswerSessionIDs, item.QuestionType)
		} else {
			metrics = longmemeval.BuildRetrievalMetrics(sessionIDs, item.AnswerSessionIDs)
		}

		var entry longmemeval.RetrievalLogEntry
		entry.QuestionID = s.QuestionID
		entry.RetrievalResults.Metrics.Session = metrics
		if err := enc.Encode(entry); err != nil {
			log.Printf("WARN writeRetrievalLog encode [%s]: %v", entry.QuestionID, err)
			break
		}
	}
	log.Printf("wrote %s", filepath.Join(cfg.OutDir, "retrieval_log.jsonl"))
}

func writeScoreReport(cfg *Config, scores []longmemeval.ScoreEntry) {
	writeScoreReportWithCompleteness(cfg, scores, scoreCompletenessFromScores(scores))
}

func writeScoreReportWithCompleteness(cfg *Config, scores []longmemeval.ScoreEntry, completeness scoreCompleteness) {
	// Deduplicate by QuestionID — last-write-wins, matching checkpoint append semantics.
	deduped := make(map[string]longmemeval.ScoreEntry, len(scores))
	for _, s := range scores {
		deduped[s.QuestionID] = s
	}

	overall := &scoreReportCounts{}
	byQType := make(map[string]*scoreReportCounts)

	for _, s := range deduped {
		if s.Status != "done" {
			continue
		}
		qbt := byQType[s.QuestionType]
		if qbt == nil {
			qbt = &scoreReportCounts{}
			byQType[s.QuestionType] = qbt
		}
		for _, bt := range []*scoreReportCounts{overall, qbt} {
			bt.Total++
			switch s.ScoreLabel {
			case "CORRECT":
				bt.Correct++
			case "PARTIALLY_CORRECT":
				bt.PartiallyCorrect++
			default:
				bt.Incorrect++
			}
		}
	}

	judgedAt := cfg.Now
	if judgedAt == nil {
		judgedAt = func() time.Time { return time.Now().UTC() }
	}

	scorerURL := redactURL(cfg.ScorerURL)
	scorerMaxTokens := cfg.ScorerMaxTokens
	if scorerMaxTokens <= 0 {
		scorerMaxTokens = longmemeval.DefaultScorerMaxTokens
	}

	report := map[string]any{
		"overall":               overall,
		"by_type":               byQType,
		"run_id":                cfg.RunID,
		"total_scored":          len(deduped),
		"expected_total":        completeness.ExpectedTotal,
		"ingest_done_total":     completeness.IngestDoneTotal,
		"completed_run_total":   completeness.CompletedRunTotal,
		"completed_score_total": completeness.CompletedScoreTotal,
		"run_error_total":       completeness.RunErrorTotal,
		"score_error_total":     completeness.ScoreErrorTotal,
		"complete":              completeness.Complete,
		"scorer_version":        cfg.ScorerVersion,
		"scorer_model":          cfg.ScorerModel,
		"scorer_url":            scorerURL,
		"scorer_thinking":       cfg.ScorerThinking,
		"scorer_max_tokens":     scorerMaxTokens,
		"judged_at":             judgedAt().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		log.Printf("WARN marshal score report: %v", err)
		return
	}
	path := filepath.Join(cfg.OutDir, "score_report.json")
	if err := os.WriteFile(path, data, privateArtifactFileMode); err != nil {
		log.Printf("WARN write score_report.json: %v", err)
		return
	}
	if err := os.Chmod(path, privateArtifactFileMode); err != nil {
		log.Printf("WARN chmod score_report.json: %v", err)
		return
	}
	log.Printf("wrote %s", path)

	writeScoreSummary(cfg.Output, cfg.ScoreOutput, report, overall)
}

func writeScoreSummary(w io.Writer, mode string, report map[string]any, overall *scoreReportCounts) {
	if w == nil {
		w = os.Stdout
	}
	if mode == "" {
		mode = "text"
	}
	switch mode {
	case "quiet":
		return
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
		return
	}

	if overall == nil || overall.Total == 0 {
		return
	}
	pct := func(n int) float64 { return float64(n) / float64(overall.Total) * 100 }
	_, _ = fmt.Fprintf(w, "\n--- Score Report (run-id: %s) ---\n", report["run_id"])
	_, _ = fmt.Fprintf(w, "Total scored:       %d\n", overall.Total)
	_, _ = fmt.Fprintf(w, "Correct:            %d (%.1f%%)\n", overall.Correct, pct(overall.Correct))
	_, _ = fmt.Fprintf(w, "Partially correct:  %d (%.1f%%)\n", overall.PartiallyCorrect, pct(overall.PartiallyCorrect))
	_, _ = fmt.Fprintf(w, "Incorrect:          %d (%.1f%%)\n", overall.Incorrect, pct(overall.Incorrect))
}

// normalizeLabel canonicalises a score label: trims surrounding whitespace
// and upper-cases the remainder. Empty input returns empty output.
func normalizeLabel(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}
