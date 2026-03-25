---
name: feedback-strategic-fixes
description: Founder prefers fixing classes of problems over patching individual instances — strategic quality work over tactical hotfixes
type: feedback
Category: feedback
---

Fix classes of problems, not instances. When multiple issues share a root cause, build a validator/gate that prevents the entire class — not N separate patches.

**Why:** Founder explicitly values strategic thinking. The quality campaign proved this: 22 Orwell observations were 5 systemic patterns. 10 validators now prevent recurrence of all 22+ future instances. Patching individually would have been 22 fixes that don't prevent #23.

**How to apply:** When triaging bugs, always ask "is this an instance of a pattern?" before fixing. If yes, design the fix at the pattern level (validator, gate, policy rule) not the instance level. Use quality campaigns for batch pattern work, Patton/Rommel for genuine one-offs only.
