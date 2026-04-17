# Claude Assistant Instructions

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
- **Review mode — judge against the reference, not the current file:** When dispatching generals for adversarial review, include: "Judge proposals against CLAUDE.md, established coding conventions, and authoritative references — not against the current state of the file under review. A change that contradicts the current file may be correct. The question is whether it's correct against the standard, not whether it differs from what's there now."

## Pre-Flight Protocol — MANDATORY

Each step has an explicit trigger. Execute the step when its trigger fires. Do not execute a step outside its trigger.

1. **ENVIRONMENT CHECK**
   - **Trigger:** Before the first write operation of a session (any `git add`, `git commit`, `Edit`, `Write`, or `Bash` command that mutates state).
   - **Action:** Run `git status`, `git branch`, `pwd`. Halt and report if any output is unexpected (wrong branch, uncommitted changes you didn't make, wrong directory).
   - **Frequency:** Once per session unless branch or directory changes.

2. **REQUEST VERIFICATION**
   - **Trigger:** Before starting any task that requires 3+ distinct tool calls or 2+ files touched.
   - **Action:** Write a one-paragraph restatement of what you understand the request to be. If any element is ambiguous, stop and ask one focused question before proceeding.
   - **Skip:** Single-file reads, single-command answers, informational questions.

3. **BUG ACCOUNTABILITY**
   - **Trigger:** Immediately upon discovering any bug, before continuing other work.
   - **Action:** Either (a) fix it now and file a closed GitHub Issue documenting the fix, or (b) file an open GitHub Issue and note the deferral. Never leave a bug undocumented.

4. **BRANCH VERIFICATION**
   - **Trigger:** After `git push` or after any `git commit` where commit landing is load-bearing for the next step.
   - **Action:** Run `git log --oneline -3` on the target branch to confirm the commit is present. If not present, diagnose before proceeding.

## Engram Memory — MANDATORY

Engram is at `http://localhost:8788/mcp`. Each rule below has an explicit trigger. Execute when the trigger fires; do not execute outside its trigger.

Known projects: `clearwatch`, `homelab`, `engram`, `global`, `3dprint`, `family`.

### Rule 1 — Session-start recall
- **Trigger:** The first user message of a new conversation, before any other tool call.
- **Action:** Call `memory_recall("current project status recent work", project="global")`, then `memory_recall("<topic of the request>", project="<relevant project>")`. If the relevant project is unclear, also recall from `global`.
- **Frequency:** Once per conversation.

### Rule 2 — Pre-decision recall
- **Trigger:** Before one of these specific actions: (a) proposing an architecture or design, (b) choosing between 2+ implementation options, (c) modifying infrastructure (K8s, DNS, cert-manager, storage), (d) modifying a Clearwatch feature area.
- **Action:** `memory_recall("<decision topic or feature area>", project="<relevant project>")`.
- **Skip:** Read-only investigation, informational answers, trivial single-file edits.

### Rule 3 — Retrieval feedback (every recall, no exceptions)
- **Trigger:** Immediately after any `memory_recall` call returns.
- **Action:**
  - If results helped → call `memory_feedback` with the IDs that actually informed the answer.
  - If results were absent or wrong for a query where context should exist → store a miss: `memory_store(content="MISS: searched '<query>', expected '<what I needed>', got '<what I got or nothing>'", memory_type="error", project="<project>", tags=["retrieval-miss"], importance=1)`.
- **Purpose:** Feeds retrieval-quality data to Peter's memory benchmarking. No exceptions.

### Rule 4 — Post-work storage
- **Trigger:** After completing one of these specific outcomes: (a) a bug fix committed, (b) an architectural decision made, (c) a pattern used 2+ times in this session, (d) end of a working session.
- **Action:** `memory_store` with the appropriate `memory_type`:
  - `decision` for architectural choices (include why)
  - `error` for bug fixes (include root cause)
  - `pattern` for patterns established
  - `context` with `importance=1, project="global"` for session summary at end

### Rule 5 — Never-store exclusion (check before every store)
- **Trigger:** Before any `memory_store` call.
- **Action:** If the content is transient operational state with expected lifespan < 4 hours (service status, DNS state, build status, migration progress, health output), do NOT store. File a GitHub Issue instead.

### Rule 6 — Fallback to filesystem
- **Trigger:** Engram unreachable after one retry within 30 seconds.
- **Action:** Fall back to `~/.claude/projects/-home-psimmons/memory/`. Files are source of truth for structure; Engram is source of truth for learned context.

### Rule 7 — Dispute tracking (Eisenhower only)
- **Trigger:** Before Eisenhower adjudicates a user-raised dispute.
- **Action:** `memory_recall("dispute-tracker <issue description>", project="<project>")`. If count ≥ 3, do NOT adjudicate — escalate to founder. Store each adjudication as: `content="DISPUTE: <description> | VERDICT: <summary> | COUNT: N | LAST: <YYYY-MM-DD>"`, `tags=["dispute-tracker", "<project>"]`, `importance=1`.

## Workflow
- **Test first.** Failing test before first line of implementation. Run tests after EVERY edit. Never batch untested changes.
- Plan mode for non-trivial tasks (3+ steps). Preserve error state if things go sideways — never push through unpredicted errors.
- **Worktree before implementation — MANDATORY.** Use `superpowers:using-git-worktrees` before any approved plan. No exceptions.
- Use skills for procedural work — authoritative over summaries here.
- **Stay in scope.** >15 min tangent → file GitHub Issue, keep moving. <15 min → fix and note it.
- `superpowers:verification-before-completion` before claiming done.
- **Graceful degradation:** For research-heavy dispatches (multi-tool, expected >8 turns), add to the dispatch brief: "If you reach turn 8 of 10 without a complete answer, stop tool calls and return a partial summary labeled `PARTIAL:` with what you have gathered. Do not wait for perfect information."
- **Two escalation modes — use the right one:**
  - **Partial-work escalation:** Agent made real progress, got stuck — preserve the partial output and hand off with context. Never discard useful work.
  - **Hard-failure escalation:** Infrastructure or tooling broke before meaningful work happened — dead letter it, retry from scratch. Don't try to salvage.

## Decisions
- 100% → Just do it | 80-99% → Do + explain | 50-80% → Propose first | <50% → Ask
- Pre-approved: logs, kubectl get/describe, health-check, diagnostics
- Always ask: delete resources, modify production, data loss risk
- **When blocked, ask one focused question** with your recommended default and what changes based on the answer.

## Bug & Defect Tracking — NON-NEGOTIABLE
GitHub Issues ARE the work. Defect not in the system = does not exist.
- Found a bug → file it before continuing. Fixed inline → file it as closed. Deferred → file it.
- **Continuity test:** Could the next session pick up every open defect from GitHub Issues alone?
- File issues FIRST, then report status.
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
- STOP and notify founder if:
  - **>$5 compute** — cumulative estimated cost per request, estimated before execution
  - **production deployment** — any kubectl/helm/terraform apply targeting prod namespaces or clusters
  - **push to main/master** — any `git push` whose target ref is `main` or `master`
  - **data loss risk** — any operation that deletes, truncates, or overwrites persistent data without a verified backup
  - **agent stuck ≥45 min** — stuck = elapsed wall time ≥45 minutes since the last successful tool output (tool calls failing, loops repeating, or output unchanged)
  - **same error 3+ times in this session** — same root cause measured by stack trace or explicit error message match; counter resets when the session ends or root cause changes

## Reference
**Tools reference:** Full patterns, options, and decision rules for all CLI tools → `~/TOOLS.md` (git-tracked, never archived)
**Skills:** Debug → `superpowers:systematic-debugging` | Implement → `superpowers:brainstorming` | GitHub docs → `github-docs` (skill at `~/.claude/skills/github-docs/SKILL.md`)
**Web Search:** ALWAYS use `search "query"` (`~/bin/search`) — hits local SearxNG (K8s deployment, `default/searxng`, 2 replicas, scales via `kubectl scale deploy/searxng -n default --replicas=N`). Aggregates Google + DuckDuckGo + Startpage. Use `--full` for snippets, `--limit N` for more results. NEVER use the WebSearch tool unless SearxNG is unreachable.
**Learning:** Detail → topic file | one-liner → MEMORY.md | rule → CLAUDE.md | `~/.claude/projects/-home-psimmons/memory/`
