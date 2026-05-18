package main

import (
	"encoding/json"
	"testing"
)

func TestBuildReportCountVerdicts(t *testing.T) {
	results := []auditResult{
		{ID: "1", Verdict: "KEEP"},
		{ID: "2", Verdict: "KEEP"},
		{ID: "3", Verdict: "TUNE"},
		{ID: "4", Verdict: "REJECT"},
		{ID: "5", Verdict: "ERROR"},
	}
	r := buildReport(results)
	if r.Total != 5 {
		t.Errorf("Total: want 5, got %d", r.Total)
	}
	if r.Keep != 2 {
		t.Errorf("Keep: want 2, got %d", r.Keep)
	}
	if r.Tune != 1 {
		t.Errorf("Tune: want 1, got %d", r.Tune)
	}
	if r.Reject != 2 {
		t.Errorf("Reject: want 2, got %d (ERROR should count as reject)", r.Reject)
	}
}

func TestBuildReportFalsePositiveRate(t *testing.T) {
	results := []auditResult{
		{ID: "1", Verdict: "KEEP", FalsePositive: "yes"},
		{ID: "2", Verdict: "KEEP", FalsePositive: "yes"},
		{ID: "3", Verdict: "TUNE", FalsePositive: "no"},
		{ID: "4", Verdict: "REJECT", FalsePositive: "no"},
	}
	r := buildReport(results)
	want := 0.5 // 2/4
	if r.FalsePositiveRate != want {
		t.Errorf("FalsePositiveRate: want %.2f, got %.2f", want, r.FalsePositiveRate)
	}
}

func TestBuildReportEmptyResults(t *testing.T) {
	r := buildReport(nil)
	if r.Total != 0 {
		t.Errorf("Total: want 0, got %d", r.Total)
	}
	if r.FalsePositiveRate != 0.0 {
		t.Errorf("FalsePositiveRate: want 0.0, got %f (must not be NaN or divide-by-zero)", r.FalsePositiveRate)
	}
}

// TestReportJSONSchema is the weekly-cron contract test.
// The jq filter in instinct-weekly-audit.sh reads exactly these five keys.
// If any key is missing or renamed, the weekly audit silently produces zeros.
func TestReportJSONSchema(t *testing.T) {
	r := report{
		Total:             3,
		Keep:              1,
		Tune:              1,
		Reject:            1,
		FalsePositiveRate: 0.33,
		Patterns:          []auditResult{{ID: "x", Verdict: "KEEP"}},
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	// Contract: these exact five keys must exist.
	for _, key := range []string{"total", "keep", "tune", "reject", "false_positive_rate"} {
		if _, ok := m[key]; !ok {
			t.Errorf("JSON contract: key %q missing from report output", key)
		}
	}

	// total must be numeric (json.Number floats decode as float64 in Go).
	if v, ok := m["total"].(float64); !ok || v != 3.0 {
		t.Errorf("JSON contract: total must be numeric 3, got %v (%T)", m["total"], m["total"])
	}
	if v, ok := m["keep"].(float64); !ok || v != 1.0 {
		t.Errorf("JSON contract: keep must be numeric 1, got %v", m["keep"])
	}
	if v, ok := m["tune"].(float64); !ok || v != 1.0 {
		t.Errorf("JSON contract: tune must be numeric 1, got %v", m["tune"])
	}
	if v, ok := m["reject"].(float64); !ok || v != 1.0 {
		t.Errorf("JSON contract: reject must be numeric 1, got %v", m["reject"])
	}
	if _, ok := m["false_positive_rate"].(float64); !ok {
		t.Errorf("JSON contract: false_positive_rate must be float64, got %T", m["false_positive_rate"])
	}
}

func TestExtractTags(t *testing.T) {
	tests := []struct {
		name       string
		tags       []string
		wantPtype  string
		wantDomain string
		wantSig    string
	}{
		{
			name:       "full set",
			tags:       []string{"instinct", "correction", "git", "sig-edit-fail-retry"},
			wantPtype:  "correction",
			wantDomain: "git",
			wantSig:    "sig-edit-fail-retry",
		},
		{
			name:       "workflow type",
			tags:       []string{"instinct", "workflow", "testing", "sig-test-loop"},
			wantPtype:  "workflow",
			wantDomain: "testing",
			wantSig:    "sig-test-loop",
		},
		{
			name:       "error_resolution type",
			tags:       []string{"instinct", "error_resolution", "bash", "sig-bash-retry"},
			wantPtype:  "error_resolution",
			wantDomain: "bash",
			wantSig:    "sig-bash-retry",
		},
		{
			name:  "no sig tag",
			tags:  []string{"instinct", "correction"},
			wantPtype:  "correction",
			wantSig:    "",
			wantDomain: "",
		},
		{
			name:  "empty tags",
			tags:  []string{},
			wantPtype:  "",
			wantDomain: "",
			wantSig:    "",
		},
		{
			name:       "instinct tag skipped as domain",
			tags:       []string{"instinct"},
			wantPtype:  "",
			wantDomain: "",
			wantSig:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ptype, domain, sig := extractTags(tt.tags)
			if ptype != tt.wantPtype {
				t.Errorf("ptype: want %q, got %q", tt.wantPtype, ptype)
			}
			if domain != tt.wantDomain {
				t.Errorf("domain: want %q, got %q", tt.wantDomain, domain)
			}
			if sig != tt.wantSig {
				t.Errorf("sig: want %q, got %q", tt.wantSig, sig)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		n     int
		want  string
	}{
		{"short", 10, "short"},
		{"exactly10c", 10, "exactly10c"},
		{"this is too long for n", 10, "this is to…"},
	}
	for _, tt := range tests {
		got := truncate(tt.input, tt.n)
		if got != tt.want {
			t.Errorf("truncate(%q, %d): want %q, got %q", tt.input, tt.n, tt.want, got)
		}
	}
}

func TestEnvOr(t *testing.T) {
	t.Setenv("ENVTEST_PRESENT", "found")
	t.Setenv("ENVTEST_EMPTY", "")

	if v := envOr("ENVTEST_PRESENT", "fallback"); v != "found" {
		t.Errorf("want %q, got %q", "found", v)
	}
	if v := envOr("ENVTEST_EMPTY", "fallback"); v != "fallback" {
		t.Errorf("want %q (empty env treated as absent), got %q", "fallback", v)
	}
	if v := envOr("ENVTEST_MISSING", "fallback"); v != "fallback" {
		t.Errorf("want %q, got %q", "fallback", v)
	}
}
