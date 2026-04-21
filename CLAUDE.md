# engram-go — Claude Instructions

## AI-Generated PR Policy

PRs submitted by AI agents (Codex, Cursor, etc.) must be labeled `ai-generated` and require adversarial review before merge.

**Required reviewers:**
- Rickover (correctness audit — boundary conditions, logic bugs, error handling)
- Spruance (coverage audit — ensure ≥ 70% function coverage on new files)
- Zero-context reviewer (fresh-eyes structural review — receives only the diff)

**Merge gate:** All three must return zero `severity/blocker` findings. `severity/nice-to-have` findings are tracked as issues but do not block merge.

**Why:** PR #162 (Codex, April 2026) passed syntax checks but had 4 logic bugs and 20/24 functions untested. The adversarial review caught all of them before merge. The clean TDD reimplementation in commit aaf56c6 documented the pattern to follow.

## Coverage Gate

CI enforces a 60% minimum statement coverage on every PR (`.github/workflows/ci.yml`). New files with < 60% coverage will fail the build. The safety tools rewrite (safety.go) serves as the reference for what adequate coverage looks like.

## Test Policy

- Write failing tests before implementation (TDD).
- New MCP tool handlers require at minimum: happy path, zero/empty input, and one boundary condition test.
- Run `go test ./... -count=1 -race` before any commit to main.

## Retrieval Miss Handling

When `memory_recall` returns nothing useful, use `memory_feedback` with `failure_class` instead of manually calling `memory_store`:

```
# Record the miss (do not reinforce — no edge boost applied)
memory_feedback(event_id="<id from recall>", memory_ids=[], failure_class="<class>")

# Triage the distribution of misses
memory_aggregate(by="failure_class")
→ [{label: "aggregation_failure", count: N, ...}]
```

Valid `failure_class` values: `vocabulary_mismatch`, `aggregation_failure`, `stale_ranking`, `missing_content`, `scope_mismatch`, `other`.

Use `memory_aggregate(by="failure_class")` periodically to see where recall is failing most. This data feeds retrieval quality benchmarking.
