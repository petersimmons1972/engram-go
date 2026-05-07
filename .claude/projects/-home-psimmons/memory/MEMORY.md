# Learning Index

**Last Updated**: 2026-05-07T02:08:20Z
**Session**: 20260506-220820

---

## Recent Activity (Last 7 Days)

- 2026-05-06: fix(hooks): fallback to Infisical backup key on auth failure — engram-go#614 #615 #616
- 2026-05-06: chore: sync CLAUDE.md and session memory files
- 2026-05-06: chore: sync armies roster, hooks, litellm config, AGENTS.md
- 2026-05-06: fix(hooks): prevent Engram MCP silent blocks — engram-go#408
- 2026-05-06: chore: reconcile history — github/master was squash merge of local commits (engram-go#399 flock fix now applied separately)
- 2026-05-06: fix(engram-session-recall): add flock on MEMORY.md — engram-go#399
- 2026-05-06: fix(engram-health-check): replace exit 1 with systemMessage — engram-go#408
- 2026-05-06: fix(hooks): prevent Engram MCP silent blocks — engram-go#408
- 2026-05-04: refactor(claude-md): compress sections + bench 15 unused skills
- 2026-05-04: feat(hooks): add engram-health-check to detect connectivity loss

**Sessions**: - No recent session files found
**Uncommitted**: ⚠️  1 modified, 0 staged

---

## Infrastructure Health

**Cluster**: ✅ All 9 nodes ready | **Services**: ⚠️  10 OK, 0 failed, 2 warnings
**Warnings**: None detected

Health check: `~/bin/health-check.sh`

**J-2 Intelligence**: J-2: All clear

---

## Key Lessons

Stored in Engram. Recall:
- Homelab patterns: `memory_recall("<topic>", project="homelab")`
- General patterns: `memory_recall("<topic>", project="global")`

Topics: K8s PVCs · Chainguard fsGroup · cert-manager DNS · Cloudflare DNS/cache · BeautifulSoup/SVG · TDD · HTML processing · URL validation · MCP config · WordPress proxy · CronJobs · subagent isolation · validation checklists · Python method shadowing

---

## Engram Offline?

If Engram is unreachable, stage entries in `memory/fallback.md` and flush to Engram on reconnect.

---

**Auto-updated at session start by `~/bin/generate-session-context.py`**
