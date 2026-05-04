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

Models escalate up the tier only when the task genuinely requires stronger **reasoning**. Capacity problems (rate limits, timeouts, connection exhaustion, tool unavailability) stay at the current tier or below — fix them in Haiku.

**Tier order:** Haiku → Sonnet → Opus

**Minimum tier rule:** Always use the lowest tier that can execute the task correctly. Before spawning any agent or choosing a model, ask: "Would a less capable model get this right?" If yes, use the lower tier.

| Tier | Use for |
|------|---------|
| **Haiku** | Classification, formatting, retries, health checks, simple lookups, mechanical transforms, bulk judge/scoring tasks, any task where output quality is independent of reasoning depth |
| **Sonnet** | Implementation, debugging, multi-file edits, code review, most engineering decisions |
| **Opus** | Architectural forks with long-term consequences, irreversible high-stakes decisions, problems where stronger reasoning materially changes the answer |

When dispatching parallel agents, set each agent's model independently — do not default all agents to Sonnet when some are doing Haiku-tier work. **Uneven agent teams are correct and preferred**: a campaign with one Sonnet coordinator, four Haiku workers, and an on-demand Opus advisor costs a fraction of an all-Sonnet team and produces the same result. Homogeneous model selection is a smell.

**The advisor pattern makes over-provisioning indefensible.** Sonnet and Haiku have a direct escalation path to Opus reasoning via the opus-advisor agent whenever they genuinely need it. There is no justification for defaulting to a higher tier "just in case" — escalate on demand, not by default.

### Sonnet Advisor — Primary Fix Agent

Spawn a Sonnet advisor for **any fix work** — debugging, code changes, prompt tuning, log analysis, or operational troubleshooting. Sonnet's job is to diagnose and implement fixes; you orchestrate.

**When to spawn Sonnet:**
- Binary error, crash, or unexpected output — diagnose and fix
- Test failure with non-obvious root cause — debug and repair
- Checkpoint corruption or worker failure — inspect logs and fix
- Rate limits, Engram slow, or infrastructure error — fix in place
- GPU/OOM/network error — resolve directly
- Scoring prompt producing wrong judgments — inspect and adjust

Brief: share the error message, relevant log excerpt, and file paths. Sonnet implements the fix. Use `subagent_type: "general-purpose"` with `model: "sonnet"`.

### Opus Advisor — Reasoning Failures Only

Spawn Opus only for **reasoning or strategy problems** — problems not fixable by implementation alone.

When the primary model encounters any of the following, **spawn the `opus-advisor` agent** via the Agent tool before proceeding:

| Trigger | Description |
|---------|-------------|
| **A1 — Architecture fork** | Choosing between 2+ implementation approaches with meaningfully different long-term consequences |
| **A2 — Infrastructure change** | Any modification to K8s manifests, DNS, cert-manager, Cloudflare, or storage configuration |
| **A3 — Large refactor** | Restructuring a module, class, or system boundary > 100 lines |
| **A4 — Stuck after 2 attempts** | The same root cause has failed twice AND the failure is a reasoning/logic problem; before the third attempt, get Opus to reframe |
| **A5 — Ambiguous high-stakes decision** | A decision that cannot be easily reversed and the right answer isn't clear |

**Skip the advisor for:** read-only investigation, single-file fixes < 50 lines with obvious root cause, routine dependency updates, **any failure caused by capacity/rate limits/timeouts/infra rather than reasoning**.

**Opus is for reasoning quality, not capacity problems.** Rate limits, timeouts, connection exhaustion, and tool unavailability are infrastructure problems — fix them in Haiku. Never upgrade tiers because something failed; only upgrade when the task genuinely requires stronger reasoning to decide correctly.

### Opus Advisor Briefing Format

When spawning the Opus advisor for a reasoning problem, include:

1. **Decision**: One sentence — what must be decided
2. **Options**: A, B, (C) — each with a one-sentence tradeoff
3. **Lean**: Which option you're currently leaning toward and why you're uncertain
4. **Context**: Relevant file paths or constraints

Use `subagent_type: "opus-advisor"` in the Agent tool call. Wait for the RECOMMENDATION before proceeding.

## Engram Memory — MANDATORY

Endpoint: `http://localhost:8788/mcp` · Known projects: `clearwatch`, `homelab`, `engram`, `global`, `3dprint`, `family`

Each rule has an explicit trigger. Execute when the trigger fires; do not execute outside its trigger.

| Rule | Trigger | Action |
|------|---------|--------|
| **R1 — Session-start recall** | First user message of a new conversation, before any other tool call | `memory_recall("current project status recent work", project="global")`, then `memory_recall("<topic of the request>", project="<relevant project>")`. If the relevant project is unclear, also recall from `global`. Once per conversation. |
| **R2 — Pre-decision recall** | Before: (a) proposing an architecture or design, (b) choosing between 2+ implementation options, (c) modifying infrastructure (K8s, DNS, cert-manager, storage), (d) modifying a Clearwatch feature area | `memory_recall("<decision topic or feature area>", project="<relevant project>")`. Skip: read-only investigation, informational answers, trivial single-file edits. |
| **R3 — Retrieval feedback** | Immediately after any `memory_recall` call — no exceptions | If results helped → `memory_feedback` with the IDs that actually informed the answer. If results were absent or wrong for a query where context should exist → `memory_store(content="MISS: searched '<query>', expected '<what I needed>', got '<what I got or nothing>'", memory_type="error", project="<project>", tags=["retrieval-miss"], importance=1)`. Feeds retrieval-quality data to Peter's memory benchmarking. |
| **R4 — Post-work storage** | After: (a) a bug fix committed, (b) an architectural decision made, (c) a pattern used 2+ times in this session, (d) end of a working session | `memory_store` with the appropriate `memory_type`: `decision` (architectural choices — include why) · `error` (bug fixes — include root cause) · `pattern` (patterns established) · `context` (`importance=1, project="global"` for session summary at end) |
| **R5 — Never-store exclusion** | Before any `memory_store` call | If content is transient operational state with expected lifespan < 4 hours (service status, DNS state, build status, migration progress, health output) → do NOT store. File a GitHub Issue instead. |
| **R6 — Fallback to filesystem** | Engram unreachable after one retry within 30 seconds | Write the memory entry to `~/.claude/projects/-home-psimmons/memory/fallback.md` using the format defined in that file. On Engram reconnect, flush all pending entries via `memory_store` and clear them from the file. `fallback.md` is a staging file only — nothing should live there permanently. |
| **R7 — Dispute tracking (Eisenhower only)** | Before Eisenhower adjudicates a user-raised dispute | `memory_recall("dispute-tracker <issue description>", project="<project>")`. If count ≥ 3, do NOT adjudicate — escalate to founder. Store each adjudication: `content="DISPUTE: <description> \| VERDICT: <summary> \| COUNT: N \| LAST: <YYYY-MM-DD>"`, `tags=["dispute-tracker", "<project>"]`, `importance=1`. |

## Test-After-Edit Protocol
- After any code edit, run the relevant test suite before moving to the next task
- When updating code that has existing tests, check whether tests encode buggy behavior — update both in the same commit
- Watch for hardcoded counts/constants (e.g., chart counts) that break when adding/removing items

## Workflow
- **Test first.** Failing test before first line of implementation. Run tests after EVERY edit. Never batch untested changes.
- Plan mode for non-trivial tasks (3+ steps). Preserve error state if things go sideways — never push through unpredicted errors.
- **Worktree before implementation — MANDATORY.** Use `superpowers:using-git-worktrees` before any approved plan. No exceptions.
- Use skills for procedural work — authoritative over summaries here.
- **Stay in scope.** >15 min tangent → file GitHub Issue, keep moving. <15 min → fix and note it.
- `superpowers:verification-before-completion` before claiming done.
- **Graceful degradation:** For research-heavy dispatches (multi-tool, expected >8 turns), add to the dispatch brief: "If you reach turn 8 of 10 without a complete answer, stop tool calls and return a partial summary labeled `PARTIAL:` with what you have gathered. Do not wait for perfect information."
- **Two escalation modes — use the right one:** Partial-work (agent made real progress, got stuck) → preserve the partial output and hand off with context, never discard useful work. Hard-failure (infrastructure or tooling broke before meaningful work happened) → dead letter it, retry from scratch, don't try to salvage.

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
- **HTTP requests:** use `xh` not `curl` — cleaner output, no flags needed for JSON
- **kubectl shortcuts:** check `just --list` before typing raw kubectl commands; use `just <recipe>` when one exists
- **Multi-pod logs:** use `stern <name> -n <ns>` not `kubectl logs` when tailing across multiple pods
- **Security reviews:** always run `semgrep scan --config auto <path>` as first step before manual review
- **Git diffs:** `git diff --staged` output is automatically rendered by delta (line numbers included)
- **Structural code search:** use `ast-grep --pattern 'def func_name($$$)'` to locate functions precisely — avoids reading large files just to find insertion points. Installed at `/home/linuxbrew/.linuxbrew/bin/ast-grep`
- **JSON field extraction:** use `gron dossier.json | grep field_name` to pull single fields from large JSON without loading the whole file into context. Installed at `/home/linuxbrew/.linuxbrew/bin/gron`
- **YAML field extraction:** `kubectl get deploy -n NS -o yaml | yq '.items[] | {"name":.metadata.name,"replicas":.spec.replicas}'` — 1123 lines → ~36 (97% reduction). Full patterns → `~/TOOLS.md`
- **kubectl output cleanup:** `kubectl get X -o yaml | kubectl-neat` — use for full-spec copy/templating; prefer `yq` for targeted field reads. Installed at `~/bin/kubectl-neat`
- **Large data files + multi-file queries:** `duckdb -c "SELECT ... FROM read_json('/tmp/pod-*.json')"` — SQL on CSV/JSON/Parquet with glob support. Full patterns → `~/TOOLS.md`
- **JSON modification without jq syntax:** `gron file.json | sed 's/old/new/' | gron --ungron` — round-trip modify. Full patterns → `~/TOOLS.md`
- **Code refactoring previews:** `ast-grep --pattern 'X' --rewrite 'Y' src/` shows diff; `--update-all` applies. Full patterns → `~/TOOLS.md`
- **Codebase size:** use `tokei pipeline/` for a one-call language/file/line breakdown before diving into an unfamiliar subsystem. Installed at `/home/linuxbrew/.linuxbrew/bin/tokei`

## Critical Rules
**NEVER:** commit secrets · `.env` with real credentials (use Infisical: `https://infisical.petersimmons.com`) · restart before checking logs · destructive ops without backup
**ALWAYS:** `git diff --staged` before every commit · check logs before restarting · verify end-to-end output · see `~/AGENTS.md` for generals · GitHub = single source of truth

## Self-Learning & Autonomous Bug Fixing
- **Never ask permission for:** low-severity bug fixes (fix, test, commit, report after) · feedback integration
- **Always ask permission for:** data-affecting fixes, breaking API changes, resource-intensive ops, actions with external visibility
- **After any user correction:** update `~/.claude/projects/-home-psimmons/memory/lessons-learned.md`
- **Retry limit:** Per agentic step, attempt 1 = initial try; attempt 2 = one retry after a fix. On the third occurrence of the same root cause (same stack trace or same explicit error message), escalate instead of retrying. Counter resets when the session ends or the root cause changes.
- **Also escalate on:** circular token loops (same tool call + same result repeated 2+ times).

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
