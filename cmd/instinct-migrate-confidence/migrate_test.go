package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Mock Engram client for unit tests.
// ---------------------------------------------------------------------------

type mockEngram struct {
	records        []engramRecord
	correctCalls   []correctCall
	correctErr     error
	queryCalled    int
}

type correctCall struct {
	id         string
	project    string
	importance float64
}

func (m *mockEngram) queryRecords(_ context.Context, _ string) ([]engramRecord, error) {
	m.queryCalled++
	return m.records, nil
}

func (m *mockEngram) correctRecord(_ context.Context, id, project string, importance float64) error {
	if m.correctErr != nil {
		return m.correctErr
	}
	m.correctCalls = append(m.correctCalls, correctCall{id: id, project: project, importance: importance})
	// Update in-memory record to simulate idempotency
	for i, r := range m.records {
		if r.ID == id && r.Project == project {
			m.records[i].Importance = importance
			break
		}
	}
	return nil
}

func (m *mockEngram) fetchRecord(_ context.Context, id, project string) (*engramRecord, error) {
	for _, r := range m.records {
		if r.ID == id && r.Project == project {
			rc := r
			return &rc, nil
		}
	}
	return nil, fmt.Errorf("record %s not found in project %s", id, project)
}

// ---------------------------------------------------------------------------
// TestDetectIntegerEncoded — golden-input table for isAffected.
// Affected set: {7, 10}. Everything else is either anomaly or not-affected.
// ---------------------------------------------------------------------------

func TestDetectIntegerEncoded(t *testing.T) {
	cases := []struct {
		importance float64
		affected   bool
		anomaly    bool
	}{
		{-1, false, true},   // negative → anomaly
		{0, false, false},   // 0 → not affected (conservative; could mean "no score")
		{0.5, false, false}, // float < 1 → correct encoding, not affected
		{1, false, false},   // 1 → ambiguous (could be float 1.0 = 100%), conservative: skip
		{1.0, false, false}, // float 1.0 → same ambiguity, skip
		{7, true, false},    // integer 7 in [2,9] → bug encoding
		{10, true, false},   // integer 10 → bug encoding (max)
		{10.5, false, true}, // out of range [0,10] → anomaly
		{11, false, true},   // > 10 → anomaly
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("importance=%.1f", c.importance), func(t *testing.T) {
			gotAffected, gotAnomaly := classifyRecord(c.importance)
			if gotAffected != c.affected {
				t.Errorf("importance=%.1f: affected=%v, want %v", c.importance, gotAffected, c.affected)
			}
			if gotAnomaly != c.anomaly {
				t.Errorf("importance=%.1f: anomaly=%v, want %v", c.importance, gotAnomaly, c.anomaly)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestDetectReportOnlyNeverWrites — detect-and-report must not call correctRecord.
// ---------------------------------------------------------------------------

func TestDetectReportOnlyNeverWrites(t *testing.T) {
	mock := &mockEngram{
		records: []engramRecord{
			{ID: "r1", Project: "clearwatch", Importance: 7, Tags: []string{"instinct", "sig-abc123"}},
			{ID: "r2", Project: "global", Importance: 3, Tags: []string{"instinct", "sig-def456"}},
			{ID: "r3", Project: "homelab", Importance: 0.5, Tags: []string{"instinct", "sig-ghi789"}},
		},
	}

	rpt, err := runDetect(context.Background(), mock, allProjects)
	if err != nil {
		t.Fatalf("runDetect() error: %v", err)
	}
	if len(mock.correctCalls) != 0 {
		t.Errorf("detect mode must not call correctRecord; got %d calls", len(mock.correctCalls))
	}
	if rpt.CandidatesForMigration != 2 {
		t.Errorf("candidates = %d, want 2", rpt.CandidatesForMigration)
	}
	if rpt.TotalRecords != 3 {
		t.Errorf("total = %d, want 3", rpt.TotalRecords)
	}
}

// ---------------------------------------------------------------------------
// TestApplyPreflightRequiresBackup — --apply errors if backup absent.
// ---------------------------------------------------------------------------

func TestApplyPreflightRequiresBackup(t *testing.T) {
	dir := t.TempDir()
	mock := &mockEngram{
		records: []engramRecord{
			{ID: "r1", Project: "clearwatch", Importance: 7, Tags: []string{"instinct", "sig-abc"}},
		},
	}
	cfg := applyConfig{
		backupDir: dir,
		logDir:    dir,
		date:      "2099-01-01", // no file will exist for this date
	}
	err := runApply(context.Background(), mock, allProjects, cfg)
	if err == nil {
		t.Fatal("runApply must return error when backup is absent")
	}
	if len(mock.correctCalls) != 0 {
		t.Errorf("must not call correctRecord when preflight fails; got %d calls", len(mock.correctCalls))
	}
}

// ---------------------------------------------------------------------------
// TestApplyConversionFormula — importance=7 → exactly 0.7.
// ---------------------------------------------------------------------------

func TestApplyConversionFormula(t *testing.T) {
	dir := t.TempDir()
	// Write a fake backup so preflight passes.
	date := time.Now().Format("2006-01-02")
	backupPath := filepath.Join(dir, "pre-migration-"+date+".jsonl")
	if err := os.WriteFile(backupPath, []byte(`{"id":"r1"}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	mock := &mockEngram{
		records: []engramRecord{
			{ID: "r1", Project: "clearwatch", Importance: 7, Tags: []string{"instinct", "sig-abc"}},
		},
	}
	cfg := applyConfig{
		backupDir: dir,
		logDir:    dir,
		date:      date,
	}
	if err := runApply(context.Background(), mock, allProjects, cfg); err != nil {
		t.Fatalf("runApply() error: %v", err)
	}
	if len(mock.correctCalls) != 1 {
		t.Fatalf("expected 1 correctCall, got %d", len(mock.correctCalls))
	}
	got := mock.correctCalls[0].importance
	want := 0.7
	if diff := got - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("converted importance = %.10f, want exactly %.10f", got, want)
	}
}

// ---------------------------------------------------------------------------
// TestApplyIdempotent — second run is no-op.
// ---------------------------------------------------------------------------

func TestApplyIdempotent(t *testing.T) {
	dir := t.TempDir()
	date := time.Now().Format("2006-01-02")
	backupPath := filepath.Join(dir, "pre-migration-"+date+".jsonl")
	_ = os.WriteFile(backupPath, []byte(`{"id":"r1"}`+"\n"), 0600)

	mock := &mockEngram{
		records: []engramRecord{
			{ID: "r1", Project: "clearwatch", Importance: 7, Tags: []string{"instinct", "sig-abc"}},
		},
	}
	cfg := applyConfig{
		backupDir: dir,
		logDir:    dir,
		date:      date,
	}

	// First run — should correct once.
	if err := runApply(context.Background(), mock, allProjects, cfg); err != nil {
		t.Fatalf("first runApply() error: %v", err)
	}
	firstCalls := len(mock.correctCalls)
	if firstCalls != 1 {
		t.Fatalf("first run: expected 1 correctCall, got %d", firstCalls)
	}

	// Second run — record now has float importance; must be no-op.
	if err := runApply(context.Background(), mock, allProjects, cfg); err != nil {
		t.Fatalf("second runApply() error: %v", err)
	}
	if len(mock.correctCalls) != firstCalls {
		t.Errorf("second run: correctCalls grew from %d to %d; expected idempotent no-op",
			firstCalls, len(mock.correctCalls))
	}
}

// ---------------------------------------------------------------------------
// TestRevertRestores — apply then revert returns to original state.
// ---------------------------------------------------------------------------

func TestRevertRestores(t *testing.T) {
	dir := t.TempDir()
	date := time.Now().Format("2006-01-02")
	backupPath := filepath.Join(dir, "pre-migration-"+date+".jsonl")
	_ = os.WriteFile(backupPath, []byte(`{"id":"r1"}`+"\n"), 0600)

	mock := &mockEngram{
		records: []engramRecord{
			{ID: "r1", Project: "clearwatch", Importance: 7, Tags: []string{"instinct", "sig-abc"}},
		},
	}
	cfg := applyConfig{
		backupDir: dir,
		logDir:    dir,
		date:      date,
	}

	// Apply.
	if err := runApply(context.Background(), mock, allProjects, cfg); err != nil {
		t.Fatalf("runApply() error: %v", err)
	}
	if mock.records[0].Importance != 0.7 {
		t.Fatalf("after apply, importance = %v, want 0.7", mock.records[0].Importance)
	}

	// Find the migration log.
	logPath := filepath.Join(dir, "migration-"+date+".log")
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("migration log not found at %s: %v", logPath, err)
	}

	// Revert.
	revertCfg := revertConfig{
		logDir: dir,
		date:   date,
	}
	if err := runRevert(context.Background(), mock, logPath, revertCfg); err != nil {
		t.Fatalf("runRevert() error: %v", err)
	}
	if mock.records[0].Importance != 7.0 {
		t.Errorf("after revert, importance = %v, want 7.0", mock.records[0].Importance)
	}
}

// ---------------------------------------------------------------------------
// TestRevertIdempotent — second revert is a no-op.
// ---------------------------------------------------------------------------

func TestRevertIdempotent(t *testing.T) {
	dir := t.TempDir()
	date := time.Now().Format("2006-01-02")
	backupPath := filepath.Join(dir, "pre-migration-"+date+".jsonl")
	_ = os.WriteFile(backupPath, []byte(`{"id":"r1"}`+"\n"), 0600)

	mock := &mockEngram{
		records: []engramRecord{
			{ID: "r1", Project: "clearwatch", Importance: 7, Tags: []string{"instinct", "sig-abc"}},
		},
	}
	cfg := applyConfig{backupDir: dir, logDir: dir, date: date}
	_ = runApply(context.Background(), mock, allProjects, cfg)

	logPath := filepath.Join(dir, "migration-"+date+".log")
	revertCfg := revertConfig{logDir: dir, date: date}

	// First revert.
	if err := runRevert(context.Background(), mock, logPath, revertCfg); err != nil {
		t.Fatalf("first revert error: %v", err)
	}
	calls1 := len(mock.correctCalls)

	// Second revert — already at original, must be no-op.
	if err := runRevert(context.Background(), mock, logPath, revertCfg); err != nil {
		t.Fatalf("second revert error: %v", err)
	}
	if len(mock.correctCalls) != calls1 {
		t.Errorf("second revert made %d new correctCalls, expected 0",
			len(mock.correctCalls)-calls1)
	}
}

// ---------------------------------------------------------------------------
// TestBackupSchema — backup JSONL has required fields per line.
// ---------------------------------------------------------------------------

func TestBackupSchema(t *testing.T) {
	dir := t.TempDir()
	date := "2026-01-15"

	mock := &mockEngram{
		records: []engramRecord{
			{ID: "r1", Project: "clearwatch", Importance: 7, Tags: []string{"instinct", "sig-abc"}, Content: "some pattern"},
			{ID: "r2", Project: "global", Importance: 0.5, Tags: []string{"instinct", "sig-def"}, Content: "another pattern"},
		},
	}

	if err := runBackup(context.Background(), mock, allProjects, dir, date); err != nil {
		t.Fatalf("runBackup() error: %v", err)
	}

	backupPath := filepath.Join(dir, "pre-migration-"+date+".jsonl")
	data, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("backup file not found: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 JSONL lines, got %d", len(lines))
	}

	for i, line := range lines {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("line %d: invalid JSON: %v", i, err)
		}
		for _, field := range []string{"id", "project", "importance", "tags"} {
			if _, ok := rec[field]; !ok {
				t.Errorf("line %d: missing required field %q", i, field)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// TestWriteReportJSON — writeReport emits valid JSON with the right schema.
// ---------------------------------------------------------------------------

func TestWriteReportJSON(t *testing.T) {
	rpt := DetectReport{
		ScannedAt:              "2026-01-01T00:00:00Z",
		TotalRecords:           5,
		CandidatesForMigration: 2,
		ByProject: map[string]ProjectStats{
			"clearwatch": {Scanned: 3, Candidates: 2},
		},
		Anomalies: []AnomalyRecord{
			{ID: "bad1", Project: "global", Importance: -1.5, Reason: "negative value"},
		},
		SampleAffected: []SampleRecord{
			{ID: "r1", Project: "clearwatch", TagSignature: "sig-abc", Importance: 7},
		},
	}

	var stdoutBuf, stderrBuf strings.Builder
	if err := writeReport(rpt, &stdoutBuf, &stderrBuf); err != nil {
		t.Fatalf("writeReport() error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(stdoutBuf.String()), &decoded); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nOutput: %s", err, stdoutBuf.String())
	}
	for _, field := range []string{"scanned_at", "total_records", "candidates_for_migration", "by_project", "anomalies", "sample_affected"} {
		if _, ok := decoded[field]; !ok {
			t.Errorf("JSON output missing field %q", field)
		}
	}
	if n := decoded["total_records"].(float64); int(n) != 5 {
		t.Errorf("total_records = %v, want 5", n)
	}

	// Stderr must have human-readable summary.
	summary := stderrBuf.String()
	if !strings.Contains(summary, "Total records") {
		t.Errorf("stderr missing 'Total records': %s", summary)
	}
	if !strings.Contains(summary, "Candidates") {
		t.Errorf("stderr missing 'Candidates': %s", summary)
	}
}

// ---------------------------------------------------------------------------
// TestAnomalyReason — covers anomalyReason for all branches.
// ---------------------------------------------------------------------------

func TestAnomalyReason(t *testing.T) {
	cases := []struct {
		val  float64
		want string
	}{
		{-1, "negative value"},
		{11, "value exceeds maximum of 10"},
		{5.5, "non-integer float in ambiguous range"},
	}
	for _, c := range cases {
		got := anomalyReason(c.val)
		if got != c.want {
			t.Errorf("anomalyReason(%.1f) = %q, want %q", c.val, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// TestSigTag — sig tag extraction edge cases.
// ---------------------------------------------------------------------------

func TestSigTag(t *testing.T) {
	if got := sigTag([]string{"instinct", "clearwatch"}); got != "" {
		t.Errorf("sigTag with no sig-* tag = %q, want empty", got)
	}
	if got := sigTag([]string{"instinct", "sig-abc123", "clearwatch"}); got != "sig-abc123" {
		t.Errorf("sigTag = %q, want sig-abc123", got)
	}
}

// ---------------------------------------------------------------------------
// TestApplyNoSleep — override rate limiter in tests so apply runs are fast.
// (This is also implicitly tested by all apply tests succeeding quickly.)
// ---------------------------------------------------------------------------

func init() {
	// Disable the 50ms rate-limit sleep in all tests.
	sleepForRateLimit = func() {}
}

// ---------------------------------------------------------------------------
// TestResolveEngram — reads endpoint/token from a temp mcp_servers.json.
// ---------------------------------------------------------------------------

func TestResolveEngram(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "mcp_servers.json")
	cfg := `{
		"mcpServers": {
			"engram": {
				"url": "http://localhost:8788/sse",
				"headers": {"Authorization": "Bearer test-token-xyz"}
			}
		}
	}`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)
	// Write to correct sub-path that resolveEngram expects.
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "mcp_servers.json"), []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}

	base, tok := resolveEngram("", "")
	if base != "http://localhost:8788" {
		t.Errorf("base = %q, want http://localhost:8788", base)
	}
	if tok != "test-token-xyz" {
		t.Errorf("token = %q, want test-token-xyz", tok)
	}
}

// ---------------------------------------------------------------------------
// TestResolveEngramFlagOverride — explicit flags bypass mcp_servers.json.
// ---------------------------------------------------------------------------

func TestResolveEngramFlagOverride(t *testing.T) {
	base, tok := resolveEngram("http://example.com", "mytoken")
	if base != "http://example.com" {
		t.Errorf("base = %q, want http://example.com", base)
	}
	if tok != "mytoken" {
		t.Errorf("token = %q, want mytoken", tok)
	}
}

// ---------------------------------------------------------------------------
// TestHTTPEngramQueryRecords — httptest server validates request shape and
// returns a fixture response. Covers newHTTPEngram, doPost, queryRecords.
// ---------------------------------------------------------------------------

func TestHTTPEngramQueryRecords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("want POST, got %s", r.Method)
		}
		if r.URL.Path != "/quick-recall" {
			t.Errorf("want /quick-recall, got %s", r.URL.Path)
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-tok" {
			t.Errorf("auth header = %q, want 'Bearer test-tok'", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{
					"id":         "m1",
					"importance": 7.0,
					"tags":       []string{"instinct", "sig-abc"},
					"content":    "test pattern",
				},
			},
		})
	}))
	defer srv.Close()

	client := newHTTPEngram(srv.URL, "test-tok")
	records, err := client.queryRecords(context.Background(), "clearwatch")
	if err != nil {
		t.Fatalf("queryRecords() error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].ID != "m1" {
		t.Errorf("record ID = %q, want m1", records[0].ID)
	}
	if records[0].Importance != 7.0 {
		t.Errorf("importance = %v, want 7.0", records[0].Importance)
	}
	if records[0].Project != "clearwatch" {
		t.Errorf("project = %q, want clearwatch", records[0].Project)
	}
}

// ---------------------------------------------------------------------------
// TestHTTPEngramCorrectRecord — validates MCP JSON-RPC call shape.
// ---------------------------------------------------------------------------

func TestHTTPEngramCorrectRecord(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" {
			t.Errorf("want /mcp, got %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"id": "m1"}})
	}))
	defer srv.Close()

	client := newHTTPEngram(srv.URL, "test-tok")
	if err := client.correctRecord(context.Background(), "m1", "clearwatch", 0.7); err != nil {
		t.Fatalf("correctRecord() error: %v", err)
	}
	if gotBody["method"] != "tools/call" {
		t.Errorf("method = %v, want tools/call", gotBody["method"])
	}
	params, _ := gotBody["params"].(map[string]any)
	if params["name"] != "memory_correct" {
		t.Errorf("tool name = %v, want memory_correct", params["name"])
	}
	args, _ := params["arguments"].(map[string]any)
	if args["memory_id"] != "m1" {
		t.Errorf("memory_id = %v, want m1", args["memory_id"])
	}
	if args["importance"] != 0.7 {
		t.Errorf("importance = %v, want 0.7", args["importance"])
	}
}

// ---------------------------------------------------------------------------
// TestHTTPEngramFetchRecord — fetchRecord finds the right record by ID.
// ---------------------------------------------------------------------------

func TestHTTPEngramFetchRecord(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"id": "m1", "importance": 0.7, "tags": []string{"instinct", "sig-abc"}},
				{"id": "m2", "importance": 5.0, "tags": []string{"instinct", "sig-def"}},
			},
		})
	}))
	defer srv.Close()

	client := newHTTPEngram(srv.URL, "tok")
	rec, err := client.fetchRecord(context.Background(), "m2", "homelab")
	if err != nil {
		t.Fatalf("fetchRecord() error: %v", err)
	}
	if rec.ID != "m2" {
		t.Errorf("id = %q, want m2", rec.ID)
	}
	if rec.Importance != 5.0 {
		t.Errorf("importance = %v, want 5.0", rec.Importance)
	}

	// Not found case.
	_, err = client.fetchRecord(context.Background(), "missing", "homelab")
	if err == nil {
		t.Error("expected error for missing record, got nil")
	}
}

// ---------------------------------------------------------------------------
// TestHTTPEngramHTTPError — doPost surfaces non-2xx status codes.
// ---------------------------------------------------------------------------

func TestHTTPEngramHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := newHTTPEngram(srv.URL, "bad-tok")
	_, err := client.queryRecords(context.Background(), "clearwatch")
	if err == nil {
		t.Error("expected error for 401, got nil")
	}
}
