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
Execute before ANY code changes or git operations. No exceptions.
1. **ENVIRONMENT CHECK** — `git status`, `git branch`, `pwd`. Halt if unexpected.
2. **REQUEST VERIFICATION** — Multi-step tasks: write one-paragraph summary of the request. Wait on ambiguous items.
3. **BUG ACCOUNTABILITY** — All bugs found must be fixed or filed as GitHub Issues.
4. **BRANCH VERIFICATION** — `git log --oneline -3` on target branch to confirm commits landed.

## Engram Memory — MANDATORY

Engram is running at `http://localhost:8788/mcp`. Use it. Every session starts cold — Engram is how context survives.

**Session start — do this before anything else:**
1. `memory_recall("current project status recent work", project="global")` — what was happening
2. `memory_recall("<topic of the request>", project="<relevant project>")` — targeted context
   - Known projects: `clearwatch`, `homelab`, `engram`, `global`, `3dprint`, `family`
   - When in doubt, also recall from `global`

**During work — recall before deciding, not after:**
- Before a technical decision: `memory_recall("<decision topic>", project="<project>")`
- Before touching infrastructure: `memory_recall("infrastructure patterns lessons", project="homelab")`
- Before a Clearwatch change: `memory_recall("<feature area>", project="clearwatch")`

**Retrieval quality feedback — ALWAYS do this:**
- After `memory_recall` returns useful results → call `memory_feedback` with the IDs that actually helped
- After `memory_recall` returns nothing (or wrong results) for a query where context should exist → store a miss: `memory_store(content="MISS: searched '<query>', expected '<what I needed>', got '<what I got or nothing>'", memory_type="error", project="<project>", tags=["retrieval-miss"], importance=1)`
- This feeds real retrieval quality data to Peter's memory benchmarking work. Do it every time without exception.

**After completing meaningful work — store it:**
- Decisions made and why → `memory_store(content, memory_type="decision", project="<project>")`
- Bugs fixed and root cause → `memory_store(content, memory_type="error", project="<project>")`
- Patterns established → `memory_store(content, memory_type="pattern", project="<project>")`
- Session summary at end → `memory_store(content, memory_type="context", project="global", importance=1)`
- **Never store:** transient operational state (service down, DNS failing, migration blocked, health status) — these become stale facts that mislead future sessions. File a GitHub Issue instead.

**Fallback only:** If Engram is unreachable, fall back to `~/.claude/projects/-home-psimmons/memory/`. Files are source of truth for structure; Engram is source of truth for learned context.

**Dispute tracking:** Before Eisenhower adjudicates any dispute, recall from Engram: `memory_recall("dispute-tracker <issue description>", project="<project>")`. If count ≥ 3, do not adjudicate — escalate to founder. Store each adjudication as: `content="DISPUTE: <description> | VERDICT: <summary> | COUNT: N | LAST: <date>"`, `tags=["dispute-tracker", "<project>"]`, `importance=1`.

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
- **Never ask permission for:** bug fixes (fix, test, commit, report after) · feedback integration
- **After any user correction:** update `~/.claude/projects/-home-psimmons/memory/lessons-learned.md`
- **Escalate only if:** same error appears **twice** — max 2 attempts per agentic step; on the third occurrence, escalate instead of retrying · circular token loops
- **Do ask permission for:** resource-intensive ops, actions with external visibility

## Project Priority Stack
1. **Clearwatch** — revenue: reports, cache, charts, grading
2. **Infrastructure** — K8s cluster, cert-manager, DNS, storage
3. **Gmail tracker / job search** — tooling and automation

## Cost Guardrails & Wake-the-Founder Triggers
- Opus: max 3 concurrent · Bulk LLM >50 calls: founder approval with cost estimate · Prefer Sonnet for routine work
- STOP and notify founder if: >$5 compute · production deployment · push to main/master · data loss · agent stuck >45 min · same error 3+ times

## Reference
**Tools reference:** Full patterns, options, and decision rules for all CLI tools → `~/TOOLS.md` (git-tracked, never archived)
**Skills:** Debug → `superpowers:systematic-debugging` | Implement → `superpowers:brainstorming` | GitHub docs → `github-docs` (skill at `~/.claude/skills/github-docs/SKILL.md`)
**Web Search:** ALWAYS use `search "query"` (`~/bin/search`) — hits local SearxNG (K8s deployment, `default/searxng`, 2 replicas, scales via `kubectl scale deploy/searxng -n default --replicas=N`). Aggregates Google + DuckDuckGo + Startpage. Use `--full` for snippets, `--limit N` for more results. NEVER use the WebSearch tool unless SearxNG is unreachable.
**Learning:** Detail → topic file | one-liner → MEMORY.md | rule → CLAUDE.md | `~/.claude/projects/-home-psimmons/memory/`
