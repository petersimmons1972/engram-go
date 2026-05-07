# Learning Index

**Last Updated**: 2026-05-07T14:41:18Z
**Session**: 20260507-104118

---

## Recent Activity (Last 7 Days)

- 2026-05-07: chore(memory): capture local GPU routing + Clearwatch batch state
- 2026-05-06: chore: sync session memory
- 2026-05-06: fix(hooks): fallback to Infisical backup key on auth failure — engram-go#614 #615 #616
- 2026-05-06: chore: sync CLAUDE.md and session memory files
- 2026-05-06: chore: sync armies roster, hooks, litellm config, AGENTS.md
- 2026-05-06: fix(hooks): prevent Engram MCP silent blocks — engram-go#408
- 2026-05-06: chore: reconcile history — github/master was squash merge of local commits (engram-go#399 flock fix now applied separately)
- 2026-05-06: fix(engram-session-recall): add flock on MEMORY.md — engram-go#399
- 2026-05-06: fix(engram-health-check): replace exit 1 with systemMessage — engram-go#408
- 2026-05-06: fix(hooks): prevent Engram MCP silent blocks — engram-go#408

**Sessions**: - No recent session files found
**Uncommitted**: ⚠️  2 modified, 0 staged

---

## Infrastructure Health

**Cluster**: ✅ All 9 nodes ready | **Services**: ⚠️  9 OK, 0 failed, 2 warnings
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

## Engram Session Recall

**1.** Infisical is being recommended as a tool for securely using environmental variables.
   *tags: practitioner, daily-digest, automated | score: 0.79*

**2.** Miss's searches yielded no results for expected context and implementation information about the current project and engram token refresh hook.
   *tags: retrieval-miss | score: 0.63*

**3.** Infisical's free tier and Doppler's free tier offer generous terms that cover unlimited secrets for up to 5 members, while Akeyless requires a paid plan after individual developer use.
   *tags: section:stage_1, pair:devops_secrets, canonical | score: 0.59*

**4.** As your company grows, customers will expect robust security controls, including SOC 2 reports, which demand proof of access controls, audit trails, and regular credential rotation, affecting how you budget for secrets management.
   *tags: section:stage_2, pair:devops_secrets, canonical | score: 0.55*

**5.** The key decision was assigning roles for the Triad security program, where Peter assigned Butler=Attacker, Dornberger=Defender, and Bradley=Coordinator/Auditor to create a structured adversarial approach.
   *tags: security-program, pending, founder-interview-needed, triad-roles | score: 0.50*

---

## Workflow guardrails

- [Don't reach for --no-verify](feedback_no_verify_bypass.md) — two failed bypasses in one session; underlying cause is almost always a buggy test or hook
- [Deterministic Python beats LLMs for structured transcription](feedback_python_beats_llm_for_transcription.md) — total mappings → script, not model
- [Safety hooks have no theory of mind](feedback_safety_hook_handoff.md) — when wrong on the merits, hand off to user, don't construct workarounds

## Tooling

- [Olla local LLM proxy](reference_olla_proxy.md) — localhost:40114, qwen3-coder:30b, OpenAI-compatible

## Project state

- [Clearwatch devops_secrets Go port](project_clearwatch_devops_secrets_go_port.md) — shipped 2026-05-07; #4707 (Phase 1b DB read), #4713 (test path bug) outstanding
