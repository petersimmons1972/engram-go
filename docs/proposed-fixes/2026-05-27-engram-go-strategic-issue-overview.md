# Engram-Go Strategic Issue Overview - 2026-05-27

This note records the issue sweep after resolving PR #893 and the rapid LongMemEval follow-up queue.

## Current outcome

Merged and closed:

- #893, #887, #891: hardened LongMemEval command failure behavior.
- #889: ignored root `.env*` files now fail secret checks when group/world-readable.
- #864, #865, #866, #868, #870: verified already-fixed reliability defects and closed with evidence.
- #867, #885: Kubernetes status truth and runbook hardening.
- #881, #882: fail closed on score checkpoint write failures.
- #888: private permissions for LongMemEval artifacts.
- #892: CLI score output and route-discovery consistency.
- #848, #874: target-date reranking and named recall repair preset.
- #847, #871, #872, #873, #875: verified stale/resolved and closed with evidence.

Still open after the sweep:

- #890: LongMemEval docs and W6800 routing guidance.
- #886: destructive prune defaults.
- #884 and #883: run provenance, completeness, and stale result-local scripts.
- #880: guard temporal date-window recall when `valid_from` is absent or untrusted.
- #879: `memory_recall` read-only annotation conflicts with telemetry mutation.
- #738: decommission source Postgres container after soak.
- #396: long-running hook daemon.

## Rapid/minor lane

These are small enough for a single focused PR and do not require new architecture.

### #890 - Documentation refresh

Classification: agent-sized documentation task.

Recommended assignment:

- Refresh `cmd/README.md` with `longmemeval` and current subcommands.
- Update `docs/lme-benchmark-learnings.md` around `--cleanup-policy`, score-only reuse, `score-efficient`, `route-discover`, and current top-k flags.
- Add copy-paste W6800/Olla verification commands to the runbook.
- Mark superseded design docs as historical rather than deleting them.

Stop condition: docs match `longmemeval --help` and current route-discovery behavior; no code behavior changes.

## Safety-critical immediate lane

These should be treated as release-blocking because they can mutate data or violate client permission expectations.

### #886 - Prune must be explicit, limited, and dry-run safe

Problem: `longmemeval prune` can mutate by default, use an implicit token, and delete without a positive limit.

Competing options:

- Option A: make `--dry-run` default and require `--execute`. Simple and safe, but scripts must change.
- Option B: keep current command shape but require `--confirm-prune <project-prefix>` and `--limit N`. Less disruptive, but easy to grow exceptions.
- Option C: split commands into `prune-plan` and `prune-apply`. Cleanest mental model, more CLI churn.

Recommended synthesis:

- Default to no mutation unless `--execute` is present.
- Require `--confirm-prefix <prefix>` matching the prune prefix for mutation.
- Require `--limit N > 0` unless `--unlimited` is present.
- Do not use auto-discovered Claude/MCP tokens for mutation unless `--use-default-token` is explicit.
- Keep `prune --dry-run` compatibility as an alias of the default plan mode.

Verification:

- Tests for default no-mutation behavior.
- Tests for confirmation mismatch.
- Tests for positive limit enforcement.
- Tests that implicit token discovery is rejected for mutation.

### #879 - Split read-only recall from telemetry-producing recall

Problem: `memory_recall` is advertised as read-only but can record retrieval telemetry and increment `times_retrieved`.

Competing options:

- Option A: add `record_event=false` to `memory_recall`. Low churn, but the same tool has two permission modes.
- Option B: split tools into `memory_recall` and `memory_recall_with_feedback`. Clear permission model, but client docs and call sites change.
- Option C: keep one tool and change annotation from read-only to mutating. Honest but worse for planning clients and benchmark measurement.

Recommended synthesis:

- Keep `memory_recall` genuinely read-only by default.
- Add an explicit mutating recall path, either as `memory_recall_recording` or a clearly named internal/API option.
- Make LongMemEval use the no-side-effect path by default.
- Gate telemetry-producing recall behind an explicit opt-in.

Verification:

- Regression test that read-only recall does not insert retrieval events.
- Regression test that read-only recall does not increment `times_retrieved`.
- LongMemEval client test proving date-window recall uses the no-side-effect mode.

## Design/refactoring lane

These issues are tractable, but the wrong fix would lock in bad semantics.

### #884 and #883 - Run provenance, completeness, and result-local scripts

Problem: result directories can look like benchmark outcomes even when they are partial, terminated, prepared-only, or dependent on stale local scripts.

Competing options:

- Option A: add `RUN_STATUS.json` only. Fast and useful, but leaves stale scripts ambiguous.
- Option B: remove or rewrite result-local scripts. Cleaner tree, but risks rewriting historical artifacts.
- Option C: introduce a run manifest and treat result-local scripts as historical unless generated by current code.

Recommended synthesis:

- Add a compact `RUN_STATUS.json` / `run_manifest.json` written by current LongMemEval commands.
- Include git SHA, dirty flag, binary path, command line, route snapshot, expected counts, completed counts, error counts, start/end time, and exit status.
- Update `score_report.json` to include expected denominator and completeness status.
- Mark old result-local scripts as historical artifacts in docs; move maintained entrypoints under `scripts/` or CLI docs.
- Do not rewrite historical result directories unless a separate archival cleanup is approved.

Verification:

- Unit tests for manifest completeness across `prepare`, `ingest`, `run`, and `score` paths.
- Fixture test where partial run output cannot be summarized as a complete benchmark.
- Documentation update tying maintained wrappers to current CLI help.

### #880 - Temporal recall when `valid_from` is missing

Problem: date-window retrieval now uses `valid_from`, but hard filtering can drop the correct memory if the store path omitted or mis-stamped `valid_from`.

Competing options:

- Option A: hard filter only. Highest precision when metadata is complete; brittle when metadata is missing.
- Option B: soft boost instead of filter. More recall, but more noisy temporal contexts.
- Option C: guarded hard filter with fallback. Use hard filter only when date coverage is trustworthy.

Recommended synthesis:

- Keep hard date filtering when the corpus has verified `valid_from` coverage for the project/run.
- If coverage is incomplete or unknown, perform a two-lane recall: date-filtered candidates first, plus a small unfiltered safety lane.
- Expose the completeness decision in run artifacts so benchmark results explain which recall mode ran.
- Treat missing `valid_from` as a data-quality signal, not silently as exclusion.

Verification:

- Fixture where the correct memory lacks `valid_from`; current hard-filter-only behavior would drop it.
- Test that guarded mode retains a bounded fallback candidate.
- Test that complete metadata still uses strict half-open date filtering.

## Hold / deferred lane

### #738 - Source Postgres decommission

Classification: hold until live soak evidence exists and explicit destructive approval is given.

Do not run destructive Docker, volume, or database operations from this issue without a fresh approval in the active turn. Before action, identify affected container, database, schema, tables, volumes, backups, and rollback path.

### #396 - Hook daemon

Classification: strategic design, not urgent implementation.

The daemon may be worthwhile, but only after measuring current hook latency, failure rate, and race incidence. The first task should be instrumentation and a small prototype behind `ENGRAM_HOOK_DAEMON=1`, not replacement of the shell hooks.

Recommended path:

- Measure current hooks over real sessions.
- Build a minimal socket daemon for one low-risk hook.
- Keep shell scripts as fallback.
- Promote only after observed reliability and latency improve.

## Suggested execution order

1. #886: fix prune safety first.
2. #879: repair read-only semantics before more benchmark runs mutate recall state.
3. #890: dispatch documentation refresh in parallel with code work.
4. #884/#883: implement run manifests and completeness status.
5. #880: design and implement guarded temporal recall.
6. #396: instrument first, then prototype.
7. #738: wait for soak evidence and explicit approval.
