---
name: feedback-tdd-invariants-for-git-ops
description: For irreversible/multi-step git operations, define read-only invariants (I1-I10 style) and run them between every mutating step; halt on failure
metadata:
  type: feedback
originSessionId: home-dir-untangle-2026-05-20
---
**Rule:** Before executing any irreversible git operation (merge across long-divergent branches, push to remote with non-trivial history, branch rewrite, etc.), enumerate read-only invariants as a checklist. After every mutating step, re-run the applicable subset. **Halt on first failure.** Recover from the snapshot tag, never push through unexpected state.

**Pattern from home-dir untangle 2026-05-20:**
| I | Invariant | Check |
|---|---|---|
| I1 | Critical SHA still reachable | `git merge-base --is-ancestor <sha> <ref>` |
| I3 | Working tree clean delta | `git status --porcelain \| wc -l` vs baseline |
| I4 | Snapshot tag exists at expected SHA | `git rev-parse <tag>` |
| I5 | Worktrees still resolve | `git worktree list` + `git -C <each> rev-parse HEAD` |
| I6 | Push succeeds without --force | `git push --dry-run` |
| I7 | Local == remote post-push | `git rev-parse <ref>` == `git rev-parse <remote>/<ref>` |
| I10 | Bundle backup verifies | `git bundle verify <bundle>` |

**Why:** This framework caught the "github remote moved during planning" risk exactly when the risk register predicted it (between Stage 0 baseline and the post-merge fetch). The mid-operation halt let us re-plan with patch-id comparison instead of bulldozing forward into a conflict.

**How to apply:**
- Define invariants in the plan, not improvised mid-execution
- Include both "what should be true" (positive: SHA reachable) and "what should not have changed" (negative: bare repo SHA unchanged)
- Make the snapshot tag step (`git tag pre-<op>-<date>`) and bundle backup step BEFORE any mutation — they're the recovery surface
- "Halt on failure" is the discipline that turns the framework into safety, not theater
