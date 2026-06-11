---
name: write-codex-plan
description: >
  Produce implementation plans destined for Codex via ~/bin/queue-agent. Enforces
  the 7-section plan structure (Context, Files, Acceptance TDD, Spec refs, Constraints,
  Out of scope, Model) and 21 operational protocols governing the Claude↔Codex Hybrid Workflow.
  Use for: Codex handoff, queue-agent brief, implementation plan for Codex, agent/codex
  GitHub issue, Codex task. Triggers: write a codex plan, queue to codex, codex handoff,
  queue-agent, write a plan for codex, implementation plan for codex, create a codex issue.
---

# write-codex-plan

Produces implementation plans destined for Codex via `~/bin/queue-agent`. Enforces
a 7-section plan structure and 21 operational protocols so handoffs do not require
out-of-band clarification.

## When This Skill Fires

- Founder asks for a plan that will be implemented by Codex
- About to invoke `queue-agent` to create an `agent/codex` GitHub issue
- Continuing work on an existing `agent/codex/*` issue
- Cross-reference: `~/projects/codex/README.md` § Claude ↔ Codex Hybrid Workflow

## The 7-Section Plan Template

Every plan handed to Codex MUST use these seven sections in this exact order.
The middle four are validated by `queue-agent` and will cause the command to
fail if absent or empty. Context, Out of scope, and Model are project conventions.

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

## Model

<!-- Claude fills this in at queue time. Sets the floor — Codex may escalate one tier
     with logged justification. -->
tier: standard        # codex | standard | elevated
effort: high          # low | medium | high | xhigh (xhigh = elevated only)
```

---

## The 21 Codex Operational Protocols (summary)

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
12. **PR serialization (overlapping files):** If the plan creates 2+ PRs that touch overlapping files, sequence them explicitly. State which PR must merge first and make it a hard prerequisite: Codex does not begin PR N+1 until PR N is confirmed merged on origin/main. Never parallelize PRs with shared file scope.
13. **Rebase gate before push:** Before any `git push` on an implementation branch, Codex must run `git fetch origin && git rebase origin/main`. If conflicts arise, resolve them and re-run the full test suite before pushing. A branch that hasn't been rebased against current main is not ready to push regardless of local test results.
14. **Append-only PR updates:** Agents push additional commits to fix CI failures — never rewrite branch history. Force-push on PR branches is prohibited. Squash at merge time. For related queue items in the same subsystem, stack PRs (each targets the previous branch, not main); do not begin a stacked PR until the base branch is confirmed merged.
15. **Auto-merge gate:** When opening a PR on olla, aifleet, or engram-go, immediately run `gh pr merge --auto --squash --delete-branch` to register auto-merge intent. GitHub will execute the merge when CI passes. Do not add manual merge-polling steps to plans for these repos. If `--auto` is rejected (e.g. GitHub Free plan restriction on private repos), fall back to a background watcher agent that polls `gh pr checks` every 60s and executes `gh pr merge --squash --delete-branch` when all checks pass. Plans must confirm: branch rebased on main, all required CI checks pass.
16. *(Reserved — available for future protocol)*
17. *(Reserved — available for future protocol)*
18. *(Reserved — available for future protocol)*
19. *(Reserved — available for future protocol)*
20. **Model Self-Assessment.** At session start, read `## Model` from the plan. Default to `tier: standard, effort: high` if absent. Self-assess scope; escalate ONE tier with logged justification if actual scope exceeds declared tier. Do not downgrade. Effort may be raised independently. See `protocol/operational-protocols.md` § Protocol 20 for full escalation triggers.
21. **Parallel Session Mandate.** The poller supports up to 3 concurrent sessions. Slot 1 claims any `agent/codex/queued` issue. Slots 2–3 require the `parallel-safe` label and must pass a runtime file-overlap backstop. Claude is responsible for `parallel-safe` correctness at queue time. See `protocol/operational-protocols.md` § Protocol 21.

## Invocation Pattern

Once the plan is written, execute in this order:

1. **Plan is complete** — all 7 sections populated, all required sections non-empty.
2. **Self-check** — confirm the 4 `queue-agent`-required sections are non-empty
   (`## Files`, `## Acceptance (TDD)`, `## Spec refs`, `## Constraints`). Confirm the
   plan is in scope per the founder's request.
3. **Tier selection** — choose `## Model` tier using the ladder:

   | Tier | Model | Use for |
   |------|-------|---------|
   | `codex` | `gpt-5.3-codex-spark` | Single-file, mechanical transforms, linting |
   | `standard` | `gpt-5.4` | Multi-file, feature additions, everyday bug fixes |
   | `elevated` | `gpt-5.5` | Architecture, >5 files, cross-system refactors, new subsystems |

4. **Parallel-safe check** — if this issue's file set does NOT overlap any currently queued or in-progress item, add `--parallel-safe`:
   ```bash
   gh issue list --repo petersimmons1972/<repo> \
     -l 'agent/codex/queued,agent/codex/in-progress' --json number,body
   ```
   Compare `## Files` sections. If no overlap: safe to add `--parallel-safe`.

5. **Save plan to file**, then invoke queue-agent:
   ```bash
   queue-agent --agent codex --repo <repo> --title "short descriptive title" \
     --brief <plan-file> --priority p<N> [--parallel-safe]
   ```
   Valid `--repo` values: `engram-go olla aifleet factvault agent-gateway instinct harness-port yourai`
   Valid `--priority` values: `p0` (critical) · `p1` (high) · `p2` (normal, default) · `p3` (low)
6. **Report the issue URL** back to the founder.

## Cross-References

- Architecture and all protocol text: `petersimmons1972/claude-codex`
- Plan that established this: `~/.claude/plans/start-with-the-bridge-flickering-torvalds.md`
- Tooling: `~/bin/queue-agent`, `petersimmons1972/codex` (`codex-handoff`, `codex-mcp`)
- Tripwire: GitHub issue petersimmons1972/homelab-config#60 (2026-06-11 review)
