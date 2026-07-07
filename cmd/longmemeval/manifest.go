package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

type runStatusSnapshot struct {
	Completeness        scoreCompleteness
	IngestRowTotal      int
	RunRowTotal         int
	ScoreRowTotal       int
	QuestionProjects    []questionProjectProvenance
	ProjectWarningTotal int
}

type questionProjectProvenance struct {
	QuestionID         string `json:"question_id"`
	Project            string `json:"project"`
	Source             string `json:"source"`
	ExpectedRunProject string `json:"expected_run_project,omitempty"`
	Warning            string `json:"warning,omitempty"`
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
	questionProjects, projectWarningTotal := questionProjectProvenanceFromIngestMap(cfg.RunID, ingestMap)
	exe, _ := os.Executable()
	provenance := scoreProvenanceForConfig(cfg)
	manifest := map[string]any{
		"run_id":                cfg.RunID,
		"stage":                 stage,
		"generation_context":    generationContextForArtifacts(cfg, stage),
		"generated_at":          time.Now().UTC().Format(time.RFC3339),
		"expected_total":        completeness.ExpectedTotal,
		"ingest_done_total":     completeness.IngestDoneTotal,
		"completed_run_total":   completeness.CompletedRunTotal,
		"completed_score_total": completeness.CompletedScoreTotal,
		"run_error_total":       completeness.RunErrorTotal,
		"score_error_total":     completeness.ScoreErrorTotal,
		"complete":              completeness.Complete,
		"question_projects":     questionProjects,
		"project_warning_total": projectWarningTotal,
		"binary_path":           exe,
		"command_line":          os.Args,
		"git_sha":               bestEffortGit("rev-parse", "HEAD"),
		"git_dirty":             bestEffortGitDirty(),
		"llm_url":               redactURL(cfg.LLMBaseURL),
		"llm_model":             cfg.LLMModel,
		"scorer_url":            redactURL(cfg.ScorerURL),
		"scorer_model":          cfg.ScorerModel,
		"provenance":            provenance,
		"scorer_lock":           cfg.ScorerLockPath,
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

func writeRunStatus(cfg *Config, stage string, startedAt, endedAt time.Time, exitCode int, commandLine []string) {
	snapshot := collectRunStatusSnapshot(cfg)
	exe, _ := os.Executable()
	provenance := scoreProvenanceForConfig(cfg)
	status := map[string]any{
		"run_id":                cfg.RunID,
		"stage":                 stage,
		"generation_context":    generationContextForArtifacts(cfg, stage),
		"started_at":            startedAt.UTC().Format(time.RFC3339),
		"ended_at":              endedAt.UTC().Format(time.RFC3339),
		"exit_code":             exitCode,
		"pid":                   os.Getpid(),
		"expected_total":        snapshot.Completeness.ExpectedTotal,
		"ingest_done_total":     snapshot.Completeness.IngestDoneTotal,
		"completed_run_total":   snapshot.Completeness.CompletedRunTotal,
		"completed_score_total": snapshot.Completeness.CompletedScoreTotal,
		"run_error_total":       snapshot.Completeness.RunErrorTotal,
		"score_error_total":     snapshot.Completeness.ScoreErrorTotal,
		"complete":              snapshot.Completeness.Complete,
		"ingest_row_total":      snapshot.IngestRowTotal,
		"run_row_total":         snapshot.RunRowTotal,
		"score_row_total":       snapshot.ScoreRowTotal,
		"question_projects":     snapshot.QuestionProjects,
		"project_warning_total": snapshot.ProjectWarningTotal,
		"binary_path":           exe,
		"command_line":          commandLine,
		"git_sha":               bestEffortGit("rev-parse", "HEAD"),
		"git_dirty":             bestEffortGitDirty(),
		"server_url":            redactURL(cfg.ServerURL),
		"llm_url":               redactURL(cfg.LLMBaseURL),
		"llm_model":             cfg.LLMModel,
		"scorer_url":            redactURL(cfg.ScorerURL),
		"scorer_model":          cfg.ScorerModel,
		"lock_file":             statusLockFile(cfg),
		"cleanup_policy":        cfg.CleanupPolicy,
		"recall_topk":           cfg.RecallTopK,
		"context_topk":          cfg.ContextTopKOverride,
		"provenance":            provenance,
		"scorer_lock":           cfg.ScorerLockPath,
		"route_snapshot": map[string]any{
			"server_url":   redactURL(cfg.ServerURL),
			"llm_url":      redactURL(cfg.LLMBaseURL),
			"llm_model":    cfg.LLMModel,
			"scorer_url":   redactURL(cfg.ScorerURL),
			"scorer_model": cfg.ScorerModel,
		},
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		log.Printf("WARN marshal RUN_STATUS.json: %v", err)
		return
	}
	path := filepath.Join(cfg.OutDir, "RUN_STATUS.json")
	if err := os.WriteFile(path, data, privateArtifactFileMode); err != nil {
		log.Printf("WARN write RUN_STATUS.json: %v", err)
		return
	}
	if err := os.Chmod(path, privateArtifactFileMode); err != nil {
		log.Printf("WARN chmod RUN_STATUS.json: %v", err)
		return
	}
	log.Printf("wrote %s", path)
}

func generationContextForArtifacts(cfg *Config, stage string) string {
	if cfg != nil && cfg.FullTimelineContext {
		return generationContextFullTimeline
	}
	switch stage {
	case "score":
		if persisted := readPersistedGenerationContext(cfg); persisted != "" {
			return persisted
		}
	}
	return generationContextRetrieval
}

func readPersistedGenerationContext(cfg *Config) string {
	if cfg == nil || strings.TrimSpace(cfg.OutDir) == "" {
		return ""
	}
	for _, name := range []string{"RUN_STATUS.json", "run_manifest.json"} {
		data, err := os.ReadFile(filepath.Join(cfg.OutDir, name))
		if err != nil {
			continue
		}
		var artifact struct {
			GenerationContext string `json:"generation_context"`
		}
		if err := json.Unmarshal(data, &artifact); err != nil {
			continue
		}
		switch artifact.GenerationContext {
		case generationContextRetrieval, generationContextFullTimeline:
			return artifact.GenerationContext
		}
	}
	return ""
}

func collectRunStatusSnapshot(cfg *Config) runStatusSnapshot {
	itemMap := loadItemMapForStatus(cfg.DataFile)
	ingestEntries := readIngestEntriesForStatus(cfg.OutDir)
	runEntries := readRunEntriesForStatus(cfg.OutDir)
	scoreEntries := readScoreEntriesForStatus(cfg.OutDir)

	ingestMap := make(map[string]longmemeval.IngestEntry, len(ingestEntries))
	for _, entry := range ingestEntries {
		if entry.QuestionID == "" {
			continue
		}
		if entry.Status == "done" {
			ingestMap[entry.QuestionID] = entry
			continue
		}
		delete(ingestMap, entry.QuestionID)
	}
	runMap := make(map[string]longmemeval.RunEntry, len(runEntries))
	for _, entry := range runEntries {
		if entry.QuestionID == "" {
			continue
		}
		runMap[entry.QuestionID] = entry
	}
	questionProjects, projectWarningTotal := questionProjectProvenanceFromIngestMap(cfg.RunID, ingestMap)
	return runStatusSnapshot{
		Completeness:        scoreCompletenessFromMaps(itemMap, ingestMap, runMap, scoreEntries),
		IngestRowTotal:      len(ingestEntries),
		RunRowTotal:         len(runEntries),
		ScoreRowTotal:       len(scoreEntries),
		QuestionProjects:    questionProjects,
		ProjectWarningTotal: projectWarningTotal,
	}
}

func questionProjectProvenanceFromIngestMap(runID string, ingestMap map[string]longmemeval.IngestEntry) ([]questionProjectProvenance, int) {
	questionIDs := make([]string, 0, len(ingestMap))
	for questionID, entry := range ingestMap {
		if questionID == "" || entry.Project == "" {
			continue
		}
		questionIDs = append(questionIDs, questionID)
	}
	sort.Strings(questionIDs)

	projects := make([]questionProjectProvenance, 0, len(questionIDs))
	warnings := 0
	for _, questionID := range questionIDs {
		project := ingestMap[questionID].Project
		provenance := questionProjectProvenance{
			QuestionID: questionID,
			Project:    project,
			Source:     "checkpoint-ingest",
		}
		if expected := projectName(runID, questionID); expected != project {
			provenance.ExpectedRunProject = expected
			provenance.Warning = "checkpoint-ingest project differs from run_id-derived project"
			warnings++
		}
		projects = append(projects, provenance)
	}
	return projects, warnings
}

func loadItemMapForStatus(path string) map[string]longmemeval.Item {
	itemMap := make(map[string]longmemeval.Item)
	if path == "" {
		return itemMap
	}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("WARN RUN_STATUS.json: read data file %q: %v", path, err)
		return itemMap
	}
	var items []longmemeval.Item
	if err := json.Unmarshal(data, &items); err != nil {
		log.Printf("WARN RUN_STATUS.json: parse data file %q: %v", path, err)
		return itemMap
	}
	for _, item := range items {
		if item.QuestionID != "" {
			itemMap[item.QuestionID] = item
		}
	}
	return itemMap
}

func readIngestEntriesForStatus(outDir string) []longmemeval.IngestEntry {
	entries, err := longmemeval.ReadAllIngest(filepath.Join(outDir, "checkpoint-ingest.jsonl"))
	if err != nil {
		log.Printf("WARN RUN_STATUS.json: read checkpoint-ingest.jsonl: %v", err)
		return nil
	}
	return entries
}

func readRunEntriesForStatus(outDir string) []longmemeval.RunEntry {
	entries, err := longmemeval.ReadAllRun(filepath.Join(outDir, "checkpoint-run.jsonl"))
	if err != nil {
		log.Printf("WARN RUN_STATUS.json: read checkpoint-run.jsonl: %v", err)
		return nil
	}
	return entries
}

func readScoreEntriesForStatus(outDir string) []longmemeval.ScoreEntry {
	entries, err := longmemeval.ReadAllScore(filepath.Join(outDir, "checkpoint-score.jsonl"))
	if err != nil {
		log.Printf("WARN RUN_STATUS.json: read checkpoint-score.jsonl: %v", err)
		return nil
	}
	return entries
}

func statusLockFile(cfg *Config) string {
	if !cfg.ExclusiveBackend || cfg.LLMBaseURL == "" {
		return ""
	}
	dir := cfg.BackendLockDir
	if dir == "" {
		dir = defaultLockDir()
	}
	return backendLockPath(dir, cfg.LLMBaseURL)
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
