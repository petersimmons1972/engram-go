//go:build longmemeval

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestHeuristicAudit_Runs(t *testing.T) {
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "benchmark.json")
	writeAuditItems(t, dataPath, []map[string]any{
		{
			"question_id":   "agg-1",
			"question_type": "aggregation",
			"question":      "How many conferences did I attend?",
			"answer":        2,
		},
		{
			"question_id":   "pref-1",
			"question_type": "single-session-preference",
			"question":      "Can you recommend a restaurant for sushi?",
			"answer":        "Sushi place",
		},
		{
			"question_id":   "temp-1",
			"question_type": "temporal-reasoning",
			"question":      "How many days ago did I call mom?",
			"answer":        3,
		},
		{
			"question_id":   "ms-1",
			"question_type": "multi-session-fact",
			"question":      "What themes came up across my recent project updates?",
			"answer":        "Testing and rollout",
		},
	})

	t.Setenv("LME_BENCHMARK_PATH", dataPath)
	reportPath := filepath.Join(dir, "heuristic-audit.json")

	report, markdown, logs, err := runHeuristicAudit(heuristicAuditConfig{
		DefaultBenchmarkPath: filepath.Join(dir, "missing.json"),
		OutputPath:           reportPath,
	})
	if err != nil {
		t.Fatalf("runHeuristicAudit: %v", err)
	}
	if markdown == "" {
		t.Fatal("markdown table is empty")
	}
	for _, line := range logs {
		t.Log(line)
	}
	t.Log("\n" + markdown)

	if report.Heuristics == nil {
		t.Fatal("report.Heuristics is nil")
	}
	for _, name := range []string{"aggregation", "preference", "temporal", "multi_session"} {
		row, ok := report.Heuristics[name]
		if !ok {
			t.Fatalf("missing heuristic row %q", name)
		}
		for label, value := range map[string]float64{
			"precision": row.Precision,
			"recall":    row.Recall,
			"f1":        row.F1,
		} {
			if value < 0 || value > 1 {
				t.Fatalf("%s.%s=%v, want value in [0,1]", name, label, value)
			}
		}
	}

	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var decoded heuristicAuditReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if _, ok := decoded.Heuristics["aggregation"]; !ok {
		t.Fatal("json report missing aggregation row")
	}
}

func TestHeuristicAudit_SkipsWhenFileAbsent(t *testing.T) {
	t.Setenv("LME_BENCHMARK_PATH", "")
	requireBenchmarkPath(t, filepath.Join(t.TempDir(), "does-not-exist.json"))
	t.Fatal("expected requireBenchmarkPath to skip")
}

type heuristicAuditConfig struct {
	DefaultBenchmarkPath string
	OutputPath           string
}

type heuristicAuditReport struct {
	Heuristics map[string]heuristicAuditRow `json:"heuristics"`
}

type heuristicAuditRow struct {
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	F1        float64 `json:"f1"`
}

func runHeuristicAudit(cfg heuristicAuditConfig) (heuristicAuditReport, string, []string, error) {
	return heuristicAuditReport{}, "", nil, errors.New("not implemented")
}

func requireBenchmarkPath(t *testing.T, defaultPath string) string {
	t.Helper()
	path := os.Getenv("LME_BENCHMARK_PATH")
	if path == "" {
		path = defaultPath
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("LongMemEval benchmark file not found at %q", path)
	}
	return path
}

func writeAuditItems(t *testing.T, path string, items []map[string]any) {
	t.Helper()
	data, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("marshal audit items: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write audit items: %v", err)
	}
}
