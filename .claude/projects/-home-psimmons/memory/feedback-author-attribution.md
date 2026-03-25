---
name: feedback-author-attribution
description: Every new file must have an author header identifying which general wrote it — enables bug accountability tracking
type: feedback
Category: feedback
---

Every new or substantially rewritten file must have an author attribution comment in the first 5 lines:

```python
# Author: <General Name> (<XP> XP) — <Operation/Task>
# Date: <YYYY-MM-DD>
```

**Why:** When buggy code is found, we need to trace it to the general who wrote it for service record accountability. Without attribution, bugs are orphaned — nobody's record reflects the failure, and nobody learns.

**How to apply:**
1. Every new file: add author header before any imports
2. Every substantial rewrite: update the author header
3. Commit messages continue to include `Co-Authored-By:` for git-level tracking
4. When a bug is found: `git blame` identifies the commit, author header identifies the general
5. The general's service record gets the bug attribution (positive or negative XP impact)

**Example:**
```python
# Author: Groves (75 XP) — Operation Data Backbone Phase 2
# Date: 2026-03-23
# Reviewed: Rickover (1,920 XP) — Phase 3 audit
```
