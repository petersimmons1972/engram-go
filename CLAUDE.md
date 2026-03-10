# Claude Assistant Instructions

## Behavioral Rules

- Never tell the user to do something manually that you can do yourself — just do it.
- When the user asks for a 'summary' or 'report', cover ALL items (open, closed, fixed, unfixed) — not just a filtered subset.
- When dispatching parallel agents, include a concrete example of the problem from the user's description. Restate the exact symptom, not your interpretation.
- Before starting work, check memory files (AGENTS.md, plan docs, GitHub issues) for current state. Verify what's actually open/remaining.
- When you find bugs that aren't immediately fixed, ALWAYS file them as GitHub issues.

## Workflow

- Plan mode for non-trivial tasks (3+ steps). Re-plan immediately if things go sideways.
- Use skills for procedural work — they are authoritative over summaries here.
- For non-trivial changes: pause and ask "is there a more elegant way?" Skip for simple fixes.
- `superpowers:verification-before-completion` before claiming done (authoritative).

## Decisions

- 100% confident → Just do it | 80-99% → Do + explain | 50-80% → Propose first | <50% → Ask
- Pre-approved: logs, kubectl get/describe, health-check, diagnostics
- Always ask: delete resources, modify production, data loss risk

## Critical Rules

**NEVER:**
- Commit secrets (API keys, passwords, tokens, .env files)
- Create `.env` files with real credentials — use Infisical (`https://infisical.petersimmons.com`); see `kubernetes/CLAUDE.md`
- Restart services before checking logs
- Perform destructive ops without verified backup

**ALWAYS:**
- `git diff --staged` before every commit
- Check logs before restarting services
- Verify end-to-end output, not just code
- **Generals**: See `~/AGENTS.md` for roster, spawn templates, operational reference (GH: petersimmons1972/generals)
- GitHub = single source of truth; deliverables must be committed

## Self-Learning & Autonomous Bug Fixing (NON-NEGOTIABLE)

**NEVER ask permission for:**
- Bug fixes (any severity) — just fix it, write tests, commit, report after
- Feedback integration and quality improvements
- Self-improvement and system refinement

**After ANY correction from the user:**
- Update `~/.claude/projects/-home-psimmons/memory/lessons-learned.md` with the pattern
- Write rules that prevent the same mistake
- Ruthlessly iterate until mistake rate drops

**ONLY escalate to user if:**
- Repeated pattern cycles — same bugs appearing multiple times without improvement
- Wasting tokens on circular self-learning loops

**DO ask permission for:**
- Running reports or generating deliverables (external impact)
- Resource-intensive operations that require deliberate scheduling
- Actions with external visibility or side effects

## Project Priority Stack (top = highest)
1. **Clearwatch** — revenue pipeline (reports, cache, charts, grading)
2. **Infrastructure** — K8s cluster stability, cert-manager, DNS, storage
3. **Security Intelligence Business** — website, LinkedIn automation
4. **Gmail tracker / job search** — tooling and automation

When competing for Opus compute or agent time, higher-priority projects win.

## Cost Guardrails
- Opus agents: max 3 concurrent unless explicitly authorized
- Bulk LLM operations (>50 calls): require founder approval with cost estimate upfront
- Prefer Sonnet for exploration/routine, Opus for production-quality output
- Per-project budget rules (e.g., grading limits) live in project CLAUDE.md files

## Wake-the-Founder Triggers
Even in autonomous mode, STOP and wait for the founder if:
- Any operation estimated at >$5 in compute
- Deployment to production namespaces (not dev/staging)
- Any git push to main/master
- Data loss or corruption detected
- Agent stuck >45 min with no progress
- Repeated pattern cycle (same bug/error 3+ times)

## Reference

**Skills:** Deep debug → `superpowers:systematic-debugging` | Before implementing → `superpowers:brainstorming`

**Web Search:** SearXNG: `https://searxng.petersimmons.com/search?q={query}&format=json` | Fallback: WebSearch tool

**Visual Output:** Use skill `visual-output-standards` for SVG charts, color specs, and formatting rules.

**Learning System:** Detail → topic file | one-liner → MEMORY.md | behavioral rule → CLAUDE.md | Files: `~/.claude/projects/-home-psimmons/memory/` | Monthly review: `homelab:monthly-review`
