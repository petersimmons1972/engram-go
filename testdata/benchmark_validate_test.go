package testdata_test

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"
)

func TestFixtureValid(t *testing.T) {
	f, err := os.Open("sample.jsonl")
	if err != nil {
		t.Fatalf("open sample.jsonl: %v", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	lineCount := 0
	for sc.Scan() {
		lineCount++
		var event map[string]any
		if err := json.Unmarshal(sc.Bytes(), &event); err != nil {
			t.Errorf("line %d: invalid JSON: %v", lineCount, err)
			continue
		}
		for _, field := range []string{"timestamp", "session_id", "project_id", "tool_name", "tool_input_hash", "tool_output_summary", "exit_status", "schema_version"} {
			if _, ok := event[field]; !ok {
				t.Errorf("line %d: missing field %q", lineCount, field)
			}
		}
	}
	if lineCount < 20 {
		t.Errorf("fixture too small: %d events (want >=20)", lineCount)
	}
	t.Logf("fixture: %d events", lineCount)
}
