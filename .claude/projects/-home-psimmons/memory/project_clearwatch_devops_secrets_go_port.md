---
name: Clearwatch devops_secrets Go port — state and follow-ups
description: Phase 0/1a/2/3 shipped 2026-05-07; Phase 1b deferred to #4707; vendor_facts is now source of truth
type: project
originSessionId: e43a9d4f-1d9c-483a-8828-b09b33374056
---
The devops_secrets segment migrated from Python/Bash to Go on 2026-05-07 (PR #4708 merged as `3d5ca4e6`, schedule shift PR #4714 as `e3a260b5`).

**Architecture:** `vendor_facts` table is the canonical source of vendor knowledge for the segment (450 rows seeded by `cmd/devops-secrets-seed`, written through the rules-validated `postgres.UpsertVendorFact` path). The Python `pipeline/devops_secrets_dossier.py:VENDORS` literal still exists as the input the seed transcribes from, but downstream readers should query `vendor_facts`, not the Python literal.

**Why:** User decision during plan phase — chose `vendor_facts` over embedded structs/YAML to avoid duplication and let new vendors be added straight into the table without recompiling.

**How to apply:** When extending the devops_secrets segment (new vendors, new fact types, new charts), write to vendor_facts; do not extend the Python `VENDORS` literal or `internal/dossier/devopssecrets/seed.go` (the latter is a transcription fixture deleted in the cleanup PR). The Go builder at `internal/dossier/devopssecrets/Build()` currently reads from an embedded snapshot (`testdata/vendors_curated.json`) — Phase 1b will swap that for a `vendor_facts` read.

**Outstanding work:**
- **#4707 (Phase 1b):** Go builder DB read path. Blocked on lack of testcontainers/dockertest harness in repo; without one, the round-trip equivalence test can't be written. Prerequisite is that test infra, not the read path itself.
- **#4713:** `tests/unit/test_adversarial_data_integrity.py` uses hardcoded absolute paths to `/home/psimmons/projects/clearwatch/bin/...` — false-fails pre-push from any worktree when master happens to be on a divergent branch. Fix is `Path(__file__).resolve().parents[2] / "bin" / ...`.

**Live K8s state:** `clearwatch-research/collector-devops-secrets-go` CronJob at `0 6 * * *` (active). Python `collector-devops-secrets` suspended. Image `registry.petersimmons.com/clearwatch/collector:latest` (and `:b8e52eef`). First validation run added 4,261 items across 11 vendors with `partial` status on the last 4 (Reddit 429 rate-limit — same ceiling the Python collector hits).

**Bash deprecation shim:** `bin/run-devops-secrets-report.sh` exec's `clearwatch generate --segment=devops_secrets`. Remove ~30 days after 2026-05-07 (≈ 2026-06-06) once any cron/runbook references have migrated.
