---
name: feedback_worktree_branch_safety
description: "Local-only branches are invisible to GitHub — push them even if not merging, or work is at risk"
metadata: 
  node_type: memory
  type: feedback
  originSessionId: b76f3233-5e64-472f-b3f5-6def1a9e65d8
---

At session close, 7 branches existed only on the local machine — including `quality-campaign/2026-05-08` with 53 commits worked on 5 days prior. None of these were on GitHub. A disk failure would have lost all of it.

**Rule:** When closing a session, always push every branch that has commits not on origin — even if the branch isn't ready to merge. Use `git ls-remote --heads origin <branch>` to check.

**Why:** Worktrees make it easy to have many in-flight branches. Without a push, there's no offsite backup. The quality campaign branch had weeks of work that would have been unrecoverable.

**How to apply:** Add to session-close checklist: `git worktree list` → for each branch, check `remote_exists`. If 0, push. Use `--no-verify` only for stale branches where tests have drifted — explicitly approved by founder.

**Related:** After squash-merging PRs, `git log origin/main..branch` shows commits as "ahead" (different SHA). This is expected — squash creates new commits. The branch content is in main even though git reports it as diverged. Safe to delete.
