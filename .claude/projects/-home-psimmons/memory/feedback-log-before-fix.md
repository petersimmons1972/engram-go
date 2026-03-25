---
name: feedback-log-before-fix
description: Always file a GitHub issue BEFORE fixing a bug — even if the fix takes 30 seconds. The issue is the historical record.
type: feedback
Category: feedback
---

File the issue BEFORE fixing the bug. Not after. Not "I'll file it when I'm done." Before.

**Why:** During the chart rebuild (2026-03-22), we fixed 5+ text overlap bugs iteratively without filing issues. The fixes landed and tests passed, but there's no historical record of: what the defect was, which reports it affected, what the root cause was, or how many times this class of problem has recurred. Without issues, we can't measure defect rates, track patterns across campaigns, or prove that an architectural change (like the chart rebuild) actually reduced a specific class of failures.

**How to apply:** When you find a bug during report generation, testing, or code review:
1. `gh issue create` with the defect details
2. THEN fix it
3. Reference the issue number in the commit message
4. Close the issue in the commit

The 30 seconds to file an issue saves hours of forensic git archaeology later. And it feeds the pattern detection that drives quality campaigns — you can't fix classes of problems if you don't know the class exists.
