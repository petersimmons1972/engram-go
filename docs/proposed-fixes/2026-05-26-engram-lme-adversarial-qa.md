# 2026-05-26 Engram-go + LongMemEval Adversarial QA Findings

Status: Round 1 findings, read-only inspection. No fixes applied.

Scope: current dirty `main` checkout, live Engram process posture, LongMemEval CLI, result artifacts, runbooks, and local process state. The sweep used the six-persona fault-finder process: skeptical staff engineer, security reviewer, new maintainer, heavy CLI user, operator/SRE, and docs-first newcomer.

Verification performed:

- `git status --short` confirmed a dirty `main` worktree with broad LongMemEval, DB, search, docs, and binary changes.
- `go test ./cmd/longmemeval ./internal/longmemeval -count=1` passed, but spawned the external Claude path during normal test execution.
- `go run ./cmd/longmemeval --help` showed current command surface.
- `/health` on `127.0.0.1:8788` returned `{"status":"ok","postgres":"ok","ollama":"ok"}`.
- `kubectl get pods,svc -n engram -o wide` showed `engram-go` ready and `engram-reembed` ready with 12 restarts.
- Current result artifacts under `results/lme-quick-stratified-live-20260526` and historical artifacts under `results/lme-camp-25-qwen3-20260523` were inspected without running ingest.

## Gate

NO-GO for treating the current Engram/LME process as production-reliable without remediation. Multiple independent reviewers found blocker/high-confidence issues around secret exposure, incorrect pipeline success semantics, stale/non-copy-paste docs, and LME result reproducibility.

## Blocker / Critical

### 1. CLI help leaks the live Engram API key

Severity: blocker. Reported by: new maintainer, heavy CLI user, docs-first newcomer.

Locations:

- `cmd/longmemeval/main.go:154-156`
- `cmd/longmemeval/main.go:408-447`
- `cmd/longmemeval/prune.go:246-248`

Problem: `mcpDefaults()` reads the bearer token from `~/.claude/mcp_servers.json`, then registers that token as the default value for `--api-key`. Flag help prints defaults, so `longmemeval ingest --help`, `run --help`, and `prune --help` can expose the live API key into terminal scrollback, logs, CI captures, and pasted diagnostics.

Recommended TDD path:

- Add a dispatch/help test proving `--api-key` help never contains the configured token.
- Keep using discovered credentials at runtime if that behavior is required, but do not register the secret as a printable flag default. Prefer a placeholder default such as empty string plus internal resolution after parsing.

### 2. `longmemeval all` ignores run-stage failure

Severity: blocker/serious. Reported by: skeptical staff engineer, heavy CLI user.

Locations:

- `cmd/longmemeval/all.go:11-18`
- `cmd/longmemeval/run.go:275-278`

Problem: `runRun(cfg)` returns non-zero when every attempted item fails, but `runAll` discards that exit code and continues into scoring. A full generation failure can still emit score artifacts and make `all` appear successful.

Recommended TDD path:

- Add a test double or minimal checkpoint scenario where `runRun` returns non-zero.
- Assert `all` stops before `score` and returns the failing exit code.
- Change `runAll` to return an exit code and have dispatch propagate it.

### 3. Scratch TTL docs contain non-existent copy-paste flags

Severity: blocker. Reported by: docs-first newcomer, new maintainer.

Location: `docs/lme-benchmark-learnings.md:393-407`

Problem: the scratch TTL section says `longmemeval ingest` accepts `--data-file`, `--out-dir`, and `--database-url`, but current ingest/shared flags are `--data`, `--out`, and no ingest `--database-url`. The same section describes `--scratch-ttl` as ingest-time TTL, while current prune help treats `--scratch-ttl` as an alias for prune `--older-than`.

Recommended TDD/doc path:

- Decide whether docs or CLI is canonical.
- If docs are right, add the aliases/tests.
- If CLI is right, update docs with copy-paste commands and state that prune uses `project_ttl.expires_at`.

## High / Serious

### 4. `memory_recall` is annotated read-only but mutates state

Severity: high/serious. Reported by: security reviewer, skeptical staff engineer.

Locations:

- `internal/mcp/server.go:243-250`
- `internal/mcp/tools_recall.go:342-370`
- `internal/longmemeval/engram.go:259-280`

Problem: `memory_recall` is in the read-only MCP annotation set, but recall writes retrieval events and increments `times_retrieved`. The LongMemEval date-filter path calls `memory_recall` with `since`/`before`, which follows the manual event write path.

Risk: plan/read-only clients can mutate retrieval telemetry without a mutation prompt. LME runs also contaminate popularity/retrieval signals while measuring retrieval quality.

Recommended TDD path:

- Add a regression test for read-only recall mode that proves no retrieval event or `times_retrieved` write occurs.
- Add a client argument such as `record_event=false` or split read-only recall from feedback-producing recall.
- For LME, default to no-side-effects recall unless a benchmark explicitly opts into feedback mutation.

### 5. Date-window temporal recall trusts incomplete `valid_from`

Severity: serious. Reported by: skeptical staff engineer.

Locations:

- `cmd/longmemeval/run.go:88-135`
- `internal/db/postgres_chunk.go:275`
- `internal/db/postgres_prune.go:260`

Problem: temporal "N ago" questions can apply a hard `valid_from` window before ranking. If a store path omitted or mis-stamped `valid_from`, the correct memory can be filtered out before reranking can recover it.

Recommended TDD path:

- Add fixture data with one correct memory missing `valid_from` and prove the current path drops it.
- Decide whether date filtering should be opt-in, soft-boosted, or guarded by dataset/store-path integrity checks.

### 6. Score stage can silently produce all-error artifacts with success exit

Severity: serious. Reported by: heavy CLI user.

Location: `cmd/longmemeval/score.go:96-150`

Problem: `scoreOne` records judge/backend failures as `Status: "error"` checkpoint rows, but `runScore()` does not aggregate outcomes into a non-zero exit. A judge outage can leave a report path looking operationally complete.

Recommended TDD path:

- Add score tests for zero-success/all-error and partial-error runs.
- Return non-zero when all attempted score rows fail. Consider a stricter flag for any-error failure in CI.

### 7. Checkpoint corruption is treated as resumable partial state

Severity: serious. Reported by: heavy CLI user.

Location: `internal/longmemeval/checkpoint.go:13-65`

Problem: `ReadSkipSet` silently skips malformed JSON lines, and `WriteCheckpoint` only logs open/encode failures. Truncated/corrupt JSONL can change denominators or lose expensive benchmark work without a hard failure.

Recommended TDD path:

- Add malformed-line tests for every checkpoint reader.
- Fail closed by default, with an explicit `--repair-checkpoint` or `--ignore-corrupt-checkpoint` path if needed.
- Return writer errors to the caller instead of draining entries after an open failure.

### 8. Result scripts are stale and not source-reproducible

Severity: serious. Reported by: skeptical staff engineer, heavy CLI user.

Locations:

- `results/lme-camp-25-qwen3-20260523/run-pipeline.sh:58-69`
- `results/longmemeval-llama3-8b/resume.sh:8`
- `results/longmemeval-llama3-8b/resume.dual.sh:26`
- `results/longmemeval-llama3-8b/auto-score.sh:3-6`

Problems:

- Camp 25 scripts pass unregistered `--llm-api-key` and `--scorer-api-key` flags.
- W6800 scripts point to a historical worktree binary, not the reviewed checkout.
- `resume.dual.sh` hardcodes `http://precision.petersimmons.com:8005/v1`, while prior verified W6800/Ollama routing used `http://precision.petersimmons.com:11434`.
- `auto-score.sh` is bound to a historical PID and scorer target.

Recommended path:

- Treat result-local scripts as historical artifacts or move maintained wrappers under `scripts/`.
- Add `route-discover` output capture to run directories.
- Stamp binary path, git SHA, dirty flag, endpoint, model, and command line into a manifest before every run.

### 9. Partial result reports can look like headline benchmark outcomes

Severity: serious. Reported by: skeptical staff engineer, operator/SRE.

Locations:

- `results/lme-camp-25-qwen3-20260523/run.log`
- `results/lme-camp-25-qwen3-20260523/score_report.json`
- `results/lme-quick-stratified-live-20260526/*`

Evidence:

- Camp 25 has 500 ingest rows, 92 run rows, 24 score rows, and `run.log` ends `Terminated`, while `score_report.json` reports 24/24 scored.
- The latest quick stratified run has 11 run checkpoint rows: 6 done and 5 error; score artifacts cover only the 6 completed rows.
- Other `lme-quick-*-live-20260526` directories are prepared-only or incomplete without a machine-readable status marker.

Recommended path:

- Add `RUN_STATUS.json` with `started_at`, `ended_at`, `exit_code`, expected item count, run rows, score rows, error counts, route snapshot, PID, and lock file.
- Include completeness fields in `score_report.json`.
- Do not allow score summaries to omit expected denominator.

### 10. K8s status is too green for recent reembed failures

Severity: serious. Reported by: operator/SRE.

Locations:

- `infra/status-k8s.sh:40`
- `docs/runbook.md:9`
- `infra/status-k8s.sh:93`

Problem: live `engram-reembed` is currently ready but has 12 restarts, and prior logs showed `litellm embed HTTP 404 Not Found: No openai endpoints available`. `status-k8s` reports ready/available status but does not fail or warn on recent restarts, previous termination errors, or authenticated metric probes.

Recommended path:

- Make `status-k8s` surface restarts, previous termination reason, recent warning events, `engram_chunks_pending_reembed`, DB pool saturation, and metrics auth failures.
- Add a Kubernetes-first incident runbook path.

### 11. Operations docs are Docker-first while live Engram is Kubernetes

Severity: serious/friction. Reported by: operator/SRE, new maintainer.

Locations:

- `docs/operations.md:165`
- `Makefile:143`
- `docs/codex/onboarding.md:52`
- `docs/architecture.md:12`

Problem: the live service is K8s-backed, but primary docs still start with Docker diagnostics and stale subsystem references such as `bench/` and LiteLLM. `make status NO_REMOTE=1` reports Docker containers missing, which is noise for the current live deployment.

Recommended path:

- Split local Docker and live K8s operations clearly.
- Update onboarding/architecture to point to `cmd/longmemeval` and `internal/longmemeval`.
- Mark LiteLLM references as historical or replace them with current Olla/hybrid routing.

### 12. `prune` is authenticated and destructive by default

Severity: medium/high. Reported by: security reviewer.

Location: `cmd/longmemeval/prune.go:239-248`

Problem: `longmemeval prune` defaults to mutating mode, no deletion limit, and auto-discovers an Engram bearer token from Claude config. A bad prefix, TTL drift, or accidental cron invocation can delete projects without explicit per-run credential intent.

Recommended TDD path:

- Make dry-run the default or require `--confirm-prune`.
- Require an explicit positive `--limit` unless `--unlimited` is passed.
- Avoid automatic token discovery for deletion unless explicitly enabled.

### 13. OAI debug logging can spill private context

Severity: medium. Reported by: security reviewer.

Location: `internal/longmemeval/claude.go:24-27`

Problem: `LME_DEBUG_REQUESTS=1` logs the OAI URL and full non-200 response body. LongMemEval prompts include recalled memory context, and upstream error bodies may echo prompt/request material.

Recommended path:

- Redact URL credentials.
- Cap and sanitize response-body excerpts.
- Avoid logging prompt-like fields by default.

### 14. LongMemEval artifacts are group/world-readable by default

Severity: medium. Reported by: security reviewer.

Locations:

- `internal/longmemeval/checkpoint.go:13-15`
- `cmd/longmemeval/score.go:153-171`
- `cmd/longmemeval/sample.go:286`

Problem: checkpoints are written `0644`, and several outputs use `os.Create` with process-umask-derived permissions. Result artifacts can expose retrieved memory IDs, generated hypotheses, scoring explanations, and logs.

Recommended path:

- Create LME output directories with private permissions by default.
- Use `0600` for checkpoints, hypotheses, retrieval logs, score reports, and sample files unless an explicit `--public-artifacts` flag is passed.

### 15. Local ignored `.env*` files are group-readable

Severity: medium. Reported by: security reviewer.

Location: ignored local `.env*` files in `/home/psimmons/projects/engram-go`

Problem: ignored secret-bearing files exist locally and several are group-readable. The review did not print their contents.

Recommended path:

- Tighten local file modes to owner-read/write.
- Add a non-content permission check to the local secret guard.

## Friction / Low

- Shared subcommand help can exit as code 2 on `flag.ErrHelp`, while other subcommands exit 0.
- `score-efficient`, `score-batch`, `sample-*`, and `route-discover` use `flag.ExitOnError` inside `dispatch()`, making wrapper/test behavior inconsistent.
- `score` writes human summary text directly to stdout and has no `--output json|text|quiet` mode.
- `route-discover` intersects model names but does not validate host state, model readiness, port reachability, auth, or purpose beyond excluding embedding-like names.
- `go test ./cmd/longmemeval ./internal/longmemeval -count=1` can invoke `claude --print --model opus`; external-Claude tests should be behind an explicit integration flag.
- The manual lock cleanup docs quote a wildcard: `rm "$XDG_RUNTIME_DIR/lme/backend-*.lock"`, preventing shell expansion.

## High-Confidence Themes

1. Docs and CLI have drifted: command catalog, quick start, scratch TTL, cleanup flags, top-k flags, architecture, and route guidance disagree.
2. LME artifacts need provenance: current result directories do not reliably encode run status, command, binary, SHA, route, endpoint, PID liveness, or completeness.
3. Read-only semantics are not clean: `memory_recall` changes telemetry despite read-only annotation, and LME measurement can change the system being measured.
4. Secret handling needs tightening: API key defaults leak through help, artifacts are permissive, debug logging is too verbose, and local secret files have loose modes.
5. Operational status needs live-K8s signals: readiness alone is not enough when reembed has recent restarts and metrics/backlog are not summarized.

## Suggested Issue Split

1. `lme: prevent API key defaults from appearing in help output`
2. `lme: make all stop on failed run stage and propagate exit codes`
3. `mcp: resolve memory_recall read-only annotation vs telemetry mutation`
4. `lme: make checkpoint corruption and writer failure fail closed`
5. `lme: add completeness/provenance manifest to score reports and run dirs`
6. `docs: refresh LongMemEval command catalog, quick start, scratch TTL, and W6800 route guidance`
7. `ops: make status-k8s warn on restarts, previous crashes, metrics/backlog`
8. `lme: make prune dry-run/limited/explicit by default`
9. `security: make LME artifacts private and redact debug logging`
10. `tests: gate external Claude LongMemEval tests behind explicit integration opt-in`
