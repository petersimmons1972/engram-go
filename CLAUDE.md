---
project: engram-go
purpose: Go v2 persistent memory service for AI agents — BM25 + vector + recency + knowledge graph recall over an MCP tool surface.
stack: [go, postgresql, docker]
status: active
entrypoints:
  - cmd/
  - engram/
storage: PostgreSQL; backups in backups/; Docker Compose variants for lan/local/prod
related: [engram]
notes: Supersedes engram (Python v1). AI-generated PRs require three-round adversarial review before merge.
---

# engram-go — Claude Instructions

## AI-Generated PR Policy

PRs submitted by any AI-assisted development (Claude Code, Cursor, GitHub Copilot, or manual LLM use) must be labeled `ai-generated` and require three rounds of adversarial review before merge:

1. **Correctness** — boundary conditions, logic bugs, error handling, nil dereferences, panics
2. **Coverage** — ≥ 70% function coverage on new files, complete tests for exported APIs
3. **Structural** — fresh-eyes review, architecture fit, naming clarity, complexity

**Merge gate:** All three must return zero `severity/blocker` findings. `severity/nice-to-have` findings are tracked as issues but do not block merge.

## Coverage Gate

CI enforces a statement-coverage floor on every PR — see `.github/workflows/ci.yml` for the current value (kept there, not hardcoded here, so it can't silently drift). The #429 integration tests are now fixed and un-skipped, so the earlier temporary-lower-bound situation is resolved; measured coverage with `TEST_DATABASE_URL` set is ~66%. Aim comfortably above the CI floor on new files (60%+ statement), and above the ≥70% function-coverage bar in the AI-PR policy above. Reference for adequate coverage: `internal/mcp/safety.go`.

## Test Policy

- Write failing tests before implementation (TDD).
- New MCP tool handlers require at minimum: happy path, zero/empty input, and one boundary condition test.
- Run `go test ./... -count=1 -race` before any commit to main.

## Retrieval Miss Handling

When `memory_recall` returns nothing useful, record the miss with `memory_feedback` (empty `memory_ids` + a `failure_class`) rather than reflexively calling `memory_store`. Empty-ids feedback records the gap **without** applying an edge boost — it captures the failure signal without reinforcing a wrong result.

`event_id` only appears in `memory_recall`'s response when the call passes `record_event=true` (off by default so plain recall stays side-effect free). Pass it when you plan to follow up:

```
memory_recall(query="...", record_event=true)
→ {..., event_id: "0197f3c1-...", feedback_hint: "..."}
memory_feedback(event_id="<from recall>", memory_ids=[], failure_class="<class>")
```

Valid `failure_class` values: `vocabulary_mismatch`, `aggregation_failure`, `stale_ranking`, `missing_content`, `scope_mismatch`, `other`. To see where recall fails most, aggregate the feedback events over `failure_class` (the exact server-side aggregate tool depends on the deployed build — derive it from the live tool surface rather than assuming a specific tool name).

## MCP Read-Only Hint Annotations

Read-side tools (recall, fetch, query, list, status, history, timeline, projects, episode/audit listings, constraint checks, diagnose) carry `ReadOnlyHint: true` on their MCP annotation. The canonical set lives in `readOnlyToolNames()` in `internal/mcp/server.go` — add new read-only tools there, not by editing `registerTools` directly. The annotation is what lets Claude Code's plan mode invoke these tools without prompting; without it, calls are silently rejected client-side. `TestReadOnlyToolAnnotations` (`internal/mcp/readonly_hints_test.go`) is the regression guard.

## Repo & internals

- **This is a PUBLIC GitHub repo.** Never commit internal strategy, infrastructure hostnames/topology, roadmap, or red-team content here, and never leave internal docs uncommitted in the working tree (an autonomous merge-on-green agent could commit them). Keep sensitive material in a private repo; deployment/infra specifics live in private memory, not this file.
- **Two reembed implementations — read the right source.** The standalone `engram-reembed` binary is **Rust** (`reembed-rs/src/main.rs`; `tracing` JSON logs, `target:"engram_reembed"`), distinct from the in-process Go worker (`internal/reembed/worker.go` / `global_worker.go`; `slog` logs). Both select `WHERE embedding IS NULL ... FOR UPDATE SKIP LOCKED`. Confirm which a container runs via `docker inspect --format '{{.Config.Entrypoint}}'` before reading source. Chunks are inserted with NULL embedding **by design** when the online embed call lags (`internal/search/engine.go`) — a reembed burst after a write spike is expected, not data loss.
- **Canonical embedder: `BAAI/bge-m3`** (1024-dim; Postgres column `vector(1024)`). `project_meta.embedder_name` = `BAAI/bge-m3` for every named project; only `lme-*`/`bench-*` scratch namespaces differ. Any replacement must be 1024-dim (or clean Matryoshka truncation) and require a full re-embed campaign — never mix vector spaces. (Serving endpoints + GPU topology are internal infra — private memory, not committed here.)
