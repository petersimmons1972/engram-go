//go:build longmemeval

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

const defaultLMEBenchmarkPath = "testdata/longmemeval/longmemeval_m_cleaned.json"

// Mirrors the recommendation-verb strip used by PreferenceRecallQuery. The
// production package does not export a standalone preference detector, so this
// audit uses the same text trigger as the existing preference-query rewrite.
var auditPreferenceQuestionRe = regexp.MustCompile(
	`(?i)^(can you |could you |would you )?(recommend|suggest|advise|give me|tell me) `)

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
		{
			"question_id":   "fp-1",
			"question_type": "single-session-user",
			"question":      "How many pets do I have?",
			"answer":        "2",
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
		if row.TP < 0 || row.FP < 0 || row.FN < 0 {
			t.Fatalf("%s has negative counts: %+v", name, row)
		}
	}
	if report.ItemCount != 5 {
		t.Fatalf("ItemCount=%d, want 5", report.ItemCount)
	}
	if report.Heuristics["aggregation"].FP == 0 {
		t.Fatal("expected synthetic dataset to exercise an aggregation false positive")
	}
	for _, name := range []string{"aggregation", "preference", "temporal", "multi_session"} {
		found := false
		for _, line := range logs {
			if strings.Contains(line, name) && strings.Contains(line, "false positives") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing false-positive log line for %s", name)
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
	BenchmarkPath string                       `json:"benchmark_path"`
	ItemCount     int                          `json:"item_count"`
	GeneratedAt   string                       `json:"generated_at"`
	Heuristics    map[string]heuristicAuditRow `json:"heuristics"`
}

type heuristicAuditRow struct {
	TargetLabel    string              `json:"target_label"`
	TP             int                 `json:"tp"`
	FP             int                 `json:"fp"`
	FN             int                 `json:"fn"`
	TN             int                 `json:"tn"`
	Precision      float64             `json:"precision"`
	Recall         float64             `json:"recall"`
	F1             float64             `json:"f1"`
	FalsePositives map[string][]string `json:"false_positives,omitempty"`
	FalseNegatives map[string][]string `json:"false_negatives,omitempty"`
}

func runHeuristicAudit(cfg heuristicAuditConfig) (heuristicAuditReport, string, []string, error) {
	benchmarkPath := os.Getenv("LME_BENCHMARK_PATH")
	if benchmarkPath == "" {
		benchmarkPath = cfg.DefaultBenchmarkPath
	}
	if benchmarkPath == "" {
		benchmarkPath = defaultLMEBenchmarkPath
	}

	items, err := loadHeuristicAuditItems(benchmarkPath)
	if err != nil {
		return heuristicAuditReport{}, "", nil, err
	}

	report := heuristicAuditReport{
		BenchmarkPath: benchmarkPath,
		ItemCount:     len(items),
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Heuristics: map[string]heuristicAuditRow{
			"aggregation":   {TargetLabel: "aggregation"},
			"preference":    {TargetLabel: "single-session-preference"},
			"temporal":      {TargetLabel: "temporal-reasoning"},
			"multi_session": {TargetLabel: "multi-session-fact"},
		},
	}

	for _, item := range items {
		predictions := map[string]bool{
			"aggregation":   longmemeval.IsAggregationQuestion(item.Question),
			"preference":    isPreferenceAuditQuestion(item.Question),
			"temporal":      isTemporalAuditQuestion(item.Question),
			"multi_session": isMultiSessionAuditQuestion(item.Question),
		}
		for name, predicted := range predictions {
			row := report.Heuristics[name]
			actual := questionTypeMatchesHeuristic(name, item.QuestionType)
			switch {
			case predicted && actual:
				row.TP++
			case predicted && !actual:
				row.FP++
				if row.FalsePositives == nil {
					row.FalsePositives = make(map[string][]string)
				}
				row.FalsePositives[item.QuestionType] = append(row.FalsePositives[item.QuestionType], item.QuestionID)
			case !predicted && actual:
				row.FN++
				if row.FalseNegatives == nil {
					row.FalseNegatives = make(map[string][]string)
				}
				row.FalseNegatives[item.QuestionType] = append(row.FalseNegatives[item.QuestionType], item.QuestionID)
			default:
				row.TN++
			}
			report.Heuristics[name] = row
		}
	}

	for name, row := range report.Heuristics {
		row.Precision = safeRatio(row.TP, row.TP+row.FP)
		row.Recall = safeRatio(row.TP, row.TP+row.FN)
		if row.Precision+row.Recall > 0 {
			row.F1 = 2 * row.Precision * row.Recall / (row.Precision + row.Recall)
		}
		report.Heuristics[name] = row
	}

	if cfg.OutputPath == "" {
		tmp, err := os.CreateTemp("", "heuristic-audit-*.json")
		if err != nil {
			return heuristicAuditReport{}, "", nil, fmt.Errorf("create temp report: %w", err)
		}
		_ = tmp.Close()
		cfg.OutputPath = tmp.Name()
	}
	if err := writeHeuristicAuditReport(cfg.OutputPath, report); err != nil {
		return heuristicAuditReport{}, "", nil, err
	}

	return report, renderHeuristicAuditMarkdown(report), renderHeuristicAuditLogs(report), nil
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

func loadHeuristicAuditItems(path string) ([]longmemeval.Item, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read benchmark %q: %w", path, err)
	}
	var items []longmemeval.Item
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("parse benchmark %q: %w", path, err)
	}
	return items, nil
}

func writeHeuristicAuditReport(path string, report heuristicAuditReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write report %q: %w", path, err)
	}
	return nil
}

func isPreferenceAuditQuestion(question string) bool {
	return auditPreferenceQuestionRe.MatchString(question)
}

func isTemporalAuditQuestion(question string) bool {
	lower := strings.ToLower(strings.TrimSpace(question))
	return temporalInterrogativeRe.MatchString(question) ||
		strings.Contains(lower, "yesterday") ||
		relativeAgoRe.MatchString(lower)
}

// No standalone production multi-session predicate exists today. For this
// audit, the multi-session classifier is treated as the residual bucket after
// the specialized aggregation, preference, and temporal gates have not fired.
func isMultiSessionAuditQuestion(question string) bool {
	return !longmemeval.IsAggregationQuestion(question) &&
		!isPreferenceAuditQuestion(question) &&
		!isTemporalAuditQuestion(question)
}

func questionTypeMatchesHeuristic(heuristicName, questionType string) bool {
	norm := normalizeQuestionType(questionType)
	switch heuristicName {
	case "aggregation":
		return strings.Contains(norm, "aggreg") ||
			strings.Contains(norm, "count") ||
			(strings.Contains(norm, "multi") && strings.Contains(norm, "count"))
	case "preference":
		return strings.Contains(norm, "preference")
	case "temporal":
		return strings.Contains(norm, "temporal")
	case "multi_session":
		return strings.Contains(norm, "multi") &&
			strings.Contains(norm, "session") &&
			!strings.Contains(norm, "count") &&
			!strings.Contains(norm, "aggreg")
	default:
		return false
	}
}

func normalizeQuestionType(questionType string) string {
	replacer := strings.NewReplacer("-", "_", " ", "_", "/", "_")
	return replacer.Replace(strings.ToLower(strings.TrimSpace(questionType)))
}

func safeRatio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func renderHeuristicAuditMarkdown(report heuristicAuditReport) string {
	names := sortedHeuristicNames(report.Heuristics)
	var b strings.Builder
	b.WriteString("| heuristic | target | tp | fp | fn | precision | recall | f1 |\n")
	b.WriteString("| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, name := range names {
		row := report.Heuristics[name]
		_, _ = fmt.Fprintf(
			&b,
			"| %s | %s | %d | %d | %d | %.3f | %.3f | %.3f |\n",
			name,
			row.TargetLabel,
			row.TP,
			row.FP,
			row.FN,
			row.Precision,
			row.Recall,
			row.F1,
		)
	}
	return b.String()
}

func renderHeuristicAuditLogs(report heuristicAuditReport) []string {
	names := sortedHeuristicNames(report.Heuristics)
	logs := make([]string, 0, len(names))
	for _, name := range names {
		row := report.Heuristics[name]
		if len(row.FalsePositives) == 0 {
			logs = append(logs, fmt.Sprintf("%s false positives: none", name))
			continue
		}
		actualTypes := make([]string, 0, len(row.FalsePositives))
		for actualType := range row.FalsePositives {
			actualTypes = append(actualTypes, actualType)
		}
		sort.Strings(actualTypes)
		parts := make([]string, 0, len(actualTypes))
		for _, actualType := range actualTypes {
			parts = append(parts, fmt.Sprintf("classified as %s but actually %s: %v", name, actualType, row.FalsePositives[actualType]))
		}
		logs = append(logs, fmt.Sprintf("%s false positives: %s", name, strings.Join(parts, "; ")))
	}
	return logs
}

func sortedHeuristicNames(rows map[string]heuristicAuditRow) []string {
	names := make([]string, 0, len(rows))
	for name := range rows {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
