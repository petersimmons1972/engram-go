> **SUPERSEDED** — Implemented in PR #807 (merged 2026-05-20). This document retained for design context only.

# Issue #751 — lme: project auto-cleanup deletes ingested projects before cross-experiment reuse is possible

**Severity:** nice-to-have
**Area:** lme-tooling
**Status:** Design only — not yet implemented

## Root cause

`cmd/longmemeval/run.go:170-175` deletes each item's Engram project immediately after the run phase completes, unless `cfg.NoCleanup` is true. The default is `NoCleanup: false` (delete). The existing opt-out flag `--no-cleanup` uses a double-negative name that does not signal the intent "I want to reuse these projects later." Exp 13 planned to reuse Exp 10's ingested projects for a different recall configuration run, but they had been auto-deleted, requiring a full re-ingest.

## Repro

```bash
# Run experiment, then try to reference the same projects in a follow-up
./bin/longmemeval run \
  --data testdata/longmemeval/longmemeval_m_cleaned.json \
  --out results/exp-10 \
  --llm-url http://oblivion:8000/v1 --llm-model inference
# Projects deleted on completion (default behaviour)

# Follow-up experiment expecting same projects — fails because projects are gone
./bin/longmemeval run \
  --data testdata/longmemeval/longmemeval_m_cleaned.json \
  --out results/exp-13 \
  --llm-url http://oblivion:8000/v1 --llm-model inference
# Error: project not found for each item
```

## Proposed patch

**Advisory gate recommendation: Option B** — add `--preserve-projects` as an alias/replacement for `--no-cleanup`. Keep delete as default; do not flip.

```diff
--- a/cmd/longmemeval/main.go
+++ b/cmd/longmemeval/main.go
@@ -27,6 +27,7 @@ type Config struct {
 	NoCleanup  bool
+	PreserveProjects bool // alias for NoCleanup with clearer semantics
```

```diff
@@ -111,6 +112,9 @@ func dispatch(...) int {
 	fs.BoolVar(&cfg.NoCleanup, "no-cleanup", false, "Skip Engram project deletion after run stage (deprecated: use --preserve-projects)")
+	fs.BoolVar(&cfg.PreserveProjects, "preserve-projects", false,
+		"Preserve Engram projects after run stage for reuse in follow-up experiments.\n"+
+		"Without this flag, projects are deleted after each item completes.\n"+
+		"Combine with --out <dir> to resume from an existing ingest checkpoint.")
```

```diff
--- a/cmd/longmemeval/run.go
+++ b/cmd/longmemeval/run.go
@@ -170,7 +170,7 @@ func runWorker(...) {
-			if !cfg.NoCleanup {
+			if !cfg.NoCleanup && !cfg.PreserveProjects {
 				if err := mcpClient.DeleteProject(ctx, ingestEntry.Project); err != nil {
```

Also add a log line when preservation is active so the operator knows cleanup is being skipped intentionally:

```diff
+	if cfg.PreserveProjects || cfg.NoCleanup {
+		log.Printf("run: projects will be preserved after run (--preserve-projects / --no-cleanup)")
+	}
```

## TDD scenarios

1. **preserve_projects_skips_delete** — Given `--preserve-projects`, when `runWorker` completes an item, then `mcpClient.DeleteProject` is NOT called for that item's project.
2. **default_deletes_project** — Given neither `--preserve-projects` nor `--no-cleanup`, when `runWorker` completes an item, then `mcpClient.DeleteProject` IS called.
3. **no_cleanup_still_works** — Given `--no-cleanup` (legacy flag), when `runWorker` completes an item, then `mcpClient.DeleteProject` is NOT called (backwards compat).
4. **both_flags_safe** — Given both `--preserve-projects` and `--no-cleanup`, when `runWorker` runs, then no panic or double-skip occurs; projects are preserved.

## Risk notes

- `--no-cleanup` is preserved for backwards compat; existing camp pipeline scripts (`results/lme-camp-*/run-pipeline.sh`) continue to work unchanged.
- The `--preserve-projects` flag does not manage project accumulation. Over many runs, projects will accumulate in the Engram namespace. See issue #754 for the companion scratch-namespace retention design.
- No schema changes required.

## Rollout

Rebuild binary. Update `results/BENCHMARK-PLAN.md` and `results/lme-camp-report.md` to use `--preserve-projects` in all future experiment commands. No infra changes.

## Out of scope (followups)

- Issue #754 (scratch namespace + TTL) addresses the accumulation problem that `--preserve-projects` creates if used frequently.
- Consider whether `all` subcommand (ingest → run → score) should default to `--preserve-projects=true` since the user has just invested in ingestion.
