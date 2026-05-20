---
name: feedback-pushback-on-destructive-choices
description: When user picks all-of-the-above including destructive options, push back specifically against the destructive item; user often reconsiders
metadata:
  type: feedback
originSessionId: harness-port-2026-05-19
---
**Rule:** When user selects multiple options including one with destructive or anti-discipline consequences, do not silently accept the bundle. Push back specifically on the problematic item with concrete reasoning. User often reconsiders.

**Why:** During harness-port brainstorm, user picked all 4 backfill options including "rewrite recent commit messages with rationale block" — destructive (force-push), fabricates reasoning, contradicts the discipline the template is meant to enforce. Pushed back with 4 specific points; user accepted the recommendation to drop it.

**How to apply:**
- Push back BEFORE locking the decision in, not after
- Name the specific anti-discipline: "this fabricates reasoning that wasn't there" beats "this might be risky"
- Frame the alternative concretely (drop X, keep others; OR limit X to scope Y where harm is contained)
- Be willing to lose the pushback — user is allowed to overrule with full information; that's different from getting forced into a bad path by default
