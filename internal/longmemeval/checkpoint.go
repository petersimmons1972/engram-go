package longmemeval

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
)

func ReadAllIngest(path string) ([]IngestEntry, error) { return readJSONL[IngestEntry](path) }
func ReadAllRun(path string) ([]RunEntry, error)       { return readJSONL[RunEntry](path) }
func ReadAllScore(path string) ([]ScoreEntry, error)   { return readJSONL[ScoreEntry](path) }
func ReadAllHypotheses(path string) ([]HypothesisLine, error) {
	return readJSONL[HypothesisLine](path)
}
func ReadSkipSet(path string) (map[string]bool, error) {
	entries, err := readJSONL[struct {
		QuestionID string `json:"question_id"`
		Status     string `json:"status"`
	}](path)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(entries))
	for _, e := range entries {
		if e.Status == "done" {
			out[e.QuestionID] = true
		}
	}
	return out, nil
}

func WriteCheckpoint[T any](path string, ch <-chan T) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for v := range ch {
		_ = enc.Encode(v)
	}
	_ = w.Flush()
}

func readJSONL[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []T
	s := bufio.NewScanner(f)
	for s.Scan() {
		if len(s.Bytes()) == 0 {
			continue
		}
		var v T
		if err := json.Unmarshal(s.Bytes(), &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, s.Err()
}
