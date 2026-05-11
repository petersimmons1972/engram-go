# Learning Index

**Last Updated**: 2026-05-11T16:55:20Z
**Session**: 20260511-125520

---

## Recent Activity (Last 7 Days)

- 2026-05-09: chore(memory): capture SIGHUP signal race fix pattern — engram-go#618
- 2026-05-08: docs: complete Stanford HAI AI Index 2026 PDF-to-markdown conversion
- 2026-05-07: chore(memory): capture session 20260508 lessons
- 2026-05-07: chore(memory): capture session 20260507 lessons + Clearwatch Go port state
- 2026-05-07: chore(memory): capture local GPU routing + Clearwatch batch state
- 2026-05-06: chore: sync session memory
- 2026-05-06: fix(hooks): fallback to Infisical backup key on auth failure — engram-go#614 #615 #616
- 2026-05-06: chore: sync CLAUDE.md and session memory files
- 2026-05-06: chore: sync armies roster, hooks, litellm config, AGENTS.md
- 2026-05-06: fix(hooks): prevent Engram MCP silent blocks — engram-go#408

**Sessions**: - No recent session files found
**Uncommitted**: ⚠️  3 modified, 0 staged

---

## Infrastructure Health

**Cluster**: ✅ All 9 nodes ready | **Services**: ⚠️  10 OK, 0 failed, 2 warnings
**Warnings**: None detected

Health check: `~/bin/health-check.sh`

**J-2 Intelligence**: J-2: All clear

---

## Substack (Clearwatch Research)

- [Publication state](project_substack_state.md) — posts 021-026 live, artwork 027-036 committed, post IDs, daily cadence
- [Workflow feedback](feedback_substack_workflow.md) — no uninstructed browser opens; artwork and publishing are separate steps

## Key Lessons

Stored in Engram. Recall:
- Homelab patterns: `memory_recall("<topic>", project="homelab")`
- General patterns: `memory_recall("<topic>", project="global")`

Topics: K8s PVCs · Chainguard fsGroup · cert-manager DNS · Cloudflare DNS/cache · BeautifulSoup/SVG · TDD · HTML processing · URL validation · MCP config · WordPress proxy · CronJobs · subagent isolation · validation checklists · Python method shadowing

## Model Selection & Cost

- [Route health_check/mechanical turns to Haiku](feedback_haiku_for_health_checks.md) — 10% of Sonnet turns are Haiku-appropriate; health_check is the top miss pattern
- [Substack model-selection series (Articles 024–025+)](project_substack_model_selection_series.md) — free articles through 024 posted 2026-05-11; audit scratch at `~/.claude/experiments/2026-05-11-024-model-overspend/`

---

## Engram Offline?

If Engram is unreachable, stage entries in `memory/fallback.md` and flush to Engram on reconnect.

---

**Auto-updated at session start by `~/bin/generate-session-context.py`**
