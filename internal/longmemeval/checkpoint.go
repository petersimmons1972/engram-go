package longmemeval

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
)

// WriteCheckpoint reads entries from ch and appends each as a JSON line to path.
// Runs until ch is closed. Designed to run in a dedicated goroutine.
func WriteCheckpoint[T any](path string, ch <-chan T) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		for range ch {
		}
		return
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for entry := range ch {
		_ = enc.Encode(entry)
	}
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
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		var entry struct {
			QuestionID string `json:"question_id"`
			Status     string `json:"status"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if entry.Status == "done" {
			skip[entry.QuestionID] = true
		}
	}
	return skip, scanner.Err()
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
	defer f.Close()

	var out []T
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		var entry T
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		out = append(out, entry)
	}
	return out, scanner.Err()
}
