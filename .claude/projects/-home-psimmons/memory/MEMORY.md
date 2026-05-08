# Learning Index

**Last Updated**: 2026-05-08T00:34:11Z
**Session**: 20260507-203411

---

## Recent Activity (Last 7 Days)

- 2026-05-07: chore(memory): capture session 20260507 lessons + Clearwatch Go port state
- 2026-05-07: chore(memory): capture local GPU routing + Clearwatch batch state
- 2026-05-06: chore: sync session memory
- 2026-05-06: fix(hooks): fallback to Infisical backup key on auth failure — engram-go#614 #615 #616
- 2026-05-06: chore: sync CLAUDE.md and session memory files
- 2026-05-06: chore: sync armies roster, hooks, litellm config, AGENTS.md
- 2026-05-06: fix(hooks): prevent Engram MCP silent blocks — engram-go#408
- 2026-05-06: chore: reconcile history — github/master was squash merge of local commits (engram-go#399 flock fix now applied separately)
- 2026-05-06: fix(engram-session-recall): add flock on MEMORY.md — engram-go#399
- 2026-05-06: fix(engram-health-check): replace exit 1 with systemMessage — engram-go#408

**Sessions**: - No recent session files found
**Uncommitted**: ⚠️  2 modified, 0 staged

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

## Local Memory Files

- [Oblivion/Spark state](project_oblivion_spark_state.md) — GPU budget, running services, gpt-oss-20b paused, reembed rebuild in progress
- [tiktoken aarch64 fix](feedback_tiktoken_aarch64.md) — TIKTOKEN_RS_CACHE_DIR must be /root/.cache/tiktoken-rs-cache on ARM64
- [engram reembed dims](feedback_engram_reembed_dims.md) — ENGRAM_EMBED_DIMENSIONS=0 required when using vLLM embedding backend

---

## Engram Offline?

If Engram is unreachable, stage entries in `memory/fallback.md` and flush to Engram on reconnect.

---

**Auto-updated at session start by `~/bin/generate-session-context.py`**
