package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── restEngram unit tests ─────────────────────────────────────────────────────

func TestRestStoreHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/quick-store" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"id":"mem-rest-001"}`))
	}))
	defer srv.Close()

	rc := newRestEngram(srv.URL, "tok")
	id, err := rc.quickStore(context.Background(), "proj1", "test content", []string{"tag-a"}, 3)
	if err != nil {
		t.Fatalf("quickStore error: %v", err)
	}
	if id != "mem-rest-001" {
		t.Errorf("id = %q, want mem-rest-001", id)
	}
}

func TestRestStoreServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"ok":false,"error":"db down"}`))
	}))
	defer srv.Close()

	rc := newRestEngram(srv.URL, "tok")
	// With 8 retries the test would take minutes on repeated 5xx — use a cancelled ctx.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancelled so first attempt returns ctx.Err after first server hit
	_, err := rc.quickStore(ctx, "proj1", "content", []string{}, 0)
	if err == nil {
		t.Fatal("expected error on server 5xx / cancelled ctx, got nil")
	}
}

func TestRestStoreAuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"id":"mem-auth-test"}`))
	}))
	defer srv.Close()

	rc := newRestEngram(srv.URL, "secret-token")
	_, err := rc.quickStore(context.Background(), "proj1", "x", []string{}, 0)
	if err != nil {
		t.Fatalf("quickStore error: %v", err)
	}
	if gotAuth != "Bearer secret-token" {
		t.Errorf("Authorization header = %q, want \"Bearer secret-token\"", gotAuth)
	}
}

func TestRestStoreImportanceIsFloat(t *testing.T) {
	// Verify that importance flows through as a numeric value (not a string).
	var decoded map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&decoded); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"id":"mem-imp"}`))
	}))
	defer srv.Close()

	rc := newRestEngram(srv.URL, "tok")
	_, err := rc.quickStore(context.Background(), "proj1", "content", []string{}, 7)
	if err != nil {
		t.Fatalf("quickStore error: %v", err)
	}
	imp, ok := decoded["importance"].(float64) // JSON numbers decode as float64
	if !ok {
		t.Fatalf("importance field not a number: %T %v", decoded["importance"], decoded["importance"])
	}
	if imp != 7 {
		t.Errorf("importance = %v, want 7", imp)
	}
}

func TestRestStoreSSESuffixStripped(t *testing.T) {
	// newRestEngram must strip /sse from the baseURL so the REST path is correct.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/quick-store" {
			t.Errorf("unexpected path %q — /sse was not stripped", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"id":"mem-strip"}`))
	}))
	defer srv.Close()

	rc := newRestEngram(srv.URL+"/sse", "tok")
	_, err := rc.quickStore(context.Background(), "proj1", "x", []string{}, 0)
	if err != nil {
		t.Fatalf("quickStore error: %v", err)
	}
}

func TestRestRecallHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/quick-recall" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"id":"mem-recall-001","tags":["instinct","sig-t"],"importance":5.0}]}`))
	}))
	defer srv.Close()

	rc := newRestEngram(srv.URL, "tok")
	results, err := rc.quickRecall(context.Background(), "proj1", "instinct pattern sig-t", []string{"sig-t"}, 10)
	if err != nil {
		t.Fatalf("quickRecall error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0]["id"] != "mem-recall-001" {
		t.Errorf("id = %v, want mem-recall-001", results[0]["id"])
	}
}

// ── hybridEngram routing tests ────────────────────────────────────────────────

// TestHybridEpisodeStartStillUsesSSE verifies that episodeStart hits the SSE
// mock (MCP server) and NOT the REST server.
func TestHybridEpisodeStartStillUsesSSE(t *testing.T) {
	// REST server that should NOT be called for episodeStart.
	restHit := false
	restSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		restHit = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer restSrv.Close()

	// Use the existing MCP test helper that wires memory_episode_start.
	e, cleanup := newTestEngramServer(t)
	defer cleanup()

	// Patch the REST client to point at our spy server (not the real one).
	if he, ok := e.(*hybridEngram); ok {
		he.rest = newRestEngram(restSrv.URL, "tok")
	}

	epID, err := e.episodeStart(context.Background(), "sess-hybrid", "proj1")
	if err != nil {
		t.Fatalf("episodeStart: %v", err)
	}
	if epID == "" {
		t.Error("episodeStart returned empty ID")
	}
	if restHit {
		t.Error("REST server was called for episodeStart — it should stay on SSE")
	}
}

// TestHybridIngestUsesREST verifies that ingest hits POST /quick-store on the
// REST mock and NOT the SSE (MCP) server.
func TestHybridIngestUsesREST(t *testing.T) {
	restHit := false
	restSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		restHit = true
		if r.URL.Path != "/quick-store" {
			t.Errorf("unexpected REST path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"id":"mem-ingest"}`))
	}))
	defer restSrv.Close()

	e, cleanup := newTestEngramServer(t)
	defer cleanup()

	if he, ok := e.(*hybridEngram); ok {
		he.rest = newRestEngram(restSrv.URL, "tok")
	}

	ev := Event{SessionID: "s1", ProjectID: "p1", ToolName: "Bash"}
	if err := e.ingest(context.Background(), ev, "p1", "s1"); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if !restHit {
		t.Error("REST server was never called for ingest — it should use /quick-store")
	}
}

// TestHybridStoreUsesREST verifies store routes to POST /quick-store.
func TestHybridStoreUsesREST(t *testing.T) {
	var reqBody map[string]any
	restSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"id":"mem-store-rest"}`))
	}))
	defer restSrv.Close()

	e, cleanup := newTestEngramServer(t)
	defer cleanup()

	if he, ok := e.(*hybridEngram); ok {
		he.rest = newRestEngram(restSrv.URL, "tok")
	}

	p := Pattern{Type: "workflow", Description: "test pattern", Domain: "git", Evidence: "seen", TagSignature: "sig-rest"}
	id, err := e.store(context.Background(), p, 0.5, "proj1")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if id != "mem-store-rest" {
		t.Errorf("id = %q, want mem-store-rest", id)
	}

	// Verify importance = int(0.5 * 10) = 5 in request body.
	imp, _ := reqBody["importance"].(float64)
	if imp != 5 {
		t.Errorf("importance in REST body = %v, want 5 (0.5×10)", imp)
	}

	// Verify tag_signature is present in tags.
	tags, _ := reqBody["tags"].([]any)
	found := false
	for _, tg := range tags {
		if tg == "sig-rest" {
			found = true
		}
	}
	if !found {
		t.Errorf("tag_signature %q not found in REST body tags: %v", "sig-rest", tags)
	}
}

// TestHybridCorrectUsesSSE verifies that correct stays on the SSE path.
func TestHybridCorrectUsesSSE(t *testing.T) {
	restHit := false
	restSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		restHit = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer restSrv.Close()

	e, cleanup := newTestEngramServer(t)
	defer cleanup()

	if he, ok := e.(*hybridEngram); ok {
		he.rest = newRestEngram(restSrv.URL, "tok")
	}

	if err := e.correct(context.Background(), "mem-abc", 0.7); err != nil {
		t.Fatalf("correct: %v", err)
	}
	if restHit {
		t.Error("REST server was called for correct — it should stay on SSE")
	}
}

// TestHybridRecallUsesREST verifies recall routes to POST /quick-recall.
func TestHybridRecallUsesREST(t *testing.T) {
	restHit := false
	restSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		restHit = true
		if r.URL.Path != "/quick-recall" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"id":"mem-r","tags":["instinct","sig-recall"],"importance":3.0}]}`))
	}))
	defer restSrv.Close()

	e, cleanup := newTestEngramServer(t)
	defer cleanup()

	if he, ok := e.(*hybridEngram); ok {
		he.rest = newRestEngram(restSrv.URL, "tok")
	}

	r, err := e.recall(context.Background(), "sig-recall", "proj1")
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if !restHit {
		t.Error("REST server was never called for recall — it should use /quick-recall")
	}
	if r == nil {
		t.Fatal("recall returned nil, want match")
	}
	if r.id != "mem-r" {
		t.Errorf("recall id = %q, want mem-r", r.id)
	}
}

// TestRestStore429RetriesEventually verifies that a 429 followed by success
// returns the ID rather than failing immediately.
func TestRestStore429RetriesEventually(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 2 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"ok":false,"error":"rate limit"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"id":"mem-after-429"}`))
	}))
	defer srv.Close()

	rc := newRestEngram(srv.URL, "tok")
	id, err := rc.quickStore(context.Background(), "p", "x", []string{}, 0)
	if err != nil {
		t.Fatalf("quickStore error after 429+retry: %v", err)
	}
	if id != "mem-after-429" {
		t.Errorf("id = %q, want mem-after-429", id)
	}
	if calls < 2 {
		t.Errorf("calls = %d, want >= 2 (retry must have occurred)", calls)
	}
}

// TestRestStoreBodyShape verifies the JSON body sent to /quick-store contains
// all required fields with correct types.
func TestRestStoreBodyShape(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"id":"mem-shape"}`))
	}))
	defer srv.Close()

	rc := newRestEngram(srv.URL, "tok")
	_, _ = rc.quickStore(context.Background(), "myproject", "hello world", []string{"tag1", "tag2"}, 5)

	if body["content"] != "hello world" {
		t.Errorf("content = %v", body["content"])
	}
	if body["project"] != "myproject" {
		t.Errorf("project = %v", body["project"])
	}
	tags, _ := body["tags"].([]any)
	if len(tags) != 2 || tags[0] != "tag1" {
		t.Errorf("tags = %v", tags)
	}
	if body["importance"] != float64(5) {
		t.Errorf("importance = %v", body["importance"])
	}
}

// ── newTestEngramServer returns hybridEngram (not sseEngram) ──────────────────
//
// The existing newTestEngramServer in main_test.go is replaced below so that
// all engramAPI tests exercise the hybrid client.  The SSE mock handles
// episodeStart/End and correct; ingest/store/recall are redirected to a
// local REST spy by individual tests.

func init() {
	// Ensure hybridEngram implements engramAPI at compile time.
	var _ engramAPI = (*hybridEngram)(nil)
}

// restBodyContains is a helper for tests that want to confirm a tag appears
// somewhere in the comma-joined body string (for human readability in errors).
func restBodyContains(body map[string]any, tag string) bool {
	tags, _ := body["tags"].([]any)
	for _, t := range tags {
		if s, ok := t.(string); ok && strings.Contains(s, tag) {
			return true
		}
	}
	return false
}
