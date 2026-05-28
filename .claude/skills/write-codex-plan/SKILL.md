---
name: write-codex-plan
description: >
  Produce implementation plans destined for Codex via ~/bin/queue-agent. Enforces
  the 6-section plan structure (Context, Files, Acceptance TDD, Spec refs, Constraints,
  Out of scope) and 10 operational protocols governing the Claude↔Codex Hybrid Workflow.
  Use for: Codex handoff, queue-agent brief, implementation plan for Codex, agent/codex
  GitHub issue, Codex task. Triggers: write a codex plan, queue to codex, codex handoff,
  queue-agent, write a plan for codex, implementation plan for codex, create a codex issue.
---

# write-codex-plan

Produces implementation plans destined for Codex via `~/bin/queue-agent`. Enforces
a 6-section plan structure and 10 operational protocols so handoffs do not require
out-of-band clarification.

## When This Skill Fires

- Founder asks for a plan that will be implemented by Codex
- About to invoke `queue-agent` to create an `agent/codex` GitHub issue
- Continuing work on an existing `agent/codex/*` issue
- Cross-reference: `~/projects/codex/README.md` § Claude ↔ Codex Hybrid Workflow

## The 6-Section Plan Template

Every plan handed to Codex MUST use these six sections in this exact order.
The middle four are validated by `queue-agent` and will cause the command to
fail if absent or empty. Context and Out of scope are project conventions.

---

```markdown
## Context

<!-- Why this work. The problem, prompting event, intended outcome. 1–2 paragraphs. -->

## Files

<!-- Bullet list of every file to be created, modified, or deleted.
     Format: `path — one-line description of the change` -->

- path/to/file.ext — description of change

## Acceptance (TDD)

<!-- Test-first acceptance criteria. Each criterion is a testable statement
     paired with the test that verifies it: file path + test name. -->

- [ ] Criterion description
  - Test: `path/to/test_file.py::test_function_name`

## Spec refs

<!-- Links to relevant docs, prior issues, ADRs, RFCs, external specs. -->

- GitHub issue #N — title
- https://link/to/doc

## Constraints

<!-- Hard rules for this plan. -->

- Must not modify X
- Must use library Y
- Do not push to main
- Must pass existing test suite with no regressions

## Out of scope

<!-- Explicit list of related work that is NOT part of this plan.
     Prevents scope creep. -->

- Feature Z — tracked separately in #N
```

---

## The 10 Codex Operational Protocols

### 1. Plan structure
Every plan handed to Codex MUST use the 6-section template above. `queue-agent`
enforces the middle 4 (`## Files`, `## Acceptance (TDD)`, `## Spec refs`,
`## Constraints`); Context and Out of scope are project conventions enforced here.

### 2. Branch strategy
Codex creates a feature branch named `agent/codex/issue-<N>-<short-slug>`, pushes
it, and opens a **draft PR** linking back to the issue. The founder reviews and
merges. Codex does NOT merge its own PRs.

### 3. Completion signal
When work is complete, Codex applies the `agent/codex/done` label, posts a final
comment containing commit hash(es), PR link, test results, and any notable findings,
then **closes the issue itself**. The PR remains open until the founder merges.

### 4. Blocked signal
When unable to proceed, Codex uses **best-effort with explicit workarounds**: continues
the work using a reasonable workaround, and posts a comment that prominently labels
what was worked around and why. Apply `agent/codex/blocked` label only if the work
cannot proceed at all even with workarounds. The blocker comment must be unmissable —
never silent compromise.

### 5. Push policy
Codex pushes `agent/codex/*` feature branches freely. **Never** pushes to
main/master/release branches. Mirrors the Claude-side publish-boundary rule
(CLAUDE.md AP.11).

### 6. Scope discipline
Pragmatic. Codex implements the plan plus **small adjacent fixes** (typos, dead imports,
broken comments, obvious linter findings in touched files). Every adjacent fix is called
out in the PR description. Anything larger goes to a new issue (see protocol 8).

### 7. Disagreement protocol
If Codex thinks the plan is wrong, it posts an objection comment on the issue AND a
**proposed alternative plan**, then waits for founder resolution. Codex does NOT implement
the original plan if it believes the alternative is materially better. @-mentions the
founder.

### 8. Out-of-scope bugs
If Codex finds a bug not in the current plan:

- **< 5-minute trivial fix** (typo, dead import, missing newline, etc.): fix in current
  commit, note in PR description under "Adjacent fixes."
- **Anything larger**: file a new GitHub issue with appropriate severity label
  (`severity/blocker`, `severity/nice-to-have`, etc.), link from current issue comment,
  stay in current scope.

Mirrors Claude-side AP.12 bug accountability.

### 9. Memory (Engram)
At end of each Codex work session:

- `memory_store` decisions, error patterns, and notable context with project tag matching
  the repo (`clearwatch`, `homelab`, `engram`, `global`, `3dprint`, `family`)
- Before any architectural or design choice during the session, call
  `memory_recall(<topic>, project=<repo>)` first

Mirrors Claude-side QC.6.

### 10. TDD discipline
**Always required.** Every commit must include a test that exercises the change — even
refactors and dependency upgrades. New behavior follows the strict failing-test-first
pattern. Refactors should retain or improve existing coverage. No exceptions for
"obvious" changes. Mirrors Claude-side QC.2/AP.8.

## Invocation Pattern

Once the plan is written, execute in this order:

1. **Plan is complete** — all 6 sections populated, all required sections non-empty.
2. **Self-check** — confirm the 4 `queue-agent`-required sections are non-empty
   (`## Files`, `## Acceptance (TDD)`, `## Spec refs`, `## Constraints`). Confirm the
   plan is in scope per the founder's request.
3. **Save plan to file**, then invoke queue-agent:
   ```bash
   queue-agent --agent codex --repo <repo> --title "short descriptive title" \
     --brief <plan-file> --priority p<N>
   ```
   Valid `--repo` values: `engram-go olla aifleet factvault agent-gateway instinct harness-port yourai`
   Valid `--priority` values: `p0` (critical) · `p1` (high) · `p2` (normal, default) · `p3` (low)
4. **Report the issue URL** back to the founder.

## Cross-References

- Architecture: `~/projects/codex/README.md` § Claude ↔ Codex Hybrid Workflow
- Plan that established this: `~/.claude/plans/start-with-the-bridge-flickering-torvalds.md`
- Tooling: `~/bin/queue-agent`, `~/projects/codex/src/bin/codex-handoff.rs`, `codex-mcp` server
- Tripwire: GitHub issue petersimmons1972/homelab-config#60 (2026-06-11 review)
