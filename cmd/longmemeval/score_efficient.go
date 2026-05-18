package main

import (
	"context"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// ollaHealthCheck returns true if the OAI /models endpoint responds with HTTP 200.
func ollaHealthCheck(baseURL string) bool {
	url := strings.TrimRight(baseURL, "/") + "/models"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// buildPreserveSkipSet splits scored labels into skip (CORRECT, preserve) and
// retry (PARTIALLY_CORRECT / INCORRECT). When preserveCorrect=false, skip is empty.
func buildPreserveSkipSet(labels map[string]string, preserveCorrect bool) (skip, retry map[string]bool) {
	skip = make(map[string]bool)
	retry = make(map[string]bool)
	for qid, label := range labels {
		if label == "CORRECT" {
			if preserveCorrect {
				skip[qid] = true
			}
		} else {
			retry[qid] = true
		}
	}
	return skip, retry
}

func runScoreEfficient(cfg *Config) int {
	items := loadItems(cfg.DataFile)
	itemMap := make(map[string]longmemeval.Item, len(items))
	for _, item := range items {
		itemMap[item.QuestionID] = item
	}

	runEntries, err := longmemeval.ReadAllRun(filepath.Join(cfg.OutDir, "checkpoint-run.jsonl"))
	if err != nil {
		log.Printf("ERROR read run checkpoint: %v", err)
		return 1
	}
	runMap := make(map[string]longmemeval.RunEntry)
	for _, r := range runEntries {
		runMap[r.QuestionID] = r
	}

	ckptPath := filepath.Join(cfg.OutDir, "checkpoint-score.jsonl")
	labels, err := longmemeval.ReadScoredLabels(ckptPath)
	if err != nil {
		log.Printf("ERROR read score labels: %v", err)
		return 1
	}

	preserveMode := cfg.PreserveCorrect && !cfg.ForceRescore
	skip, _ := buildPreserveSkipSet(labels, preserveMode)

	// Decide backend: olla OAI or Haiku subprocess fallback
	useOAI := cfg.ScorerURL != "" && ollaHealthCheck(cfg.ScorerURL)
	if useOAI {
		log.Printf("score-efficient: backend=olla url=%s model=%s", cfg.ScorerURL, cfg.ScorerModel)
	} else {
		log.Printf("score-efficient: olla unavailable — fallback to claude --print --model haiku")
	}

	work := make(chan longmemeval.RunEntry, len(runEntries))
	var skipped, queued int
	for _, r := range runEntries {
		if r.Status != "done" {
			continue
		}
		if skip[r.QuestionID] {
			skipped++
			continue
		}
		work <- r
		queued++
	}
	close(work)
	log.Printf("score-efficient: skipped=%d(CORRECT) queued=%d", skipped, queued)

	ckptCh := make(chan longmemeval.ScoreEntry, cfg.Workers*2)
	var wgWriter sync.WaitGroup
	wgWriter.Add(1)
	go func() {
		defer wgWriter.Done()
		longmemeval.WriteCheckpoint(ckptPath, ckptCh)
	}()

	var wg sync.WaitGroup
	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			scoreEfficientWorker(cfg, itemMap, work, ckptCh, useOAI)
		}()
	}
	wg.Wait()
	close(ckptCh)
	wgWriter.Wait()

	allScores, err := longmemeval.ReadAllScore(ckptPath)
	if err != nil {
		log.Printf("ERROR read final scores: %v", err)
		return 1
	}
	writeOutputs(cfg, itemMap, make(map[string]longmemeval.IngestEntry), runMap, allScores)
	log.Printf("score-efficient: complete")
	return 0
}

func scoreEfficientWorker(cfg *Config, itemMap map[string]longmemeval.Item,
	work <-chan longmemeval.RunEntry, out chan<- longmemeval.ScoreEntry, useOAI bool) {
	ctx := context.Background()
	for r := range work {
		item, ok := itemMap[r.QuestionID]
		if !ok {
			out <- longmemeval.ScoreEntry{QuestionID: r.QuestionID, Status: "error", Error: "item not in data file"}
			continue
		}
		var result longmemeval.ScoreResult
		var err error
		if useOAI {
			result, err = longmemeval.ScoreOAIEfficient(ctx,
				item.Question, string(item.Answer), r.Hypothesis,
				cfg.ScorerURL, cfg.ScorerModel, cfg.Retries)
		} else {
			result, err = longmemeval.Score(ctx,
				item.Question, string(item.Answer), r.Hypothesis, cfg.Retries)
		}
		if err != nil {
			out <- longmemeval.ScoreEntry{
				QuestionID:   r.QuestionID,
				QuestionType: item.QuestionType,
				Hypothesis:   r.Hypothesis,
				Status:       "error",
				Error:        err.Error(),
			}
			log.Printf("score-efficient [%s] error: %v", r.QuestionID, err)
			continue
		}
		out <- longmemeval.ScoreEntry{
			QuestionID:   r.QuestionID,
			QuestionType: item.QuestionType,
			Hypothesis:   r.Hypothesis,
			ScoreLabel:   result.Label,
			Explanation:  result.Explanation,
			Status:       "done",
		}
		log.Printf("score-efficient [%s] label=%s", r.QuestionID, result.Label)
	}
}
