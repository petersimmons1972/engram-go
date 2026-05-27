package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

type scoreCompleteness struct {
	ExpectedTotal       int
	IngestDoneTotal     int
	CompletedRunTotal   int
	CompletedScoreTotal int
	RunErrorTotal       int
	ScoreErrorTotal     int
	Complete            bool
}

func scoreCompletenessFromScores(scores []longmemeval.ScoreEntry) scoreCompleteness {
	scoreDone, scoreErrors := scoreOutcomeCounts(scores)
	total := scoreDone + scoreErrors
	return scoreCompleteness{
		ExpectedTotal:       total,
		CompletedScoreTotal: scoreDone,
		ScoreErrorTotal:     scoreErrors,
		Complete:            total > 0 && scoreDone == total,
	}
}

func scoreCompletenessFromMaps(
	itemMap map[string]longmemeval.Item,
	ingestMap map[string]longmemeval.IngestEntry,
	runMap map[string]longmemeval.RunEntry,
	scores []longmemeval.ScoreEntry,
) scoreCompleteness {
	runDone, runErrors := runOutcomeCounts(runMap)
	scoreDone, scoreErrors := scoreOutcomeCounts(scores)
	expected := len(itemMap)
	return scoreCompleteness{
		ExpectedTotal:       expected,
		IngestDoneTotal:     len(ingestMap),
		CompletedRunTotal:   runDone,
		CompletedScoreTotal: scoreDone,
		RunErrorTotal:       runErrors,
		ScoreErrorTotal:     scoreErrors,
		Complete:            expected > 0 && runDone == expected && scoreDone == expected && runErrors == 0 && scoreErrors == 0,
	}
}

func runOutcomeCounts(runMap map[string]longmemeval.RunEntry) (done, errors int) {
	for _, entry := range runMap {
		switch entry.Status {
		case "done":
			done++
		case "error":
			errors++
		}
	}
	return done, errors
}

func scoreOutcomeCounts(scores []longmemeval.ScoreEntry) (done, errors int) {
	deduped := make(map[string]longmemeval.ScoreEntry, len(scores))
	for _, entry := range scores {
		deduped[entry.QuestionID] = entry
	}
	for _, entry := range deduped {
		switch entry.Status {
		case "done":
			done++
		case "error":
			errors++
		}
	}
	return done, errors
}

func writeRunManifest(
	cfg *Config,
	stage string,
	itemMap map[string]longmemeval.Item,
	ingestMap map[string]longmemeval.IngestEntry,
	runMap map[string]longmemeval.RunEntry,
	scores []longmemeval.ScoreEntry,
) {
	completeness := scoreCompletenessFromMaps(itemMap, ingestMap, runMap, scores)
	exe, _ := os.Executable()
	manifest := map[string]any{
		"run_id":                cfg.RunID,
		"stage":                 stage,
		"generated_at":          time.Now().UTC().Format(time.RFC3339),
		"expected_total":        completeness.ExpectedTotal,
		"ingest_done_total":     completeness.IngestDoneTotal,
		"completed_run_total":   completeness.CompletedRunTotal,
		"completed_score_total": completeness.CompletedScoreTotal,
		"run_error_total":       completeness.RunErrorTotal,
		"score_error_total":     completeness.ScoreErrorTotal,
		"complete":              completeness.Complete,
		"binary_path":           exe,
		"command_line":          os.Args,
		"git_sha":               bestEffortGit("rev-parse", "HEAD"),
		"git_dirty":             bestEffortGitDirty(),
		"llm_url":               redactURL(cfg.LLMBaseURL),
		"llm_model":             cfg.LLMModel,
		"scorer_url":            redactURL(cfg.ScorerURL),
		"scorer_model":          cfg.ScorerModel,
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		log.Printf("WARN marshal run_manifest.json: %v", err)
		return
	}
	path := filepath.Join(cfg.OutDir, "run_manifest.json")
	if err := os.WriteFile(path, data, privateArtifactFileMode); err != nil {
		log.Printf("WARN write run_manifest.json: %v", err)
		return
	}
	if err := os.Chmod(path, privateArtifactFileMode); err != nil {
		log.Printf("WARN chmod run_manifest.json: %v", err)
		return
	}
	log.Printf("wrote %s", path)
}

func bestEffortGit(args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func bestEffortGitDirty() bool {
	out := bestEffortGit("status", "--porcelain")
	return out != ""
}
