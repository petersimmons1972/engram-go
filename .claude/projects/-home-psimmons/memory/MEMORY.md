# Learning Index

**Last Updated**: 2026-05-09T12:09:17Z
**Session**: 20260509-080917

---

## Recent Activity (Last 7 Days)

- 2026-05-08: docs: complete Stanford HAI AI Index 2026 PDF-to-markdown conversion
- 2026-05-07: chore(memory): capture session 20260508 lessons
- 2026-05-07: chore(memory): capture session 20260507 lessons + Clearwatch Go port state
- 2026-05-07: chore(memory): capture local GPU routing + Clearwatch batch state
- 2026-05-06: chore: sync session memory
- 2026-05-06: fix(hooks): fallback to Infisical backup key on auth failure — engram-go#614 #615 #616
- 2026-05-06: chore: sync CLAUDE.md and session memory files
- 2026-05-06: chore: sync armies roster, hooks, litellm config, AGENTS.md
- 2026-05-06: fix(hooks): prevent Engram MCP silent blocks — engram-go#408
- 2026-05-06: chore: reconcile history — github/master was squash merge of local commits (engram-go#399 flock fix now applied separately)

**Sessions**: - No recent session files found
**Uncommitted**: ⚠️  3 modified, 0 staged

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

Topics: K8s PVCs · Chainguard fsGroup · cert-manager DNS · Cloudflare DNS/cache · BeautifulSoup/SVG · TDD · HTML processing · URL validation · MCP config · WordPress proxy · CronJobs · subagent isolation · validation checklists · Python method shadowing · Go signal test races

- [SIGHUP test races](feedback_sighup_test_races.md) — `ready` channel + `<-goroutineDone` pattern for Go tests that send signals to themselves

---

## Engram Offline?

If Engram is unreachable, stage entries in `memory/fallback.md` and flush to Engram on reconnect.

---

**Auto-updated at session start by `~/bin/generate-session-context.py`**
