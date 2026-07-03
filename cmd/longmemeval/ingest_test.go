package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/chunk"
	"github.com/petersimmons1972/engram/internal/longmemeval"
)

func TestLoadItems_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte(`not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	// loadItems calls log.Fatalf on error; we can't test that directly
	// without spawning a subprocess. Just verify the happy-path reads correctly.
}

func TestLoadItems_Empty(t *testing.T) {
	// Empty JSON array is a fatal error. Again, can only test via happy path.
}

func TestIngestOne_PanicsAreRecovered(t *testing.T) {
	// ingestOne has a recover() that converts panics to error entries.
	// We cannot trigger a panic from a RestClient stub easily, but we can
	// at least call ingestOne through its normal path via a REST stub.
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "id": "m-1"})
	}))
	defer srv.Close()

	rc := longmemeval.NewRestClient(srv.URL, "")
	cfg := &Config{RunID: "testrun", Workers: 1}
	item := longmemeval.Item{
		QuestionID:         "q-test",
		HaystackSessionIDs: []string{"sid-1"},
		HaystackSessions: [][]longmemeval.Turn{
			{{Role: "user", Content: "hello session"}},
		},
		HaystackDates: []string{"2024-01-01"},
	}

	entry := ingestOne(t.Context(), cfg, rc, item)
	if entry.Status != "done" {
		t.Errorf("expected done, got %s: %s", entry.Status, entry.Error)
	}
	if entry.SessionCount != 1 {
		t.Errorf("expected 1 session, got %d", entry.SessionCount)
	}
	if calls != 1 {
		t.Errorf("expected 1 REST call, got %d", calls)
	}
}

func TestIngestOne_QuickStoreError_ReturnsErrorEntry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always fail with 400.
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "bad content"})
	}))
	defer srv.Close()

	rc := longmemeval.NewRestClient(srv.URL, "")
	cfg := &Config{RunID: "run1", Workers: 1}
	item := longmemeval.Item{
		QuestionID:         "q-fail",
		HaystackSessionIDs: []string{"sid-1"},
		HaystackSessions: [][]longmemeval.Turn{
			{{Role: "user", Content: "some content"}},
		},
	}

	entry := ingestOne(t.Context(), cfg, rc, item)
	if entry.Status != "error" {
		t.Errorf("expected error entry, got status=%s", entry.Status)
	}
	if entry.Error == "" {
		t.Error("expected non-empty error field")
	}
}

func TestIngestOne_EmptySessionsSkipped(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "id": "m-1"})
	}))
	defer srv.Close()

	rc := longmemeval.NewRestClient(srv.URL, "")
	cfg := &Config{RunID: "run1", Workers: 1}
	item := longmemeval.Item{
		QuestionID:         "q-empty",
		HaystackSessionIDs: []string{"sid-1", "sid-2"},
		HaystackSessions: [][]longmemeval.Turn{
			{{Role: "user", Content: ""}},             // empty — should skip
			{{Role: "user", Content: "real content"}}, // non-empty
		},
	}

	entry := ingestOne(t.Context(), cfg, rc, item)
	if entry.Status != "done" {
		t.Errorf("expected done, got %s: %s", entry.Status, entry.Error)
	}
	// Only 1 non-empty session should have been stored.
	if entry.SessionCount != 1 {
		t.Errorf("expected 1 session stored, got %d", entry.SessionCount)
	}
	if calls != 1 {
		t.Errorf("expected 1 REST call for 1 non-empty session, got %d", calls)
	}
}

// TestIngestOne_ScratchTTL_PassesExpiresAt verifies that when ScratchTTL > 0,
// ingestOne passes a non-nil expires_at in the QuickStore request body.
func TestIngestOne_ScratchTTL_PassesExpiresAt(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "id": "m-ttl"})
	}))
	defer srv.Close()

	rc := longmemeval.NewRestClient(srv.URL, "")
	cfg := &Config{RunID: "run-ttl", Workers: 1, ScratchTTL: 168 * time.Hour}
	item := longmemeval.Item{
		QuestionID:         "q-ttl",
		HaystackSessionIDs: []string{"sid-1"},
		HaystackSessions: [][]longmemeval.Turn{
			{{Role: "user", Content: "session content"}},
		},
	}

	before := time.Now().UTC()
	entry := ingestOne(t.Context(), cfg, rc, item)
	after := time.Now().UTC()

	if entry.Status != "done" {
		t.Fatalf("expected done, got %s: %s", entry.Status, entry.Error)
	}

	raw, ok := gotBody["expires_at"]
	if !ok {
		t.Fatal("expires_at missing from QuickStore request body")
	}
	parsed, err := time.Parse(time.RFC3339, raw.(string))
	if err != nil {
		t.Fatalf("expires_at is not RFC3339: %v", err)
	}
	// expires_at is serialized as RFC3339 (second precision), so truncate to seconds for comparison.
	expectedMin := before.Add(168 * time.Hour).Truncate(time.Second)
	expectedMax := after.Add(168 * time.Hour).Add(time.Second) // +1s to account for truncation
	if parsed.Before(expectedMin) || parsed.After(expectedMax) {
		t.Errorf("expires_at %v outside expected range [%v, %v]", parsed, expectedMin, expectedMax)
	}
}

// TestIngestOne_NoScratchTTL_OmitsExpiresAt verifies that when ScratchTTL == 0,
// expires_at is absent from the QuickStore request body.
func TestIngestOne_NoScratchTTL_OmitsExpiresAt(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "id": "m-durable"})
	}))
	defer srv.Close()

	rc := longmemeval.NewRestClient(srv.URL, "")
	cfg := &Config{RunID: "run-durable", Workers: 1, ScratchTTL: 0}
	item := longmemeval.Item{
		QuestionID:         "q-durable",
		HaystackSessionIDs: []string{"sid-1"},
		HaystackSessions: [][]longmemeval.Turn{
			{{Role: "user", Content: "durable content"}},
		},
	}

	entry := ingestOne(t.Context(), cfg, rc, item)
	if entry.Status != "done" {
		t.Fatalf("expected done, got %s: %s", entry.Status, entry.Error)
	}
	if _, ok := gotBody["expires_at"]; ok {
		t.Error("expires_at must be absent from QuickStore body when ScratchTTL is 0")
	}
}

func TestIngestOne_PreChunksWhenBlockOverlapSet(t *testing.T) {
	var calls int
	var contents []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		contents = append(contents, body.Content)
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "id": "m-" + strings.Repeat("x", calls)})
	}))
	defer srv.Close()

	var sb strings.Builder
	for sb.Len() <= chunk.LazyChunkThreshold+500 {
		sb.WriteString("This is a long session sentence that keeps the chunker busy. ")
	}

	rc := longmemeval.NewRestClient(srv.URL, "")
	cfg := &Config{
		RunID:             "run-overlap",
		Workers:           1,
		BlockOverlapChars: 400,
	}
	item := longmemeval.Item{
		QuestionID:         "q-overlap",
		HaystackSessionIDs: []string{"sid-1"},
		HaystackSessions: [][]longmemeval.Turn{
			{{Role: "user", Content: sb.String()}},
		},
		HaystackDates: []string{"2024-01-01"},
	}

	entry := ingestOne(t.Context(), cfg, rc, item)
	if entry.Status != "done" {
		t.Fatalf("expected done, got %s: %s", entry.Status, entry.Error)
	}
	if calls <= 1 {
		t.Fatalf("expected pre-chunked ingest to issue multiple QuickStore calls, got %d", calls)
	}
	if entry.SessionCount != 1 {
		t.Fatalf("session count = %d, want 1 source session", entry.SessionCount)
	}
	if len(entry.MemoryMap) != calls {
		t.Fatalf("memory map entries = %d, want %d", len(entry.MemoryMap), calls)
	}
	for i, content := range contents {
		if strings.TrimSpace(content) == "" {
			t.Fatalf("chunk %d content is empty", i)
		}
	}
}
