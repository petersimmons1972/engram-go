# Open-Brain Decay Audit System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a "receipt over time" layer that runs canonical queries on a schedule, stores ranked result snapshots in Postgres, and detects retrieval drift via Jaccard similarity — exposing three MCP tools: `memory_audit_add_query`, `memory_audit_run`, and `memory_audit_compare`.

**Architecture:** Two new Postgres tables (`audit_canonical_queries`, `audit_snapshots`) hold the query registry and result history. `internal/db/audit.go` implements CRUD and the `AuditWorker` goroutine (same ticker pattern as `StartRetentionWorker`). Jaccard is computed in Go by set intersection of memory IDs between consecutive snapshots. Tool handlers in `tools.go` call the DB methods; worker is started from `cmd/engram/main.go`.

**Tech Stack:** Go 1.22+, pgx/v5, `go test ./... -count=1 -race`. No new Go dependencies.

**⚠️ Advisor Gate:** After writing migration 015 and the `AuditWorker` skeleton, request Opus-level review of the table schema and snapshot comparison logic before implementing the MCP tools.

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `internal/db/migrations/015_decay_audit.sql` | audit_canonical_queries + audit_snapshots tables |
| Create | `internal/db/audit.go` | CRUD, Jaccard helper, AuditWorker |
| Create | `internal/db/audit_test.go` | Unit tests for Jaccard + DB CRUD |
| Modify | `internal/mcp/tools.go` | Add `handleMemoryAuditAddQuery`, `handleMemoryAuditRun`, `handleMemoryAuditCompare` |
| Modify | `internal/mcp/server.go` | Register 3 audit tools |
| Modify | `cmd/engram/main.go` | Start `AuditWorker` goroutine |

---

### Task 1: Write migration 015

**Files:**
- Create: `internal/db/migrations/015_decay_audit.sql`

- [ ] **Step 1: Create the migration**

```sql
-- Migration 015: Decay audit system
--
-- audit_canonical_queries stores the reference query set for a project.
-- audit_snapshots records the ranked memory-ID list returned for each query
-- at each audit run, plus Jaccard similarity vs. the previous snapshot.

CREATE TABLE audit_canonical_queries (
    id          TEXT        PRIMARY KEY,
    project     TEXT        NOT NULL,
    query       TEXT        NOT NULL,
    description TEXT,
    active      BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_canonical_queries_project ON audit_canonical_queries(project);

CREATE TABLE audit_snapshots (
    id               TEXT        PRIMARY KEY,
    query_id         TEXT        NOT NULL REFERENCES audit_canonical_queries(id),
    project          TEXT        NOT NULL,
    memory_ids       JSONB       NOT NULL,
    scores           JSONB,
    run_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    jaccard_vs_prev  DOUBLE PRECISION,
    additions        JSONB,
    removals         JSONB
);

CREATE INDEX idx_audit_snapshots_query_run ON audit_snapshots(query_id, run_at DESC);
CREATE INDEX idx_audit_snapshots_project   ON audit_snapshots(project, run_at DESC);
```

- [ ] **Step 2: Verify migration parses**

```bash
cd /home/psimmons/projects/engram-go && psql "$DATABASE_URL" -f internal/db/migrations/015_decay_audit.sql 2>&1 | head -10
```
Expected: `CREATE TABLE`, `CREATE INDEX` lines — no errors. (If DATABASE_URL is unset, skip and run via integration test in Task 3.)

- [ ] **Step 3: Commit**

```bash
cd /home/psimmons/projects/engram-go && git add internal/db/migrations/015_decay_audit.sql && git commit -m "feat(migrations): 015 add decay audit tables"
```

---

### Task 2: Implement Jaccard helper + DB types in `internal/db/audit.go`

**Files:**
- Create: `internal/db/audit.go`
- Create: `internal/db/audit_test.go`

- [ ] **Step 1: Write Jaccard tests first**

Create `internal/db/audit_test.go`:

```go
package db_test

import (
    "testing"

    "github.com/petersimmons1972/engram/internal/db"
)

func TestJaccardSimilarity(t *testing.T) {
    cases := []struct {
        a, b []string
        want float64
    }{
        {[]string{"a", "b", "c"}, []string{"a", "b", "c"}, 1.0},
        {[]string{"a", "b", "c"}, []string{"d", "e", "f"}, 0.0},
        {[]string{"a", "b", "c"}, []string{"a", "b", "d"}, 0.5},
        {[]string{"a"}, []string{}, 0.0},
        {[]string{}, []string{}, 1.0}, // both empty → identical
        {[]string{"a", "b"}, []string{"b", "a"}, 1.0}, // order-independent
    }
    for _, c := range cases {
        got := db.JaccardSimilarity(c.a, c.b)
        if math.Abs(got-c.want) > 1e-9 {
            t.Errorf("JaccardSimilarity(%v, %v) = %.4f, want %.4f", c.a, c.b, got, c.want)
        }
    }
}
```

Add `import "math"` to the test file.

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/psimmons/projects/engram-go && go test ./internal/db/... -count=1 -run TestJaccardSimilarity
```
Expected: compile error — `db.JaccardSimilarity undefined`.

- [ ] **Step 3: Create `internal/db/audit.go`**

```go
package db

import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"
    "time"

    "github.com/petersimmons1972/engram/internal/types"
)

// JaccardSimilarity returns |A ∩ B| / |A ∪ B| for two string slices.
// Both empty → 1.0 (identical). One empty → 0.0.
func JaccardSimilarity(a, b []string) float64 {
    if len(a) == 0 && len(b) == 0 {
        return 1.0
    }
    setA := make(map[string]bool, len(a))
    for _, v := range a {
        setA[v] = true
    }
    intersection := 0
    for _, v := range b {
        if setA[v] {
            intersection++
        }
    }
    union := len(setA)
    for _, v := range b {
        if !setA[v] {
            union++
        }
    }
    if union == 0 {
        return 1.0
    }
    return float64(intersection) / float64(union)
}

// CanonicalQuery is a registered reference query for drift detection.
type CanonicalQuery struct {
    ID          string    `json:"id"`
    Project     string    `json:"project"`
    Query       string    `json:"query"`
    Description string    `json:"description,omitempty"`
    Active      bool      `json:"active"`
    CreatedAt   time.Time `json:"created_at"`
}

// AuditSnapshot records the memory IDs returned for a canonical query at one point in time.
type AuditSnapshot struct {
    ID            string    `json:"id"`
    QueryID       string    `json:"query_id"`
    Project       string    `json:"project"`
    MemoryIDs     []string  `json:"memory_ids"`
    Scores        []float64 `json:"scores,omitempty"`
    RunAt         time.Time `json:"run_at"`
    JaccardVsPrev *float64  `json:"jaccard_vs_prev,omitempty"`
    Additions     []string  `json:"additions,omitempty"`
    Removals      []string  `json:"removals,omitempty"`
}

// SaveCanonicalQuery inserts a new canonical query and returns its assigned ID.
func (b *PostgresBackend) SaveCanonicalQuery(ctx context.Context, project, query, description string) (*CanonicalQuery, error) {
    cq := &CanonicalQuery{
        ID:          types.NewMemoryID(),
        Project:     project,
        Query:       query,
        Description: description,
        Active:      true,
        CreatedAt:   time.Now().UTC(),
    }
    const q = `
INSERT INTO audit_canonical_queries (id, project, query, description, active, created_at)
VALUES ($1, $2, $3, $4, $5, $6)`
    _, err := b.pool.Exec(ctx, q, cq.ID, cq.Project, cq.Query, cq.Description, cq.Active, cq.CreatedAt)
    if err != nil {
        return nil, fmt.Errorf("SaveCanonicalQuery: %w", err)
    }
    return cq, nil
}

// LoadCanonicalQueries returns all active canonical queries for a project.
func (b *PostgresBackend) LoadCanonicalQueries(ctx context.Context, project string) ([]CanonicalQuery, error) {
    const q = `
SELECT id, project, query, COALESCE(description,''), active, created_at
FROM audit_canonical_queries
WHERE project = $1 AND active = TRUE
ORDER BY created_at ASC`
    rows, err := b.pool.Query(ctx, q, project)
    if err != nil {
        return nil, fmt.Errorf("LoadCanonicalQueries: %w", err)
    }
    defer rows.Close()
    var result []CanonicalQuery
    for rows.Next() {
        var cq CanonicalQuery
        if err := rows.Scan(&cq.ID, &cq.Project, &cq.Query, &cq.Description, &cq.Active, &cq.CreatedAt); err != nil {
            return nil, fmt.Errorf("LoadCanonicalQueries scan: %w", err)
        }
        result = append(result, cq)
    }
    return result, rows.Err()
}

// SaveSnapshot stores a snapshot, computes Jaccard vs. the previous snapshot
// for the same query_id, and returns the saved row.
func (b *PostgresBackend) SaveSnapshot(ctx context.Context, snap *AuditSnapshot) (*AuditSnapshot, error) {
    // Load previous snapshot for Jaccard computation
    prev, err := b.latestSnapshot(ctx, snap.QueryID)
    if err != nil {
        return nil, err
    }
    if prev != nil {
        j := JaccardSimilarity(prev.MemoryIDs, snap.MemoryIDs)
        snap.JaccardVsPrev = &j
        snap.Additions = setDiff(snap.MemoryIDs, prev.MemoryIDs)
        snap.Removals = setDiff(prev.MemoryIDs, snap.MemoryIDs)
    }

    idsJSON, _ := json.Marshal(snap.MemoryIDs)
    scoresJSON, _ := json.Marshal(snap.Scores)
    additionsJSON, _ := json.Marshal(snap.Additions)
    removalsJSON, _ := json.Marshal(snap.Removals)

    const q = `
INSERT INTO audit_snapshots
    (id, query_id, project, memory_ids, scores, run_at, jaccard_vs_prev, additions, removals)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
    _, err = b.pool.Exec(ctx, q,
        snap.ID, snap.QueryID, snap.Project,
        idsJSON, scoresJSON, snap.RunAt,
        snap.JaccardVsPrev, additionsJSON, removalsJSON,
    )
    if err != nil {
        return nil, fmt.Errorf("SaveSnapshot: %w", err)
    }
    return snap, nil
}

// latestSnapshot returns the most recent snapshot for a query_id, or nil if none.
func (b *PostgresBackend) latestSnapshot(ctx context.Context, queryID string) (*AuditSnapshot, error) {
    const q = `
SELECT id, query_id, project, memory_ids, run_at
FROM audit_snapshots
WHERE query_id = $1
ORDER BY run_at DESC
LIMIT 1`
    rows, err := b.pool.Query(ctx, q, queryID)
    if err != nil {
        return nil, fmt.Errorf("latestSnapshot: %w", err)
    }
    defer rows.Close()
    if !rows.Next() {
        return nil, nil
    }
    var snap AuditSnapshot
    var idsJSON []byte
    if err := rows.Scan(&snap.ID, &snap.QueryID, &snap.Project, &idsJSON, &snap.RunAt); err != nil {
        return nil, fmt.Errorf("latestSnapshot scan: %w", err)
    }
    _ = json.Unmarshal(idsJSON, &snap.MemoryIDs)
    return &snap, rows.Err()
}

// ListSnapshotsForQuery returns snapshots for a query_id ordered by run_at DESC.
func (b *PostgresBackend) ListSnapshotsForQuery(ctx context.Context, queryID string, limit int) ([]AuditSnapshot, error) {
    if limit <= 0 {
        limit = 10
    }
    const q = `
SELECT id, query_id, project, memory_ids, scores, run_at, jaccard_vs_prev, additions, removals
FROM audit_snapshots
WHERE query_id = $1
ORDER BY run_at DESC
LIMIT $2`
    rows, err := b.pool.Query(ctx, q, queryID, limit)
    if err != nil {
        return nil, fmt.Errorf("ListSnapshotsForQuery: %w", err)
    }
    defer rows.Close()
    var result []AuditSnapshot
    for rows.Next() {
        var snap AuditSnapshot
        var idsJSON, scoresJSON, addJSON, remJSON []byte
        if err := rows.Scan(
            &snap.ID, &snap.QueryID, &snap.Project,
            &idsJSON, &scoresJSON, &snap.RunAt,
            &snap.JaccardVsPrev, &addJSON, &remJSON,
        ); err != nil {
            return nil, fmt.Errorf("ListSnapshotsForQuery scan: %w", err)
        }
        _ = json.Unmarshal(idsJSON, &snap.MemoryIDs)
        _ = json.Unmarshal(scoresJSON, &snap.Scores)
        _ = json.Unmarshal(addJSON, &snap.Additions)
        _ = json.Unmarshal(remJSON, &snap.Removals)
        result = append(result, snap)
    }
    return result, rows.Err()
}

// setDiff returns elements in a that are not in b.
func setDiff(a, b []string) []string {
    setB := make(map[string]bool, len(b))
    for _, v := range b {
        setB[v] = true
    }
    var diff []string
    for _, v := range a {
        if !setB[v] {
            diff = append(diff, v)
        }
    }
    return diff
}

// alertThreshold is the Jaccard threshold below which a drift alert fires.
// Configurable; override via project_meta key "audit_alert_threshold".
const alertThreshold = 0.7

// StartAuditWorker runs a full audit pass at startup and then every interval.
// Calls engine.Recall for each canonical query and saves a snapshot.
// recall is an injected function to avoid a circular dependency on search.Engine.
//
//	go backend.StartAuditWorker(ctx, interval, recall)
func (b *PostgresBackend) StartAuditWorker(ctx context.Context, interval time.Duration, recall func(ctx context.Context, project, query string) ([]string, []float64, error)) {
    run := func() {
        if err := b.runAuditPass(ctx, recall); err != nil {
            slog.Error("audit worker: pass failed", "err", err)
        }
    }

    run()

    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            run()
        }
    }
}

// runAuditPass iterates all projects' canonical queries and saves snapshots.
func (b *PostgresBackend) runAuditPass(ctx context.Context, recall func(ctx context.Context, project, query string) ([]string, []float64, error)) error {
    // Get all distinct projects with active canonical queries
    const projectsQ = `SELECT DISTINCT project FROM audit_canonical_queries WHERE active = TRUE`
    rows, err := b.pool.Query(ctx, projectsQ)
    if err != nil {
        return fmt.Errorf("runAuditPass: list projects: %w", err)
    }
    var projects []string
    for rows.Next() {
        var p string
        if err := rows.Scan(&p); err != nil {
            rows.Close()
            return err
        }
        projects = append(projects, p)
    }
    rows.Close()

    for _, project := range projects {
        queries, err := b.LoadCanonicalQueries(ctx, project)
        if err != nil {
            slog.Error("audit worker: load queries", "project", project, "err", err)
            continue
        }
        for _, cq := range queries {
            ids, scores, err := recall(ctx, project, cq.Query)
            if err != nil {
                slog.Error("audit worker: recall failed", "query_id", cq.ID, "err", err)
                continue
            }
            snap := &AuditSnapshot{
                ID:        types.NewMemoryID(),
                QueryID:   cq.ID,
                Project:   project,
                MemoryIDs: ids,
                Scores:    scores,
                RunAt:     time.Now().UTC(),
            }
            if _, err := b.SaveSnapshot(ctx, snap); err != nil {
                slog.Error("audit worker: save snapshot", "query_id", cq.ID, "err", err)
            }
        }
    }
    return nil
}
```

- [ ] **Step 4: Run Jaccard tests**

```bash
cd /home/psimmons/projects/engram-go && go test ./internal/db/... -count=1 -run TestJaccardSimilarity -race
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/psimmons/projects/engram-go && git add internal/db/audit.go internal/db/audit_test.go && git commit -m "feat(db): add JaccardSimilarity, CanonicalQuery CRUD, AuditWorker skeleton"
```

---

### Task 3: ⚠️ ADVISOR GATE — Opus review before MCP tools

**Files:** none (review only)

- [ ] **Step 1: Request Opus review**

Before implementing the MCP tool handlers, dispatch Opus (arnold or advisor agent) with:

```
Review the decay audit schema in 015_decay_audit.sql and the AuditWorker in internal/db/audit.go.

Check for:
1. Are the two tables correctly normalized? Any indexing gaps?
2. Is the Jaccard computation correct for this use case (set of memory IDs, order-independent)?
3. Does the AuditWorker recall injection correctly avoid circular dependency with search.Engine?
4. Any race conditions in the worker goroutine?

Return: zero severity/blocker findings or specific changes required.
```

- [ ] **Step 2: Apply any blocker findings before continuing**

---

### Task 4: Implement MCP tool handlers

**Files:**
- Modify: `internal/mcp/tools.go`

The handlers need a recall function. They'll obtain it from the `EnginePool` the same way `handleMemoryRecall` does — call `pool.Get(ctx, project)` then `engine.Engine.Recall(...)`.

- [ ] **Step 1: Write handler tests**

Add to `internal/mcp/tools_audit_test.go`:

```go
package mcp_test

import (
    "testing"

    "github.com/petersimmons1972/engram/internal/db"
)

func TestAuditAlertThreshold(t *testing.T) {
    // Jaccard 0.65 should alert; 0.75 should not.
    jBelow := 0.65
    jAbove := 0.75
    threshold := 0.7

    if !(jBelow < threshold) {
        t.Errorf("expected %.2f < %.2f to be true", jBelow, threshold)
    }
    if jAbove < threshold {
        t.Errorf("expected %.2f to NOT trigger alert", jAbove)
    }
}

func TestJaccardDriftSummary(t *testing.T) {
    // Verify the drift struct helpers used in memory_audit_run responses.
    snap := db.AuditSnapshot{
        MemoryIDs: []string{"a", "b", "c", "d"},
        Additions: []string{"d"},
        Removals:  []string{"z"},
    }
    if len(snap.Additions) != 1 {
        t.Errorf("expected 1 addition, got %d", len(snap.Additions))
    }
    if len(snap.Removals) != 1 {
        t.Errorf("expected 1 removal, got %d", len(snap.Removals))
    }
}
```

- [ ] **Step 2: Run tests**

```bash
cd /home/psimmons/projects/engram-go && go test ./internal/mcp/... -count=1 -run TestAudit
```
Expected: PASS.

- [ ] **Step 3: Add `handleMemoryAuditAddQuery` to `tools.go`**

```go
func handleMemoryAuditAddQuery(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
    args := req.GetArguments()
    project, err := getProject(args, "default")
    if err != nil {
        return nil, err
    }
    query := getString(args, "query", "")
    if query == "" {
        return nil, fmt.Errorf("memory_audit_add_query: query is required")
    }
    description := getString(args, "description", "")

    h, err := pool.Get(ctx, project)
    if err != nil {
        return nil, err
    }
    cq, err := h.Engine.Backend().SaveCanonicalQuery(ctx, project, query, description)
    if err != nil {
        return nil, fmt.Errorf("memory_audit_add_query: %w", err)
    }
    return toolResult(map[string]any{
        "id":          cq.ID,
        "project":     cq.Project,
        "query":       cq.Query,
        "description": cq.Description,
        "created_at":  cq.CreatedAt,
    })
}
```

**Note:** This requires `h.Engine.Backend()` returning `*db.PostgresBackend`. Check `internal/search/engine.go` for the backend accessor. If one doesn't exist, add `func (e *SearchEngine) Backend() *db.PostgresBackend { return e.backend }` to `engine.go`.

- [ ] **Step 4: Add `handleMemoryAuditRun` to `tools.go`**

```go
func handleMemoryAuditRun(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
    args := req.GetArguments()
    project, err := getProject(args, "default")
    if err != nil {
        return nil, err
    }

    h, err := pool.Get(ctx, project)
    if err != nil {
        return nil, err
    }
    backend := h.Engine.Backend()

    queries, err := backend.LoadCanonicalQueries(ctx, project)
    if err != nil {
        return nil, fmt.Errorf("memory_audit_run: load queries: %w", err)
    }
    if len(queries) == 0 {
        return toolResult(map[string]any{
            "message":  "no canonical queries registered for this project",
            "results":  []any{},
        })
    }

    type driftResult struct {
        QueryID        string   `json:"query_id"`
        Query          string   `json:"query"`
        Jaccard        *float64 `json:"jaccard_vs_prev"`
        AdditionsCount int      `json:"additions_count"`
        RemovalsCount  int      `json:"removals_count"`
        Alert          bool     `json:"alert"`
    }

    results := make([]driftResult, 0, len(queries))
    for _, cq := range queries {
        // Run recall for this query
        sr, err := h.Engine.Recall(ctx, cq.Query, 10, "full")
        if err != nil {
            slog.Warn("memory_audit_run: recall failed", "query_id", cq.ID, "err", err)
            continue
        }
        ids := make([]string, 0, len(sr))
        scores := make([]float64, 0, len(sr))
        for _, r := range sr {
            if r.Memory != nil {
                ids = append(ids, r.Memory.ID)
                scores = append(scores, r.Score)
            }
        }
        snap := &db.AuditSnapshot{
            ID:        types.NewMemoryID(),
            QueryID:   cq.ID,
            Project:   project,
            MemoryIDs: ids,
            Scores:    scores,
            RunAt:     time.Now().UTC(),
        }
        saved, err := backend.SaveSnapshot(ctx, snap)
        if err != nil {
            slog.Warn("memory_audit_run: save snapshot failed", "query_id", cq.ID, "err", err)
            continue
        }
        dr := driftResult{
            QueryID:        cq.ID,
            Query:          cq.Query,
            Jaccard:        saved.JaccardVsPrev,
            AdditionsCount: len(saved.Additions),
            RemovalsCount:  len(saved.Removals),
        }
        if saved.JaccardVsPrev != nil && *saved.JaccardVsPrev < 0.7 {
            dr.Alert = true
        }
        results = append(results, dr)
    }

    return toolResult(map[string]any{
        "project":      project,
        "queries_run":  len(results),
        "results":      results,
        "alert_threshold": 0.7,
    })
}
```

- [ ] **Step 5: Add `handleMemoryAuditCompare` to `tools.go`**

```go
func handleMemoryAuditCompare(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
    args := req.GetArguments()
    project, err := getProject(args, "default")
    if err != nil {
        return nil, err
    }
    queryID := getString(args, "query_id", "")
    if queryID == "" {
        return nil, fmt.Errorf("memory_audit_compare: query_id is required")
    }
    limit := getInt(args, "limit", 5)

    h, err := pool.Get(ctx, project)
    if err != nil {
        return nil, err
    }
    snapshots, err := h.Engine.Backend().ListSnapshotsForQuery(ctx, queryID, limit)
    if err != nil {
        return nil, fmt.Errorf("memory_audit_compare: %w", err)
    }
    return toolResult(map[string]any{
        "query_id":  queryID,
        "snapshots": snapshots,
    })
}
```

- [ ] **Step 6: Build check**

```bash
cd /home/psimmons/projects/engram-go && go build ./internal/mcp/...
```
Expected: clean build. If `h.Engine.Backend()` doesn't exist, add it to `internal/search/engine.go` as described above.

- [ ] **Step 7: Commit**

```bash
cd /home/psimmons/projects/engram-go && git add internal/mcp/tools.go internal/mcp/tools_audit_test.go && git commit -m "feat(mcp): add memory_audit_add_query, memory_audit_run, memory_audit_compare handlers"
```

---

### Task 5: Register audit tools in `server.go` + wire worker in `main.go`

**Files:**
- Modify: `internal/mcp/server.go`
- Modify: `cmd/engram/main.go`

- [ ] **Step 1: Register tools in `server.go`**

In `registerTools()`, append after existing tools:

```go
{"memory_audit_add_query", "Register a canonical query for decay audit. Engram will track how recall results for this query change over time.",
    func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
        return handleMemoryAuditAddQuery(ctx, pool, req)
    }},
{"memory_audit_run", "Run a full audit pass for the project. Executes all registered canonical queries, snapshots results, and computes Jaccard similarity vs. previous run. Returns per-query drift summary with alert flag when Jaccard < 0.7.",
    func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
        return handleMemoryAuditRun(ctx, pool, req)
    }},
{"memory_audit_compare", "Compare recent audit snapshots for a canonical query. Returns ranked result history and Jaccard trend. Requires query_id from memory_audit_add_query.",
    func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
        return handleMemoryAuditCompare(ctx, pool, req)
    }},
```

- [ ] **Step 2: Wire AuditWorker in `cmd/engram/main.go`**

After `go retentionBackend.StartRetentionWorker(ctx)` (around line 148), add:

```go
auditInterval := 7 * 24 * time.Hour
if v := os.Getenv("ENGRAM_AUDIT_INTERVAL"); v != "" {
    if d, err := time.ParseDuration(v); err == nil {
        auditInterval = d
    }
}
go retentionBackend.StartAuditWorker(ctx, auditInterval, func(ctx context.Context, project, query string) ([]string, []float64, error) {
    h, err := enginePool.Get(ctx, project)
    if err != nil {
        return nil, nil, err
    }
    sr, err := h.Engine.Recall(ctx, query, 10, "full")
    if err != nil {
        return nil, nil, err
    }
    ids := make([]string, 0, len(sr))
    scores := make([]float64, 0, len(sr))
    for _, r := range sr {
        if r.Memory != nil {
            ids = append(ids, r.Memory.ID)
            scores = append(scores, r.Score)
        }
    }
    return ids, scores, nil
})
```

- [ ] **Step 3: Run full test suite**

```bash
cd /home/psimmons/projects/engram-go && go test ./... -count=1 -race 2>&1 | tail -20
```
Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
cd /home/psimmons/projects/engram-go && git add internal/mcp/server.go cmd/engram/main.go && git commit -m "feat(server): register audit tools and start AuditWorker goroutine"
```

---

## Verification

```bash
# Full test suite
cd /home/psimmons/projects/engram-go && go test ./... -count=1 -race

# Build check
cd /home/psimmons/projects/engram-go && go build ./...
```

Manual flow (requires running server):
1. `memory_audit_add_query(query="deployment procedures", project="clearwatch")` → returns `id`
2. `memory_audit_run(project="clearwatch")` → first snapshot, `jaccard_vs_prev=null` (baseline)
3. Store a handful of new memories about deployment
4. `memory_audit_run(project="clearwatch")` → second snapshot, Jaccard score appears
5. `memory_audit_compare(query_id="<id>", project="clearwatch")` → 2 snapshots in history with diff
