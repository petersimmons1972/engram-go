# engram-go — Claude Instructions

## AI-Generated PR Policy

PRs submitted by any AI-assisted development (Claude Code, Cursor, GitHub Copilot, or manual LLM use) must be labeled `ai-generated` and require three rounds of adversarial review before merge:

1. **Correctness** — boundary conditions, logic bugs, error handling, nil dereferences, panics
2. **Coverage** — ≥ 70% function coverage on new files, complete tests for exported APIs
3. **Structural** — fresh-eyes review, architecture fit, naming clarity, complexity

**Merge gate:** All three must return zero `severity/blocker` findings. `severity/nice-to-have` findings are tracked as issues but do not block merge.

**Tools are tools:** Use Claude Code, GitHub Copilot, or any AI assistance freely. Label the PR `ai-generated` upfront. Depth of review, not prohibition on AI use, is the control.

**Why:** PR #162 (April 2026) had 4 logic bugs and 20/24 untested functions. Adversarial review caught all of them. Commit aaf56c6 documents adequate AI-assisted work.

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

## MCP Read-Only Hint Annotations

Read-side tools (recall, fetch, query, list, status, history, timeline, projects, episode/audit listings, constraint checks, diagnose) carry `ReadOnlyHint: true` on their MCP annotation. The canonical set lives in `readOnlyToolNames()` in `internal/mcp/server.go` — add new read-only tools there, not by editing `registerTools` directly. The annotation is what lets Claude Code's plan mode invoke these tools without prompting; without it, calls are silently rejected client-side. `TestReadOnlyToolAnnotations` (`internal/mcp/readonly_hints_test.go`) is the regression guard.
