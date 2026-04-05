# Learning Index

**Last Updated**: 2026-04-05T04:15:53Z
**Session**: 20260405-001553

---

## Recent Activity (Last 7 Days)

- 2026-04-05: ops: archive 15 more generals — 42→28 active agents
- 2026-04-04: ops: archive 14 more generals — 56→42 active agents
- 2026-04-04: ops: archive 8 redundant generals — 64→56 active agents
- 2026-04-03: docs: implementation plan for longhorn-nfs migration — 11 tasks across 3 phases
- 2026-04-03: docs: spec self-review fixes — Qdrant in A.1 table, TrueNAS image note, snapshot retention clarity
- 2026-04-03: docs: longhorn-nfs migration design spec — Longhorn→NFS + DB consolidation
- 2026-04-03: ops: calypso15 integration — adversarial review protocols and stall detection
- 2026-04-01: ops: make Engram recall mandatory — dedicated section with explicit triggers
- 2026-04-01: chore: remove stale memory topic file references — homelab files now in Engram
- 2026-04-01: chore: sync local skills — add 5 new, remove 3 stale, broaden adversarial-source-check

**Sessions**: - No recent session files found
**Uncommitted**: ⚠️  3 modified, 0 staged

---

## Infrastructure Health

**Cluster**: ✅ All 9 nodes ready | **Services**: ⚠️  8 OK, 0 failed, 2 warnings
**Warnings**: None detected

Health check: `~/bin/health-check.sh`

**J-2 Intelligence**: J-2 INTEL [2026-04-05T04:00]: Pods In Error State (WARNING, 43 patrol(s))

---

## Key Lessons

Stored in Engram. Recall:
- Homelab patterns: `memory_recall("<topic>", project="homelab")`
- General patterns: `memory_recall("<topic>", project="global")`

Topics: K8s PVCs · Chainguard fsGroup · cert-manager DNS · Cloudflare DNS/cache · BeautifulSoup/SVG · TDD · HTML processing · URL validation · MCP config · WordPress proxy · CronJobs · subagent isolation · validation checklists · Python method shadowing

---

## Topic Files

- Homelab quick reference (triage, fixes, warnings, anti-patterns) → memory/homelab-quick-reference.md
- cert-manager patterns → memory/homelab-cert-manager.md
- K8s deployment patterns → memory/homelab-k8s-patterns.md
- Incident summaries → memory/homelab-incidents.md
- URL validation patterns → memory/url-validation-patterns.md
- Chart regression analysis → memory/chart-regression-2026-02-06.md
- HTML processing patterns → memory/html-processing-patterns.md
- Projects catalog → [PROJECTS-CATALOG.md](/home/psimmons/PROJECTS-CATALOG.md)
- Generals Accountability System → memory/generals-accountability-system.md

---

**Auto-updated at session start by `~/bin/generate-session-context.py`**
