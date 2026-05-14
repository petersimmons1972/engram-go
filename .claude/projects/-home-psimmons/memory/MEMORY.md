# Learning Index

**Last Updated**: 2026-05-14T05:45:44Z
**Session**: 20260514-014544

---

## Recent Activity (Last 7 Days)

- 2026-05-14: feat(hooks): task-specific Engram recall on first user message
- 2026-05-13: docs(claude): extract detail sections to ~/docs/ — Article 028 A1-a
- 2026-05-13: chore(hooks): add per-hook wall-time telemetry — #396 measurement
- 2026-05-13: security(settings): replace bare Bash grant with tool-specific patterns — Article 028
- 2026-05-12: docs(claude): add Core Principles, strengthen Advisory Protocol quality floor
- 2026-05-11: chore: sync MEMORY.md, session-end hook auth fix, settings model move
- 2026-05-11: chore(memory): capture model-selection audit insights — Article 024
- 2026-05-09: chore(memory): capture SIGHUP signal race fix pattern — engram-go#618
- 2026-05-08: docs: complete Stanford HAI AI Index 2026 PDF-to-markdown conversion
- 2026-05-07: chore(memory): capture session 20260508 lessons

**Sessions**: - No recent session files found
**Uncommitted**: ⚠️  2 modified, 0 staged

---

## Infrastructure Health

**Cluster**: ✅ All 9 nodes ready | **Services**: ⚠️  11 OK, 0 failed, 1 warnings
**Warnings**: None detected

Health check: `~/bin/health-check.sh`

**J-2 Intelligence**: J-2: All clear

---

## Key Lessons

Stored in Engram. Recall:
- Homelab patterns: `memory_recall("<topic>", project="homelab")`
- General patterns: `memory_recall("<topic>", project="global")`

Topics: K8s PVCs · Chainguard fsGroup · cert-manager DNS · Cloudflare DNS/cache · BeautifulSoup/SVG · TDD · HTML processing · URL validation · MCP config · WordPress proxy · CronJobs · subagent isolation · validation checklists · Python method shadowing

## Memory Files

- [Landscape claims extraction bugs](feedback_landscape_claims_extraction.md) — 3 Stage 3 failures on first devops_secrets run; Gate 1 segment, int/str claim leak, framework pseudo-claim
- [Worktree branch safety](feedback_worktree_branch_safety.md) — 7 local-only branches found; always push at session close even if not merging
- [Credential in working tree](feedback_credential_in_working_tree.md) — real DB password in research_store.py comment across all worktrees; caught before commit
- [Stage 3 parallel enhancement](project_clearwatch_stage3_parallel.md) — #4803; edr_xdr primary target; needs thread-safety spec before implementation

---

## Engram Offline?

If Engram is unreachable, stage entries in `memory/fallback.md` and flush to Engram on reconnect.

---

**Auto-updated at session start by `~/bin/generate-session-context.py`**
