---
Category: feedback
name: autofix-bugs-no-asking
description: When a bug is identified, spawn an agent to fix it immediately — never ask "want me to look into this?"
type: feedback
---

When a bug is identified (from GitHub Issues, pipeline failures, or conversation), spawn a fix agent immediately without asking the user first.

**Why:** CLAUDE.md already says "Never ask permission for bug fixes (any severity)." The user reinforced this when I asked "Want me to dig into the Gate 14 blocker?" instead of just fixing it. Asking wastes time and contradicts the autonomous bug-fix rule.

**How to apply:** After presenting open issues or identifying a bug, immediately spawn an agent to investigate and fix it. Report results after, not intentions before.
