package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

func runScore(cfg *Config) {
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

	runEntries, err := longmemeval.ReadAllRun(cfg.OutDir + "/checkpoint-run.jsonl")
	if err != nil {
		log.Fatalf("read run checkpoint: %v", err)
	}
	runMap := make(map[string]longmemeval.RunEntry, len(runEntries))
	for _, r := range runEntries {
		runMap[r.QuestionID] = r
	}

	ckptPath := cfg.OutDir + "/checkpoint-score.jsonl"
	skip, err := longmemeval.ReadSkipSet(ckptPath)
	if err != nil {
		log.Fatalf("read score checkpoint: %v", err)
	}
	log.Printf("score: %d run entries loaded, %d already done", len(runEntries), len(skip))

	ckptCh := make(chan longmemeval.ScoreEntry, cfg.Workers*2)
	var wgWriter sync.WaitGroup
	wgWriter.Add(1)
	go func() {
		defer wgWriter.Done()
		longmemeval.WriteCheckpoint(ckptPath, ckptCh)
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
			scoreWorker(cfg, itemMap, work, ckptCh)
		}()
	}
	wg.Wait()
	close(ckptCh)
	wgWriter.Wait()

	// Load all score entries and write output files.
	allScores, err := longmemeval.ReadAllScore(ckptPath)
	if err != nil {
		log.Fatalf("read final scores: %v", err)
	}
	writeOutputs(cfg, itemMap, ingestMap, runMap, allScores)
	log.Printf("score: complete")
}

func scoreWorker(cfg *Config, itemMap map[string]longmemeval.Item, work <-chan longmemeval.RunEntry, out chan<- longmemeval.ScoreEntry) {
	ctx := context.Background()
	for runEntry := range work {
		item, ok := itemMap[runEntry.QuestionID]
		if !ok {
			out <- longmemeval.ScoreEntry{QuestionID: runEntry.QuestionID, Status: "error", Error: "item not in data file"}
			continue
		}
		entry := scoreOne(ctx, cfg, item, runEntry)
		out <- entry
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
		result, err = longmemeval.ScoreOAI(ctx, item.Question, fmt.Sprint(item.Answer), run.Hypothesis, cfg.LLMBaseURL, cfg.LLMModel, cfg.Retries)
	} else {
		result, err = longmemeval.Score(ctx, item.Question, fmt.Sprint(item.Answer), run.Hypothesis, cfg.Retries)
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
	writeScoreReport(cfg, scores)
}

func writeHypotheses(cfg *Config, scores []longmemeval.ScoreEntry) {
	f, err := os.Create(cfg.OutDir + "/hypotheses.jsonl")
	if err != nil {
		log.Printf("WARN write hypotheses.jsonl: %v", err)
		return
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, s := range scores {
		if err := enc.Encode(longmemeval.HypothesisLine{QuestionID: s.QuestionID, Hypothesis: s.Hypothesis}); err != nil {
			log.Printf("WARN writeHypotheses encode [%s]: %v", s.QuestionID, err)
			break
		}
	}
	log.Printf("wrote %s/hypotheses.jsonl", cfg.OutDir)
}

func writeRetrievalLog(cfg *Config, itemMap map[string]longmemeval.Item, ingestMap map[string]longmemeval.IngestEntry, runMap map[string]longmemeval.RunEntry, scores []longmemeval.ScoreEntry) {
	f, err := os.Create(cfg.OutDir + "/retrieval_log.jsonl")
	if err != nil {
		log.Printf("WARN write retrieval_log.jsonl: %v", err)
		return
	}
	defer f.Close()
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
		metrics := longmemeval.BuildRetrievalMetrics(sessionIDs, item.AnswerSessionIDs)

		var entry longmemeval.RetrievalLogEntry
		entry.QuestionID = s.QuestionID
		entry.RetrievalResults.Metrics.Session = metrics
		if err := enc.Encode(entry); err != nil {
			log.Printf("WARN writeRetrievalLog encode [%s]: %v", entry.QuestionID, err)
			break
		}
	}
	log.Printf("wrote %s/retrieval_log.jsonl", cfg.OutDir)
}

func writeScoreReport(cfg *Config, scores []longmemeval.ScoreEntry) {
	type byType struct {
		Correct          int `json:"correct"`
		PartiallyCorrect int `json:"partially_correct"`
		Incorrect        int `json:"incorrect"`
		Total            int `json:"total"`
	}
	overall := &byType{}
	byQType := make(map[string]*byType)

	for _, s := range scores {
		if s.Status != "done" {
			continue
		}
		qbt := byQType[s.QuestionType]
		if qbt == nil {
			qbt = &byType{}
			byQType[s.QuestionType] = qbt
		}
		for _, bt := range []*byType{overall, qbt} {
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

	report := map[string]any{
		"overall":      overall,
		"by_type":      byQType,
		"run_id":       cfg.RunID,
		"total_scored": len(scores),
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		log.Printf("WARN marshal score report: %v", err)
		return
	}
	path := cfg.OutDir + "/score_report.json"
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("WARN write score_report.json: %v", err)
		return
	}
	log.Printf("wrote %s", path)

	// Print summary.
	if overall.Total > 0 {
		pct := func(n int) float64 { return float64(n) / float64(overall.Total) * 100 }
		fmt.Printf("\n--- Score Report (run-id: %s) ---\n", cfg.RunID)
		fmt.Printf("Total scored:       %d\n", overall.Total)
		fmt.Printf("Correct:            %d (%.1f%%)\n", overall.Correct, pct(overall.Correct))
		fmt.Printf("Partially correct:  %d (%.1f%%)\n", overall.PartiallyCorrect, pct(overall.PartiallyCorrect))
		fmt.Printf("Incorrect:          %d (%.1f%%)\n", overall.Incorrect, pct(overall.Incorrect))
	}
}

// normalizeLabel is kept for backward compatibility with score checkpoint files
// written by the old runner.
func normalizeLabel(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }
