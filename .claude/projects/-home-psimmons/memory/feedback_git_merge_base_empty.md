---
name: feedback-git-merge-base-empty
description: When git merge-base A B returns empty, A and B have NO common ancestor — wholly unrelated histories, not just divergent
metadata:
  type: feedback
originSessionId: home-dir-untangle-2026-05-20
---
**Rule:** `git merge-base A B` returns empty output → A and B are **wholly unrelated histories** sharing only branch/ref names. Not a deep divergence; not the same project. Treat as separate repos.

**Why:** During home-dir untangle 2026-05-20, `git merge-base master trunas/master` returned empty. Initial reading: deep divergence requiring complex merge. Correct reading: trunas bare repo at `/mnt/public/git-repos/home.git` is an unrelated project that happens to share `master` as a branch name. The 48 disjoint commits represented some other machine's home-dir or an early dotfiles experiment.

The `git rev-list --left-right --count A...B` output is the second diagnostic: when histories are unrelated, every commit on each side appears as "ahead" because there's no shared base. So `168\t48` against an empty merge-base means "168 on left, 48 on right, ZERO shared."

**How to apply:**
- Empty merge-base → STOP and ask "is this the same project at all?" before considering merge strategies
- `--allow-unrelated-histories` is almost always wrong; merging unrelated projects conflates them
- The right action is usually to remove or rename the misconfigured remote, then audit the orphan history separately
- Verify with second test: `git log --format=%h <remote>/<branch> | head` — does the most-recent commit look like it belongs to this project?
