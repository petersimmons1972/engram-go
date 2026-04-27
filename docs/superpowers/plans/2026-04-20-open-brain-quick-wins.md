# Open-Brain Quick Wins: Failure Class + Relation Types Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the `other` failure class and 4 new semantic relation types (`supports`, `derived_from`, `part_of`, `follows`) from open-brain into engram-go's type system with full test coverage.

**Architecture:** Both changes are purely additive — new constants in `internal/types/types.go` plus corresponding entries in the closed validation maps. No data migration is required (both columns are free-text). A documentation-only migration (014) records the expanded vocabulary for ops clarity.

**Tech Stack:** Go 1.22+, `go test ./... -count=1 -race`, `pgx/v5` (no schema changes)

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Modify | `internal/types/types.go` | Add `FailureClassOther` constant + map entry; add 4 `RelType*` constants + map entries |
| Modify | `internal/types/types_test.go` | Extend `TestValidateFailureClass`, `TestFailureClassConstants`, `TestValidateRelationType`, `TestRelationTypeConstants` |
| Create | `internal/db/migrations/014_relation_types.sql` | Documentation migration listing expanded vocabulary |
| Modify | `internal/mcp/server.go` | Update `memory_connect` description (11 types); update `memory_feedback` description (6 classes) |

---

### Task 1: Add `FailureClassOther` constant

**Files:**
- Modify: `internal/types/types.go:41-54`

- [ ] **Step 1: Write the failing test**

Add at the end of `TestValidateFailureClass` in `internal/types/types_test.go` — but first, verify the test file compiles and the new constant doesn't exist yet:

```bash
grep -n "FailureClassOther" /home/psimmons/projects/engram-go/internal/types/types.go
```
Expected: no output.

- [ ] **Step 2: Add the failing test case**

In `internal/types/types_test.go`, locate `TestValidateFailureClass` (line 239) and extend the `valid` slice and `TestFailureClassConstants`:

```go
// In TestValidateFailureClass — extend valid slice (line ~242):
valid := []string{
    "",
    types.FailureClassVocabularyMismatch,
    types.FailureClassAggregationFailure,
    types.FailureClassStaleRanking,
    types.FailureClassMissingContent,
    types.FailureClassScopeMismatch,
    types.FailureClassOther,  // ADD
}
```

```go
// In TestFailureClassConstants — add new case (line ~207):
cases := []struct {
    got  string
    want string
}{
    {types.FailureClassVocabularyMismatch, "vocabulary_mismatch"},
    {types.FailureClassAggregationFailure, "aggregation_failure"},
    {types.FailureClassStaleRanking, "stale_ranking"},
    {types.FailureClassMissingContent, "missing_content"},
    {types.FailureClassScopeMismatch, "scope_mismatch"},
    {types.FailureClassOther, "other"},  // ADD
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
cd /home/psimmons/projects/engram-go && go test ./internal/types/... -count=1 -run TestValidateFailureClass
```
Expected: compile error — `types.FailureClassOther undefined`.

- [ ] **Step 4: Add `FailureClassOther` to `internal/types/types.go`**

Locate the `FailureClass` const block (lines 40-46) and the `validFailureClasses` map (lines 48-54):

```go
// Replace the FailureClass const block:
const (
    FailureClassVocabularyMismatch = "vocabulary_mismatch"
    FailureClassAggregationFailure = "aggregation_failure"
    FailureClassStaleRanking       = "stale_ranking"
    FailureClassMissingContent     = "missing_content"
    FailureClassScopeMismatch      = "scope_mismatch"
    FailureClassOther              = "other" // catch-all for unclassified failures
)

// Replace the validFailureClasses map:
var validFailureClasses = map[string]bool{
    FailureClassVocabularyMismatch: true,
    FailureClassAggregationFailure: true,
    FailureClassStaleRanking:       true,
    FailureClassMissingContent:     true,
    FailureClassScopeMismatch:      true,
    FailureClassOther:              true,
}
```

- [ ] **Step 5: Run tests**

```bash
cd /home/psimmons/projects/engram-go && go test ./internal/types/... -count=1 -race
```
Expected: PASS — all types tests green.

- [ ] **Step 6: Commit**

```bash
cd /home/psimmons/projects/engram-go && git add internal/types/types.go internal/types/types_test.go && git commit -m "feat(types): add FailureClassOther for unclassified retrieval failures"
```

---

### Task 2: Add 4 semantic relation type constants

**Files:**
- Modify: `internal/types/types.go:23-86`
- Modify: `internal/types/types_test.go`

- [ ] **Step 1: Write the failing tests**

In `internal/types/types_test.go`, extend `TestValidateRelationType` (line 26) and `TestRelationTypeConstants` (line 183):

```go
// TestValidateRelationType — extend valid slice (line ~28):
valid := []string{
    "caused_by", "relates_to", "depends_on", "supersedes",
    "used_in", "resolved_by",
    "supports", "derived_from", "part_of", "follows",  // ADD
}
```

```go
// TestRelationTypeConstants — add 4 new entries (line ~185):
expected := map[string]string{
    "caused_by":    types.RelTypeCausedBy,
    "relates_to":   types.RelTypeRelatesTo,
    "depends_on":   types.RelTypeDependsOn,
    "supersedes":   types.RelTypeSupersedes,
    "used_in":      types.RelTypeUsedIn,
    "resolved_by":  types.RelTypeResolvedBy,
    "supports":     types.RelTypeSupports,      // ADD
    "derived_from": types.RelTypeDerivedFrom,   // ADD
    "part_of":      types.RelTypePartOf,        // ADD
    "follows":      types.RelTypeFollows,       // ADD
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/psimmons/projects/engram-go && go test ./internal/types/... -count=1 -run TestValidateRelationType
```
Expected: compile error — `types.RelTypeSupports undefined` (and 3 others).

- [ ] **Step 3: Add 4 relation type constants to `internal/types/types.go`**

Locate the RelType const block (lines 22-31) and `validRelationTypes` map (lines 78-86):

```go
// Replace the RelType const block:
const (
    RelTypeCausedBy    = "caused_by"
    RelTypeRelatesTo   = "relates_to"
    RelTypeDependsOn   = "depends_on"
    RelTypeSupersedes  = "supersedes"
    RelTypeUsedIn      = "used_in"
    RelTypeResolvedBy  = "resolved_by"
    RelTypeContradicts = "contradicts" // set by sleep consolidation daemon

    // Semantic types from open-brain vocabulary (additive merge, v3.x)
    RelTypeSupports    = "supports"      // one memory strengthens another's evidence
    RelTypeDerivedFrom = "derived_from"  // citation chain — memory derived from source
    RelTypePartOf      = "part_of"       // hierarchical containment
    RelTypeFollows     = "follows"       // temporal or sequential ordering
)

// Replace the validRelationTypes map:
var validRelationTypes = map[string]bool{
    RelTypeCausedBy:    true,
    RelTypeRelatesTo:   true,
    RelTypeDependsOn:   true,
    RelTypeSupersedes:  true,
    RelTypeUsedIn:      true,
    RelTypeResolvedBy:  true,
    RelTypeContradicts: true,
    RelTypeSupports:    true,
    RelTypeDerivedFrom: true,
    RelTypePartOf:      true,
    RelTypeFollows:     true,
}
```

- [ ] **Step 4: Run full types test suite**

```bash
cd /home/psimmons/projects/engram-go && go test ./internal/types/... -count=1 -race
```
Expected: PASS — all 12 tests green.

- [ ] **Step 5: Commit**

```bash
cd /home/psimmons/projects/engram-go && git add internal/types/types.go internal/types/types_test.go && git commit -m "feat(types): add 4 semantic relation types from open-brain vocabulary"
```

---

### Task 3: Create documentation migration 014

**Files:**
- Create: `internal/db/migrations/014_relation_types.sql`

- [ ] **Step 1: Create the migration file**

```sql
-- Migration 014: Expand relation type vocabulary (additive merge, v3.x)
--
-- New valid values for relationships.rel_type:
--   supports     – one memory strengthens another's evidence
--   derived_from – citation chain; memory derived from source
--   part_of      – hierarchical containment
--   follows      – temporal or sequential ordering
--
-- New valid value for retrieval_events.failure_class:
--   other – catch-all for unclassified retrieval failures
--
-- Existing edges and events are unaffected.
-- No constraint on rel_type or failure_class columns (both free-text).
-- See /internal/types/types.go for the full closed constant sets.
SELECT 1; -- no-op; migration runner requires at least one statement
```

- [ ] **Step 2: Verify the server still starts**

```bash
cd /home/psimmons/projects/engram-go && go build ./... 2>&1
```
Expected: no output (clean build).

- [ ] **Step 3: Commit**

```bash
cd /home/psimmons/projects/engram-go && git add internal/db/migrations/014_relation_types.sql && git commit -m "chore(migrations): 014 document expanded relation + failure_class vocabulary"
```

---

### Task 4: Update MCP tool descriptions

**Files:**
- Modify: `internal/mcp/server.go:471` (memory_connect description)
- Modify: `internal/mcp/server.go:503` (memory_feedback description)

- [ ] **Step 1: Update `memory_connect` description**

Locate line 471 in `internal/mcp/server.go`:
```go
{"memory_connect", "Create a directed relationship between two memories",
```

Replace with:
```go
{"memory_connect", "Create a directed relationship between two memories. relation_type values: caused_by, relates_to, depends_on, supersedes, used_in, resolved_by, contradicts, supports, derived_from, part_of, follows",
```

- [ ] **Step 2: Update `memory_feedback` description**

Locate line 503 in `internal/mcp/server.go`:
```go
{"memory_feedback", "Record positive access signal for memories",
```

Replace with:
```go
{"memory_feedback", "Record retrieval feedback. failure_class values (for misses): vocabulary_mismatch, aggregation_failure, stale_ranking, missing_content, scope_mismatch, other",
```

- [ ] **Step 3: Run the full test suite**

```bash
cd /home/psimmons/projects/engram-go && go test ./... -count=1 -race 2>&1 | tail -20
```
Expected: all tests pass; no compilation errors.

- [ ] **Step 4: Commit**

```bash
cd /home/psimmons/projects/engram-go && git add internal/mcp/server.go && git commit -m "docs(mcp): update memory_connect and memory_feedback tool descriptions for expanded vocabulary"
```

---

## Verification

```bash
# Full test suite
cd /home/psimmons/projects/engram-go && go test ./... -count=1 -race

# Confirm new constants in binary
cd /home/psimmons/projects/engram-go && go build ./... && strings bin/engram 2>/dev/null | grep -E "derived_from|part_of|follows|supports" | head -5
```

- `memory_feedback` with `failure_class: "other"` → accepted without validation error
- `memory_connect` with `rel_type: "derived_from"` → accepted
- `memory_aggregate(by="failure_class")` → `other` appears in results after seeding
