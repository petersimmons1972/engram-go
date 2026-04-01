# Claude Assistant Instructions

## Behavioral Rules
- Never tell the user to do something manually that you can do yourself — just do it.
- **Markdown tables**: pad columns for alignment, use emoji swatches (🔵🟡🟢⚫⚪✅❌⚠️), never leave hex codes unformatted in a cell.
- When asked for a 'summary' or 'report', cover ALL items — not just a filtered subset.
- Before starting work, check memory files (AGENTS.md, plan docs, GitHub issues) for current state.
- **Art direction:** See `~/projects/art-direction-research/ART-DIRECTION-RULE.md` and AGENTS.md §9. No generic AI design tools.

## Parallel Agent Rules
- **Pre-validation:** ONE agent analyzes 2–3 samples first. Present findings. Only then dispatch remaining agents with the confirmed problem definition.
- List which functions each agent will touch. Two agents on the same function → flag it, run full test suite after.
- **Always include one zero-context reviewer** — receives only raw inputs, no prior findings.

## Pre-Flight Protocol — MANDATORY
Execute before ANY code changes or git operations. No exceptions.
1. **ENVIRONMENT CHECK** — `git status`, `git branch`, `pwd`. Halt if unexpected.
2. **REQUEST VERIFICATION** — Multi-step tasks: write one-paragraph summary of the request. Wait on ambiguous items.
3. **BUG ACCOUNTABILITY** — All bugs found must be fixed or filed as GitHub Issues.
4. **BRANCH VERIFICATION** — `git log --oneline -3` on target branch to confirm commits landed.

## Workflow
- **Test first.** Failing test before first line of implementation. Run tests after EVERY edit. Never batch untested changes.
- Plan mode for non-trivial tasks (3+ steps). Preserve error state if things go sideways — never push through unpredicted errors.
- **Worktree before implementation — MANDATORY.** Use `superpowers:using-git-worktrees` before any approved plan. No exceptions.
- Use skills for procedural work — authoritative over summaries here.
- **Stay in scope.** >15 min tangent → file GitHub Issue, keep moving. <15 min → fix and note it.
- `superpowers:verification-before-completion` before claiming done.
- **Engram supplements files, never replaces them.** `memory_recall()` when available; fall back to `~/.claude/projects/-home-psimmons/memory/`. Files are source of truth.

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

## Critical Rules
**NEVER:** commit secrets · `.env` with real credentials (use Infisical: `https://infisical.petersimmons.com`) · restart before checking logs · destructive ops without backup
**ALWAYS:** `git diff --staged` before every commit · check logs before restarting · verify end-to-end output · see `~/AGENTS.md` for generals · GitHub = single source of truth

## Self-Learning & Autonomous Bug Fixing
- **Never ask permission for:** bug fixes (fix, test, commit, report after) · feedback integration
- **After any user correction:** update `~/.claude/projects/-home-psimmons/memory/lessons-learned.md`
- **Escalate only if:** same bug 3+ times or circular token loops
- **Do ask permission for:** resource-intensive ops, actions with external visibility

## Project Priority Stack
1. **Clearwatch** — revenue: reports, cache, charts, grading
2. **Infrastructure** — K8s cluster, cert-manager, DNS, storage
3. **Gmail tracker / job search** — tooling and automation

## Cost Guardrails & Wake-the-Founder Triggers
- Opus: max 3 concurrent · Bulk LLM >50 calls: founder approval with cost estimate · Prefer Sonnet for routine work
- STOP and notify founder if: >$5 compute · production deployment · push to main/master · data loss · agent stuck >45 min · same error 3+ times

## Reference
**Skills:** Debug → `superpowers:systematic-debugging` | Implement → `superpowers:brainstorming`
**Web Search:** `https://searxng.petersimmons.com/search?q={query}&format=json` | Fallback: WebSearch tool
**Learning:** Detail → topic file | one-liner → MEMORY.md | rule → CLAUDE.md | `~/.claude/projects/-home-psimmons/memory/`
