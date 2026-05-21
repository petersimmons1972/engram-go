# scratch-ttl MCP Routing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Route `--scratch-ttl` through the `/quick-store` REST body so LME can set project TTL against a remote Engram server without a direct Postgres connection.

**Architecture:** Thread an optional `expires_at` (RFC3339, `*time.Time`) through the `/quick-store` POST body. The server calls `SetProjectTTL` via the engine pool after a successful store. The LME `RestClient.QuickStore` gains a final `expiresAt *time.Time` parameter. The direct-DB `ttlStamper` path and `--database-url` flag are removed entirely.

**Tech Stack:** Go 1.23+, `net/http`, `encoding/json`, `time`, `log/slog`. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-05-20-scratch-ttl-mcp-routing-design.md`

**Issue:** #837

---

### Task 1: Failing tests — server-side `expires_at` handling

**Files:**
- Modify: `internal/mcp/quick_store_handler_test.go`

- [ ] **Step 1: Add `ttlCaptureBackend` and `newQuickStoreServerWithBackend` helpers**

Append to `internal/mcp/quick_store_handler_test.go` (inside `package mcp`) after the existing `newQuickStoreServer` function:

```go
// ttlCaptureBackend embeds storeBackend and records SetProjectTTL calls.
type ttlCaptureBackend struct {
	storeBackend
	mu              sync.Mutex
	capturedProject string
	capturedExpires *time.Time
	returnErr       error
}

func (b *ttlCaptureBackend) SetProjectTTL(_ context.Context, project string, _ time.Time, expiresAt *time.Time) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.capturedProject = project
	if expiresAt != nil {
		t := *expiresAt
		b.capturedExpires = &t
	}
	return b.returnErr
}

// newQuickStoreServerWithBackend builds a *Server that uses the given backend,
// letting tests observe SetProjectTTL calls.
func newQuickStoreServerWithBackend(t *testing.T, backend db.Backend) *Server {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, backend, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	pool := NewEnginePool(factory)
	cfg := testConfig()
	return &Server{pool: pool, cfg: cfg, embedderHealth: cfg.EmbedderHealth}
}
```

Add `"sync"` to the import block if not already present.

- [ ] **Step 2: Add the four new test functions**

Append to `internal/mcp/quick_store_handler_test.go`:

```go
// TestQuickStoreHandler_ExpiresAt_FutureTimestamp verifies that a POST with a
// future expires_at stores the memory and calls SetProjectTTL with the given time.
func TestQuickStoreHandler_ExpiresAt_FutureTimestamp(t *testing.T) {
	backend := &ttlCaptureBackend{}
	s := newQuickStoreServerWithBackend(t, backend)

	future := time.Now().UTC().Add(48 * time.Hour)
	body, _ := json.Marshal(map[string]any{
		"content":    "lme session content",
		"project":    "lme-run1-q001",
		"tags":       []string{"lme"},
		"expires_at": future.Format(time.RFC3339),
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-store", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, true, resp["ok"])

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Equal(t, "lme-run1-q001", backend.capturedProject, "SetProjectTTL should be called with the correct project")
	require.NotNil(t, backend.capturedExpires, "SetProjectTTL should receive a non-nil expiresAt")
	delta := backend.capturedExpires.Sub(future)
	if delta < 0 {
		delta = -delta
	}
	require.Less(t, delta, 2*time.Second, "captured expiresAt should be within 2s of the requested value")
}

// TestQuickStoreHandler_ExpiresAt_PastTimestamp verifies that a past expires_at
// is rejected with 400 before the store is written.
func TestQuickStoreHandler_ExpiresAt_PastTimestamp(t *testing.T) {
	backend := &ttlCaptureBackend{}
	s := newQuickStoreServerWithBackend(t, backend)

	past := time.Now().UTC().Add(-1 * time.Hour)
	body, _ := json.Marshal(map[string]any{
		"content":    "lme session content",
		"project":    "lme-run1-q001",
		"expires_at": past.Format(time.RFC3339),
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-store", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Empty(t, backend.capturedProject, "SetProjectTTL must not be called when expires_at is in the past")
}

// TestQuickStoreHandler_ExpiresAt_Absent verifies that omitting expires_at stores
// the memory successfully without calling SetProjectTTL.
func TestQuickStoreHandler_ExpiresAt_Absent(t *testing.T) {
	backend := &ttlCaptureBackend{}
	s := newQuickStoreServerWithBackend(t, backend)

	body, _ := json.Marshal(map[string]any{
		"content": "ordinary memory",
		"project": "global",
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-store", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Empty(t, backend.capturedProject, "SetProjectTTL must not be called when expires_at is absent")
}

// TestQuickStoreHandler_ExpiresAt_TTLError verifies that a SetProjectTTL error
// does not fail the store — the response is still 200 (best-effort semantics).
func TestQuickStoreHandler_ExpiresAt_TTLError(t *testing.T) {
	backend := &ttlCaptureBackend{returnErr: fmt.Errorf("simulated TTL write failure")}
	s := newQuickStoreServerWithBackend(t, backend)

	future := time.Now().UTC().Add(24 * time.Hour)
	body, _ := json.Marshal(map[string]any{
		"content":    "lme session content",
		"project":    "lme-run1-q002",
		"expires_at": future.Format(time.RFC3339),
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/quick-store", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleQuickStore(w, req)

	// Store must succeed even when TTL stamping fails.
	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, true, resp["ok"])
}
```

- [ ] **Step 3: Run the new tests — verify they fail**

```
cd /home/psimmons/projects/engram-go
go test ./internal/mcp/ -run 'TestQuickStoreHandler_ExpiresAt' -v -count=1 2>&1 | head -40
```

Expected: compile error or test failures — `ExpiresAt` field not yet in body struct, `sync` import missing, etc.

---

### Task 2: Implement server-side `expires_at` handling

**Files:**
- Modify: `internal/mcp/server.go` (lines 1554–1631, `handleQuickStore`)

- [ ] **Step 1: Add `ExpiresAt` field to the body struct and validation**

In `handleQuickStore`, replace the body struct (around line 1560):

```go
var body struct {
    Content    string     `json:"content"`
    Project    string     `json:"project"`
    Tags       []string   `json:"tags"`
    Importance int        `json:"importance"`
    ExpiresAt  *time.Time `json:"expires_at"`
}
```

After the existing `validateQuickStoreInput` call (around line 1580), add:

```go
if body.ExpiresAt != nil && !body.ExpiresAt.After(time.Now()) {
    writeJSONError(w, http.StatusBadRequest, "expires_at must be a future timestamp")
    return
}
```

- [ ] **Step 2: Call `SetProjectTTL` after successful store**

After the `writeJSON(w, http.StatusOK, ...)` line (line 1630), insert the SetProjectTTL call. The full tail of `handleQuickStore` should become:

```go
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})

	// #837: if expires_at was provided, stamp project_ttl via the engine pool.
	// Best-effort: failure is logged but does not affect the store response.
	if body.ExpiresAt != nil {
		if h, poolErr := s.pool.Get(r.Context(), project); poolErr == nil {
			if ttlErr := h.Engine.Backend().SetProjectTTL(r.Context(), project, time.Now().UTC(), body.ExpiresAt); ttlErr != nil {
				slog.Warn("quick-store: SetProjectTTL failed", "project", project, "err", ttlErr)
			}
		}
	}
}
```

Note: the closing `}` ends `handleQuickStore`. Remove the old closing brace that was after `writeJSON`.

- [ ] **Step 3: Run the new tests — verify they pass**

```
cd /home/psimmons/projects/engram-go
go test ./internal/mcp/ -run 'TestQuickStoreHandler_ExpiresAt' -v -count=1 2>&1
```

Expected: all four `TestQuickStoreHandler_ExpiresAt_*` tests pass.

- [ ] **Step 4: Run the full mcp package test suite**

```
cd /home/psimmons/projects/engram-go
go test ./internal/mcp/ -count=1 -race 2>&1 | tail -20
```

Expected: `ok internal/mcp ...` with no failures. Existing quick-store tests must still pass.

- [ ] **Step 5: Commit**

```bash
git add internal/mcp/server.go internal/mcp/quick_store_handler_test.go
git commit -m "feat(quick-store): accept expires_at and call SetProjectTTL (#837)"
```

---

### Task 3: Failing tests — `RestClient.QuickStore` with `expiresAt`

**Files:**
- Modify: `internal/longmemeval/engram_test.go`

- [ ] **Step 1: Add two tests that verify `expires_at` body field behaviour**

Append to `internal/longmemeval/engram_test.go`:

```go
// TestRestClient_QuickStore_ExpiresAt_Set verifies that a non-nil expiresAt is
// serialized as "expires_at" (RFC3339) in the POST body.
func TestRestClient_QuickStore_ExpiresAt_Set(t *testing.T) {
	future := time.Now().UTC().Add(72 * time.Hour)
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "id": "mem-ttl"})
	}))
	defer srv.Close()

	rc := longmemeval.NewRestClient(srv.URL, "")
	id, err := rc.QuickStore(context.Background(), "proj-ttl", "content", nil, &future)
	if err != nil {
		t.Fatalf("QuickStore: %v", err)
	}
	if id != "mem-ttl" {
		t.Errorf("id = %q, want mem-ttl", id)
	}
	raw, ok := gotBody["expires_at"]
	if !ok {
		t.Fatal("expires_at field missing from request body")
	}
	parsed, err := time.Parse(time.RFC3339, raw.(string))
	if err != nil {
		t.Fatalf("expires_at is not RFC3339: %v", err)
	}
	delta := parsed.Sub(future)
	if delta < 0 {
		delta = -delta
	}
	if delta > 2*time.Second {
		t.Errorf("expires_at delta = %v, want < 2s", delta)
	}
}

// TestRestClient_QuickStore_ExpiresAt_Nil verifies that a nil expiresAt does NOT
// include an "expires_at" key in the POST body (not null — fully absent).
func TestRestClient_QuickStore_ExpiresAt_Nil(t *testing.T) {
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "id": "mem-no-ttl"})
	}))
	defer srv.Close()

	rc := longmemeval.NewRestClient(srv.URL, "")
	id, err := rc.QuickStore(context.Background(), "proj-no-ttl", "content", nil, nil)
	if err != nil {
		t.Fatalf("QuickStore: %v", err)
	}
	if id != "mem-no-ttl" {
		t.Errorf("id = %q, want mem-no-ttl", id)
	}
	if _, ok := gotBody["expires_at"]; ok {
		t.Error("expires_at must be absent from request body when expiresAt is nil")
	}
}
```

- [ ] **Step 2: Update the three existing QuickStore call sites in the test file**

In `internal/longmemeval/engram_test.go`, update these three existing calls to pass `nil` as the new final argument:

Line ~58: `id, err := rc.QuickStore(ctx, "proj-1", "content here", []string{"tag1"})` → `id, err := rc.QuickStore(ctx, "proj-1", "content here", []string{"tag1"}, nil)`

Line ~86: `id, err := rc.QuickStore(ctx, "proj-x", "hello", nil)` → `id, err := rc.QuickStore(ctx, "proj-x", "hello", nil, nil)`

Line ~115: `id, err := rc.QuickStore(ctx, "p", "c", nil)` → `id, err := rc.QuickStore(ctx, "p", "c", nil, nil)`

- [ ] **Step 3: Run new tests — verify compile failure**

```
cd /home/psimmons/projects/engram-go
go test ./internal/longmemeval/ -run 'TestRestClient_QuickStore_ExpiresAt' -v -count=1 2>&1 | head -20
```

Expected: compile error — `QuickStore` does not yet have 5 arguments.

---

### Task 4: Update `RestClient.QuickStore` signature

**Files:**
- Modify: `internal/longmemeval/engram.go`

- [ ] **Step 1: Update `QuickStore` to accept `expiresAt *time.Time`**

In `internal/longmemeval/engram.go`, replace the `QuickStore` method (starting at the `func (r *RestClient) QuickStore` line, currently around line 376):

```go
// QuickStore stores a single memory via POST /quick-store and returns its ID.
// When expiresAt is non-nil, the server stamps project_ttl so the project can
// be swept later by lme prune. Retries on 429 and 5xx with exponential backoff.
func (r *RestClient) QuickStore(ctx context.Context, project, content string, tags []string, expiresAt *time.Time) (string, error) {
	body := map[string]any{
		"content": content,
		"project": project,
		"tags":    tags,
	}
	if expiresAt != nil {
		body["expires_at"] = expiresAt.UTC().Format(time.RFC3339)
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal QuickStore body: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<min(attempt-1, 4)) * time.Second
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/quick-store", bytes.NewReader(data))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+r.token)

		resp, err := r.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		var result struct {
			OK    bool   `json:"ok"`
			ID    string `json:"id"`
			Error string `json:"error"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&result)
		_ = resp.Body.Close()
		if decodeErr != nil {
			lastErr = fmt.Errorf("quick-store decode: %w", decodeErr)
			continue
		}
		if resp.StatusCode == 429 {
			lastErr = fmt.Errorf("quick-store rate limited (status 429)")
			continue
		}
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("quick-store server error (status %d): %s", resp.StatusCode, result.Error)
			continue
		}
		if !result.OK || result.ID == "" {
			return "", fmt.Errorf("quick-store failed: %s (status %d)", result.Error, resp.StatusCode)
		}
		return result.ID, nil
	}
	return "", lastErr
}
```

- [ ] **Step 2: Run the new tests — verify they pass**

```
cd /home/psimmons/projects/engram-go
go test ./internal/longmemeval/ -run 'TestRestClient_QuickStore' -v -count=1 2>&1
```

Expected: all five `TestRestClient_QuickStore_*` tests pass.

- [ ] **Step 3: Run full longmemeval package**

```
cd /home/psimmons/projects/engram-go
go test ./internal/longmemeval/ -count=1 -race 2>&1
```

Expected: `ok internal/longmemeval ...` with no failures.

- [ ] **Step 4: Commit**

```bash
git add internal/longmemeval/engram.go internal/longmemeval/engram_test.go
git commit -m "feat(lme/rest): add expiresAt param to RestClient.QuickStore (#837)"
```

---

### Task 5: Update LME ingest — remove ttlStamper, wire `expiresAt`

**Files:**
- Modify: `cmd/longmemeval/ingest.go`
- Modify: `cmd/longmemeval/main.go`

- [ ] **Step 1: Rewrite `ingest.go` — remove `ttlStamper`, compute `expiresAt` in `ingestOne`**

Replace the entire contents of `cmd/longmemeval/ingest.go` with:

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

func runIngest(cfg *Config) {
	items := loadItems(cfg.DataFile)
	ckptPath := filepath.Join(cfg.OutDir, "checkpoint-ingest.jsonl")

	skip, err := longmemeval.ReadSkipSet(ckptPath)
	if err != nil {
		log.Fatalf("read ingest checkpoint: %v", err)
	}
	log.Printf("ingest: %d items loaded, %d already done", len(items), len(skip))

	ckptCh := make(chan longmemeval.IngestEntry, cfg.Workers*2)
	var wgWriter sync.WaitGroup
	wgWriter.Add(1)
	go func() {
		defer wgWriter.Done()
		longmemeval.WriteCheckpoint(ckptPath, ckptCh)
	}()

	work := make(chan longmemeval.Item, len(items))
	for _, item := range items {
		if !skip[item.QuestionID] {
			work <- item
		}
	}
	close(work)

	var wg sync.WaitGroup
	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ingestWorker(cfg, work, ckptCh)
		}()
	}
	wg.Wait()
	close(ckptCh)
	wgWriter.Wait()

	log.Printf("ingest: complete")
}

func ingestWorker(cfg *Config, work <-chan longmemeval.Item, out chan<- longmemeval.IngestEntry) {
	ctx := context.Background()
	restClient := longmemeval.NewRestClient(cfg.ServerURL, cfg.APIKey)
	for item := range work {
		entry := ingestOne(ctx, cfg, restClient, item)
		out <- entry
		log.Printf("ingest [%s] project=%s sessions=%d status=%s error=%q", item.QuestionID, entry.Project, entry.SessionCount, entry.Status, entry.Error)
	}
}

func ingestOne(ctx context.Context, cfg *Config, restClient *longmemeval.RestClient, item longmemeval.Item) (entry longmemeval.IngestEntry) {
	defer func() {
		if r := recover(); r != nil {
			entry = longmemeval.IngestEntry{
				QuestionID: item.QuestionID,
				Status:     "error",
				Error:      fmt.Sprintf("panic: %v", r),
			}
		}
	}()

	project := projectName(cfg.RunID, item.QuestionID)

	// #837: compute expiresAt once per question. Passed to every QuickStore call;
	// SetProjectTTL is idempotent (ON CONFLICT DO UPDATE) so repeated upserts are safe.
	var expiresAt *time.Time
	if cfg.ScratchTTL > 0 {
		t := time.Now().UTC().Add(cfg.ScratchTTL)
		expiresAt = &t
	}

	// Collect non-empty sessions with their IDs.
	type sessionEntry struct {
		sessionID string
		item      longmemeval.BatchItem
	}
	var sessions []sessionEntry
	for i, session := range item.HaystackSessions {
		if i >= len(item.HaystackSessionIDs) {
			break
		}
		sessionID := item.HaystackSessionIDs[i]
		content := longmemeval.SessionContent(session)
		if strings.TrimSpace(content) == "" {
			continue
		}
		tags := []string{"lme", "sid:" + sessionID}
		if i < len(item.HaystackDates) && item.HaystackDates[i] != "" {
			content = "Session date: " + item.HaystackDates[i] + "\n" + content
			tags = append(tags, "date:"+item.HaystackDates[i])
		}
		sessions = append(sessions, sessionEntry{
			sessionID: sessionID,
			item:      longmemeval.BatchItem{Content: content, Tags: tags},
		})
	}

	memoryMap := make(map[string]string, len(sessions))
	for i, s := range sessions {
		id, err := restClient.QuickStore(ctx, project, s.item.Content, s.item.Tags, expiresAt)
		if err != nil {
			return longmemeval.IngestEntry{
				QuestionID: item.QuestionID,
				Project:    project,
				Status:     "error",
				Error:      fmt.Sprintf("quick-store offset %d: %v", i, err),
			}
		}
		memoryMap[id] = s.sessionID
	}

	return longmemeval.IngestEntry{
		QuestionID:   item.QuestionID,
		Project:      project,
		SessionCount: len(memoryMap),
		MemoryMap:    memoryMap,
		Status:       "done",
	}
}

// loadItems parses the LongMemEval JSON file.
func loadItems(path string) []longmemeval.Item {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("read data file %q: %v", path, err)
	}
	var items []longmemeval.Item
	if err := json.Unmarshal(data, &items); err != nil {
		log.Fatalf("parse data file: %v", err)
	}
	if len(items) == 0 {
		log.Fatal("data file is empty")
	}
	return items
}
```

- [ ] **Step 2: Remove `DatabaseURL` from `Config` and `--database-url` flag from `main.go`**

In `cmd/longmemeval/main.go`:

Remove these two lines from the `Config` struct:
```go
// DatabaseURL is consulted by ingest to write project_ttl rows. Falls back
// to env DATABASE_URL.
DatabaseURL string
```

Remove this flag registration line (around line 178):
```go
fs.StringVar(&cfg.DatabaseURL, "database-url", envOr("DATABASE_URL", ""), "PostgreSQL DSN; required when --scratch-ttl > 0 so ingest can write project_ttl rows")
```

- [ ] **Step 3: Verify the build compiles cleanly**

```
cd /home/psimmons/projects/engram-go
go build ./cmd/longmemeval/ 2>&1
```

Expected: no output (clean build).

---

### Task 6: Update and add ingest tests

**Files:**
- Modify: `cmd/longmemeval/ingest_test.go`

- [ ] **Step 1: Add two new test functions verifying `expiresAt` propagation**

Append to `cmd/longmemeval/ingest_test.go`:

```go
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
	expectedMin := before.Add(168 * time.Hour)
	expectedMax := after.Add(168 * time.Hour)
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
```

- [ ] **Step 2: Run all ingest tests — verify they pass**

```
cd /home/psimmons/projects/engram-go
go test ./cmd/longmemeval/ -run 'TestIngest' -v -count=1 2>&1
```

Expected: all `TestIngest*` tests pass, including the two new ones.

- [ ] **Step 3: Commit**

```bash
git add cmd/longmemeval/ingest.go cmd/longmemeval/main.go cmd/longmemeval/ingest_test.go
git commit -m "feat(lme/ingest): route scratch-ttl through /quick-store; remove --database-url (#837)"
```

---

### Task 7: Full suite verification and close issue

**Files:** None modified.

- [ ] **Step 1: Run the full test suite**

```
cd /home/psimmons/projects/engram-go
go test ./... -count=1 -race 2>&1 | tail -30
```

Expected: all packages pass. No new failures.

- [ ] **Step 2: Verify `--database-url` is gone from help output**

```
cd /home/psimmons/projects/engram-go
go run ./cmd/longmemeval/ ingest --help 2>&1 | grep -i database
```

Expected: no output — the flag is gone.

- [ ] **Step 3: Verify `--scratch-ttl` still appears in help**

```
cd /home/psimmons/projects/engram-go
go run ./cmd/longmemeval/ ingest --help 2>&1 | grep scratch
```

Expected: a line describing `--scratch-ttl`.

- [ ] **Step 4: Open the PR**

```bash
git push origin main
gh pr create \
  --repo petersimmons1972/engram-go \
  --base main \
  --title "fix(lme): route --scratch-ttl through /quick-store REST body (#837)" \
  --label "ai-generated" \
  --body "$(cat <<'EOF'
## Summary

- Extends `POST /quick-store` body to accept optional `expires_at` (RFC3339)
- Server calls `SetProjectTTL` after a successful store when `expires_at` is present; failure is WARN-logged, not propagated
- `RestClient.QuickStore` gains `expiresAt *time.Time` final parameter; nil omits the field from the body
- `ingestOne` computes `now + ScratchTTL` and passes it to every `QuickStore` call when `ScratchTTL > 0`
- Removes `ttlStamper` interface, direct `db.NewPostgresBackend` block, `DatabaseURL` field, and `--database-url` flag entirely

Closes #837.

## Test plan

- [ ] `TestQuickStoreHandler_ExpiresAt_*` (4 tests) — server-side TTL stamping, past-timestamp rejection, absent field, error tolerance
- [ ] `TestRestClient_QuickStore_ExpiresAt_*` (2 tests) — body serialization with and without `expiresAt`
- [ ] `TestIngestOne_ScratchTTL_*` (2 tests) — `expires_at` present when TTL > 0, absent when TTL == 0
- [ ] Full suite `go test ./... -race` — no regressions

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 5: Close issue #837**

```bash
gh issue close 837 --repo petersimmons1972/engram-go --comment "Fixed in the PR opened in Task 7 Step 4. TTL is now routed through /quick-store REST body; --database-url flag removed."
```
