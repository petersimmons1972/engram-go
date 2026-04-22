# LongMemEval Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a `longmemeval` CLI that ingests the LongMemEval-M dataset into per-question Engram projects, runs retrieval + `claude --print` generation, judges answers with `claude --print`, and outputs both LongMemEval-compatible JSONL and a self-contained score report.

**Architecture:** Three resumable pipeline stages (`ingest → run → score`) each backed by an append-safe JSONL checkpoint file. N parallel workers per stage, each operating on an isolated Engram project (`lme-<run-id>-q<NNN>`). A prerequisite `memory_delete_project` MCP tool is added to engram-go before the harness is built.

**Tech Stack:** Go 1.25, `mcp-go` SSE client (existing pattern from `cmd/eval/main.go`), `claude --print` subprocess, `encoding/json`, `sync.WaitGroup` + channels for worker pools.

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `internal/db/backend.go` | Modify | Add `DeleteProject` to `Backend` interface |
| `internal/db/postgres_memory.go` | Modify | Implement `DeleteProject` |
| `internal/mcp/tools.go` | Modify | Add `handleMemoryDeleteProject` |
| `internal/mcp/server.go` | Modify | Register `memory_delete_project` tool |
| `internal/mcp/tools_delete_project_test.go` | Create | Tests for `handleMemoryDeleteProject` |
| `internal/longmemeval/types.go` | Create | All shared structs |
| `internal/longmemeval/metrics.go` | Create | `RecallAny`, `RecallAll` + NDCG wrappers |
| `internal/longmemeval/metrics_test.go` | Create | Metric unit tests |
| `internal/longmemeval/checkpoint.go` | Create | Append-safe JSONL writer goroutine + reader |
| `internal/longmemeval/checkpoint_test.go` | Create | Checkpoint round-trip tests |
| `internal/longmemeval/claude.go` | Create | `claude --print` subprocess wrapper |
| `internal/longmemeval/claude_test.go` | Create | Subprocess wrapper tests |
| `internal/longmemeval/engram.go` | Create | MCP client wrappers (store, recall, fetch, delete) |
| `cmd/longmemeval/main.go` | Create | CLI entry, flag parsing, subcommand dispatch |
| `cmd/longmemeval/ingest.go` | Create | `ingest` subcommand |
| `cmd/longmemeval/run.go` | Create | `run` subcommand |
| `cmd/longmemeval/score.go` | Create | `score` subcommand + output file writing |
| `cmd/longmemeval/all.go` | Create | `all` subcommand (chains stages) |

---

## Task 1: Add `DeleteProject` to the DB Backend interface

**Files:**
- Modify: `internal/db/backend.go` (after line 57, inside the `Backend` interface)

- [ ] **Step 1: Add method signature to the interface**

In `internal/db/backend.go`, insert after the `DeleteMemoryAtomic` declaration (line ~57):

```go
// DeleteProject hard-deletes all memories and associated data for a project
// in a single transaction. Returns the number of memories deleted.
// Used by the eval harness to clean up per-question isolation projects.
DeleteProject(ctx context.Context, project string) (int64, error)
```

- [ ] **Step 2: Run the build to verify the interface is unsatisfied**

```bash
cd ~/projects/engram-go && go build ./...
```

Expected: compile error mentioning `DeleteProject` not implemented on `*PostgresBackend`.

---

## Task 2: Implement `DeleteProject` in Postgres

**Files:**
- Modify: `internal/db/postgres_memory.go` (add after `DeleteMemoryAtomic`, around line 263)

- [ ] **Step 1: Write the failing test**

Create `internal/db/postgres_memory_delete_project_test.go`:

```go
package db_test

// NOTE: This is an integration test requiring a running Postgres instance.
// Run with: go test ./internal/db/... -run TestDeleteProject -v
// The test is skipped automatically when ENGRAM_TEST_DSN is unset.

import (
	"context"
	"os"
	"testing"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/types"
)

func TestDeleteProject(t *testing.T) {
	dsn := os.Getenv("ENGRAM_TEST_DSN")
	if dsn == "" {
		t.Skip("ENGRAM_TEST_DSN not set")
	}
	ctx := context.Background()
	project := "test-delete-project-" + t.Name()

	b, err := db.NewPostgresBackend(ctx, project, dsn)
	if err != nil {
		t.Fatalf("NewPostgresBackend: %v", err)
	}
	defer b.Close()

	// Store two memories.
	m1 := &types.Memory{Project: project, Content: "hello world", Tags: []string{"a"}}
	m2 := &types.Memory{Project: project, Content: "second memory", Tags: []string{"b"}}
	if err := b.StoreMemory(ctx, m1); err != nil {
		t.Fatalf("StoreMemory m1: %v", err)
	}
	if err := b.StoreMemory(ctx, m2); err != nil {
		t.Fatalf("StoreMemory m2: %v", err)
	}

	// Delete the project.
	n, err := b.DeleteProject(ctx, project)
	if err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	if n != 2 {
		t.Errorf("DeleteProject returned %d, want 2", n)
	}

	// Verify memories are gone.
	got, err := b.GetMemory(ctx, m1.ID)
	if err != nil {
		t.Fatalf("GetMemory after delete: %v", err)
	}
	if got != nil {
		t.Errorf("memory %s still exists after DeleteProject", m1.ID)
	}
}

func TestDeleteProject_Empty(t *testing.T) {
	dsn := os.Getenv("ENGRAM_TEST_DSN")
	if dsn == "" {
		t.Skip("ENGRAM_TEST_DSN not set")
	}
	ctx := context.Background()
	b, err := db.NewPostgresBackend(ctx, "test-dp-empty", dsn)
	if err != nil {
		t.Fatalf("NewPostgresBackend: %v", err)
	}
	defer b.Close()

	n, err := b.DeleteProject(ctx, "nonexistent-project-xyz")
	if err != nil {
		t.Fatalf("DeleteProject on empty project: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 deletions, got %d", n)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd ~/projects/engram-go && go test ./internal/db/... -run TestDeleteProject -v
```

Expected: compile error — `DeleteProject` not yet implemented on `*PostgresBackend`.

- [ ] **Step 3: Implement `DeleteProject` in `postgres_memory.go`**

Add after `DeleteMemoryAtomic` (around line 263):

```go
// DeleteProject removes all memories and associated data for a project in
// a single atomic transaction. Cleans: chunks, relationships, memories,
// project_meta, weight_config, canonical_entities, entity_extraction_jobs,
// and episodes. Returns the number of memories deleted.
func (b *PostgresBackend) DeleteProject(ctx context.Context, project string) (int64, error) {
	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Clean dependent tables first.
	depTables := []string{
		"DELETE FROM chunks WHERE memory_id IN (SELECT id FROM memories WHERE project=$1)",
		"DELETE FROM relationships WHERE source_id IN (SELECT id FROM memories WHERE project=$1) OR target_id IN (SELECT id FROM memories WHERE project=$1)",
	}
	for _, q := range depTables {
		if _, err := tx.Exec(ctx, q, project); err != nil {
			return 0, fmt.Errorf("DeleteProject cleanup: %w", err)
		}
	}

	// Delete memories, capture count.
	tag, err := tx.Exec(ctx, "DELETE FROM memories WHERE project=$1", project)
	if err != nil {
		return 0, err
	}
	deleted := tag.RowsAffected()

	// Clean project-level tables.
	projectTables := []string{
		"DELETE FROM project_meta WHERE project=$1",
		"DELETE FROM weight_config WHERE project=$1",
		"DELETE FROM canonical_entities WHERE project=$1",
		"DELETE FROM entity_extraction_jobs WHERE project=$1",
		"DELETE FROM episodes WHERE project=$1",
	}
	for _, q := range projectTables {
		if _, err := tx.Exec(ctx, q, project); err != nil {
			return 0, fmt.Errorf("DeleteProject cleanup: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return deleted, nil
}
```

- [ ] **Step 4: Run tests**

```bash
cd ~/projects/engram-go && go test ./internal/db/... -run TestDeleteProject -v
```

Expected: SKIP (no `ENGRAM_TEST_DSN`). Build must succeed with no compile errors.

```bash
cd ~/projects/engram-go && go build ./...
```

Expected: clean build.

- [ ] **Step 5: Commit**

```bash
cd ~/projects/engram-go && git add internal/db/backend.go internal/db/postgres_memory.go internal/db/postgres_memory_delete_project_test.go && git commit -m "feat: add DeleteProject to DB backend — bulk project cleanup for eval harness"
```

---

## Task 3: Add `memory_delete_project` MCP tool

**Files:**
- Modify: `internal/mcp/tools.go` (add handler after `handleMemoryForget`)
- Modify: `internal/mcp/server.go` (register tool after `memory_forget` entry)
- Create: `internal/mcp/tools_delete_project_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/mcp/tools_delete_project_test.go`:

```go
package mcp

import (
	"context"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestHandleMemoryDeleteProject_MissingProject(t *testing.T) {
	pool := newTestPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, err := handleMemoryDeleteProject(context.Background(), pool, req)
	require.NoError(t, err)
	require.True(t, result.IsError, "expected error result for missing project")
}

func TestHandleMemoryDeleteProject_Empty(t *testing.T) {
	pool := newTestPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{"project": "nonexistent-lme-project-xyz"}
	result, err := handleMemoryDeleteProject(context.Background(), pool, req)
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Contains(t, resultText(t, result), `"deleted"`)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd ~/projects/engram-go && go test ./internal/mcp/... -run TestHandleMemoryDeleteProject -v
```

Expected: compile error — `handleMemoryDeleteProject` undefined.

- [ ] **Step 3: Add `handleMemoryDeleteProject` to `tools.go`**

Add after `handleMemoryForget` (around line 1007 in `internal/mcp/tools.go`):

```go
// handleMemoryDeleteProject bulk-deletes all memories and associated data for
// a project. Intended for eval harness cleanup — not for normal user use.
func handleMemoryDeleteProject(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	project, err := getProject(args, "")
	if err != nil {
		return nil, err
	}
	if project == "" {
		return mcpgo.NewToolResultError("project is required"), nil
	}
	h, err := pool.Get(ctx, project)
	if err != nil {
		return nil, err
	}
	deleted, err := h.Engine.DB().DeleteProject(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("delete project %q: %w", project, err)
	}
	return toolResult(map[string]any{"project": project, "deleted": deleted})
}
```

**Note:** If `h.Engine.DB()` is not the right accessor, check `internal/engine/` for how the DB backend is exposed. It may be `h.DB.DeleteProject(ctx, project)` or similar — find the pattern used in `handleMemoryMigrateEmbedder` (line ~1367) where it accesses the DB directly.

- [ ] **Step 4: Register in `server.go`**

In `internal/mcp/server.go`, add after the `memory_forget` entry (around line 482):

```go
{"memory_delete_project", "Hard-delete all memories and project data for an eval isolation project. Not for normal use.",
    func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
        return handleMemoryDeleteProject(ctx, pool, req)
    }},
```

- [ ] **Step 5: Run tests**

```bash
cd ~/projects/engram-go && go test ./internal/mcp/... -run TestHandleMemoryDeleteProject -v && go build ./...
```

Expected: tests PASS, build clean.

- [ ] **Step 6: Commit**

```bash
cd ~/projects/engram-go && git add internal/mcp/tools.go internal/mcp/server.go internal/mcp/tools_delete_project_test.go && git commit -m "feat: add memory_delete_project MCP tool — bulk eval project cleanup"
```

---

## Task 4: `internal/longmemeval/types.go`

**Files:**
- Create: `internal/longmemeval/types.go`

- [ ] **Step 1: Create the file**

```go
// Package longmemeval implements the LongMemEval benchmark harness for engram-go.
package longmemeval

// Item is one entry from the LongMemEval dataset JSON file.
type Item struct {
	QuestionID       string      `json:"question_id"`
	QuestionType     string      `json:"question_type"`
	Question         string      `json:"question"`
	Answer           string      `json:"answer"`
	QuestionDate     string      `json:"question_date"`
	HaystackSessionIDs []string  `json:"haystack_session_ids"`
	HaystackDates    []string    `json:"haystack_dates"`
	HaystackSessions [][]Turn    `json:"haystack_sessions"`
	AnswerSessionIDs []string    `json:"answer_session_ids"`
}

// Turn is one exchange within a haystack session.
type Turn struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	HasAnswer bool   `json:"has_answer,omitempty"`
}

// IngestEntry is one line written to checkpoint-ingest.jsonl.
type IngestEntry struct {
	QuestionID   string            `json:"question_id"`
	Project      string            `json:"project"`
	SessionCount int               `json:"session_count"`
	MemoryMap    map[string]string `json:"memory_map"` // memory_id → session_id
	Status       string            `json:"status"`     // "done" | "error"
	Error        string            `json:"error,omitempty"`
}

// RunEntry is one line written to checkpoint-run.jsonl.
type RunEntry struct {
	QuestionID   string   `json:"question_id"`
	Hypothesis   string   `json:"hypothesis"`
	RetrievedIDs []string `json:"retrieved_ids"` // memory IDs in ranked order
	Status       string   `json:"status"`
	Error        string   `json:"error,omitempty"`
}

// ScoreEntry is one line written to checkpoint-score.jsonl.
type ScoreEntry struct {
	QuestionID  string `json:"question_id"`
	QuestionType string `json:"question_type"`
	Hypothesis  string `json:"hypothesis"`
	ScoreLabel  string `json:"score_label"` // CORRECT | PARTIALLY_CORRECT | INCORRECT
	Explanation string `json:"explanation"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
}

// HypothesisLine is one line in the LongMemEval-compatible hypotheses.jsonl output.
type HypothesisLine struct {
	QuestionID string `json:"question_id"`
	Hypothesis string `json:"hypothesis"`
}

// RetrievalMetrics holds session-level retrieval metrics for one question.
type RetrievalMetrics struct {
	RecallAll5  float64 `json:"recall_all@5"`
	NDCGAny5    float64 `json:"ndcg_any@5"`
	RecallAll10 float64 `json:"recall_all@10"`
	NDCGAny10   float64 `json:"ndcg_any@10"`
}

// RetrievalLogEntry is one line in retrieval_log.jsonl (LongMemEval-compatible).
type RetrievalLogEntry struct {
	QuestionID string `json:"question_id"`
	RetrievalResults struct {
		Metrics struct {
			Session RetrievalMetrics `json:"session"`
		} `json:"metrics"`
	} `json:"retrieval_results"`
}
```

- [ ] **Step 2: Build to confirm no errors**

```bash
cd ~/projects/engram-go && go build ./internal/longmemeval/...
```

Expected: PASS (empty package builds clean).

- [ ] **Step 3: Commit**

```bash
cd ~/projects/engram-go && git add internal/longmemeval/types.go && git commit -m "feat(longmemeval): types.go — LongMemEval data structures"
```

---

## Task 5: `internal/longmemeval/metrics.go`

**Files:**
- Create: `internal/longmemeval/metrics.go`
- Create: `internal/longmemeval/metrics_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/longmemeval/metrics_test.go`:

```go
package longmemeval_test

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

func TestRecallAny(t *testing.T) {
	retrieved := []string{"sid-a", "sid-b", "sid-c"}
	relevant := map[string]bool{"sid-b": true, "sid-z": true}

	// sid-b is in top-3 → recall_any@3 = 1
	if got := longmemeval.RecallAny(retrieved, relevant, 3); got != 1.0 {
		t.Errorf("RecallAny@3 = %.2f, want 1.0", got)
	}
	// sid-b is in top-2 → recall_any@2 = 1
	if got := longmemeval.RecallAny(retrieved, relevant, 2); got != 1.0 {
		t.Errorf("RecallAny@2 = %.2f, want 1.0", got)
	}
	// sid-b is NOT in top-1 → recall_any@1 = 0
	if got := longmemeval.RecallAny(retrieved, relevant, 1); got != 0.0 {
		t.Errorf("RecallAny@1 = %.2f, want 0.0", got)
	}
}

func TestRecallAll(t *testing.T) {
	retrieved := []string{"sid-a", "sid-b", "sid-c", "sid-d"}
	relevant := map[string]bool{"sid-b": true, "sid-c": true}

	// Both present in top-4 → recall_all@4 = 1
	if got := longmemeval.RecallAll(retrieved, relevant, 4); got != 1.0 {
		t.Errorf("RecallAll@4 = %.2f, want 1.0", got)
	}
	// Only sid-b in top-2, sid-c missing → recall_all@2 = 0
	if got := longmemeval.RecallAll(retrieved, relevant, 2); got != 0.0 {
		t.Errorf("RecallAll@2 = %.2f, want 0.0", got)
	}
}

func TestRecallAny_Empty(t *testing.T) {
	if got := longmemeval.RecallAny(nil, map[string]bool{"x": true}, 5); got != 0.0 {
		t.Errorf("RecallAny with nil retrieved = %.2f, want 0.0", got)
	}
	if got := longmemeval.RecallAny([]string{"a"}, nil, 5); got != 0.0 {
		t.Errorf("RecallAny with nil relevant = %.2f, want 0.0", got)
	}
}

func TestSessionIDs(t *testing.T) {
	memoryMap := map[string]string{
		"mem-1": "sid-a",
		"mem-2": "sid-b",
		"mem-3": "sid-c",
	}
	retrieved := []string{"mem-2", "mem-3", "mem-1"}
	want := []string{"sid-b", "sid-c", "sid-a"}
	got := longmemeval.SessionIDs(retrieved, memoryMap)
	for i, g := range got {
		if g != want[i] {
			t.Errorf("SessionIDs[%d] = %q, want %q", i, g, want[i])
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd ~/projects/engram-go && go test ./internal/longmemeval/... -run TestRecall -v
```

Expected: compile error — `RecallAny`, `RecallAll`, `SessionIDs` undefined.

- [ ] **Step 3: Implement**

Create `internal/longmemeval/metrics.go`:

```go
package longmemeval

import "github.com/petersimmons1972/engram/internal/eval"

// SessionIDs maps a ranked list of memory IDs to session IDs using the
// memory_id → session_id map built during ingestion. IDs not found in the
// map are omitted.
func SessionIDs(memoryIDs []string, memoryMap map[string]string) []string {
	out := make([]string, 0, len(memoryIDs))
	for _, mid := range memoryIDs {
		if sid, ok := memoryMap[mid]; ok {
			out = append(out, sid)
		}
	}
	return out
}

// RecallAny returns 1.0 if at least one relevant session appears in the
// top-k retrieved sessions, 0.0 otherwise.
func RecallAny(retrieved []string, relevant map[string]bool, k int) float64 {
	if len(retrieved) == 0 || len(relevant) == 0 || k <= 0 {
		return 0
	}
	limit := k
	if limit > len(retrieved) {
		limit = len(retrieved)
	}
	for i := 0; i < limit; i++ {
		if relevant[retrieved[i]] {
			return 1.0
		}
	}
	return 0
}

// RecallAll returns 1.0 if all relevant sessions appear in the top-k
// retrieved sessions, 0.0 otherwise.
func RecallAll(retrieved []string, relevant map[string]bool, k int) float64 {
	if len(retrieved) == 0 || len(relevant) == 0 || k <= 0 {
		return 0
	}
	limit := k
	if limit > len(retrieved) {
		limit = len(retrieved)
	}
	inTopK := make(map[string]bool, limit)
	for i := 0; i < limit; i++ {
		inTopK[retrieved[i]] = true
	}
	for sid := range relevant {
		if !inTopK[sid] {
			return 0
		}
	}
	return 1.0
}

// NDCGAny wraps internal/eval.NDCG treating any relevant session as binary
// relevant (gain=1).
func NDCGAny(retrieved []string, relevant map[string]bool, k int) float64 {
	return eval.NDCG(retrieved, relevant, k)
}

// BuildRetrievalMetrics computes the four session-level metrics LongMemEval's
// print_retrieval_metrics.py expects, given ranked session IDs and the set of
// evidence session IDs for this question.
func BuildRetrievalMetrics(rankedSessionIDs []string, answerSessionIDs []string) RetrievalMetrics {
	relevant := make(map[string]bool, len(answerSessionIDs))
	for _, sid := range answerSessionIDs {
		relevant[sid] = true
	}
	return RetrievalMetrics{
		RecallAll5:  RecallAll(rankedSessionIDs, relevant, 5),
		NDCGAny5:    NDCGAny(rankedSessionIDs, relevant, 5),
		RecallAll10: RecallAll(rankedSessionIDs, relevant, 10),
		NDCGAny10:   NDCGAny(rankedSessionIDs, relevant, 10),
	}
}
```

- [ ] **Step 4: Run tests**

```bash
cd ~/projects/engram-go && go test ./internal/longmemeval/... -run TestRecall -run TestSessionIDs -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
cd ~/projects/engram-go && git add internal/longmemeval/metrics.go internal/longmemeval/metrics_test.go && git commit -m "feat(longmemeval): metrics.go — RecallAny, RecallAll, NDCGAny, BuildRetrievalMetrics"
```

---

## Task 6: `internal/longmemeval/checkpoint.go`

**Files:**
- Create: `internal/longmemeval/checkpoint.go`
- Create: `internal/longmemeval/checkpoint_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/longmemeval/checkpoint_test.go`:

```go
package longmemeval_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

func TestCheckpointRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	ch := make(chan longmemeval.IngestEntry, 4)
	done := make(chan struct{})
	go func() {
		defer close(done)
		longmemeval.WriteCheckpoint(path, ch)
	}()

	ch <- longmemeval.IngestEntry{QuestionID: "q001", Status: "done", SessionCount: 3}
	ch <- longmemeval.IngestEntry{QuestionID: "q002", Status: "error", Error: "timeout"}
	close(ch)
	<-done

	skip, err := longmemeval.ReadSkipSet(path)
	if err != nil {
		t.Fatalf("ReadSkipSet: %v", err)
	}
	if !skip["q001"] {
		t.Error("q001 (done) should be in skip set")
	}
	if skip["q002"] {
		t.Error("q002 (error) should NOT be in skip set")
	}
}

func TestReadSkipSet_Missing(t *testing.T) {
	skip, err := longmemeval.ReadSkipSet("/tmp/nonexistent-ckpt-xyz.jsonl")
	if err != nil {
		t.Fatalf("ReadSkipSet on missing file: %v", err)
	}
	if len(skip) != 0 {
		t.Errorf("expected empty skip set, got %d entries", len(skip))
	}
}

func TestReadAllIngest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ingest.jsonl")

	ch := make(chan longmemeval.IngestEntry, 2)
	done := make(chan struct{})
	go func() {
		defer close(done)
		longmemeval.WriteCheckpoint(path, ch)
	}()
	ch <- longmemeval.IngestEntry{QuestionID: "q001", Status: "done", Project: "lme-x-q001", MemoryMap: map[string]string{"m1": "s1"}}
	close(ch)
	<-done

	entries, err := longmemeval.ReadAllIngest(path)
	if err != nil {
		t.Fatalf("ReadAllIngest: %v", err)
	}
	if len(entries) != 1 || entries[0].Project != "lme-x-q001" {
		t.Errorf("unexpected entries: %+v", entries)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd ~/projects/engram-go && go test ./internal/longmemeval/... -run TestCheckpoint -run TestReadSkipSet -run TestReadAllIngest -v
```

Expected: compile error — `WriteCheckpoint`, `ReadSkipSet`, `ReadAllIngest` undefined.

- [ ] **Step 3: Implement**

Create `internal/longmemeval/checkpoint.go`:

```go
package longmemeval

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
)

// WriteCheckpoint reads entries from ch and appends each as a JSON line to
// path. Runs until ch is closed. Designed to run in a dedicated goroutine.
// Generic over entry type using any — caller supplies the concrete type.
func WriteCheckpoint[T any](path string, ch <-chan T) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		// Non-recoverable — log and drain to unblock callers.
		for range ch {
		}
		return
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for entry := range ch {
		_ = enc.Encode(entry) // best-effort; individual write failures are non-fatal
	}
}

// ReadSkipSet reads a checkpoint file and returns a set of question IDs
// whose status == "done". Returns an empty set (not an error) if the file
// does not exist.
func ReadSkipSet(path string) (map[string]bool, error) {
	skip := make(map[string]bool)
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return skip, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20) // 1 MB per line
	for scanner.Scan() {
		var entry struct {
			QuestionID string `json:"question_id"`
			Status     string `json:"status"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue // skip malformed lines
		}
		if entry.Status == "done" {
			skip[entry.QuestionID] = true
		}
	}
	return skip, scanner.Err()
}

// ReadAllIngest reads all entries from a checkpoint-ingest.jsonl file.
// Used by the run stage to retrieve the project + memory_map for each question.
func ReadAllIngest(path string) ([]IngestEntry, error) {
	return readAll[IngestEntry](path)
}

// ReadAllRun reads all entries from a checkpoint-run.jsonl file.
// Used by the score stage to retrieve hypotheses.
func ReadAllRun(path string) ([]RunEntry, error) {
	return readAll[RunEntry](path)
}

func readAll[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []T
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		var entry T
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		out = append(out, entry)
	}
	return out, scanner.Err()
}
```

- [ ] **Step 4: Run tests**

```bash
cd ~/projects/engram-go && go test ./internal/longmemeval/... -run TestCheckpoint -run TestReadSkipSet -run TestReadAllIngest -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
cd ~/projects/engram-go && git add internal/longmemeval/checkpoint.go internal/longmemeval/checkpoint_test.go && git commit -m "feat(longmemeval): checkpoint.go — append-safe JSONL writer + reader"
```

---

## Task 7: `internal/longmemeval/claude.go`

**Files:**
- Create: `internal/longmemeval/claude.go`
- Create: `internal/longmemeval/claude_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/longmemeval/claude_test.go`:

```go
package longmemeval_test

import (
	"context"
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

func TestParseScoreLabel_Valid(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"CORRECT\nBecause X.", "CORRECT"},
		{"PARTIALLY_CORRECT\nSome details missing.", "PARTIALLY_CORRECT"},
		{"INCORRECT\nWrong answer.", "INCORRECT"},
		{"  correct  \nexplanation here", "CORRECT"},
	}
	for _, c := range cases {
		got, _ := longmemeval.ParseScoreLabel(c.input)
		if got != c.want {
			t.Errorf("ParseScoreLabel(%q) label = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestParseScoreLabel_Invalid(t *testing.T) {
	label, _ := longmemeval.ParseScoreLabel("I'm not sure about this one.")
	if label != "PARTIALLY_CORRECT" {
		t.Errorf("unrecognised label default = %q, want PARTIALLY_CORRECT", label)
	}
}

func TestParseScoreLabel_Explanation(t *testing.T) {
	_, explanation := longmemeval.ParseScoreLabel("CORRECT\nThe answer matches the reference exactly.")
	if !strings.Contains(explanation, "matches") {
		t.Errorf("explanation = %q, want it to contain 'matches'", explanation)
	}
}

// TestGenerate_RequiresClaude is skipped unless LONGMEMEVAL_TEST_CLAUDE=1.
// It verifies the subprocess wrapper returns non-empty output for a trivial prompt.
func TestGenerate_RequiresClaude(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	ctx := context.Background()
	out, err := longmemeval.Generate(ctx, "Reply with only the word: HELLO", 1)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if out == "" {
		t.Error("Generate returned empty output")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd ~/projects/engram-go && go test ./internal/longmemeval/... -run TestParseScoreLabel -v
```

Expected: compile error.

- [ ] **Step 3: Implement**

Create `internal/longmemeval/claude.go`:

```go
package longmemeval

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const generateTimeout = 90 * time.Second

// Generate calls `claude --print prompt` and returns trimmed stdout.
// retries is the number of additional attempts on non-zero exit or timeout (0 = try once).
func Generate(ctx context.Context, prompt string, retries int) (string, error) {
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		out, err := runClaude(ctx, prompt)
		if err == nil {
			return out, nil
		}
		lastErr = err
	}
	return "", lastErr
}

func runClaude(ctx context.Context, prompt string) (string, error) {
	tctx, cancel := context.WithTimeout(ctx, generateTimeout)
	defer cancel()
	cmd := exec.CommandContext(tctx, "claude", "--print", prompt)
	raw, err := cmd.Output()
	if err != nil {
		if tctx.Err() != nil {
			return "", fmt.Errorf("claude --print timed out after %s", generateTimeout)
		}
		return "", fmt.Errorf("claude --print: %w", err)
	}
	return strings.TrimSpace(string(raw)), nil
}

// GenerationPrompt builds the prompt sent to claude --print for answer generation.
func GenerationPrompt(question, questionDate string, contextBlocks []string) string {
	ctx := strings.Join(contextBlocks, "\n\n---\n\n")
	return fmt.Sprintf(`You are answering questions about a person's conversation history.

Relevant memory context:
%s

Question (asked on %s): %s

Answer the question based only on the provided context. If the answer cannot be determined from the context, respond with exactly: I don't know.`, ctx, questionDate, question)
}

// ScoringPrompt builds the judge prompt for answer scoring.
func ScoringPrompt(question, referenceAnswer, hypothesis string) string {
	return fmt.Sprintf(`You are judging whether a generated answer correctly answers a question about conversation history.

Question: %s

Reference answer: %s

Generated answer: %s

Is the generated answer correct? Reply with exactly one of these labels on the first line:
CORRECT
PARTIALLY_CORRECT
INCORRECT

Then on the second line, briefly explain why (one sentence).`, question, referenceAnswer, hypothesis)
}

// ScoreResult holds the parsed output of the judge prompt.
type ScoreResult struct {
	Label       string
	Explanation string
}

// Score calls claude --print with the judge prompt and parses the result.
func Score(ctx context.Context, question, referenceAnswer, hypothesis string, retries int) (ScoreResult, error) {
	prompt := ScoringPrompt(question, referenceAnswer, hypothesis)
	out, err := Generate(ctx, prompt, retries)
	if err != nil {
		return ScoreResult{Label: "PARTIALLY_CORRECT"}, err
	}
	label, explanation := ParseScoreLabel(out)
	return ScoreResult{Label: label, Explanation: explanation}, nil
}

// ParseScoreLabel extracts the label and explanation from raw judge output.
// Returns PARTIALLY_CORRECT as default if the label is unrecognised.
func ParseScoreLabel(raw string) (label, explanation string) {
	lines := strings.SplitN(strings.TrimSpace(raw), "\n", 2)
	first := strings.ToUpper(strings.TrimSpace(lines[0]))
	switch first {
	case "CORRECT", "PARTIALLY_CORRECT", "INCORRECT":
		label = first
	default:
		label = "PARTIALLY_CORRECT"
	}
	if len(lines) > 1 {
		explanation = strings.TrimSpace(lines[1])
	}
	return label, explanation
}
```

- [ ] **Step 4: Run tests**

```bash
cd ~/projects/engram-go && go test ./internal/longmemeval/... -run TestParseScoreLabel -v
```

Expected: all PASS. (Skip `TestGenerate_RequiresClaude` — it's guarded by `testing.Short()`.)

- [ ] **Step 5: Commit**

```bash
cd ~/projects/engram-go && git add internal/longmemeval/claude.go internal/longmemeval/claude_test.go && git commit -m "feat(longmemeval): claude.go — subprocess wrapper, prompts, ParseScoreLabel"
```

---

## Task 8: `internal/longmemeval/engram.go`

**Files:**
- Create: `internal/longmemeval/engram.go`

- [ ] **Step 1: Create the file**

This is the MCP client layer. Tests require a live Engram server so they are integration tests skipped without `ENGRAM_URL` set. The unit-testable logic lives in other packages.

```go
package longmemeval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// Client wraps the MCP SSE client with retry logic for eval use.
type Client struct {
	mcp    *client.Client
	retries int
}

// Connect creates an authenticated MCP SSE client connected to serverURL.
// apiKey may be empty.
func Connect(ctx context.Context, serverURL, apiKey string) (*Client, error) {
	sseURL := strings.TrimRight(serverURL, "/") + "/sse"
	headers := map[string]string{}
	if apiKey != "" {
		headers["Authorization"] = "Bearer " + apiKey
	}
	c, err := client.NewSSEMCPClient(sseURL, transport.WithHeaders(headers))
	if err != nil {
		return nil, err
	}
	if err := c.Start(ctx); err != nil {
		return nil, err
	}
	_, err = c.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "longmemeval", Version: "1.0.0"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("initialize MCP: %w", err)
	}
	return &Client{mcp: c, retries: 1}, nil
}

// Store stores one session as a memory and returns the memory ID.
// tags should include "sid:<session_id>" for later mapping.
func (c *Client) Store(ctx context.Context, project, content string, tags []string) (string, error) {
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		id, err := c.store(ctx, project, content, tags)
		if err == nil {
			return id, nil
		}
		lastErr = err
		if attempt < c.retries {
			time.Sleep(5 * time.Second)
		}
	}
	return "", lastErr
}

func (c *Client) store(ctx context.Context, project, content string, tags []string) (string, error) {
	result, err := c.mcp.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "memory_store",
			Arguments: map[string]any{
				"content": content,
				"project": project,
				"tags":    tags,
			},
		},
	})
	if err != nil {
		return "", err
	}
	if result.IsError {
		return "", fmt.Errorf("memory_store tool error")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		return "", fmt.Errorf("unexpected content type")
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(tc.Text), &resp); err != nil {
		return "", fmt.Errorf("parse store response: %w", err)
	}
	return resp.ID, nil
}

// Recall calls memory_recall and returns ranked memory IDs.
func (c *Client) Recall(ctx context.Context, project, query string, topK int) ([]string, error) {
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		ids, err := c.recall(ctx, project, query, topK)
		if err == nil {
			return ids, nil
		}
		lastErr = err
		if attempt < c.retries {
			time.Sleep(5 * time.Second)
		}
	}
	return nil, lastErr
}

func (c *Client) recall(ctx context.Context, project, query string, topK int) ([]string, error) {
	result, err := c.mcp.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "memory_recall",
			Arguments: map[string]any{
				"query":   query,
				"project": project,
				"top_k":   topK,
				"detail":  "summary",
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return nil, fmt.Errorf("memory_recall tool error")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		return nil, fmt.Errorf("unexpected content type")
	}
	var resp struct {
		Handles []struct {
			ID string `json:"id"`
		} `json:"handles"`
	}
	if err := json.Unmarshal([]byte(tc.Text), &resp); err != nil {
		return nil, fmt.Errorf("parse recall response: %w", err)
	}
	ids := make([]string, 0, len(resp.Handles))
	for _, h := range resp.Handles {
		ids = append(ids, h.ID)
	}
	return ids, nil
}

// FetchContent fetches the full content of a memory by ID.
func (c *Client) FetchContent(ctx context.Context, id string) (string, error) {
	result, err := c.mcp.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "memory_fetch",
			Arguments: map[string]any{
				"id":     id,
				"detail": "full",
			},
		},
	})
	if err != nil {
		return "", err
	}
	if result.IsError {
		return "", fmt.Errorf("memory_fetch tool error")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		return "", fmt.Errorf("unexpected content type")
	}
	var resp struct {
		Memory struct {
			Content string `json:"content"`
		} `json:"memory"`
	}
	if err := json.Unmarshal([]byte(tc.Text), &resp); err != nil {
		return "", fmt.Errorf("parse fetch response: %w", err)
	}
	return resp.Memory.Content, nil
}

// DeleteProject calls memory_delete_project to clean up an isolation project.
func (c *Client) DeleteProject(ctx context.Context, project string) error {
	result, err := c.mcp.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "memory_delete_project",
			Arguments: map[string]any{"project": project},
		},
	})
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("memory_delete_project tool error for %q", project)
	}
	return nil
}

// SessionContent concatenates the user turns of a session into a single string.
// Matches LongMemEval's flat-index approach.
func SessionContent(turns []Turn) string {
	var sb strings.Builder
	for _, t := range turns {
		if t.Role == "user" {
			if sb.Len() > 0 {
				sb.WriteByte(' ')
			}
			sb.WriteString(t.Content)
		}
	}
	return sb.String()
}
```

- [ ] **Step 2: Build**

```bash
cd ~/projects/engram-go && go build ./internal/longmemeval/...
```

Expected: clean build.

- [ ] **Step 3: Commit**

```bash
cd ~/projects/engram-go && git add internal/longmemeval/engram.go && git commit -m "feat(longmemeval): engram.go — MCP client wrappers with retry"
```

---

## Task 9: `cmd/longmemeval/main.go`

**Files:**
- Create: `cmd/longmemeval/main.go`

- [ ] **Step 1: Create the file**

```go
// longmemeval runs the LongMemEval benchmark against a live engram-go MCP server.
// Usage: longmemeval <ingest|run|score|all> [flags]
package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
)

// Config holds flags shared across all subcommands.
type Config struct {
	DataFile    string
	Workers     int
	RunID       string
	ServerURL   string
	APIKey      string
	NoCleanup   bool
	Retries     int
	OutDir      string
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: longmemeval <ingest|run|score|all> [flags]")
		os.Exit(1)
	}
	subcommand := os.Args[1]

	fs := flag.NewFlagSet(subcommand, flag.ExitOnError)
	cfg := &Config{}
	fs.StringVar(&cfg.DataFile, "data", "", "Path to longmemeval_m_cleaned.json (required)")
	fs.IntVar(&cfg.Workers, "workers", 4, "Number of parallel workers")
	fs.StringVar(&cfg.RunID, "run-id", "", "Run ID (hex); auto-generated if empty")
	fs.StringVar(&cfg.ServerURL, "url", envOr("ENGRAM_URL", "http://localhost:8788"), "Engram server URL")
	fs.StringVar(&cfg.APIKey, "api-key", os.Getenv("ENGRAM_API_KEY"), "Engram API key")
	fs.BoolVar(&cfg.NoCleanup, "no-cleanup", false, "Skip Engram project deletion after run stage")
	fs.IntVar(&cfg.Retries, "retries", 1, "Retry count for claude --print and Engram calls")
	fs.StringVar(&cfg.OutDir, "out", ".", "Output directory for checkpoint and result files")

	if err := fs.Parse(os.Args[2:]); err != nil {
		log.Fatal(err)
	}

	if cfg.DataFile == "" && subcommand != "help" {
		log.Fatal("--data is required")
	}

	if cfg.RunID == "" {
		cfg.RunID = newRunID()
	}

	switch subcommand {
	case "ingest":
		runIngest(cfg)
	case "run":
		runRun(cfg)
	case "score":
		runScore(cfg)
	case "all":
		runAll(cfg)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", subcommand)
		os.Exit(1)
	}
}

func newRunID() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func projectName(runID, questionID string) string {
	return fmt.Sprintf("lme-%s-%s", runID, questionID)
}
```

- [ ] **Step 2: Build (will fail until other cmd files exist — just check for syntax)**

```bash
cd ~/projects/engram-go && go vet ./cmd/longmemeval/ 2>&1 | head -5
```

Expected: errors about undefined `runIngest`, `runRun`, `runScore`, `runAll` — correct, those come in next tasks.

- [ ] **Step 3: Commit**

```bash
cd ~/projects/engram-go && git add cmd/longmemeval/main.go && git commit -m "feat(longmemeval): cmd/main.go — subcommand dispatch + shared config"
```

---

## Task 10: `cmd/longmemeval/ingest.go`

**Files:**
- Create: `cmd/longmemeval/ingest.go`

- [ ] **Step 1: Create the file**

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
	mcpClient, err := longmemeval.Connect(ctx, cfg.ServerURL, cfg.APIKey)
	if err != nil {
		log.Printf("WARN ingest worker: connect failed: %v", err)
		for item := range work {
			out <- longmemeval.IngestEntry{QuestionID: item.QuestionID, Status: "error", Error: err.Error()}
		}
		return
	}

	for item := range work {
		entry := ingestOne(ctx, cfg, mcpClient, item)
		out <- entry
		log.Printf("ingest [%s] project=%s sessions=%d status=%s",
			item.QuestionID, entry.Project, entry.SessionCount, entry.Status)
	}
}

func ingestOne(ctx context.Context, cfg *Config, mcpClient *longmemeval.Client, item longmemeval.Item) (entry longmemeval.IngestEntry) {
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
	memoryMap := make(map[string]string, len(item.HaystackSessions))

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
		memID, err := mcpClient.Store(ctx, project, content, tags)
		if err != nil {
			return longmemeval.IngestEntry{
				QuestionID: item.QuestionID,
				Project:    project,
				Status:     "error",
				Error:      fmt.Sprintf("store session %s: %v", sessionID, err),
			}
		}
		memoryMap[memID] = sessionID
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

- [ ] **Step 2: Build**

```bash
cd ~/projects/engram-go && go build ./cmd/longmemeval/ 2>&1 | head -20
```

Expected: errors only about `runRun`, `runScore`, `runAll` being undefined. No errors in ingest.go itself.

- [ ] **Step 3: Commit**

```bash
cd ~/projects/engram-go && git add cmd/longmemeval/ingest.go && git commit -m "feat(longmemeval): ingest.go — parallel session ingestion with checkpoint resume"
```

---

## Task 11: `cmd/longmemeval/run.go`

**Files:**
- Create: `cmd/longmemeval/run.go`

- [ ] **Step 1: Create the file**

```go
package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"sync"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

const recallTopK = 50
const contextTopK = 10

func runRun(cfg *Config) {
	items := loadItems(cfg.DataFile)
	itemMap := make(map[string]longmemeval.Item, len(items))
	for _, item := range items {
		itemMap[item.QuestionID] = item
	}

	ingestEntries, err := longmemeval.ReadAllIngest(filepath.Join(cfg.OutDir, "checkpoint-ingest.jsonl"))
	if err != nil {
		log.Fatalf("read ingest checkpoint: %v", err)
	}
	ingestMap := make(map[string]longmemeval.IngestEntry, len(ingestEntries))
	for _, e := range ingestEntries {
		if e.Status == "done" {
			ingestMap[e.QuestionID] = e
		}
	}

	ckptPath := filepath.Join(cfg.OutDir, "checkpoint-run.jsonl")
	skip, err := longmemeval.ReadSkipSet(ckptPath)
	if err != nil {
		log.Fatalf("read run checkpoint: %v", err)
	}
	log.Printf("run: %d ingest entries loaded, %d already done", len(ingestMap), len(skip))

	ckptCh := make(chan longmemeval.RunEntry, cfg.Workers*2)
	var wgWriter sync.WaitGroup
	wgWriter.Add(1)
	go func() {
		defer wgWriter.Done()
		longmemeval.WriteCheckpoint(ckptPath, ckptCh)
	}()

	work := make(chan longmemeval.IngestEntry, len(ingestEntries))
	for _, e := range ingestEntries {
		if e.Status == "done" && !skip[e.QuestionID] {
			work <- e
		}
	}
	close(work)

	var wg sync.WaitGroup
	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runWorker(cfg, itemMap, work, ckptCh)
		}()
	}
	wg.Wait()
	close(ckptCh)
	wgWriter.Wait()

	log.Printf("run: complete")
}

func runWorker(cfg *Config, itemMap map[string]longmemeval.Item, work <-chan longmemeval.IngestEntry, out chan<- longmemeval.RunEntry) {
	ctx := context.Background()
	mcpClient, err := longmemeval.Connect(ctx, cfg.ServerURL, cfg.APIKey)
	if err != nil {
		log.Printf("WARN run worker: connect failed: %v", err)
		for e := range work {
			out <- longmemeval.RunEntry{QuestionID: e.QuestionID, Status: "error", Error: err.Error()}
		}
		return
	}

	for ingestEntry := range work {
		item, ok := itemMap[ingestEntry.QuestionID]
		if !ok {
			out <- longmemeval.RunEntry{QuestionID: ingestEntry.QuestionID, Status: "error", Error: "item not found in data file"}
			continue
		}
		entry := runOne(ctx, cfg, mcpClient, item, ingestEntry)
		out <- entry
		log.Printf("run [%s] status=%s hypothesis_len=%d", item.QuestionID, entry.Status, len(entry.Hypothesis))

		if !cfg.NoCleanup {
			if err := mcpClient.DeleteProject(ctx, ingestEntry.Project); err != nil {
				log.Printf("WARN run [%s] cleanup failed: %v", item.QuestionID, err)
			}
		}
	}
}

func runOne(ctx context.Context, cfg *Config, mcpClient *longmemeval.Client, item longmemeval.Item, ingest longmemeval.IngestEntry) (entry longmemeval.RunEntry) {
	defer func() {
		if r := recover(); r != nil {
			entry = longmemeval.RunEntry{
				QuestionID: item.QuestionID,
				Status:     "error",
				Error:      fmt.Sprintf("panic: %v", r),
			}
		}
	}()

	retrievedIDs, err := mcpClient.Recall(ctx, ingest.Project, item.Question, recallTopK)
	if err != nil {
		return longmemeval.RunEntry{
			QuestionID: item.QuestionID,
			Status:     "error",
			Error:      fmt.Sprintf("recall: %v", err),
		}
	}

	// Fetch content for top contextTopK memories.
	contextLimit := contextTopK
	if contextLimit > len(retrievedIDs) {
		contextLimit = len(retrievedIDs)
	}
	contextBlocks := make([]string, 0, contextLimit)
	for _, id := range retrievedIDs[:contextLimit] {
		content, err := mcpClient.FetchContent(ctx, id)
		if err != nil {
			log.Printf("WARN run [%s] fetch %s: %v", item.QuestionID, id, err)
			continue
		}
		if content != "" {
			contextBlocks = append(contextBlocks, content)
		}
	}

	prompt := longmemeval.GenerationPrompt(item.Question, item.QuestionDate, contextBlocks)
	hypothesis, err := longmemeval.Generate(ctx, prompt, cfg.Retries)
	if err != nil {
		return longmemeval.RunEntry{
			QuestionID:   item.QuestionID,
			RetrievedIDs: retrievedIDs,
			Status:       "error",
			Error:        fmt.Sprintf("generate: %v", err),
		}
	}

	// Abstention questions (_abs suffix) are excluded from retrieval scoring
	// in writeRetrievalLog, but still need a hypothesis for the QA output.

	return longmemeval.RunEntry{
		QuestionID:   item.QuestionID,
		Hypothesis:   hypothesis,
		RetrievedIDs: retrievedIDs,
		Status:       "done",
	}
}
```

- [ ] **Step 2: Build**

```bash
cd ~/projects/engram-go && go build ./cmd/longmemeval/ 2>&1 | head -20
```

Expected: only `runScore` and `runAll` undefined.

- [ ] **Step 3: Commit**

```bash
cd ~/projects/engram-go && git add cmd/longmemeval/run.go && git commit -m "feat(longmemeval): run.go — recall + claude generation with project cleanup"
```

---

## Task 12: `cmd/longmemeval/score.go`

**Files:**
- Create: `cmd/longmemeval/score.go`

- [ ] **Step 1: Create the file**

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

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

func runScore(cfg *Config) {
	items := loadItems(cfg.DataFile)
	itemMap := make(map[string]longmemeval.Item, len(items))
	for _, item := range items {
		itemMap[item.QuestionID] = item
	}

	ingestEntries, err := longmemeval.ReadAllIngest(filepath.Join(cfg.OutDir, "checkpoint-ingest.jsonl"))
	if err != nil {
		log.Fatalf("read ingest checkpoint: %v", err)
	}
	ingestMap := make(map[string]longmemeval.IngestEntry, len(ingestEntries))
	for _, e := range ingestEntries {
		if e.Status == "done" {
			ingestMap[e.QuestionID] = e
		}
	}

	runEntries, err := longmemeval.ReadAllRun(filepath.Join(cfg.OutDir, "checkpoint-run.jsonl"))
	if err != nil {
		log.Fatalf("read run checkpoint: %v", err)
	}

	ckptPath := filepath.Join(cfg.OutDir, "checkpoint-score.jsonl")
	skip, err := longmemeval.ReadSkipSet(ckptPath)
	if err != nil {
		log.Fatalf("read score checkpoint: %v", err)
	}
	log.Printf("score: %d run entries loaded, %d already done", len(runEntries), len(skip))

	ckptCh := make(chan longmemeval.ScoreEntry, cfg.Workers*2)
	var wgWriter sync.WaitGroup
	wgWriter.Add(1)
	go func() {
		defer wgWriter.Done()
		longmemeval.WriteCheckpoint(ckptPath, ckptCh)
	}()

	work := make(chan longmemeval.RunEntry, len(runEntries))
	for _, e := range runEntries {
		if e.Status == "done" && !skip[e.QuestionID] {
			work <- e
		}
	}
	close(work)

	var wg sync.WaitGroup
	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			scoreWorker(cfg, itemMap, work, ckptCh)
		}()
	}
	wg.Wait()
	close(ckptCh)
	wgWriter.Wait()

	// Load all score entries and write output files.
	allScores, err := longmemeval.ReadAllScore(ckptPath)
	if err != nil {
		log.Fatalf("read final scores: %v", err)
	}
	writeOutputs(cfg, itemMap, ingestMap, allScores)
	log.Printf("score: complete")
}

func scoreWorker(cfg *Config, itemMap map[string]longmemeval.Item, work <-chan longmemeval.RunEntry, out chan<- longmemeval.ScoreEntry) {
	ctx := context.Background()
	for runEntry := range work {
		item, ok := itemMap[runEntry.QuestionID]
		if !ok {
			out <- longmemeval.ScoreEntry{QuestionID: runEntry.QuestionID, Status: "error", Error: "item not in data file"}
			continue
		}
		entry := scoreOne(ctx, cfg, item, runEntry)
		out <- entry
		log.Printf("score [%s] label=%s", runEntry.QuestionID, entry.ScoreLabel)
	}
}

func scoreOne(ctx context.Context, cfg *Config, item longmemeval.Item, run longmemeval.RunEntry) (entry longmemeval.ScoreEntry) {
	defer func() {
		if r := recover(); r != nil {
			entry = longmemeval.ScoreEntry{
				QuestionID: item.QuestionID,
				Status:     "error",
				Error:      fmt.Sprintf("panic: %v", r),
			}
		}
	}()

	result, err := longmemeval.Score(ctx, item.Question, item.Answer, run.Hypothesis, cfg.Retries)
	if err != nil {
		return longmemeval.ScoreEntry{
			QuestionID:   item.QuestionID,
			QuestionType: item.QuestionType,
			Hypothesis:   run.Hypothesis,
			ScoreLabel:   result.Label,
			Explanation:  result.Explanation,
			Status:       "error",
			Error:        err.Error(),
		}
	}
	return longmemeval.ScoreEntry{
		QuestionID:   item.QuestionID,
		QuestionType: item.QuestionType,
		Hypothesis:   run.Hypothesis,
		ScoreLabel:   result.Label,
		Explanation:  result.Explanation,
		Status:       "done",
	}
}

func writeOutputs(cfg *Config, itemMap map[string]longmemeval.Item, ingestMap map[string]longmemeval.IngestEntry, scores []longmemeval.ScoreEntry) {
	writeHypotheses(cfg, scores)
	writeRetrievalLog(cfg, itemMap, ingestMap, scores)
	writeScoreReport(cfg, scores)
}

func writeHypotheses(cfg *Config, scores []longmemeval.ScoreEntry) {
	f, err := os.Create(filepath.Join(cfg.OutDir, "hypotheses.jsonl"))
	if err != nil {
		log.Printf("WARN write hypotheses.jsonl: %v", err)
		return
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, s := range scores {
		_ = enc.Encode(longmemeval.HypothesisLine{QuestionID: s.QuestionID, Hypothesis: s.Hypothesis})
	}
	log.Printf("wrote %s", filepath.Join(cfg.OutDir, "hypotheses.jsonl"))
}

func writeRetrievalLog(cfg *Config, itemMap map[string]longmemeval.Item, ingestMap map[string]longmemeval.IngestEntry, scores []longmemeval.ScoreEntry) {
	// Build a run-entry map from the run checkpoint for retrieved IDs.
	runEntries, _ := longmemeval.ReadAllRun(filepath.Join(cfg.OutDir, "checkpoint-run.jsonl"))
	runMap := make(map[string]longmemeval.RunEntry, len(runEntries))
	for _, r := range runEntries {
		runMap[r.QuestionID] = r
	}

	f, err := os.Create(filepath.Join(cfg.OutDir, "retrieval_log.jsonl"))
	if err != nil {
		log.Printf("WARN write retrieval_log.jsonl: %v", err)
		return
	}
	defer f.Close()
	enc := json.NewEncoder(f)

	for _, s := range scores {
		item, ok := itemMap[s.QuestionID]
		if !ok {
			continue
		}
		// Skip abstention questions per LongMemEval convention.
		if strings.Contains(s.QuestionID, "_abs") {
			continue
		}
		ingest, ok := ingestMap[s.QuestionID]
		if !ok {
			continue
		}
		run, ok := runMap[s.QuestionID]
		if !ok {
			continue
		}
		sessionIDs := longmemeval.SessionIDs(run.RetrievedIDs, ingest.MemoryMap)
		metrics := longmemeval.BuildRetrievalMetrics(sessionIDs, item.AnswerSessionIDs)

		var entry longmemeval.RetrievalLogEntry
		entry.QuestionID = s.QuestionID
		entry.RetrievalResults.Metrics.Session = metrics
		_ = enc.Encode(entry)
	}
	log.Printf("wrote %s", filepath.Join(cfg.OutDir, "retrieval_log.jsonl"))
}

func writeScoreReport(cfg *Config, scores []longmemeval.ScoreEntry) {
	type byType struct {
		Correct          int `json:"correct"`
		PartiallyCorrect int `json:"partially_correct"`
		Incorrect        int `json:"incorrect"`
		Total            int `json:"total"`
	}
	overall := &byType{}
	byQType := make(map[string]*byType)

	for _, s := range scores {
		if s.Status != "done" {
			continue
		}
		qbt := byQType[s.QuestionType]
		if qbt == nil {
			qbt = &byType{}
			byQType[s.QuestionType] = qbt
		}
		for _, bt := range []*byType{overall, qbt} {
			bt.Total++
			switch s.ScoreLabel {
			case "CORRECT":
				bt.Correct++
			case "PARTIALLY_CORRECT":
				bt.PartiallyCorrect++
			default:
				bt.Incorrect++
			}
		}
	}

	report := map[string]any{
		"overall":    overall,
		"by_type":    byQType,
		"run_id":     cfg.RunID,
		"total_scored": len(scores),
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		log.Printf("WARN marshal score report: %v", err)
		return
	}
	path := filepath.Join(cfg.OutDir, "score_report.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("WARN write score_report.json: %v", err)
		return
	}
	log.Printf("wrote %s", path)

	// Print summary.
	if overall.Total > 0 {
		pct := func(n int) float64 { return float64(n) / float64(overall.Total) * 100 }
		fmt.Printf("\n--- Score Report (run-id: %s) ---\n", cfg.RunID)
		fmt.Printf("Total scored:       %d\n", overall.Total)
		fmt.Printf("Correct:            %d (%.1f%%)\n", overall.Correct, pct(overall.Correct))
		fmt.Printf("Partially correct:  %d (%.1f%%)\n", overall.PartiallyCorrect, pct(overall.PartiallyCorrect))
		fmt.Printf("Incorrect:          %d (%.1f%%)\n", overall.Incorrect, pct(overall.Incorrect))
	}
}
```

- [ ] **Step 2: Add `ReadAllScore` to `checkpoint.go`**

In `internal/longmemeval/checkpoint.go`, add at the end:

```go
// ReadAllScore reads all entries from a checkpoint-score.jsonl file.
func ReadAllScore(path string) ([]ScoreEntry, error) {
	return readAll[ScoreEntry](path)
}
```

- [ ] **Step 3: Build**

```bash
cd ~/projects/engram-go && go build ./cmd/longmemeval/ 2>&1 | head -20
```

Expected: only `runAll` undefined.

- [ ] **Step 4: Commit**

```bash
cd ~/projects/engram-go && git add cmd/longmemeval/score.go internal/longmemeval/checkpoint.go && git commit -m "feat(longmemeval): score.go — claude judge + hypotheses/retrieval_log/score_report output"
```

---

## Task 13: `cmd/longmemeval/all.go`

**Files:**
- Create: `cmd/longmemeval/all.go`

- [ ] **Step 1: Create the file**

```go
package main

import "log"

func runAll(cfg *Config) {
	if cfg.RunID == "" {
		cfg.RunID = newRunID()
	}
	log.Printf("all: run-id=%s data=%s workers=%d", cfg.RunID, cfg.DataFile, cfg.Workers)

	log.Println("--- Stage 1: ingest ---")
	runIngest(cfg)

	log.Println("--- Stage 2: run ---")
	runRun(cfg)

	log.Println("--- Stage 3: score ---")
	runScore(cfg)

	log.Printf("all: complete (run-id=%s)", cfg.RunID)
}
```

- [ ] **Step 2: Build the complete binary**

```bash
cd ~/projects/engram-go && go build ./cmd/longmemeval/ && ls -lh longmemeval 2>/dev/null || ls -lh cmd/longmemeval/longmemeval 2>/dev/null
```

Actually confirm the binary was produced:

```bash
cd ~/projects/engram-go && go build -o /tmp/longmemeval ./cmd/longmemeval/ && echo "BUILD OK" && /tmp/longmemeval 2>&1 | head -3
```

Expected: `BUILD OK`, then usage line.

- [ ] **Step 3: Run full test suite**

```bash
cd ~/projects/engram-go && go test ./... -count=1 -race 2>&1 | tail -20
```

Expected: PASS (all existing tests green; new tests in internal/longmemeval pass; DB integration tests skip without ENGRAM_TEST_DSN).

- [ ] **Step 4: Commit**

```bash
cd ~/projects/engram-go && git add cmd/longmemeval/all.go && git commit -m "feat(longmemeval): all.go — chains ingest→run→score as single command"
```

---

## Task 14: Download dataset and smoke test

- [ ] **Step 1: Download `longmemeval_m_cleaned.json`**

```bash
mkdir -p ~/projects/engram-go/testdata/longmemeval
cd ~/projects/engram-go/testdata/longmemeval
wget "https://huggingface.co/datasets/xiaowu0162/longmemeval-cleaned/resolve/main/longmemeval_m_cleaned.json"
```

Verify:

```bash
wc -c ~/projects/engram-go/testdata/longmemeval/longmemeval_m_cleaned.json
python3 -c "import json; d=json.load(open('testdata/longmemeval/longmemeval_m_cleaned.json')); print(f'{len(d)} items, first id: {d[0][\"question_id\"]}')" 2>/dev/null || echo "python3 not available — verify manually"
```

Expected: file size > 1 MB; 500 items.

- [ ] **Step 2: Smoke test with one question (`--workers 1`, oracle-style)**

Test that the full pipeline runs end-to-end on the first question. Use `--no-cleanup` for debugging:

```bash
cd ~/projects/engram-go
mkdir -p /tmp/lme-smoke
# Extract just the first item for a fast smoke test.
python3 -c "import json; d=json.load(open('testdata/longmemeval/longmemeval_m_cleaned.json')); open('/tmp/lme-smoke/one.json','w').write(json.dumps([d[0]]))"
/tmp/longmemeval all \
  --data /tmp/lme-smoke/one.json \
  --workers 1 \
  --out /tmp/lme-smoke \
  --no-cleanup \
  2>&1 | tee /tmp/lme-smoke/run.log
```

Expected: log lines for ingest, run, score; `score_report.json` and `hypotheses.jsonl` written to `/tmp/lme-smoke/`.

```bash
cat /tmp/lme-smoke/score_report.json
cat /tmp/lme-smoke/hypotheses.jsonl
cat /tmp/lme-smoke/retrieval_log.jsonl
```

Expected: valid JSON in all three files.

- [ ] **Step 3: Commit smoke test artifacts to .gitignore**

Add to `~/projects/engram-go/.gitignore`:

```
testdata/longmemeval/*.json
```

```bash
cd ~/projects/engram-go && echo "testdata/longmemeval/*.json" >> .gitignore && git add .gitignore && git commit -m "chore: gitignore LongMemEval dataset files"
```

---

## Running the Full Benchmark

Once smoke test passes:

```bash
cd ~/projects/engram-go
mkdir -p results/longmemeval
/tmp/longmemeval all \
  --data testdata/longmemeval/longmemeval_m_cleaned.json \
  --workers 4 \
  --out results/longmemeval \
  2>&1 | tee results/longmemeval/run.log
```

To score retrieval with LongMemEval's Python tools (optional):

```bash
# In the LongMemEval repo:
python3 src/evaluation/print_retrieval_metrics.py results/longmemeval/retrieval_log.jsonl
python3 src/evaluation/evaluate_qa.py gpt-4o results/longmemeval/hypotheses.jsonl testdata/longmemeval/longmemeval_m_cleaned.json
```
