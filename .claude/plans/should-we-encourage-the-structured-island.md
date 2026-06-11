# Plan: Parallel Queue Processing + Codex Model Selection Mandates

## Context

Two related enhancements to the Claude-Codex protocol:

**1. Parallel Queue Processing.** The protocol currently enforces one Codex session at a time via a global PID lock. Worktree isolation is already built per-issue, so the only blockers are doctrine ("work one issue at a time") and the poller lock. This change lifts the cap to 3 concurrent sessions using a hybrid overlap guard: Claude labels issues `parallel-safe` at queue time; the runtime does a lightweight file-path backstop before claiming a second or third slot.

**2. Codex Model Selection Mandates.** No tier doctrine exists for Codex model selection. The current config hardcodes `gpt-5.3-codex-spark` for all work regardless of complexity. Claude should encode a floor in the plan; Codex should be able to self-escalate (with logged justification) if it discovers higher complexity mid-session. This mirrors how Claude's M1/M2 mandates and ADV.1-ADV.5 work for Anthropic model selection.

---

## Model Tier Ladder (Codex)

| Tier name | Model | Use for |
|-----------|-------|---------|
| `codex` (low) | `gpt-5.3-codex-spark` | Single-file changes, mechanical transforms, linting/formatting, single-function fixes |
| `standard` (mid) | `gpt-5.4` | Multi-file implementation, feature additions, everyday bug fixes |
| `elevated` (high) | `gpt-5.5` | Architecture changes, >5 files, cross-system refactors, new subsystems, performance work |

Effort ladder (all tiers): `low` → `medium` → `high` → `xhigh` (GPT-5.5 only)

---

## Design

### Part A — Parallel Queue Processing

**Label additions (`label-state-machine.md`):**
- `parallel-safe` — set by Claude at queue time; signals no file-set overlap with any other queued/in-progress issue. Required on slots 2 and 3 (slot 1 can claim any queued issue without this label).

**`queue-agent` changes:**
- Add `--parallel-safe` flag → adds `parallel-safe` label to the created issue
- Also parse `## Model` section from the brief and add cosmetic label `model/codex`, `model/standard`, or `model/elevated`

**`codex-poll` changes (locate with `fd codex-poll ~/bin ~/projects`):**
- Replace single global PID lock with slot-based locking: up to 3 slots, each with its own lock file (`codex-poll.slot{1,2,3}.pid` in `~/.cache/claude-codex/`)
- Slot 1 claims any `agent/codex/queued` issue (existing behavior)
- Slots 2–3 require `parallel-safe` label on candidate issue AND runtime backstop:
  - For each `agent/codex/in-progress` issue: fetch its branch, run `git diff --name-only origin/main...HEAD` in its worktree
  - Extract `## Files` section from the candidate issue body
  - If any file appears in both sets: skip candidate, log `"file-overlap: skipping #N"`
- Worktree creation is already per-issue — no change needed there

**`AGENTS.md` (claude-codex repo) doctrine update:**
- Change: "Work one issue at a time"
- To: "Work up to 3 issues in parallel; slots 2–3 require the `parallel-safe` label and pass the runtime file-overlap backstop"

### Part B — Model Selection Mandates

**New `## Model` section in `plan-template.md`** (7th section, appended after `## Out of scope`):

```markdown
## Model

<!-- Claude fills this in at queue time. Sets the floor — Codex may escalate one tier
     with logged justification. -->
tier: standard        # codex | standard | elevated
effort: high          # low | medium | high | xhigh (xhigh = elevated only)
```

**`queue-agent` validation**: parse `## Model` section; warn (not block) if section is absent.

**New protocol in `operational-protocols.md`** (next available number after current max):

> **Model Selection Mandate.** At session start, read `## Model` from the plan.
> Self-assess: if actual scope exceeds declared tier, escalate one tier and log the reason
> in the first PR comment (`Model escalated: standard → elevated — plan touches 8 files,
> crosses auth/db boundary`). Do not downgrade below the declared floor. Effort level may
> be raised independently of tier.
>
> Escalation triggers:
> - Plan touches >5 files but tier is `codex` → escalate to `standard`
> - Plan involves a new abstraction boundary or architectural change but tier is `standard` → escalate to `elevated`
> - Stuck/blocked reasoning after 2 attempts → raise effort first, then tier if still stuck

**New protocol in `operational-protocols.md`** (adjacent to Model Selection):

> **Parallel Session Mandate.** The poller supports up to 3 concurrent sessions.
> Slot 1: claim any `agent/codex/queued` issue.
> Slots 2–3: candidate must have `parallel-safe` label; runtime backstop must pass (no file-path overlap with any in-progress branch).
> Claude is responsible for `parallel-safe` correctness at queue time; runtime backstop is a safety net, not primary enforcement.

**`write-codex-plan` skill update** (`~/.claude/skills/write-codex-plan/SKILL.md`):
- Add `## Model` to the section list (now 7 sections)
- Add tier-selection guidance with the ladder table above
- Add reminder: set `--parallel-safe` when the issue's file set doesn't overlap any currently queued/in-progress item (check with `gh issue list -l 'agent/codex/queued,agent/codex/in-progress'`)

---

## Files to Modify

| File | Change |
|------|--------|
| `~/projects/claude-codex/protocol/plan-template.md` | Add `## Model` section |
| `~/projects/claude-codex/protocol/operational-protocols.md` | Add Model Selection + Parallel Session mandates (next available protocol numbers) |
| `~/projects/claude-codex/protocol/label-state-machine.md` | Add `parallel-safe` label definition + transition rules |
| `~/projects/claude-codex/AGENTS.md` | Update one-at-a-time doctrine; add model self-assessment section |
| `~/.claude/skills/write-codex-plan/SKILL.md` | Add `## Model` guidance + `--parallel-safe` reminder |
| `~/bin/queue-agent` | Add `--parallel-safe` flag; parse `## Model` for model-tier label |
| `~/bin/codex-poll` *(verify path first)* | Slot-based locking (3 slots); file-overlap backstop for slots 2–3 |

---

## Verification

1. **Slot locking**: Start two `codex-poll` instances; confirm second acquires slot 2, third slot 3, fourth is rejected cleanly
2. **Overlap backstop**: Queue two `parallel-safe` issues touching the same file; confirm runtime skips the second with `file-overlap` log entry
3. **Model label**: Run `queue-agent --parallel-safe --brief <plan-with-model-section>`; confirm `model/standard` and `parallel-safe` labels appear on the created issue
4. **Plan template**: Verify `write-codex-plan` skill prompts for `## Model` tier + effort
5. **End-to-end**: Queue two non-overlapping `parallel-safe` p2 issues; observe two concurrent Codex sessions, each opening their own PRs on separate branches
