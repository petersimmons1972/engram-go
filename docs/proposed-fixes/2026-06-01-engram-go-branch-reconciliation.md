# Engram-Go Branch Reconciliation Triage (2026-05-31 snapshot)

Context: this note records the decision for stale and rescued branches identified in
[`#942`](https://github.com/petersimmons1972/engram-go/issues/942), aligned with the
reconciliation snapshot taken 2026-05-31.

## Action decisions

| Branch | Ahead | Behind | Tip commit | Decision | Notes |
| --- | ---: | ---: | --- | --- | --- |
| `lme-campaign-2026-05-19` | +10 | -33 | `feat(lme-scorer): add --scorer-api-key flag` | KEEP | Preserved for review; contains partial campaign wiring and is not ready for merge as-is. |
| `fix/temporal-retrieval-f1-f2` | +2 | -33 | `fix(search): F1 — rank-normalize recency decay` | KEEP | Useful scoring experiment; continue running with reconciliation checks before merge. |
| `codex/agent-p0p1-engram` | +2 | -21 | `Merge remote-tracking origin/main into branch` | KEEP | Utility branch used for Codex-side merge-work; retain to avoid losing unresolved diff context. |
| `codex/lme-codex-execution-20260530` | +2 | -3 | `chore(docs): add global protocol pointer to Codex onboarding` | KEEP | Mostly documentation/process updates with light drift; keep for possible merge in next tidy-up. |
| `chore/rename-litellm-to-engram-router` | +1 | -37 | `chore: rename LITELLM_URL → ENGRAM_ROUTER_URL (closes #636)` | MERGE | Explicit merge-needed rename consolidation; this branch was consolidated from duplicated worktree branches. |
| `rescue/pr920-startup-probe-guard` | +3 | -2 | `fix(embed): guard startup probe EmbedWithModel type assertion` | KEEP | Rescue branch should be retained to preserve fix until main branch merge target is ready. |
| `rescue/verify-881-ops-truth` | +2 | -19 | `wip(rescue): verify-881 working tree (ops status truth + longmemeval)` | KEEP | Rescue branch contains ops-trace recovery notes; keep for continuity. |
| `rescue/verify-commit-881-score-checkpoint` | +2 | -19 | `fix(lme): fail closed on score checkpoint failures` | KEEP | Keep as a working recovery branch while score-checkpoint fixes stabilize. |
| `rescue/orphan-8c23223-docker-heredoc` | n/a | n/a | `8c2322336dc33cfb7dc91e46b03db7c660914014` | KEEP | Preserved orphaned commit branch; should be reviewed for docs/ops utility. |
| `rescue/orphan-46da1f5-db-unique-ids` | n/a | n/a | `46da1f578571b6219cb6c8c9cd48070fda528b6b` | KEEP | Preserved orphaned commit branch; review before deciding merge/rebase. |

## Notes

- No branches were marked `ABANDON` in this triage pass.
- `chore/rename-litellm-to-engram-router` is the only branch assigned `MERGE` and should be pulled into the next feature batch.
- Rescue and orphaned branches are retained per the issue correction note because they are now pinned and were previously at risk of GC.

## Recommended follow-ups

1. Resolve merge prerequisites for `chore/rename-litellm-to-engram-router` (including conflict check).
2. Re-run a fresh snapshot after merge PRs from this pass are accepted or abandoned.
3. For any branch still marked `KEEP` after 14 days, force a second checkpoint with a new `KEEP/MERGE/ABANDON` decision.
