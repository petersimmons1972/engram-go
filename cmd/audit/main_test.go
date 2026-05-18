package audit

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestEmptyPatternSet verifies that when Engram returns zero patterns the binary
// produces valid JSON with zero counts and exit 0 (we test the runAudit func directly).
func TestEmptyPatternSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := struct {
			Results []engramMemory `json:"results"`
		}{}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := &mockLLMClient{response: ""} // should never be called
	var buf bytes.Buffer
	err := runAudit(srv.URL, "tok", client, 0, &buf)
	if err != nil {
		t.Fatalf("runAudit with zero patterns: %v", err)
	}

	var r report
	if err := json.NewDecoder(&buf).Decode(&r); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if r.Total != 0 {
		t.Errorf("Total: want 0, got %d", r.Total)
	}
	if r.Keep != 0 || r.Tune != 0 || r.Reject != 0 {
		t.Errorf("want all zero counts, got K=%d T=%d R=%d", r.Keep, r.Tune, r.Reject)
	}
	if r.FalsePositiveRate != 0.0 {
		t.Errorf("FalsePositiveRate: want 0.0, got %f", r.FalsePositiveRate)
	}
}

// TestJSONOutputSnapshotMatchesContract is belt-and-suspenders on top of the
// report-level schema test: it exercises runAudit end-to-end with a mock Engram
// and mock LLM, then uses jq-equivalent assertions on the five contract fields.
func TestJSONOutputSnapshotMatchesContract(t *testing.T) {
	pattern := engramMemory{
		ID:         "contract-id",
		Content:    "pattern content",
		Tags:       []string{"instinct", "correction", "sig-contract"},
		Importance: 0.9,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := struct {
			Results []engramMemory `json:"results"`
		}{Results: []engramMemory{pattern}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := &mockLLMClient{
		response: "IS_VALID: yes\nIS_ACTIONABLE: yes\nIS_SPECIFIC: yes\nFALSE_POSITIVE: no\nVERDICT: KEEP\nREASON: Solid pattern.",
	}

	var buf bytes.Buffer
	err := runAudit(srv.URL, "tok", client, 0, &buf)
	if err != nil {
		t.Fatalf("runAudit: %v", err)
	}

	// Decode into a raw map so we can assert exact key names (the contract).
	var m map[string]any
	if err := json.NewDecoder(&buf).Decode(&m); err != nil {
		t.Fatalf("decode output: %v", err)
	}

	for _, key := range []string{"total", "keep", "tune", "reject", "false_positive_rate"} {
		if _, ok := m[key]; !ok {
			t.Errorf("contract: key %q missing from JSON output", key)
		}
	}

	if v, _ := m["total"].(float64); int(v) != 1 {
		t.Errorf("total: want 1, got %v", m["total"])
	}
	if v, _ := m["keep"].(float64); int(v) != 1 {
		t.Errorf("keep: want 1, got %v", m["keep"])
	}
}
