---
name: feedback-scope-discipline-sub-projects
description: When project is decomposed into sub-projects, focus tightly on the current one; don't pre-scope future sub-projects
metadata:
  type: feedback
originSessionId: harness-port-2026-05-19
---
**Rule:** Once a multi-component project has been decomposed (A/B/C sub-projects), focus brainstorming and clarifying questions on the current sub-project only. Do not surface questions about the next sub-project's design.

**Why:** During harness-port brainstorm, after decomposing into Projects A/B/C, user said: "You do not need to scope out all the projects at once. We can plan the later ones... later." Drift into B/C wastes context and pre-commits decisions that should be re-litigated when B/C actually begin.

**How to apply:**
- After decomposition is agreed, mark B and C tasks as DEFERRED in TaskList
- Phrase brainstorming questions for the current sub-project only
- If a question requires a B/C decision to answer, note the cross-cutting concern but defer the actual decision
- Acceptable to mention "this principle will be cited by B" — not acceptable to design B's hooks
