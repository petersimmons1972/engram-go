# Open-Brain Adaptive Weight Tuning Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the feedback loop: failure_class aggregates drive a background `WeightTuner` worker that adjusts the 4 search weights within guardrails, stores history for auditability, and makes scoring DB-driven so weights can evolve without redeployment.

**Architecture:** Two new Postgres tables (`weight_config`, `weight_history`) store active weights per project and their adjustment history. `internal/weight/tuner.go` implements the `WeightTuner` worker and tuning rules. `internal/search/score.go` gains an optional `Weights *WeightConfig` field in `ScoreInput` — when non-nil it overrides compile-time constants; nil = compile-time defaults (backward compatible). The search engine caches weights per project with a 15-min TTL refresh. One new MCP tool: `memory_weight_history`.

**Tech Stack:** Go 1.22+, pgx/v5, `go test ./... -count=1 -race`

**⚠️ Advisor Gate:** Request Opus review of the tuning rules and guardrail bounds BEFORE making weights DB-driven. Production scoring impact: verify the guardrail table covers all failure classes, weights normalize to 1.0, and min-50-event gate is correct.

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `internal/db/migrations/016_weight_config.sql` | weight_config + weight_history tables |
| Create | `internal/db/weights.go` | GetWeightConfig, SetWeightConfig, SaveWeightHistory, ListWeightHistory |
| Create | `internal/db/weights_test.go` | Unit tests for weight CRUD |
| Create | `internal/weight/tuner.go` | WeightTuner worker, tuning rules, guardrails |
| Create | `internal/weight/tuner_test.go` | Unit tests for tuning rules |
| Modify | `internal/search/score.go` | Add `Weights *WeightConfig` to `ScoreInput`; use in `CompositeScore` |
| Modify | `internal/search/score_test.go` (if exists) | Test weight override path |
| Modify | `internal/search/engine.go` | Load weights from DB cache, inject into ScoreInput |
| Modify | `internal/mcp/tools.go` | Add `handleMemoryWeightHistory` |
| Modify | `internal/mcp/server.go` | Register `memory_weight_history` |
| Modify | `cmd/engram/main.go` | Start `WeightTuner` goroutine |

---

### Task 1: Write migration 016

**Files:**
- Create: `internal/db/migrations/016_weight_config.sql`

- [ ] **Step 1: Create the migration**

```sql
-- Migration 016: Adaptive weight tuning tables
--
-- weight_config holds the active scoring weights per project (one row per project).
-- weight_history records every adjustment, what triggered it, and what changed.

CREATE TABLE weight_config (
    project          TEXT             PRIMARY KEY,
    weight_vector    DOUBLE PRECISION NOT NULL DEFAULT 0.45,
    weight_bm25      DOUBLE PRECISION NOT NULL DEFAULT 0.30,
    weight_recency   DOUBLE PRECISION NOT NULL DEFAULT 0.10,
    weight_precision DOUBLE PRECISION NOT NULL DEFAULT 0.15,
    updated_at       TIMESTAMPTZ      NOT NULL DEFAULT NOW()
);

CREATE TABLE weight_history (
    id               TEXT             PRIMARY KEY,
    project          TEXT             NOT NULL,
    applied_at       TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    weight_vector    DOUBLE PRECISION NOT NULL,
    weight_bm25      DOUBLE PRECISION NOT NULL,
    weight_recency   DOUBLE PRECISION NOT NULL,
    weight_precision DOUBLE PRECISION NOT NULL,
    trigger_data     JSONB,
    notes            TEXT
);

CREATE INDEX idx_weight_history_project_applied ON weight_history(project, applied_at DESC);
```

- [ ] **Step 2: Verify migration parses**

```bash
cd /home/psimmons/projects/engram-go && go build ./... 2>&1
```
Expected: clean build (migration is applied at runtime by the migration runner).

- [ ] **Step 3: Commit**

```bash
cd /home/psimmons/projects/engram-go && git add internal/db/migrations/016_weight_config.sql && git commit -m "feat(migrations): 016 add weight_config and weight_history tables"
```

---

### Task 2: Create `internal/db/weights.go` and tests

**Files:**
- Create: `internal/db/weights.go`
- Create: `internal/db/weights_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/db/weights_test.go`:

```go
package db_test

import (
    "testing"

    "github.com/petersimmons1972/engram/internal/db"
)

func TestWeightConfigDefaults(t *testing.T) {
    cfg := db.DefaultWeightConfig()
    total := cfg.Vector + cfg.BM25 + cfg.Recency + cfg.Precision
    if total < 0.9999 || total > 1.0001 {
        t.Errorf("DefaultWeightConfig weights do not sum to 1.0: got %.6f", total)
    }
    if cfg.Vector != 0.45 {
        t.Errorf("expected Vector=0.45, got %.4f", cfg.Vector)
    }
    if cfg.BM25 != 0.30 {
        t.Errorf("expected BM25=0.30, got %.4f", cfg.BM25)
    }
    if cfg.Recency != 0.10 {
        t.Errorf("expected Recency=0.10, got %.4f", cfg.Recency)
    }
    if cfg.Precision != 0.15 {
        t.Errorf("expected Precision=0.15, got %.4f", cfg.Precision)
    }
}

func TestWeightConfigClamp(t *testing.T) {
    cfg := db.WeightConfig{Vector: 0.80, BM25: 0.80, Recency: 0.80, Precision: 0.80}
    clamped := cfg.Clamped()
    if clamped.Vector > 0.65 {
        t.Errorf("Vector should be clamped to 0.65, got %.4f", clamped.Vector)
    }
    if clamped.BM25 > 0.45 {
        t.Errorf("BM25 should be clamped to 0.45, got %.4f", clamped.BM25)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/psimmons/projects/engram-go && go test ./internal/db/... -count=1 -run TestWeightConfig
```
Expected: compile error — `db.WeightConfig undefined`.

- [ ] **Step 3: Create `internal/db/weights.go`**

```go
package db

import (
    "context"
    "fmt"
    "time"

    "github.com/petersimmons1972/engram/internal/types"
)

// WeightConfig holds the 4 composite scoring weights for a project.
// Weights must sum to 1.0 (enforced by Normalize).
type WeightConfig struct {
    Vector    float64 `json:"weight_vector"`
    BM25      float64 `json:"weight_bm25"`
    Recency   float64 `json:"weight_recency"`
    Precision float64 `json:"weight_precision"`
}

// Guardrail bounds — each weight is clamped to [min, max].
var weightBounds = map[string][2]float64{
    "vector":    {0.30, 0.65},
    "bm25":      {0.15, 0.45},
    "recency":   {0.05, 0.20},
    "precision": {0.05, 0.25},
}

// DefaultWeightConfig returns compile-time default weights (sum = 1.0).
func DefaultWeightConfig() WeightConfig {
    return WeightConfig{
        Vector:    0.45,
        BM25:      0.30,
        Recency:   0.10,
        Precision: 0.15,
    }
}

// Clamped returns a copy with each weight clamped to its guardrail bounds.
// Does not normalize — call Normalize() afterward.
func (w WeightConfig) Clamped() WeightConfig {
    clamp := func(v float64, bounds [2]float64) float64 {
        if v < bounds[0] {
            return bounds[0]
        }
        if v > bounds[1] {
            return bounds[1]
        }
        return v
    }
    return WeightConfig{
        Vector:    clamp(w.Vector, weightBounds["vector"]),
        BM25:      clamp(w.BM25, weightBounds["bm25"]),
        Recency:   clamp(w.Recency, weightBounds["recency"]),
        Precision: clamp(w.Precision, weightBounds["precision"]),
    }
}

// Normalize scales weights so they sum to 1.0.
// If all weights are zero (degenerate), returns DefaultWeightConfig.
func (w WeightConfig) Normalize() WeightConfig {
    total := w.Vector + w.BM25 + w.Recency + w.Precision
    if total < 1e-9 {
        return DefaultWeightConfig()
    }
    return WeightConfig{
        Vector:    w.Vector / total,
        BM25:      w.BM25 / total,
        Recency:   w.Recency / total,
        Precision: w.Precision / total,
    }
}

// GetWeightConfig returns the active weights for a project.
// Falls back to DefaultWeightConfig if no row exists for the project.
func (b *PostgresBackend) GetWeightConfig(ctx context.Context, project string) (WeightConfig, error) {
    const q = `
SELECT weight_vector, weight_bm25, weight_recency, weight_precision
FROM weight_config
WHERE project = $1`
    rows, err := b.pool.Query(ctx, q, project)
    if err != nil {
        return DefaultWeightConfig(), fmt.Errorf("GetWeightConfig: %w", err)
    }
    defer rows.Close()
    if rows.Next() {
        var cfg WeightConfig
        if err := rows.Scan(&cfg.Vector, &cfg.BM25, &cfg.Recency, &cfg.Precision); err != nil {
            return DefaultWeightConfig(), fmt.Errorf("GetWeightConfig scan: %w", err)
        }
        return cfg, nil
    }
    return DefaultWeightConfig(), nil
}

// SetWeightConfig upserts weight_config for a project and records to weight_history.
func (b *PostgresBackend) SetWeightConfig(ctx context.Context, project string, cfg WeightConfig, triggerData any, notes string) error {
    cfg = cfg.Clamped().Normalize()
    now := time.Now().UTC()

    const upsertQ = `
INSERT INTO weight_config (project, weight_vector, weight_bm25, weight_recency, weight_precision, updated_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (project) DO UPDATE SET
    weight_vector    = EXCLUDED.weight_vector,
    weight_bm25      = EXCLUDED.weight_bm25,
    weight_recency   = EXCLUDED.weight_recency,
    weight_precision = EXCLUDED.weight_precision,
    updated_at       = EXCLUDED.updated_at`
    if _, err := b.pool.Exec(ctx, upsertQ, project, cfg.Vector, cfg.BM25, cfg.Recency, cfg.Precision, now); err != nil {
        return fmt.Errorf("SetWeightConfig upsert: %w", err)
    }

    const histQ = `
INSERT INTO weight_history (id, project, applied_at, weight_vector, weight_bm25, weight_recency, weight_precision, trigger_data, notes)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
    _, err := b.pool.Exec(ctx, histQ,
        types.NewMemoryID(), project, now,
        cfg.Vector, cfg.BM25, cfg.Recency, cfg.Precision,
        triggerData, notes,
    )
    return err
}

// WeightHistoryRow is one row from the weight_history table.
type WeightHistoryRow struct {
    ID          string    `json:"id"`
    Project     string    `json:"project"`
    AppliedAt   time.Time `json:"applied_at"`
    Weights     WeightConfig `json:"weights"`
    TriggerData any       `json:"trigger_data,omitempty"`
    Notes       string    `json:"notes,omitempty"`
}

// ListWeightHistory returns recent weight adjustment rows for a project.
func (b *PostgresBackend) ListWeightHistory(ctx context.Context, project string, limit int) ([]WeightHistoryRow, error) {
    if limit <= 0 {
        limit = 20
    }
    const q = `
SELECT id, project, applied_at, weight_vector, weight_bm25, weight_recency, weight_precision, trigger_data, COALESCE(notes,'')
FROM weight_history
WHERE project = $1
ORDER BY applied_at DESC
LIMIT $2`
    rows, err := b.pool.Query(ctx, q, project, limit)
    if err != nil {
        return nil, fmt.Errorf("ListWeightHistory: %w", err)
    }
    defer rows.Close()
    var result []WeightHistoryRow
    for rows.Next() {
        var r WeightHistoryRow
        var td []byte
        if err := rows.Scan(
            &r.ID, &r.Project, &r.AppliedAt,
            &r.Weights.Vector, &r.Weights.BM25, &r.Weights.Recency, &r.Weights.Precision,
            &td, &r.Notes,
        ); err != nil {
            return nil, fmt.Errorf("ListWeightHistory scan: %w", err)
        }
        if len(td) > 0 {
            _ = json.Unmarshal(td, &r.TriggerData)
        }
        result = append(result, r)
    }
    return result, rows.Err()
}
```

Add `"encoding/json"` to the imports.

- [ ] **Step 4: Run tests**

```bash
cd /home/psimmons/projects/engram-go && go test ./internal/db/... -count=1 -run TestWeightConfig -race
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/psimmons/projects/engram-go && git add internal/db/weights.go internal/db/weights_test.go && git commit -m "feat(db): add WeightConfig CRUD with clamping, normalization, and history"
```

---

### Task 3: Create `internal/weight/tuner.go` and tests

**Files:**
- Create: `internal/weight/tuner.go`
- Create: `internal/weight/tuner_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/weight/tuner_test.go`:

```go
package weight_test

import (
    "testing"

    "github.com/petersimmons1972/engram/internal/db"
    "github.com/petersimmons1972/engram/internal/weight"
)

func TestTuneWeightsNoOp(t *testing.T) {
    // When no dominant failure class, weights should not change
    current := db.DefaultWeightConfig()
    counts := map[string]int{
        "vocabulary_mismatch": 10,
        "stale_ranking":       10,
        "scope_mismatch":      10,
    } // no dominant class, total = 30 < 50 minimum
    result := weight.TuneWeights(current, counts)
    if result != current {
        t.Errorf("expected no change when total < 50, got %+v", result)
    }
}

func TestTuneWeightsVocabularyMismatch(t *testing.T) {
    // Dominant vocabulary_mismatch should increase BM25, decrease vector
    current := db.DefaultWeightConfig()
    counts := map[string]int{
        "vocabulary_mismatch": 60,
        "stale_ranking":       5,
    }
    result := weight.TuneWeights(current, counts)
    if result.BM25 <= current.BM25 {
        t.Errorf("expected BM25 to increase on vocabulary_mismatch, got %.4f (was %.4f)", result.BM25, current.BM25)
    }
    if result.Vector >= current.Vector {
        t.Errorf("expected Vector to decrease on vocabulary_mismatch, got %.4f (was %.4f)", result.Vector, current.Vector)
    }
    // Weights must still sum to 1.0
    total := result.Vector + result.BM25 + result.Recency + result.Precision
    if total < 0.9999 || total > 1.0001 {
        t.Errorf("weights do not sum to 1.0 after tuning: %.6f", total)
    }
}

func TestTuneWeightsStaleRanking(t *testing.T) {
    current := db.DefaultWeightConfig()
    counts := map[string]int{"stale_ranking": 70}
    result := weight.TuneWeights(current, counts)
    if result.Recency <= current.Recency {
        t.Errorf("expected Recency to increase on stale_ranking")
    }
}

func TestTuneWeightsScopeMismatch(t *testing.T) {
    current := db.DefaultWeightConfig()
    counts := map[string]int{"scope_mismatch": 70}
    result := weight.TuneWeights(current, counts)
    if result.Precision <= current.Precision {
        t.Errorf("expected Precision to increase on scope_mismatch")
    }
}

func TestTuneWeightsGuardrailCeiling(t *testing.T) {
    // Force vector to max ceiling; further vocabulary_mismatch adjustments should be clamped
    current := db.WeightConfig{Vector: 0.30, BM25: 0.45, Recency: 0.10, Precision: 0.15}
    counts := map[string]int{"vocabulary_mismatch": 100}
    result := weight.TuneWeights(current, counts)
    if result.BM25 > 0.45 {
        t.Errorf("BM25 should not exceed guardrail ceiling 0.45, got %.4f", result.BM25)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/psimmons/projects/engram-go && go test ./internal/weight/... -count=1 2>&1 | head -5
```
Expected: directory does not exist or `weight.TuneWeights undefined`.

- [ ] **Step 3: Create `internal/weight/tuner.go`**

```go
// Package weight implements adaptive weight tuning for the composite scoring function.
// The WeightTuner background worker reads failure_class aggregates and adjusts
// the 4 search weights (vector, bm25, recency, precision) within guardrails.
package weight

import (
    "context"
    "log/slog"
    "time"

    "github.com/petersimmons1972/engram/internal/db"
    "github.com/petersimmons1972/engram/internal/types"
)

// minEventGate is the minimum number of failure events in the rolling window
// before any adjustment fires. Prevents premature tuning on sparse data.
const minEventGate = 50

// adjustStep is the per-run weight delta applied to each relevant signal.
const adjustStep = 0.05

// TuneWeights reads failure_class counts (last 30 days) and returns adjusted
// weights. Returns current unchanged if total events < minEventGate or no class
// is dominant (>50% of total).
//
// Tuning rules:
//   - vocabulary_mismatch dominant → +bm25, −vector
//   - stale_ranking dominant       → +recency, −vector
//   - scope_mismatch dominant      → +precision, −bm25
//   - aggregation_failure/missing_content/other → no change
func TuneWeights(current db.WeightConfig, counts map[string]int) db.WeightConfig {
    total := 0
    for _, n := range counts {
        total += n
    }
    if total < minEventGate {
        return current
    }

    // Find dominant class (>50% of total)
    dominant := ""
    for class, n := range counts {
        if float64(n)/float64(total) > 0.50 {
            dominant = class
            break
        }
    }
    if dominant == "" {
        return current
    }

    next := current
    switch dominant {
    case types.FailureClassVocabularyMismatch:
        next.BM25 += adjustStep
        next.Vector -= adjustStep
    case types.FailureClassStaleRanking:
        next.Recency += adjustStep
        next.Vector -= adjustStep
    case types.FailureClassScopeMismatch:
        next.Precision += adjustStep
        next.BM25 -= adjustStep
    default:
        // aggregation_failure, missing_content, other → no weight change
        return current
    }

    return next.Clamped().Normalize()
}

// WeightTunerBackend is the subset of db.PostgresBackend used by WeightTuner.
type WeightTunerBackend interface {
    GetWeightConfig(ctx context.Context, project string) (db.WeightConfig, error)
    SetWeightConfig(ctx context.Context, project string, cfg db.WeightConfig, triggerData any, notes string) error
    AggregateFailureClasses(ctx context.Context, project string, limit int) ([]types.AggregateRow, error)
    ListProjects(ctx context.Context) ([]string, error)
}

// StartWeightTuner runs weight adjustment for all projects every interval.
// Safe to call as a goroutine; exits when ctx is cancelled.
//
//	go StartWeightTuner(ctx, backend, 7*24*time.Hour)
func StartWeightTuner(ctx context.Context, backend WeightTunerBackend, interval time.Duration) {
    run := func() {
        projects, err := backend.ListProjects(ctx)
        if err != nil {
            slog.Error("weight tuner: list projects", "err", err)
            return
        }
        for _, project := range projects {
            if err := tuneProject(ctx, backend, project); err != nil {
                slog.Error("weight tuner: project failed", "project", project, "err", err)
            }
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

func tuneProject(ctx context.Context, backend WeightTunerBackend, project string) error {
    rows, err := backend.AggregateFailureClasses(ctx, project, 10)
    if err != nil {
        return err
    }
    if len(rows) == 0 {
        return nil
    }

    counts := make(map[string]int, len(rows))
    for _, r := range rows {
        counts[r.Label] = r.Count
    }

    current, err := backend.GetWeightConfig(ctx, project)
    if err != nil {
        return err
    }

    next := TuneWeights(current, counts)
    if next == current {
        return nil // no change
    }

    notes := "adaptive weight tuning"
    return backend.SetWeightConfig(ctx, project, next, counts, notes)
}
```

**Note:** `WeightTunerBackend.ListProjects` may not exist on `PostgresBackend` yet. If absent, add to `internal/db/` a `ListProjects(ctx) ([]string, error)` method that queries `SELECT DISTINCT project FROM memories WHERE valid_to IS NULL`.

- [ ] **Step 4: Run tuner tests**

```bash
cd /home/psimmons/projects/engram-go && go test ./internal/weight/... -count=1 -race
```
Expected: all 5 tuner tests pass.

- [ ] **Step 5: Commit**

```bash
cd /home/psimmons/projects/engram-go && git add internal/weight/tuner.go internal/weight/tuner_test.go && git commit -m "feat(weight): add WeightTuner worker with failure_class-driven adjustment rules"
```

---

### Task 4: ⚠️ ADVISOR GATE — Opus review before DB-driven scoring

**Files:** none (review only)

- [ ] **Step 1: Request Opus review**

Dispatch Opus with:

```
Review the adaptive weight tuning system in internal/weight/tuner.go and internal/db/weights.go.

Check for:
1. Do the tuning rules (vocabulary_mismatch→+bm25, stale_ranking→+recency, scope_mismatch→+precision) correctly address each failure class?
2. Are the guardrail bounds correct? (vector [0.30,0.65], bm25 [0.15,0.45], recency [0.05,0.20], precision [0.05,0.25])
3. Does the minEventGate=50 protect against premature tuning on low-data projects?
4. Does Normalize() guarantee weights sum to 1.0 after clamping removes the step?
5. Is the adjustStep=0.05 safe? Could oscillation occur (flip-flop between classes)?

Return: zero severity/blocker findings or specific changes required.
```

- [ ] **Step 2: Apply any blocker findings before continuing**

---

### Task 5: Make `CompositeScore` accept optional weight overrides

**Files:**
- Modify: `internal/search/score.go`

- [ ] **Step 1: Write the failing test**

Check if `internal/search/score_test.go` exists:
```bash
ls /home/psimmons/projects/engram-go/internal/search/score_test.go 2>/dev/null || echo "missing"
```

If missing, create it. If it exists, add to it:

```go
func TestCompositeScore_WeightOverride(t *testing.T) {
    // With custom weights (bm25 dominant), a high BM25 score should dominate
    highBM25 := search.ScoreInput{
        Cosine:     0.3,
        BM25:       0.9,
        HoursSince: 1,
        Importance: 2,
        Weights: &search.WeightConfig{
            Vector:    0.10,
            BM25:      0.70,
            Recency:   0.10,
            Precision: 0.10,
        },
    }
    defaultWeights := search.ScoreInput{
        Cosine:     0.3,
        BM25:       0.9,
        HoursSince: 1,
        Importance: 2,
    }
    scoreCustom := search.CompositeScore(highBM25)
    scoreDefault := search.CompositeScore(defaultWeights)
    if scoreCustom <= scoreDefault {
        t.Errorf("expected custom BM25-heavy weight to produce higher score (%.4f vs default %.4f)", scoreCustom, scoreDefault)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/psimmons/projects/engram-go && go test ./internal/search/... -count=1 -run TestCompositeScore_WeightOverride
```
Expected: compile error — `search.WeightConfig undefined` or `ScoreInput.Weights undefined`.

- [ ] **Step 3: Add `WeightConfig` and `Weights` field to `score.go`**

In `internal/search/score.go`, add before `ScoreInput`:

```go
// WeightConfig holds overrideable weights for CompositeScore.
// When Weights is set in ScoreInput, these values replace the compile-time constants.
type WeightConfig struct {
    Vector    float64
    BM25      float64
    Recency   float64
    Precision float64
}
```

Add `Weights *WeightConfig` to `ScoreInput`:

```go
type ScoreInput struct {
    Cosine             float64
    BM25               float64
    HoursSince         float64
    Importance         int
    DynamicImportance  *float64
    RetrievalPrecision *float64
    Weights            *WeightConfig // nil = use compile-time constants
}
```

Update `CompositeScore` to use the override when set:

```go
func CompositeScore(in ScoreInput) float64 {
    wVec := weightVector
    wBM25 := weightBM25
    wRecency := weightRecency
    wPrecision := weightPrecision
    if in.Weights != nil {
        wVec = in.Weights.Vector
        wBM25 = in.Weights.BM25
        wRecency = in.Weights.Recency
        wPrecision = in.Weights.Precision
    }

    recency := RecencyDecay(in.HoursSince)
    var boost float64
    if in.DynamicImportance != nil {
        boost = math.Max(0.1, *in.DynamicImportance)
    } else {
        boost = ImportanceBoost(in.Importance)
    }
    precision := 0.5
    if in.RetrievalPrecision != nil {
        precision = *in.RetrievalPrecision
    }
    raw := wVec*in.Cosine + wBM25*in.BM25 + wRecency*recency + wPrecision*precision
    return raw * boost
}
```

- [ ] **Step 4: Run all search tests**

```bash
cd /home/psimmons/projects/engram-go && go test ./internal/search/... -count=1 -race
```
Expected: all pass including existing `TestCompositeScore_*` tests (nil Weights = backward compatible).

- [ ] **Step 5: Commit**

```bash
cd /home/psimmons/projects/engram-go && git add internal/search/score.go && git commit -m "feat(search): add optional WeightConfig override to ScoreInput (backward compatible)"
```

---

### Task 6: Wire weight cache into search engine

**Files:**
- Modify: `internal/search/engine.go`

The search engine needs to load weights from the DB and inject them into `ScoreInput`. Use a simple per-engine cache with a 15-minute TTL rather than a global.

- [ ] **Step 1: Find the CompositeScore call site**

```bash
grep -n "CompositeScore" /home/psimmons/projects/engram-go/internal/search/engine.go
```
Expected: one line at ~539. This is inside a loop over candidate memories.

- [ ] **Step 2: Add weight cache fields to `SearchEngine`**

In `internal/search/engine.go`, find the `SearchEngine` struct definition. Add:

```go
// weightCache caches project weights to avoid per-query DB lookups.
weightCache     *WeightConfig
weightCachedAt  time.Time
weightCacheTTL  time.Duration // default 15 min
weightProject   string        // project this cache entry belongs to
weightMu        sync.Mutex
```

- [ ] **Step 3: Add `currentWeights()` method to engine**

```go
// currentWeights returns cached weights for the engine's project, refreshing
// from DB when the cache is stale (TTL = 15 min) or empty.
func (e *SearchEngine) currentWeights(ctx context.Context) *WeightConfig {
    e.weightMu.Lock()
    defer e.weightMu.Unlock()
    ttl := e.weightCacheTTL
    if ttl == 0 {
        ttl = 15 * time.Minute
    }
    if e.weightCache != nil && time.Since(e.weightCachedAt) < ttl {
        return e.weightCache
    }
    cfg, err := e.backend.GetWeightConfig(ctx, e.project)
    if err != nil {
        // On error, return nil so CompositeScore falls back to compile-time constants
        return nil
    }
    wc := &WeightConfig{
        Vector:    cfg.Vector,
        BM25:      cfg.BM25,
        Recency:   cfg.Recency,
        Precision: cfg.Precision,
    }
    e.weightCache = wc
    e.weightCachedAt = time.Now()
    return wc
}
```

**Note:** This requires `engine.go` to import `"github.com/petersimmons1972/engram/internal/db"`. Check if it already does. Also requires `e.backend *db.PostgresBackend` and `e.project string` fields on `SearchEngine` — check if they exist. Add them if missing.

- [ ] **Step 4: Inject weights into `ScoreInput` at the CompositeScore call site**

At line ~539 where `CompositeScore(input)` is called, first obtain weights:

```go
// Before the scoring loop (once per Recall call, not per memory):
weights := e.currentWeights(ctx)

// Inside the loop, add to ScoreInput:
input := ScoreInput{
    Cosine:             /* existing */,
    BM25:               /* existing */,
    HoursSince:         /* existing */,
    Importance:         /* existing */,
    DynamicImportance:  /* existing */,
    RetrievalPrecision: /* existing */,
    Weights:            weights, // ADD
}
score := CompositeScore(input)
```

- [ ] **Step 5: Run full test suite**

```bash
cd /home/psimmons/projects/engram-go && go test ./... -count=1 -race 2>&1 | tail -20
```
Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
cd /home/psimmons/projects/engram-go && git add internal/search/engine.go && git commit -m "feat(search): inject DB-driven weight cache into CompositeScore — weights now adapt without redeployment"
```

---

### Task 7: Add `memory_weight_history` tool + register everything

**Files:**
- Modify: `internal/mcp/tools.go`
- Modify: `internal/mcp/server.go`
- Modify: `cmd/engram/main.go`

- [ ] **Step 1: Add `handleMemoryWeightHistory` to `tools.go`**

```go
func handleMemoryWeightHistory(ctx context.Context, pool *EnginePool, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
    args := req.GetArguments()
    project, err := getProject(args, "default")
    if err != nil {
        return nil, err
    }
    limit := getInt(args, "limit", 10)

    h, err := pool.Get(ctx, project)
    if err != nil {
        return nil, err
    }
    history, err := h.Engine.Backend().ListWeightHistory(ctx, project, limit)
    if err != nil {
        return nil, fmt.Errorf("memory_weight_history: %w", err)
    }
    current, err := h.Engine.Backend().GetWeightConfig(ctx, project)
    if err != nil {
        return nil, fmt.Errorf("memory_weight_history: get current: %w", err)
    }
    return toolResult(map[string]any{
        "project":  project,
        "current":  current,
        "history":  history,
    })
}
```

- [ ] **Step 2: Register tool in `server.go`**

```go
{"memory_weight_history", "Show current scoring weights and recent adjustment history for a project. Displays what failure classes triggered each change and the before/after weight values.",
    func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
        return handleMemoryWeightHistory(ctx, pool, req)
    }},
```

- [ ] **Step 3: Wire `WeightTuner` in `cmd/engram/main.go`**

After the `AuditWorker` goroutine (or after `StartRetentionWorker`), add:

```go
tunerInterval := 7 * 24 * time.Hour
if v := os.Getenv("ENGRAM_WEIGHT_TUNER_INTERVAL"); v != "" {
    if d, err := time.ParseDuration(v); err == nil {
        tunerInterval = d
    }
}
go weight.StartWeightTuner(ctx, retentionBackend, tunerInterval)
```

Add import: `"github.com/petersimmons1972/engram/internal/weight"`

- [ ] **Step 4: Run full test suite**

```bash
cd /home/psimmons/projects/engram-go && go test ./... -count=1 -race 2>&1 | tail -20
```
Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
cd /home/psimmons/projects/engram-go && git add internal/mcp/tools.go internal/mcp/server.go cmd/engram/main.go && git commit -m "feat(mcp): add memory_weight_history and start WeightTuner worker"
```

---

## Verification

```bash
# Full test suite
cd /home/psimmons/projects/engram-go && go test ./... -count=1 -race

# Build check
cd /home/psimmons/projects/engram-go && go build ./...
```

Manual flow (requires running server with 015+016 migrations applied):

1. Seed 60+ failure events with dominant `vocabulary_mismatch`:
   ```
   memory_feedback(event_id="...", memory_ids=[], failure_class="vocabulary_mismatch")
   ```
   (repeat 60+ times or insert directly into retrieval_events)

2. Reduce tuner interval for testing:
   ```bash
   ENGRAM_WEIGHT_TUNER_INTERVAL=1m docker restart engram
   ```

3. Wait 1 minute, then:
   ```
   memory_weight_history(project="clearwatch")
   ```
   Expected: history row showing BM25 increased (e.g., 0.30 → 0.35), Vector decreased.

4. Verify weights sum to 1.0 in the response.

5. Confirm `memory_recall` still works normally (nil Weights path unchanged).
