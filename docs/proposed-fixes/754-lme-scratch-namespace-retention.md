# Issue #754 — lme: no retention policy for benchmark projects — scratch namespace design

**Severity:** nice-to-have
**Area:** db, lme-tooling
**Status:** Design only — not yet implemented

## Root cause

LME benchmark projects are named `lme-<question_id>` (set in `cmd/longmemeval/ingest.go`) and stored in the global Engram memory store alongside production memories. There is no namespace separation, no TTL, and no automatic expiry. This has two failure modes: (1) benchmark data pollutes production recall queries that iterate all projects; (2) projects accumulate indefinitely unless the operator manually triggers cleanup. The Exp 13 failure (planning to reuse Exp 10 projects that had been auto-deleted) is the concrete incident.

## Repro

```bash
# Run two experiments; observe project accumulation with no expiry mechanism
./bin/longmemeval ingest --out /tmp/exp-10 --data ...
./bin/longmemeval run --out /tmp/exp-10 --no-cleanup --data ...
# lme-<question_id> projects now live in the global store indefinitely

# Production recall query touches all projects — benchmark data may interfere
curl -s http://localhost:8788/mcp -d '{"method":"memory_recall","params":{"query":"test"}}' | jq '.result | length'
# Returns memories from both production and benchmark namespaces
```

## Proposed patch

**Advisory gate recommendation: Option A** — `scratch/lme-*` prefix + TTL retention.

### Step 1: Naming convention change (no schema migration)

```diff
--- a/cmd/longmemeval/ingest.go
+++ b/cmd/longmemeval/ingest.go
@@ -90,7 +90,13 @@ func ingestWorker(cfg *Config, work <-chan longmemeval.Item, out chan<- longmemeval.IngestEntry) {
 	restClient := longmemeval.NewRestClient(cfg.ServerURL, cfg.APIKey)
 	for item := range work {
-		projectName := fmt.Sprintf("lme-%s", item.QuestionID)
+		// Use scratch/ prefix so benchmark projects are namespace-isolated from
+		// production memories. TTL-based cleanup (see Issue #754) can target
+		// scratch/* without touching production projects.
+		runSuffix := cfg.RunID
+		if runSuffix == "" {
+			runSuffix = "untagged"
+		}
+		projectName := fmt.Sprintf("scratch/lme-%s-%s", item.QuestionID, runSuffix)
```

Note: the `scratch/` prefix is a naming convention only. No DB schema change is required for the prefix. Existing projects with the old naming (`lme-<question_id>`) are not migrated — they persist until manually deleted.

### Step 2: TTL retention (schema migration required)

Add a `scratch_expires_at` column to the `projects` table:

```sql
-- Migration: add scratch_expires_at
ALTER TABLE projects ADD COLUMN scratch_expires_at TIMESTAMPTZ;
CREATE INDEX idx_projects_scratch_expires ON projects (scratch_expires_at)
  WHERE scratch_expires_at IS NOT NULL;
```

At project creation time, if the project name starts with `scratch/`, set `scratch_expires_at = NOW() + INTERVAL '72 hours'`:

```diff
--- a/internal/db/projects.go (hypothetical)
+++ b/internal/db/projects.go
@@ -CreateProject
+	var expiresAt *time.Time
+	if strings.HasPrefix(projectName, "scratch/") {
+		t := time.Now().Add(72 * time.Hour)
+		expiresAt = &t
+	}
```

Add a purge job (cron or MCP tool `memory_cleanup`):

```sql
-- Purge expired scratch projects (run daily via pg_cron or external cron)
DELETE FROM projects WHERE scratch_expires_at < NOW();
-- Cascade deletes memories, embeddings in those projects.
```

### Step 3: Recall filter

Ensure production `memory_recall` calls exclude `scratch/*` projects by default. Add an opt-in `include_scratch: true` parameter for benchmark use:

```diff
--- a/internal/mcp/recall.go
+++ b/internal/mcp/recall.go
@@ -RecallHandler
+	// Exclude scratch/* projects from production recall by default.
+	if !params.IncludeScratch {
+		query = query.Where("project NOT LIKE 'scratch/%'")
+	}
```

## TDD scenarios

1. **scratch_prefix_set_on_ingest** — Given a `runRun` invocation with `--run-id abc123`, when ingest creates a project, then the project name is `scratch/lme-<question_id>-abc123`.
2. **scratch_project_gets_expiry** — Given a project created with `scratch/lme-*` prefix, when `CreateProject` is called, then `scratch_expires_at` is set to approximately `NOW() + 72h`.
3. **production_project_no_expiry** — Given a project created without `scratch/` prefix, when `CreateProject` is called, then `scratch_expires_at` is NULL.
4. **production_recall_excludes_scratch** — Given memories in both `scratch/lme-*` and production projects, when `memory_recall` is called without `include_scratch`, then no memories from `scratch/*` appear in results.
5. **purge_job_removes_expired** — Given a `scratch/lme-*` project with `scratch_expires_at < NOW()`, when the purge SQL runs, then the project and its memories are deleted; non-expired scratch projects and all production projects are unaffected.

## Risk notes

- **Breaking change**: Renaming the project prefix from `lme-<question_id>` to `scratch/lme-<question_id>-<run_id>` means existing ingest checkpoints (`checkpoint-ingest.jsonl`) reference old project names. The `run` stage reads project names from the ingest checkpoint — if ingest is re-run with the new naming, old run checkpoints are incompatible. Migration: either re-ingest, or add a `--legacy-project-prefix` flag that generates old-style names.
- **Schema migration**: The `scratch_expires_at` column requires a DB migration. Use the existing `internal/db/migrations/` pattern. The column is nullable; existing rows get NULL (no expiry).
- **Recall filter change**: Excluding `scratch/*` from default recall may break benchmarks that intentionally query benchmark data from the same server. The `include_scratch: true` parameter resolves this.
- **72h TTL**: Chosen to outlast the longest expected experiment run (~35h per the camp state). Adjust via `LME_SCRATCH_TTL_HOURS` env var.

## Rollout

1. Apply DB migration: `scratch_expires_at` column.
2. Deploy updated binary with `scratch/` prefix naming.
3. Add pg_cron job (or external cron) for daily purge: `DELETE FROM projects WHERE scratch_expires_at < NOW()`.
4. Update `results/BENCHMARK-PLAN.md` to document the new naming convention.
5. Existing `lme-<question_id>` projects remain until manually cleaned; they are not affected by the TTL purge (they have `scratch_expires_at = NULL`).

## Out of scope (followups)

- Issue #751 (`--preserve-projects` flag): complement to this design — the TTL gives automatic cleanup for abandoned scratch projects, while `--preserve-projects` lets operators extend beyond the TTL for intentional cross-experiment reuse.
- A `longmemeval projects list` subcommand to show live scratch projects with their expiry times.
- Configurable TTL per experiment via a `--scratch-ttl` flag.
