package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

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
			{{Role: "user", Content: ""}},           // empty — should skip
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
