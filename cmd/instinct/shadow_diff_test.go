// Package main_test contains Phase 2 shadow-run validation tests.
package main_test

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// diffRecord represents one line of a diff-YYYY-MM-DD.jsonl report.
type diffRecord struct {
	Date             string   `json:"date"`
	TagSignature     string   `json:"tag_signature"`
	PythonConfidence *float64 `json:"python_confidence"`
	GoConfidence     *float64 `json:"go_confidence"`
	PythonVerdict    *string  `json:"python_verdict"`
	GoVerdict        *string  `json:"go_verdict"`
	DeltaKind        string   `json:"delta_kind"`
}

// validDeltaKinds is the exhaustive set of allowed delta_kind values.
var validDeltaKinds = map[string]bool{
	"MISS_GO":       true,
	"MISS_PY":       true,
	"VERDICT_DIFF":  true,
	"CONF_DRIFT":    true,
	"WRITE_FAILURE": true,
}

// TestDiffJSONLSchema validates that every line of every diff-*.jsonl report
// present in ~/.local/state/instinct/ has the required fields and valid values.
//
// This test guards against silent schema drift in instinct-shadow-diff.sh
// that would mask real divergence (e.g., delta_kind missing → diff tools skip
// the line, making Go look better than it is).
//
// The test is designed to pass when no diff files exist yet (early in Phase 2).
func TestDiffJSONLSchema(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("cannot determine home dir: %v", err)
	}

	instinctDir := filepath.Join(home, ".local", "state", "instinct")
	pattern := filepath.Join(instinctDir, "diff-*.jsonl")
	files, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob %s: %v", pattern, err)
	}

	if len(files) == 0 {
		t.Logf("No diff-*.jsonl files found — shadow run hasn't produced output yet (expected early in Phase 2).")
		return // not a failure; Phase 2 has just started
	}

	totalLines := 0
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			fh, err := os.Open(f)
			if err != nil {
				t.Fatalf("open %s: %v", f, err)
			}
			defer fh.Close()

			lineNum := 0
			sc := bufio.NewScanner(fh)
			for sc.Scan() {
				line := strings.TrimSpace(sc.Text())
				lineNum++
				totalLines++
				if line == "" {
					continue
				}

				// 1. Must parse as JSON object
				var raw map[string]json.RawMessage
				if err := json.Unmarshal([]byte(line), &raw); err != nil {
					t.Errorf("line %d: not valid JSON: %v\n  content: %s", lineNum, err, line)
					continue
				}

				// 2. Must have all required top-level keys
				required := []string{
					"tag_signature",
					"python_confidence",
					"go_confidence",
					"python_verdict",
					"go_verdict",
					"delta_kind",
				}
				for _, key := range required {
					if _, ok := raw[key]; !ok {
						t.Errorf("line %d: missing required field %q\n  content: %s", lineNum, key, line)
					}
				}

				// 3. delta_kind must be a known value
				var rec diffRecord
				if err := json.Unmarshal([]byte(line), &rec); err != nil {
					t.Errorf("line %d: failed to unmarshal into diffRecord: %v", lineNum, err)
					continue
				}
				if rec.DeltaKind == "" {
					t.Errorf("line %d: delta_kind is empty", lineNum)
				} else if !validDeltaKinds[rec.DeltaKind] {
					t.Errorf("line %d: unknown delta_kind %q (allowed: MISS_GO, MISS_PY, VERDICT_DIFF, CONF_DRIFT, WRITE_FAILURE)", lineNum, rec.DeltaKind)
				}

				// 4. tag_signature must be non-empty
				if rec.TagSignature == "" {
					t.Errorf("line %d: tag_signature is empty", lineNum)
				}

				// 5. Confidence values, when present, must be in [0.0, 1.0]
				if rec.PythonConfidence != nil {
					if *rec.PythonConfidence < 0.0 || *rec.PythonConfidence > 1.0 {
						t.Errorf("line %d: python_confidence %.4f out of [0,1]", lineNum, *rec.PythonConfidence)
					}
				}
				if rec.GoConfidence != nil {
					if *rec.GoConfidence < 0.0 || *rec.GoConfidence > 1.0 {
						t.Errorf("line %d: go_confidence %.4f out of [0,1]", lineNum, *rec.GoConfidence)
					}
				}

				// 6. Semantic consistency checks
				switch rec.DeltaKind {
				case "MISS_GO":
					// python must have written, go must not
					if rec.PythonConfidence == nil && rec.PythonVerdict == nil {
						t.Errorf("line %d: MISS_GO but both python_confidence and python_verdict are null", lineNum)
					}
				case "MISS_PY":
					// go must have written, python must not
					if rec.GoConfidence == nil && rec.GoVerdict == nil {
						t.Errorf("line %d: MISS_PY but both go_confidence and go_verdict are null", lineNum)
					}
				case "CONF_DRIFT":
					if rec.PythonConfidence == nil || rec.GoConfidence == nil {
						t.Errorf("line %d: CONF_DRIFT but one confidence is null (py=%v go=%v)", lineNum, rec.PythonConfidence, rec.GoConfidence)
					} else {
						drift := *rec.PythonConfidence - *rec.GoConfidence
						if drift < 0 {
							drift = -drift
						}
						if drift <= 0.1 {
							t.Errorf("line %d: CONF_DRIFT but |drift|=%.4f ≤ 0.1 threshold", lineNum, drift)
						}
					}
				}
			}

			if err := sc.Err(); err != nil {
				t.Fatalf("scanner error: %v", err)
			}
		})
	}
	t.Logf("Validated %d diff lines across %d file(s)", totalLines, len(files))
}

// TestDiffJSONLSyntheticSchema validates the schema using a synthetic in-memory
// diff file — this ensures the test itself is always exercised regardless of
// whether a real diff file exists.
func TestDiffJSONLSyntheticSchema(t *testing.T) {
	cases := []struct {
		name    string
		line    string
		wantErr bool
	}{
		{
			name: "valid MISS_GO",
			line: `{"date":"2026-05-18","tag_signature":"edit_loop_detection","python_confidence":0.7,"go_confidence":null,"python_verdict":"KEEP","go_verdict":null,"delta_kind":"MISS_GO"}`,
		},
		{
			name: "valid MISS_PY",
			line: `{"date":"2026-05-18","tag_signature":"bash_timeout_pattern","python_confidence":null,"go_confidence":0.5,"python_verdict":null,"go_verdict":"KEEP","delta_kind":"MISS_PY"}`,
		},
		{
			name: "valid VERDICT_DIFF",
			line: `{"date":"2026-05-18","tag_signature":"file_read_pattern","python_confidence":0.6,"go_confidence":0.6,"python_verdict":"KEEP","go_verdict":"TUNE","delta_kind":"VERDICT_DIFF"}`,
		},
		{
			name: "valid CONF_DRIFT",
			line: `{"date":"2026-05-18","tag_signature":"agent_dispatch","python_confidence":0.8,"go_confidence":0.5,"python_verdict":"KEEP","go_verdict":"KEEP","delta_kind":"CONF_DRIFT"}`,
		},
		{
			name: "valid WRITE_FAILURE",
			line: `{"date":"2026-05-18","tag_signature":"__write_failures__","python_confidence":null,"go_confidence":null,"python_verdict":null,"go_verdict":null,"delta_kind":"WRITE_FAILURE"}`,
		},
		{
			name:    "missing delta_kind",
			line:    `{"date":"2026-05-18","tag_signature":"foo","python_confidence":0.5,"go_confidence":null,"python_verdict":"KEEP","go_verdict":null}`,
			wantErr: true,
		},
		{
			name:    "invalid delta_kind",
			line:    `{"date":"2026-05-18","tag_signature":"foo","python_confidence":0.5,"go_confidence":null,"python_verdict":"KEEP","go_verdict":null,"delta_kind":"UNKNOWN_KIND"}`,
			wantErr: true,
		},
		{
			name:    "empty tag_signature",
			line:    `{"date":"2026-05-18","tag_signature":"","python_confidence":0.5,"go_confidence":null,"python_verdict":"KEEP","go_verdict":null,"delta_kind":"MISS_GO"}`,
			wantErr: true,
		},
		{
			name:    "confidence out of range",
			line:    `{"date":"2026-05-18","tag_signature":"foo","python_confidence":1.5,"go_confidence":null,"python_verdict":"KEEP","go_verdict":null,"delta_kind":"MISS_GO"}`,
			wantErr: true,
		},
		{
			name:    "CONF_DRIFT with drift below threshold",
			line:    `{"date":"2026-05-18","tag_signature":"foo","python_confidence":0.5,"go_confidence":0.55,"python_verdict":"KEEP","go_verdict":"KEEP","delta_kind":"CONF_DRIFT"}`,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotErr := validateDiffLine(t, tc.line)
			if tc.wantErr && !gotErr {
				t.Errorf("expected validation error but got none")
			}
			if !tc.wantErr && gotErr {
				t.Errorf("expected no validation error")
			}
		})
	}
}

// validateDiffLine runs the same schema checks as TestDiffJSONLSchema against
// a single line. Returns true if any check failed.
func validateDiffLine(t *testing.T, line string) (hadError bool) {
	t.Helper()

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		t.Logf("not valid JSON: %v", err)
		return true
	}

	required := []string{"tag_signature", "python_confidence", "go_confidence", "python_verdict", "go_verdict", "delta_kind"}
	for _, key := range required {
		if _, ok := raw[key]; !ok {
			t.Logf("missing required field %q", key)
			hadError = true
		}
	}

	var rec diffRecord
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		t.Logf("unmarshal failed: %v", err)
		return true
	}

	if rec.DeltaKind == "" || !validDeltaKinds[rec.DeltaKind] {
		t.Logf("invalid delta_kind: %q", rec.DeltaKind)
		hadError = true
	}
	if rec.TagSignature == "" {
		t.Logf("empty tag_signature")
		hadError = true
	}
	if rec.PythonConfidence != nil && (*rec.PythonConfidence < 0 || *rec.PythonConfidence > 1.0) {
		t.Logf("python_confidence %.4f out of range", *rec.PythonConfidence)
		hadError = true
	}
	if rec.GoConfidence != nil && (*rec.GoConfidence < 0 || *rec.GoConfidence > 1.0) {
		t.Logf("go_confidence %.4f out of range", *rec.GoConfidence)
		hadError = true
	}
	if rec.DeltaKind == "CONF_DRIFT" && rec.PythonConfidence != nil && rec.GoConfidence != nil {
		drift := *rec.PythonConfidence - *rec.GoConfidence
		if drift < 0 {
			drift = -drift
		}
		if drift <= 0.1 {
			t.Logf("CONF_DRIFT drift=%.4f ≤ 0.1 threshold", drift)
			hadError = true
		}
	}
	return hadError
}
