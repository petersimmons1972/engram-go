package main

import (
	"context"
	"log"
	"path/filepath"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// runScoreBatch scores all pending run entries in a single Anthropic Message
// Batches API call. Items already scored CORRECT are preserved when
// --preserve-correct is set (default). Falls back gracefully: if ScoreBatch
// returns an error (e.g. missing API key), exits non-zero so the caller can
// retry with score-efficient or score.
func runScoreBatch(cfg *Config) int {
	if cfg.ScorerBatchAPIKey == "" {
		log.Printf("ERROR score-batch: ANTHROPIC_API_KEY is not set; use --api-key-anthropic or set the env var")
		return 1
	}

	items := loadItems(cfg.DataFile)
	itemMap := make(map[string]longmemeval.Item, len(items))
	for _, item := range items {
		itemMap[item.QuestionID] = item
	}

	runEntries, err := longmemeval.ReadAllRun(filepath.Join(cfg.OutDir, "checkpoint-run.jsonl"))
	if err != nil {
		log.Printf("ERROR score-batch: read run checkpoint: %v", err)
		return 1
	}
	runMap := make(map[string]longmemeval.RunEntry)
	for _, r := range runEntries {
		runMap[r.QuestionID] = r
	}

	ckptPath := filepath.Join(cfg.OutDir, "checkpoint-score.jsonl")
	labels, err := longmemeval.ReadScoredLabels(ckptPath)
	if err != nil {
		log.Printf("ERROR score-batch: read score labels: %v", err)
		return 1
	}

	preserveMode := cfg.PreserveCorrect && !cfg.ForceRescore
	skip, _ := buildPreserveSkipSet(labels, preserveMode)

	// Collect items to score.
	var batchItems []longmemeval.BatchScoringItem
	var skipped int
	for _, r := range runEntries {
		if r.Status != "done" {
			continue
		}
		if skip[r.QuestionID] {
			skipped++
			continue
		}
		item, ok := itemMap[r.QuestionID]
		if !ok {
			log.Printf("WARN score-batch: item %s not in data file — skipping", r.QuestionID)
			continue
		}
		batchItems = append(batchItems, longmemeval.BatchScoringItem{
			QuestionID:      r.QuestionID,
			Question:        item.Question,
			ReferenceAnswer: string(item.Answer),
			Hypothesis:      r.Hypothesis,
		})
	}
	log.Printf("score-batch: skipped=%d(CORRECT) queued=%d model=%s", skipped, len(batchItems), cfg.ScorerModel)

	if len(batchItems) == 0 {
		log.Printf("score-batch: nothing to score")
	} else {
		ctx := context.Background()
		results, err := longmemeval.ScoreBatch(ctx, batchItems, cfg.ScorerBatchAPIKey, cfg.ScorerModel)
		if err != nil {
			log.Printf("ERROR score-batch: ScoreBatch failed: %v", err)
			return 1
		}
		log.Printf("score-batch: batch returned %d results", len(results))

		// Write results to checkpoint.
		ckptCh := make(chan longmemeval.ScoreEntry, len(results)+1)
		for _, bi := range batchItems {
			r, ok := results[bi.QuestionID]
			if !ok {
				// Item absent from results — treat as error.
				ckptCh <- longmemeval.ScoreEntry{
					QuestionID: bi.QuestionID,
					Status:     "error",
					Error:      "missing from batch results",
				}
				continue
			}
			item := itemMap[bi.QuestionID]
			ckptCh <- longmemeval.ScoreEntry{
				QuestionID:   bi.QuestionID,
				QuestionType: item.QuestionType,
				Hypothesis:   bi.Hypothesis,
				ScoreLabel:   r.Label,
				Explanation:  r.Explanation,
				Status:       "done",
			}
			log.Printf("score-batch [%s] label=%s", bi.QuestionID, r.Label)
		}
		close(ckptCh)
		// WriteCheckpoint is designed for goroutine use but works synchronously
		// when the channel is already closed.
		longmemeval.WriteCheckpoint(ckptPath, ckptCh)
	}

	// Load all scores and write output files.
	allScores, err := longmemeval.ReadAllScore(ckptPath)
	if err != nil {
		log.Printf("ERROR score-batch: read final scores: %v", err)
		return 1
	}
	writeOutputs(cfg, itemMap, make(map[string]longmemeval.IngestEntry), runMap, allScores)
	log.Printf("score-batch: complete")
	return 0
}
