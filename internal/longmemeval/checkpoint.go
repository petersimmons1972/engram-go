package longmemeval

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
)

// WriteCheckpoint reads entries from ch and appends each as a JSON line to path.
// Runs until ch is closed. Designed to run in a dedicated goroutine.
func WriteCheckpoint[T any](path string, ch <-chan T) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("WARN WriteCheckpoint: open %s: %v — all entries will be lost", path, err)
		for range ch {
		}
		return err
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	var encodeErrs int
	var firstErr error
	for entry := range ch {
		if err := enc.Encode(entry); err != nil {
			encodeErrs++
			if firstErr == nil {
				firstErr = err
			}
			// Log every individual failure so on-call can identify which entry
			// was lost. #670: previously discarded silently — silent loss of
			// expensive LLM-call results is unacceptable.
			log.Printf("WARN WriteCheckpoint: encode failed for entry in %s: %v", path, err)
		}
	}
	if encodeErrs > 0 {
		log.Printf("WARN WriteCheckpoint: %d entries failed to encode in %s — results may be incomplete (#670)", encodeErrs, path)
	}
	if firstErr != nil {
		return fmt.Errorf("write checkpoint %s: %d encode failures: %w", path, encodeErrs, firstErr)
	}
	return nil
}

// ReadSkipSet reads a checkpoint file and returns a set of question IDs with
// status == "done". Returns an empty set (not an error) if the file does not exist.
func ReadSkipSet(path string) (map[string]bool, error) {
	skip := make(map[string]bool)
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return skip, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	line := 0
	for scanner.Scan() {
		line++
		var entry struct {
			QuestionID string `json:"question_id"`
			Status     string `json:"status"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, fmt.Errorf("read checkpoint %s line %d: %w", path, line, err)
		}
		if entry.Status == "done" {
			skip[entry.QuestionID] = true
		}
	}
	return skip, scanner.Err()
}

// ReadScoredLabels returns the latest score label by question_id for
// checkpoint-score.jsonl. A later non-done entry clears an earlier done label,
// matching append-only checkpoint last-write-wins semantics. Returns an empty
// map (not error) if the file does not exist.
func ReadScoredLabels(path string) (map[string]string, error) {
	labels := make(map[string]string)
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return labels, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	line := 0
	for scanner.Scan() {
		line++
		var entry struct {
			QuestionID string `json:"question_id"`
			Status     string `json:"status"`
			ScoreLabel string `json:"score_label"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, fmt.Errorf("read checkpoint %s line %d: %w", path, line, err)
		}
		if entry.QuestionID == "" {
			continue
		}
		if entry.Status == "done" {
			labels[entry.QuestionID] = entry.ScoreLabel
			continue
		}
		delete(labels, entry.QuestionID)
	}
	return labels, scanner.Err()
}

// ReadAllIngest reads all entries from a checkpoint-ingest.jsonl file.
func ReadAllIngest(path string) ([]IngestEntry, error) {
	return readAll[IngestEntry](path)
}

// ReadAllRun reads all entries from a checkpoint-run.jsonl file.
func ReadAllRun(path string) ([]RunEntry, error) {
	return readAll[RunEntry](path)
}

// ReadAllScore reads all entries from a checkpoint-score.jsonl file.
func ReadAllScore(path string) ([]ScoreEntry, error) {
	return readAll[ScoreEntry](path)
}

func readAll[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var out []T
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	line := 0
	for scanner.Scan() {
		line++
		var entry T
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, fmt.Errorf("read checkpoint %s line %d: %w", path, line, err)
		}
		out = append(out, entry)
	}
	return out, scanner.Err()
}
