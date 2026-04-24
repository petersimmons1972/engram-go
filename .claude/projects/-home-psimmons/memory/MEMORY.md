# Learning Index

**Last Updated**: 2026-04-24T01:24:34Z
**Session**: 20260423-212434

---

## Recent Activity (Last 7 Days)

- 2026-04-23: fix: prune stale bench profiles from ~/.claude/agents/ on sync
- 2026-04-23: feat: add PreCompact hook to store session snapshot to engram before compaction
- 2026-04-23: docs: telemetry sink design spec + Go-default preference
- 2026-04-22: remove security-program-mcp — moved to repo at ~/projects/security-program/ops/mcp-launcher.sh
- 2026-04-22: add security-program MCP launcher wrapper
- 2026-04-17: chore: memory janitor compression + health-check updates
- 2026-04-17: docs: engram-go vs. Iusztin GraphRAG comparison report + workflow memory
- 2026-04-17: feat: Opus 4.7 migration — Tier 1 literal-interpretation fixes
- 2026-04-17: chore: snapshot rickover-validator.md as migration baseline
- 2026-04-17: chore: snapshot live CLAUDE.md into migration branch

**Sessions**: - No recent session files found
**Uncommitted**: ⚠️  41 modified, 0 staged

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
