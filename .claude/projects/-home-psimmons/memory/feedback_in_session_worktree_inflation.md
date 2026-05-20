---
name: feedback-in-session-worktree-inflation
description: Mid-session subagent dispatches (Explore, Plan, opus-advisor) can create background worktrees that inflate git worktree list count; account for in invariants
metadata:
  type: feedback
originSessionId: home-dir-untangle-2026-05-20
---
**Rule:** When dispatching subagents (`Agent` tool with `subagent_type: Explore`, `Plan`, `opus-advisor`, etc.) mid-session, the harness may create worktrees in `~/.claude/worktrees/agent-<hash>` for them. These appear in `git worktree list` at the current HEAD, marked `locked`. They survive past the agent's exit.

This means a "worktree count" invariant taken at Stage 0 will look inflated by Stage N if subagents have been dispatched between. The semantic invariant ("no worktree breaks") still holds — they all resolve their HEADs cleanly — but the count check is unreliable.

**Why:** During home-dir untangle 2026-05-20, Stage 0 baseline showed 4 worktrees. After Explore + opus-advisor dispatches mid-stage, Stage 2 post-merge invariant check showed 8. Triggered a false "I5 failed" alarm. Investigation showed 4 of those were locked agent worktrees at the post-merge tip (`0a5150f`) created during my dispatches earlier in the session — not broken, not stale, just new.

**How to apply:**
- Worktree-count invariants in a plan: anchor to a semantic check ("each worktree's `git rev-parse HEAD` succeeds") rather than a count equality
- If a plan must rely on count: dispatch all subagents BEFORE Stage 0 baseline, or count again after each dispatch
- Agent worktrees clean up when their parent agent completes — they're inert if locked but not a bug
