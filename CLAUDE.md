# Claude Assistant Instructions

## Repository Scope
- Active project: **Clearwatch only**. The SIB repo is DEPRECATED — never dispatch agents, fixes, or commits to it.
- Confirm target repo is active before editing, even when bugs are found in other repos.
- Confirm target repo and storage layer before starting multi-file operations.

## Core Principles

- **Simplicity first.** Make every change as simple as possible. Impact minimal code. Three similar lines beat a premature abstraction.
- **No laziness.** Find root causes. No temporary fixes. No hand-holding required — point at logs, errors, and failing tests, then resolve them.
- **Minimal impact.** Changes should only touch what's necessary. If a fix feels hacky, find the clean solution — "Knowing everything I know now, implement the elegant solution."

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
- **Model floor rule:** Always set `model:` explicitly on every agent dispatch. Default to the lowest tier that can do the job correctly — Haiku first, Sonnet only when the task requires judgment or multi-file synthesis, Opus only per the A1–A5 triggers below. Homogeneous Sonnet teams are a smell. If you cannot articulate why Haiku is insufficient for a given agent, use Haiku.
- **Advisory mandate:** Every implementation agent brief must include: "Before proposing or selecting any implementation approach, invoke the `advisory-gate` skill if 2+ approaches exist with meaningfully different consequences (A1-A5 triggers)."
- **Engram context mandate:** When dispatching implementation agents, include relevant Engram recall results from the current session in the brief. Subagents receive no session hooks — coordinator is responsible for seeding their context.

## Pre-Flight Protocol — MANDATORY

Each step has an explicit trigger. Execute the step when its trigger fires. Do not execute a step outside its trigger.

| # | Step | Trigger | Action | Notes |
|---|------|---------|--------|-------|
| 1 | ENVIRONMENT CHECK | Before the first write operation of a session (any `git add`, `git commit`, `Edit`, `Write`, or `Bash` command that mutates state) | Run `git status`, `git branch`, `pwd`. Halt and report if any output is unexpected (wrong branch, uncommitted changes you didn't make, wrong directory). | Once per session unless branch or directory changes |
| 2 | REQUEST VERIFICATION | Before starting any task that requires 3+ distinct tool calls or 2+ files touched | Write a one-paragraph restatement of what you understand the request to be. If any element is ambiguous, stop and ask one focused question before proceeding. | Skip: single-file reads, single-command answers, informational questions |
| 3 | BUG ACCOUNTABILITY | Immediately upon discovering any bug, before continuing other work | Either (a) fix it now and file a closed GitHub Issue documenting the fix, or (b) file an open GitHub Issue and note the deferral. Never leave a bug undocumented. | Every bug |
| 4 | BRANCH VERIFICATION | After `git push` or after any `git commit` where commit landing is load-bearing for the next step | Run `git log --oneline -3` on the target branch to confirm the commit is present. If not present, diagnose before proceeding. | After every qualifying event |
| 5 | EXPENSIVE OPERATION CHECK | Before running any benchmark, full pipeline, or deployment | Quote estimated cost and duration. Wait for explicit confirmation — "go" or "yes". Never interpret a bare number ('1', '2', etc.) as confirmation. | Every qualifying event |

## Advisory Protocol — Tiered Self-Escalation

**Quality floor:** Before presenting non-trivial work, ask "Is there a more elegant way?" Quality bar: **"Would a staff engineer approve this?"** If no, implement the clean solution. If execution hits an unpredicted wall, STOP and re-plan; capacity failures never escalate tiers.

**Tier rule:** Lowest tier that decides correctly. Uneven teams preferred; homogeneous selection is a smell.

| Tier | Use for |
|------|---------|
| **Haiku** | Classification, formatting, retries, health checks, mechanical transforms, bulk judge/scoring |
| **Sonnet** | Implementation, debugging, multi-file edits, code review, executing diagnosed fixes |
| **Opus** | Architecture decisions, irreversible high-stakes choices, reframing stuck diagnoses |

**Spawn Sonnet** (`subagent_type: "general-purpose"`, `model: "sonnet"`) to execute a diagnosed fix. **Spawn `opus-advisor`** for A1–A5 decisions — triggers and briefing format → `~/docs/advisory-protocol.md`.

## Engram Memory — MANDATORY

Endpoint: `http://localhost:8788/mcp` · Projects: `clearwatch`, `homelab`, `engram`, `global`, `3dprint`, `family`

**Skip:** read-only, informational, trivial single-file edits, transient state <4h TTL.

| Rule | Trigger | Action |
|------|---------|--------|
| **R1** | Session start | `memory_recall("current project status recent work", project="global")` + topic. Once per conversation. |
| **R2** | Before arch/design/infra decision | `memory_recall("<topic>", project="<project>")` |
| **R3** | After every recall | `memory_feedback` with informing IDs; MISS entry if absent/wrong |
| **R4** | After work / session end | `memory_store` type: `decision` · `error` · `pattern` · `context` |
| **R6** | Engram unreachable (1 retry/30s) | Stage to `fallback.md`; flush on reconnect |

*R7 (Eisenhower only) — dispute tracking:* full protocol → `~/docs/engram-memory-rules.md`.

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

## Container Image Standard — NON-NEGOTIABLE
Default: Chainguard base images. Python-with-tools: `python:latest-dev` build stage → `wolfi-base` runtime; nonroot UID **65532**, tini at `/sbin/tini`. K8s: `fsGroup: 65532` MANDATORY or volume mounts crashloop; `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`. Full pattern → `~/docs/container-images.md`.

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
**Web Search:** Use `searxng_web_search` MCP tool (local SearXNG at `searxng.petersimmons.com`, aggregates Google + DDG + Startpage). Fallback: `~/bin/search`. NEVER use the built-in WebSearch tool.
**Learning:** Detail → topic file | one-liner → MEMORY.md | rule → CLAUDE.md | `~/.claude/projects/-home-psimmons/memory/`
**Advisory Protocol detail** (A1–A5 triggers + Opus briefing format) → `~/docs/advisory-protocol.md`
**Engram Memory full rules** (verbose R-table + R7 dispute tracking) → `~/docs/engram-memory-rules.md`
**Container image standard** (Chainguard full pattern + K8s security context) → `~/docs/container-images.md`
