---
name: project-harness-port
description: New project porting selected ideas from rocklambros/harness-engineering into local Claude Code setup; three sub-projects, GitHub private repo
metadata:
  type: project
originSessionId: harness-port-2026-05-19
---
**Fact:** Created 2026-05-19. Repo at `~/projects/harness-port/`, pushed to private GitHub repo `petersimmons1972/harness-port`. Three sub-projects decomposed:

- **Project A — Process layer refactor.** QC.1-QC.7 + AP.1-AP.12 (gaps AP.3/AP.10) + ADV.1-ADV.5 vocabulary, commit rationale template, removal-test pass, two principle docs in `~/docs/` symlinked from harness-port, commit-msg validator + line-budget linter + tested uninstall. Spec at `docs/superpowers/specs/2026-05-19-project-a-process-layer-design.md` (commit 98f2ea1). Plan at `docs/superpowers/plans/2026-05-19-project-a-process-layer-plan.md` (commit 2ba50fa). 8 stages, 42 TDD test cases across 4 bats suites. NOT YET EXECUTED.
- **Project B — Hooks migration.** Deferred. Move Pre-Flight Steps 1 & 4 to deterministic hooks; Semgrep PostToolUse on Write/Edit feeding findings back in-session.
- **Project C — Cache & context audit.** Deferred. Audit Anthropic SDK callers (Engram clients, agent dispatch, Cassandre) for explicit `cache_control.ttl="1h"`. Count CLAUDE.md hierarchy total.

**Why:** Reading rocklambros's harness-engineering surfaced 8 portable ideas. One mixed project would tangle advisory edits, hook engineering, and SDK audits. Decomposition keeps each sub-project shaped for one spec → one plan → one implementation cycle.

**How to apply:** When asked about harness-port work, check current state in [[reference-harness-port-locations]]. Sub-project A must ship before B (defines vocabulary B cites). C is parallel-eligible with B. Cross-reference [[reference-rocklambros-harness-engineering]] for source material.
