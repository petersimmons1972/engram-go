# QA Personas — Six-Persona Fault-Finder Sweep

## What This Skill Does

The QA Personas sweep dispatches six independent adversarial reviewers against
the same artifact in parallel. Each persona reviews through a distinct lens
with no awareness of the others' findings. The coordinator (you) then
deduplicates, ranks by severity, and gates the ship decision.

The personas are **observer-only and read-only** — no writes, no edits, no
code changes. They report findings; you decide what to fix.

Source: spawn-patterns.md Pattern 6 / `~/.claude/commands/qa-personas.md`

## When to Invoke

**Mandatory pre-ship gate for any user-facing work.** Invoke before:

- Merging a feature branch
- Closing any GitHub Issue that touches user-facing code, CLI, or docs
- After a "feels solid" milestone
- After any documentation update
- Before publishing a CLI release

## The Six Personas

| Slug | Display Name | Lens |
|------|--------------|------|
| `skeptical-staff-engineer` | Skeptical Staff Engineer | side effects, hidden deps, trust assumptions |
| `security-reviewer` | Security Reviewer | secret leaks, permission boundaries, RCE surface |
| `new-maintainer` | New Maintainer | docs clarity, onboarding path, source-required gaps |
| `heavy-cli-user` | Heavy CLI User | command consistency, composability, idempotency |
| `operator-sre` | Operator / SRE | observability, failure recovery, alerting |
| `docs-first-newcomer` | Docs-First Newcomer | README accuracy, example correctness |

Full persona profiles are in `persona-<slug>.md` files alongside this README.

## Two-Round Methodology

**Round 1:** Run all six personas against the target. Aggregate findings.
Classify by severity. Any `blocker` or `critical` finding blocks ship.

**Fix blockers:** Address every blocker/critical from Round 1.

**Round 2:** Re-run the same six personas against the post-fix artifact.
Verify (do not assume) that all Round 1 blockers are resolved.

**Ship only when Round 2 returns zero blockers/critical.**

If Round 2 still has blockers, the fixes were incomplete or introduced
regressions. Do not ship. Fix and re-run.

## Exit Criteria

- Round 2 complete
- Zero `blocker` and zero `critical` findings in Round 2 output
- All Round 1 blockers confirmed resolved (read Round 2 output; don't assume)
- Remaining findings are `friction`, `medium`, `nitpick`, or `low`
- These remaining findings are filed as GitHub Issues with `severity/nice-to-have`

## Severity Rubric (Aggregated)

| Severity | Action |
|----------|--------|
| `blocker` / `critical` | Block ship. Fix before Round 2. |
| `serious` / `high` | Request changes. File as Issue. Can ship with commitment to fix. |
| `friction` / `medium` | Note and file. Does not block. |
| `nitpick` / `low` | Mention and file as `severity/nice-to-have`. |

## Deduplication Rule

If two or more personas independently raise the same finding, tag it
**HIGH-CONFIDENCE** (multi-lens convergence). Prioritize these in remediation.
