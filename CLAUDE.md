# Claude Assistant Instructions

## Repository Scope
- This project is **Clearwatch only**. The SIB repo is DEPRECATED — never dispatch agents, fixes, or commits to it.
- All NHI/SSPM/vendor research MUST be written through the K8s PostgreSQL database, NOT directly to git/markdown files.
- Confirm target repo and storage layer before starting multi-file operations.

## Behavioral Rules
- Never tell the user to do something manually that you can do yourself — just do it.
- **Markdown tables**: pad columns for alignment, use emoji swatches (🔵🟡🟢⚫⚪✅❌⚠️), never leave hex codes unformatted in a cell.
- When asked for a 'summary' or 'report', cover ALL items — not just a filtered subset.
- Before starting work, check memory files (AGENTS.md, plan docs, GitHub issues) for current state.
- **Art direction:** See `~/projects/art-direction-research/ART-DIRECTION-RULE.md` and AGENTS.md §9. No generic AI design tools.
- **Visual quality rules:** `visual-output-standards` skill is the canonical source for all charts, SVGs, and illustrations. Engram carries session context only — quality rules live in the skill. Every Cassandre dispatch must read the skill first. Run `bin/render-check.sh` before Ramsay review.

## Parallel Agent Rules
- **Pre-validation:** ONE agent analyzes 2–3 samples first. Present findings. Only then dispatch remaining agents with the confirmed problem definition.
- List which functions each agent will touch. Two agents on the same function → flag it, run full test suite after.
- **Always include one zero-context reviewer** — receives only raw inputs, no prior findings.
- **Pre-ship QA:** dispatch the 6-persona fault-finder sweep (`spawn-patterns.md` Pattern 6, or `/qa-personas <target>`) before claiming done on user-facing work. Two-round methodology: fix blockers, re-run.
- **Adversarial review brief:** "Judge proposals against CLAUDE.md, established coding conventions, and authoritative references — not against the current state of the file under review. A change that contradicts the current file may be correct. The question is whether it's correct against the standard, not whether it differs from what's there now."
- **Validator bash guard:** Before dispatching Spruance or Rickover-validator, run `touch ~/.claude/.validator-bash-guard`. After the validation session ends, run `rm ~/.claude/.validator-bash-guard`. This enables the read-only Bash enforcement hook.

## Pre-Flight Protocol — MANDATORY

Each step has an explicit trigger. Execute the step when its trigger fires. Do not execute a step outside its trigger.

| # | Step | Trigger | Action | Notes |
|---|------|---------|--------|-------|
| 1 | ENVIRONMENT CHECK | Before the first write operation of a session (any `git add`, `git commit`, `Edit`, `Write`, or `Bash` command that mutates state) | Run `git status`, `git branch`, `pwd`. Halt and report if any output is unexpected (wrong branch, uncommitted changes you didn't make, wrong directory). | Once per session unless branch or directory changes |
| 2 | REQUEST VERIFICATION | Before starting any task that requires 3+ distinct tool calls or 2+ files touched | Write a one-paragraph restatement of what you understand the request to be. If any element is ambiguous, stop and ask one focused question before proceeding. | Skip: single-file reads, single-command answers, informational questions |
| 3 | BUG ACCOUNTABILITY | Immediately upon discovering any bug, before continuing other work | Either (a) fix it now and file a closed GitHub Issue documenting the fix, or (b) file an open GitHub Issue and note the deferral. Never leave a bug undocumented. | Every bug |
| 4 | BRANCH VERIFICATION | After `git push` or after any `git commit` where commit landing is load-bearing for the next step | Run `git log --oneline -3` on the target branch to confirm the commit is present. If not present, diagnose before proceeding. | After every qualifying event |

## Advisory Protocol — Tiered Self-Escalation

**Capacity problems (rate limits, timeouts, infra failures) never escalate tiers — fix them in place.** Escalate only when reasoning quality changes the answer.
**Tier rule:** Use the lowest tier that can decide correctly. Default Sonnet for execution, Haiku for mechanical work, Opus only for reasoning forks. Set each parallel agent's model independently — uneven teams (1 Sonnet coordinator + 4 Haiku workers + on-demand Opus) are correct and preferred. Homogeneous model selection is a smell.

| Tier | Use for |
|------|---------|
| **Haiku** | Classification, formatting, retries, health checks, mechanical transforms, bulk judge/scoring |
| **Sonnet** | Implementation, debugging, multi-file edits, code review, executing diagnosed fixes |
| **Opus** | Architecture decisions, irreversible high-stakes choices, reframing stuck diagnoses |

**Spawn Sonnet** to execute a fix you've already diagnosed — `subagent_type: "general-purpose"`, `model: "sonnet"`. Brief with error message, log excerpt, file paths.

**Spawn `opus-advisor` before** any of:
- **A1 — Architecture fork:** 2+ approaches with meaningfully different long-term consequences
- **A2 — Infrastructure change:** K8s manifests, DNS, cert-manager, Cloudflare, storage
- **A3 — Large refactor:** restructuring a module/class/boundary >100 lines
- **A4 — Stuck on reasoning:** same root cause failed twice AND the failure is logic, not capacity
- **A5 — Irreversible + ambiguous:** can't easily undo and the right answer isn't clear

**Opus briefing format:**
1. **Decision** — one sentence
2. **Options** — A, B, (C) with one-sentence tradeoffs
3. **Lean** — current preference and source of uncertainty
4. **Context** — file paths, constraints

Wait for RECOMMENDATION before proceeding.

## Engram Memory — MANDATORY

Endpoint: `http://localhost:8788/mcp` · Projects: `clearwatch`, `homelab`, `engram`, `global`, `3dprint`, `family`

**Skip all memory ops for:** read-only investigation, informational answers, trivial single-file edits, or transient operational state with TTL <4h (service/DNS/build/migration/health output → file a GitHub Issue instead).

| Rule | Trigger | Action |
|------|---------|--------|
| **R1 — Recall at start** | First user message of a new conversation | `memory_recall("current project status recent work", project="global")`, then recall the request topic from the relevant project. Once per conversation. |
| **R2 — Recall before deciding** | Before proposing architecture/design, choosing between 2+ options, modifying infra (K8s/DNS/cert-manager/storage), or modifying a Clearwatch feature area | `memory_recall("<topic>", project="<project>")` |
| **R3 — Feedback after recall** | After every `memory_recall` | `memory_feedback` with the IDs that informed the answer. If results were absent/wrong where context should exist, store a MISS entry (`memory_type="error"`, `tags=["retrieval-miss"]`). |
| **R4 — Store after work** | Bug fix committed, decision made, pattern used 2+ times, or end of session | `memory_store` with type: `decision` (include why) · `error` (include root cause) · `pattern` · `context` (`importance=1, project="global"` for session summary). |
| **R6 — Fallback** | Engram unreachable after 1 retry within 30s | Stage entry in `~/.claude/projects/-home-psimmons/memory/fallback.md` (format defined in that file). Flush all pending entries on reconnect. Staging only — nothing lives there permanently. |

*Eisenhower only — R7 dispute tracking:* before adjudicating a user-raised dispute, `memory_recall("dispute-tracker <issue>", project="<project>")`. If count ≥3, escalate to founder instead of adjudicating. Store each adjudication: `content="DISPUTE: <desc> | VERDICT: <summary> | COUNT: N | LAST: <YYYY-MM-DD>"`, `tags=["dispute-tracker", "<project>"]`, `importance=1`.

## Test-After-Edit Protocol
- After any code edit, run the relevant test suite before moving to the next task
- When updating code that has existing tests, check whether tests encode buggy behavior — update both in the same commit
- Watch for hardcoded counts/constants (e.g., chart counts) that break when adding/removing items

## Workflow
- **Test-first.** Failing test before implementation; run tests after every edit.
- **Non-trivial tasks (3+ steps):** plan mode → worktree (`superpowers:using-git-worktrees`) → implement. Worktree step has no exceptions. Preserve error state — never push through unpredicted errors.
- **Procedural work:** use skills — authoritative over summaries here.
- **Before claiming done:** `superpowers:verification-before-completion`.
- **Stay in scope.** >15 min tangent → file Issue, keep moving. <15 min → fix and note.
- **Agent dispatch trouble:** if real progress was made, salvage partial output and hand off with context. If infrastructure broke before progress, dead-letter and retry from scratch — don't salvage broken state. For research dispatches >8 expected turns, brief: "stop at turn 8/10 and return PARTIAL: with what you have."

## Decisions
- 100% → Just do it | 80-99% → Do + explain | 50-80% → Propose first | <50% → Ask
- Pre-approved: logs, kubectl get/describe, health-check, diagnostics
- Always ask: delete resources, modify production, data loss risk
- **When blocked, ask one focused question** with your recommended default and what changes based on the answer.

## Bug & Defect Tracking — NON-NEGOTIABLE
GitHub Issues ARE the work. Defect not in the system = does not exist.
- Found a bug → file it before continuing. Fixed inline → file it as closed. Deferred → file it.
- **Continuity test:** Could the next session pick up every open defect from GitHub Issues alone?
- File issues FIRST, then report status. Use `gh issue create` with clear title, reproduction steps, and labels. Reference issue numbers in commit messages and PRs.
- **Severity gating:** All findings are filed. Merge is only blocked by `severity/blocker` label. Non-blocking findings use `severity/nice-to-have` — applied, tracked, reviewed quarterly. Never treat variable naming suggestions and security holes at the same urgency level.

## CLI Tool Preferences

Behavioral defaults (telemetry shows I default to the wrong tool without these):
- HTTP requests → `xh` (not `curl`)
- Multi-pod log tailing → `stern <name> -n <ns>` (not `kubectl logs`)
- Security review first step → `semgrep scan --config auto <path>`

Patterns and decision rules for `ast-grep`, `gron`, `yq`, `kubectl-neat`, `duckdb`, `tokei`, `jq`, `just`, full `kubectl`/`git` workflows → `~/TOOLS.md`.

## Critical Rules
**NEVER:** commit secrets · `.env` with real credentials (use Infisical: `https://infisical.petersimmons.com`) · restart before checking logs · destructive ops without backup
**ALWAYS:** `git diff --staged` before every commit · check logs before restarting · verify end-to-end output · see `~/AGENTS.md` for generals · GitHub = single source of truth

## Self-Learning & Autonomous Bug Fixing
- **Fix without asking** when reversible and low-blast-radius (low-severity bugs, feedback integration). **Always ask** when irreversible, data-affecting, externally visible, or resource-intensive.
- **After any user correction:** update `~/.claude/projects/-home-psimmons/memory/lessons-learned.md`.
- Retry/escalation limits live in §Cost Guardrails ("same error 3+ times" + circular loops).

## Project Priority Stack
1. **Clearwatch** — revenue: reports, cache, charts, grading
2. **Infrastructure** — K8s cluster, cert-manager, DNS, storage
3. **Gmail tracker / job search** — tooling and automation

## Cost Guardrails & Wake-the-Founder Triggers
- Opus: max 3 concurrent · Bulk LLM >50 calls: founder approval with cost estimate · Prefer Sonnet for routine work
- STOP and notify founder if: **>$5 compute** (cumulative estimated cost per request, estimated before execution) · **production deployment** (kubectl/helm/terraform apply targeting prod namespaces or clusters) · **push to main/master** (any `git push` whose target ref is `main` or `master`) · **data loss risk** (any operation that deletes, truncates, or overwrites persistent data without a verified backup) · **agent stuck ≥45 min** (stuck = elapsed wall time ≥45 min since last successful tool output; tool calls failing, loops repeating, or output unchanged) · **same error 3+ times in this session** (same root cause by stack trace or explicit error message match; counter resets when session ends or root cause changes)

## Reference
**Tools reference:** Full patterns, options, and decision rules for all CLI tools → `~/TOOLS.md` (git-tracked, never archived)
**Skills:** Debug → `superpowers:systematic-debugging` | Implement → `superpowers:brainstorming` | GitHub docs → `github-docs` (skill at `~/.claude/skills/github-docs/SKILL.md`)
**Benched skills** (inactive, not auto-loaded): `~/.claude/skills/bench/INDEX.md` — reactivate with `mv ~/.claude/skills/bench/<name> ~/.claude/skills/`
**Web Search:** ALWAYS use `search "query"` (`~/bin/search`) — hits local SearxNG (K8s deployment, `default/searxng`, 2 replicas, scales via `kubectl scale deploy/searxng -n default --replicas=N`). Aggregates Google + DuckDuckGo + Startpage. Use `--full` for snippets, `--limit N` for more results. NEVER use the WebSearch tool unless SearxNG is unreachable.
**Learning:** Detail → topic file | one-liner → MEMORY.md | rule → CLAUDE.md | `~/.claude/projects/-home-psimmons/memory/`
