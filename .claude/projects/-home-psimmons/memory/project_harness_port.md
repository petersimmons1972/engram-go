---
name: project-harness-port
description: Port of selected ideas from rocklambros/harness-engineering into local Claude Code setup; three sub-projects; JOURNEY.md and untangle detour landed 2026-05-20
metadata:
  type: project
originSessionId: home-dir-untangle-2026-05-20
---
**State (2026-05-20).** Repo at `~/projects/harness-port/`, pushed to private GitHub `petersimmons1972/harness-port`. `main` at `b60674f`. Three sub-projects + one detour completed:

- **Project A — Process layer refactor.** SPEC at `docs/superpowers/specs/2026-05-19-project-a-process-layer-design.md` (98f2ea1). PLAN at `docs/superpowers/plans/2026-05-19-project-a-process-layer-plan.md` (2ba50fa). 8 stages, 42 TDD test cases. **NOT YET EXECUTED.**
- **Project B — Hooks migration.** Deferred.
- **Project C — Cache & context audit.** Deferred.
- **Detour 2026-05-20 — Home-dir untangle.** Plan at `~/.claude/plans/write-up-the-plan-lexical-eich.md`. Executed cleanly: merged github (with -X ours for a duplicate commit from another machine), removed trunas remote, filed audit issue. JOURNEY.md anchors the reasoning trail. See [[feedback-tdd-invariants-for-git-ops]] for the framework that worked.

**Open issues on the repo:** #1 (audit + possibly delete legacy bare repo at /mnt/public/git-repos/home.git).

**Vocabulary that lands when Project A executes:** QC.1-QC.7, AP.1-AP.12 (gaps at AP.3 + AP.10), ADV.1-ADV.5 renamed from existing A1-A5.

**How to apply:** When asked about harness-port work, default state is "Project A planned + ready, untangle detour done." Cross-reference [[reference-rocklambros-harness-engineering]] for source material.
