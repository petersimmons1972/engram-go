package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

type samplePrepareConfig struct {
	DataFile    string
	Source      string
	OutDir      string
	Limit       int
	MaxPerType  int
	CopyRun     bool
	CopyScore   bool
	Description string
}

type samplePrepareResult struct {
	Items         int    `json:"items"`
	IngestEntries int    `json:"ingest_entries"`
	DataFile      string `json:"data_file"`
	OutDir        string `json:"out_dir"`
	Source        string `json:"source"`
}

type sampleAnalyzeConfig struct {
	DataFile   string
	ResultsDir string
}

type sampleAnalyzeSummary struct {
	Items                        int                      `json:"items"`
	RunDone                      int                      `json:"run_done"`
	RunErrors                    int                      `json:"run_errors"`
	Scored                       int                      `json:"scored"`
	Correct                      int                      `json:"correct"`
	PartiallyCorrect             int                      `json:"partially_correct"`
	Incorrect                    int                      `json:"incorrect"`
	ScoreErrors                  int                      `json:"score_errors"`
	RetrievedGoldSession         int                      `json:"retrieved_gold_session"`
	RetrievalMiss                int                      `json:"retrieval_miss"`
	ContextPresentGenerationMiss int                      `json:"context_present_generation_miss"`
	ByType                       map[string]sampleTypeRow `json:"by_type"`
}

type sampleTypeRow struct {
	Items                        int `json:"items"`
	RunDone                      int `json:"run_done"`
	Scored                       int `json:"scored"`
	Correct                      int `json:"correct"`
	PartiallyCorrect             int `json:"partially_correct"`
	Incorrect                    int `json:"incorrect"`
	RetrievedGoldSession         int `json:"retrieved_gold_session"`
	RetrievalMiss                int `json:"retrieval_miss"`
	ContextPresentGenerationMiss int `json:"context_present_generation_miss"`
}

func prepareSample(cfg samplePrepareConfig) (samplePrepareResult, error) {
	if cfg.DataFile == "" {
		return samplePrepareResult{}, errors.New("--data is required")
	}
	if cfg.Source == "" {
		return samplePrepareResult{}, errors.New("--source is required")
	}
	if cfg.OutDir == "" {
		return samplePrepareResult{}, errors.New("--out is required")
	}
	if cfg.Limit < 0 || cfg.MaxPerType < 0 {
		return samplePrepareResult{}, errors.New("--limit and --max-per-type must be non-negative")
	}

	items, err := loadItemsFile(cfg.DataFile)
	if err != nil {
		return samplePrepareResult{}, err
	}
	items = selectSampleItems(items, cfg.Limit, cfg.MaxPerType)
	if len(items) == 0 {
		return samplePrepareResult{}, errors.New("sample selected zero items")
	}
	idSet := make(map[string]bool, len(items))
	for _, item := range items {
		idSet[item.QuestionID] = true
	}

	ingest, err := longmemeval.ReadAllIngest(filepath.Join(cfg.Source, "checkpoint-ingest.jsonl"))
	if err != nil {
		return samplePrepareResult{}, fmt.Errorf("read source ingest checkpoint: %w", err)
	}
	filteredIngest := make([]longmemeval.IngestEntry, 0, len(items))
	for _, entry := range ingest {
		if idSet[entry.QuestionID] {
			filteredIngest = append(filteredIngest, entry)
		}
	}
	if len(filteredIngest) != len(items) {
		return samplePrepareResult{}, fmt.Errorf("source ingest checkpoint contains %d/%d selected items", len(filteredIngest), len(items))
	}

	if err := os.MkdirAll(cfg.OutDir, privateArtifactDirMode); err != nil {
		return samplePrepareResult{}, fmt.Errorf("create output dir: %w", err)
	}
	if err := os.Chmod(cfg.OutDir, privateArtifactDirMode); err != nil {
		return samplePrepareResult{}, fmt.Errorf("chmod output dir: %w", err)
	}
	if err := writeJSON(filepath.Join(cfg.OutDir, "data.json"), items); err != nil {
		return samplePrepareResult{}, err
	}
	if err := writeJSONL(filepath.Join(cfg.OutDir, "checkpoint-ingest.jsonl"), filteredIngest); err != nil {
		return samplePrepareResult{}, err
	}
	if cfg.CopyRun {
		if err := copyFilteredJSONL(filepath.Join(cfg.Source, "checkpoint-run.jsonl"), filepath.Join(cfg.OutDir, "checkpoint-run.jsonl"), idSet); err != nil {
			return samplePrepareResult{}, err
		}
	}
	if cfg.CopyScore {
		if err := copyFilteredJSONL(filepath.Join(cfg.Source, "checkpoint-score.jsonl"), filepath.Join(cfg.OutDir, "checkpoint-score.jsonl"), idSet); err != nil {
			return samplePrepareResult{}, err
		}
	}

	result := samplePrepareResult{
		Items:         len(items),
		IngestEntries: len(filteredIngest),
		DataFile:      filepath.Join(cfg.OutDir, "data.json"),
		OutDir:        cfg.OutDir,
		Source:        cfg.Source,
	}
	manifest := map[string]any{
		"created_at":    time.Now().UTC().Format(time.RFC3339),
		"description":   cfg.Description,
		"source":        cfg.Source,
		"source_data":   cfg.DataFile,
		"items":         len(items),
		"max_per_type":  cfg.MaxPerType,
		"limit":         cfg.Limit,
		"copy_run":      cfg.CopyRun,
		"copy_score":    cfg.CopyScore,
		"route_command": "go run ./cmd/longmemeval route-discover --purpose generation",
		"run_command":   fmt.Sprintf("go run ./cmd/longmemeval run --data %s --out %s --cleanup-policy=never --llm-url $(go run ./cmd/longmemeval route-discover --purpose generation | jq -r .llm_url) --llm-model $(go run ./cmd/longmemeval route-discover --purpose generation | jq -r .llm_model)", result.DataFile, cfg.OutDir),
	}
	if err := writeJSON(filepath.Join(cfg.OutDir, "sample_manifest.json"), manifest); err != nil {
		return samplePrepareResult{}, err
	}
	return result, nil
}

func analyzeSample(cfg sampleAnalyzeConfig) (sampleAnalyzeSummary, error) {
	if cfg.ResultsDir == "" {
		return sampleAnalyzeSummary{}, errors.New("--results is required")
	}
	ingestEntries, err := longmemeval.ReadAllIngest(filepath.Join(cfg.ResultsDir, "checkpoint-ingest.jsonl"))
	if err != nil {
		return sampleAnalyzeSummary{}, err
	}
	ingestMap := make(map[string]longmemeval.IngestEntry, len(ingestEntries))
	for _, entry := range ingestEntries {
		ingestMap[entry.QuestionID] = entry
	}
	runEntries, err := longmemeval.ReadAllRun(filepath.Join(cfg.ResultsDir, "checkpoint-run.jsonl"))
	if err != nil {
		return sampleAnalyzeSummary{}, err
	}
	runMap := make(map[string]longmemeval.RunEntry, len(runEntries))
	for _, entry := range runEntries {
		runMap[entry.QuestionID] = entry
	}
	scoreEntries, err := longmemeval.ReadAllScore(filepath.Join(cfg.ResultsDir, "checkpoint-score.jsonl"))
	if err != nil {
		return sampleAnalyzeSummary{}, err
	}
	scoreMap := make(map[string]longmemeval.ScoreEntry, len(scoreEntries))
	for _, entry := range scoreEntries {
		scoreMap[entry.QuestionID] = entry
	}

	items, err := analyzeItems(cfg.DataFile, runMap, scoreMap)
	if err != nil {
		return sampleAnalyzeSummary{}, err
	}
	summary := sampleAnalyzeSummary{
		Items:  len(items),
		ByType: make(map[string]sampleTypeRow),
	}
	for _, item := range items {
		row := summary.ByType[item.QuestionType]
		row.Items++

		if run, ok := runMap[item.QuestionID]; ok {
			switch run.Status {
			case "done":
				summary.RunDone++
				row.RunDone++
				if hasGoldSession(run.RetrievedIDs, ingestMap[item.QuestionID].MemoryMap, item.AnswerSessionIDs) {
					summary.RetrievedGoldSession++
					row.RetrievedGoldSession++
				}
			case "error":
				summary.RunErrors++
			}
		}
		if score, ok := scoreMap[item.QuestionID]; ok {
			switch score.Status {
			case "done":
				summary.Scored++
				row.Scored++
				switch score.ScoreLabel {
				case "CORRECT":
					summary.Correct++
					row.Correct++
				case "PARTIALLY_CORRECT":
					summary.PartiallyCorrect++
					row.PartiallyCorrect++
				default:
					summary.Incorrect++
					row.Incorrect++
				}
				if score.ScoreLabel != "CORRECT" && len(item.AnswerSessionIDs) > 0 {
					run, runOK := runMap[item.QuestionID]
					retrievedGold := runOK && run.Status == "done" && hasGoldSession(run.RetrievedIDs, ingestMap[item.QuestionID].MemoryMap, item.AnswerSessionIDs)
					if retrievedGold {
						summary.ContextPresentGenerationMiss++
						row.ContextPresentGenerationMiss++
					} else {
						summary.RetrievalMiss++
						row.RetrievalMiss++
					}
				}
			case "error":
				summary.ScoreErrors++
			}
		}
		summary.ByType[item.QuestionType] = row
	}
	return summary, nil
}

func analyzeItems(dataFile string, runMap map[string]longmemeval.RunEntry, scoreMap map[string]longmemeval.ScoreEntry) ([]longmemeval.Item, error) {
	if dataFile != "" {
		return loadItemsFile(dataFile)
	}
	ids := make(map[string]longmemeval.Item, len(runMap)+len(scoreMap))
	for id := range runMap {
		ids[id] = longmemeval.Item{QuestionID: id, QuestionType: "unknown"}
	}
	for id, score := range scoreMap {
		item := ids[id]
		item.QuestionID = id
		if score.QuestionType != "" {
			item.QuestionType = score.QuestionType
		} else if item.QuestionType == "" {
			item.QuestionType = "unknown"
		}
		ids[id] = item
	}
	items := make([]longmemeval.Item, 0, len(ids))
	for _, item := range ids {
		items = append(items, item)
	}
	return items, nil
}

func selectSampleItems(items []longmemeval.Item, limit, maxPerType int) []longmemeval.Item {
	if limit == 0 && maxPerType == 0 {
		return items
	}
	out := make([]longmemeval.Item, 0, len(items))
	byType := make(map[string]int)
	for _, item := range items {
		if maxPerType > 0 && byType[item.QuestionType] >= maxPerType {
			continue
		}
		out = append(out, item)
		byType[item.QuestionType]++
		if limit > 0 && len(out) == limit {
			break
		}
	}
	return out
}

func hasGoldSession(retrievedIDs []string, memoryMap map[string]string, answerSessionIDs []string) bool {
	if len(retrievedIDs) == 0 || len(memoryMap) == 0 || len(answerSessionIDs) == 0 {
		return false
	}
	answerSet := make(map[string]bool, len(answerSessionIDs))
	for _, id := range answerSessionIDs {
		answerSet[id] = true
	}
	for _, memoryID := range retrievedIDs {
		if answerSet[memoryMap[memoryID]] {
			return true
		}
	}
	return false
}

func loadItemsFile(path string) ([]longmemeval.Item, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read data file %q: %w", path, err)
	}
	var items []longmemeval.Item
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("parse data file %q: %w", path, err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("data file %q is empty", path)
	}
	return items, nil
}

func writeJSON(path string, v any) error {
	f, err := createPrivateArtifact(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	return nil
}

func writeJSONL[T any](path string, entries []T) error {
	f, err := createPrivateArtifact(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	for _, entry := range entries {
		if err := enc.Encode(entry); err != nil {
			return fmt.Errorf("encode %s: %w", path, err)
		}
	}
	return nil
}

func copyFilteredJSONL(src, dst string, idSet map[string]bool) error {
	in, err := os.Open(src)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()
	out, err := createPrivateArtifact(dst)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer func() { _ = out.Close() }()

	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		var line struct {
			QuestionID string `json:"question_id"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil || !idSet[line.QuestionID] {
			continue
		}
		if _, err := out.Write(scanner.Bytes()); err != nil {
			return fmt.Errorf("write %s: %w", dst, err)
		}
		if _, err := io.WriteString(out, "\n"); err != nil {
			return fmt.Errorf("write newline %s: %w", dst, err)
		}
	}
	return scanner.Err()
}
