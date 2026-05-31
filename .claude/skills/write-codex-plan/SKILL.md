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

## The 11 Codex Operational Protocols (summary)

The canonical protocol text lives at `petersimmons1972/claude-codex/protocol/operational-protocols.md`.
Full text: **petersimmons1972/claude-codex/protocol/operational-protocols.md**

Summary:

1. **Plan structure** — 6-section template required; `queue-agent` validates 4 sections
2. **Branch strategy** — `agent/codex/issue-<N>-<slug>`; draft PR; founder merges
3. **Completion signal** — `done` label + commit hash + PR link + test results; close issue
4. **Blocked signal** — best-effort with explicit workarounds; `blocked` label only when truly stuck
5. **Push policy** — push feature branches freely; never push to main/master/release
6. **Scope discipline** — implement plan + small adjacent fixes; anything larger → new issue
7. **Disagreement protocol** — post objection + alternative plan; wait for founder; no silent substitution
8. **Out-of-scope bugs** — trivial (<5 min): fix in commit; larger: file new issue, stay in scope
9. **Memory (Engram)** — `memory_store` decisions/errors at session end; `memory_recall` before arch choices
10. **TDD discipline** — every commit includes a test; failing test before implementation; no exceptions
11. **Subagent delegation** — delegate independent/parallelizable work to subagents; keep orchestration in main context

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

- Architecture and all protocol text: `petersimmons1972/claude-codex`
- Plan that established this: `~/.claude/plans/start-with-the-bridge-flickering-torvalds.md`
- Tooling: `~/bin/queue-agent`, `petersimmons1972/codex` (`codex-handoff`, `codex-mcp`)
- Tripwire: GitHub issue petersimmons1972/homelab-config#60 (2026-06-11 review)
