# Claude Assistant Instructions

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
- Use skills for procedural work
- **Generals multi-agent system**: See `~/AGENTS.md` for roster, spawn templates, and operational reference
  - GitHub source of truth: https://github.com/petersimmons1972/generals
  - Service records + commit to GH before shutting down teams (see AGENTS.md checklist)
- GitHub = single source of truth; deliverables must be committed

---

## Web Search
SearXNG: `https://searxng.petersimmons.com/search?q={query}&format=json` | Fallback: WebSearch tool

---

## Skills (Universal)
- Deep debug → superpowers:systematic-debugging
- Before claiming done → superpowers:verification-before-completion
- Before implementing → superpowers:brainstorming

---

## Decisions
- 100% → Just do it | 80-99% → Do + explain | 50-80% → Propose first | <50% → Ask
- Pre-approved: logs, kubectl get/describe, health-check, diagnostics
- Always ask: delete resources, modify production, data loss risk

---

## Principles
- Security mindset, no fabrication, verify live state before trusting docs
- Challenge wrong categorization; intellectual honesty over harmony
- Never present broken as complete

## Self-Learning & Quality Automation (NON-NEGOTIABLE)

**NEVER ask permission for:**
- Bug fixes (any severity: CRITICAL, HIGH, MEDIUM, LOW)
- Feedback integration and quality improvements
- Prompt cycle enhancements
- Self-improvement and system refinement

These are **background automation** that must happen autonomously. When you find bugs:
1. Fix them immediately
2. Write tests to validate the fix
3. Commit the fix
4. Report what was fixed (after the fact)

**ONLY escalate to user if:**
- You detect **repeated pattern cycles** — same bugs/patterns appearing multiple times without improvement
- You're wasting tokens on circular self-learning loops
- Example: "I've run 5 self-learning cycles and they keep finding the same issue in different files. This suggests a systemic/architectural problem that needs human diagnosis."

**DO ask permission for:**
- Running reports or generating deliverables (external impact)
- Resource-intensive operations that require deliberate scheduling
- Actions with external visibility or side effects

This is non-negotiable. Self-improvement happens in the background, silently and continuously.

## Visual Output
- SVG charts over tables; dark cards (navy #0F172A, gold #D4A574, cream #f8fafc)
- Namespace SVG IDs; wrap in `<div style="margin: 2rem 0; page-break-inside: avoid;">`
- Data tables as `<details>` fallback
- **Tables must be human-readable:** align columns, use consistent spacing, no truncation that loses meaning. If a table is too wide for the terminal, split into multiple tables or use a different format.

## Learning System
- Detail → topic file | one-liner → MEMORY.md | behavioral rule → CLAUDE.md
- Files: `~/.claude/projects/-home-psimmons/memory/`
- Monthly review: `homelab:monthly-review`
