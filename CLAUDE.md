# Claude Assistant Instructions

## Behavioral Rules

- Never tell the user to do something manually that you can do yourself — just do it.
- **Markdown tables must be human-readable in raw form**: pad columns so they align, use emoji swatches for color/status values (🔵🟡🟢⚫⚪✅❌⚠️), and never leave hex codes or long values unformatted in a table cell.
- When the user asks for a 'summary' or 'report', cover ALL items (open, closed, fixed, unfixed) — not just a filtered subset.
- Before starting work, check memory files (AGENTS.md, plan docs, GitHub issues) for current state. Verify what's actually open/remaining.
- **See "Bug & Defect Tracking" section below — NON-NEGOTIABLE.**

## Parallel Agent Rules

- **Pre-validation (mandatory):** Before dispatching parallel agents, first use ONE agent to analyze 2–3 samples and confirm the exact problem definition. Present findings to user. Only after approval, dispatch remaining agents with that confirmed definition.
- Include a concrete example of the problem from the user's description. Restate the exact symptom, not your interpretation.
- Explicitly list which functions each agent will touch. If two agents touch the same function, flag it and run the full test suite after all agents complete.
- **Always include one zero-context reviewer** in review panels — receives only raw inputs, no prior findings. Domain experts anchor on scanner output and miss data integrity issues.

## Pre-Flight Protocol — MANDATORY

Execute before ANY code changes or git operations. No exceptions.

1. **ENVIRONMENT CHECK** — `git status`, `git branch`, `pwd`. Confirm correct repo/branch. Halt if unexpected.
2. **REQUEST VERIFICATION** — For multi-step tasks, write a one-paragraph summary distinguishing similar-sounding problems. Wait for confirmation on ambiguous requests.
3. **BUG ACCOUNTABILITY** — All bugs found must be fixed or filed as GitHub Issues. Never leave bugs unreported.
4. **BRANCH VERIFICATION ON COMPLETION** — Run `git log --oneline -3` on target branch to confirm commits landed.

## Workflow

- **Test first.** Write the failing test before the first line of implementation. After EVERY code edit, immediately run the most relevant test(s) before moving to the next task. Never batch untested changes.
- Plan mode for non-trivial tasks (3+ steps). If things go sideways: **preserve the error state** (logs, failing output, broken branch), then re-plan. Never push through errors you didn't predict.
- **Worktree before implementation — MANDATORY.** Before executing any approved plan (whether from plan mode, brainstorming, or operational order), create an isolated git worktree via `superpowers:using-git-worktrees`. No exceptions. This applies regardless of how the plan was created — plan mode bypasses the brainstorming skill chain and does not automatically create a worktree.
- Use skills for procedural work — they are authoritative over summaries here.
- **Stay in scope.** Tangential issues found during work that would take >15 minutes → file as GitHub Issue, don't fix inline. Quick fixes (<15 min) are fine — initiative is rewarded.
- `superpowers:verification-before-completion` before claiming done.

## Project Lifecycle

### Creating a New Project
Every new project directory MUST have:
1. README.md with a STATUS line in the first 10 lines
2. CLAUDE.md if any agent sessions will run in that directory

Naming convention: kebab-case, descriptive, no version numbers in dir name.

### Project Status Definitions
- **active** — code changes weekly
- **operational** — deployed, maintenance only
- **reference** — useful patterns/data, no active dev
- **experimental** — proof-of-concept, may be abandoned
- **archived** — moved to archive/, no longer relevant

### Archive Criteria
Archive when ANY are true: no commits in 90 days (and not a dependency), explicitly superseded, or one-time task completed.

### No Orphan Files at projects/ Root
All files must live in a subdirectory. Loose files at `~/projects/` root → move to active project, archive, or delete.

## Decisions

- 100% confident → Just do it | 80-99% → Do + explain | 50-80% → Propose first | <50% → Ask
- Pre-approved: logs, kubectl get/describe, health-check, diagnostics
- Always ask: delete resources, modify production, data loss risk
- **When blocked, ask one focused question.** Include your recommended default and what changes based on the answer. Make embedded assumptions explicit — never hide decisions inside a "should I proceed?" question.

## Bug & Defect Tracking — NON-NEGOTIABLE

GitHub Issues ARE the work. If a defect is not in the issue system, it does not exist.

**ZERO EXCEPTIONS:**
- Found a bug while working? File it before continuing.
- Fixed a bug inline? Still file it — as closed, with the fix documented.
- Unsure of the fix? File it. "Unknown fix" is a valid issue body.
- Deferred to later? File it. "Later" without an issue number means never.

**The continuity test:** If this session ended now, could the next session pick up every open defect from GitHub Issues alone? If not, something is unfiled.

**Enforcement:** File issues FIRST, then report status. A status report that names unfiled defects is self-contradicting.

## Critical Rules

**NEVER:**
- Commit secrets (API keys, passwords, tokens, .env files)
- Create `.env` files with real credentials — use Infisical (`https://infisical.petersimmons.com`)
- Restart services before checking logs
- Perform destructive ops without verified backup

**ALWAYS:**
- `git diff --staged` before every commit
- Check logs before restarting services
- Verify end-to-end output, not just code
- **Generals**: See `~/AGENTS.md` for roster and spawn templates
- GitHub = single source of truth; deliverables must be committed

## Self-Learning & Autonomous Bug Fixing (NON-NEGOTIABLE)

**NEVER ask permission for:**
- Bug fixes (any severity) — fix, test, commit, report after
- Feedback integration and quality improvements

**After ANY correction from the user:**
- Update `~/.claude/projects/-home-psimmons/memory/lessons-learned.md` with the pattern
- Write rules that prevent the same mistake

**ONLY escalate to user if:**
- Repeated pattern cycles (same bug 3+ times)
- Wasting tokens on circular loops

**DO ask permission for:**
- Running reports or generating deliverables (external impact)
- Resource-intensive operations requiring deliberate scheduling
- Actions with external visibility or side effects

## Project Priority Stack (top = highest)
1. **Clearwatch** — revenue pipeline (reports, cache, charts, grading)
2. **Infrastructure** — K8s cluster stability, cert-manager, DNS, storage
3. **Gmail tracker / job search** — tooling and automation

## Cost Guardrails
- Opus agents: max 3 concurrent unless explicitly authorized
- Bulk LLM operations (>50 calls): require founder approval with cost estimate
- Prefer Sonnet for exploration/routine, Opus for production-quality output

## Wake-the-Founder Triggers
Even in autonomous mode, STOP and wait if:
- Any operation estimated at >$5 in compute
- Deployment to production namespaces (not dev/staging)
- Any git push to main/master
- Data loss or corruption detected
- Agent stuck >45 min with no progress
- Same bug/error 3+ times

## Reference

**Skills:** Deep debug → `superpowers:systematic-debugging` | Before implementing → `superpowers:brainstorming`
**Web Search:** SearXNG: `https://searxng.petersimmons.com/search?q={query}&format=json` | Fallback: WebSearch tool
**Visual Output:** Use skill `visual-output-standards` for SVG charts, color specs, and formatting rules.
**Learning System:** Detail → topic file | one-liner → MEMORY.md | behavioral rule → CLAUDE.md | Files: `~/.claude/projects/-home-psimmons/memory/`
