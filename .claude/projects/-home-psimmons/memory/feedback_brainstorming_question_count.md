---
name: feedback-brainstorming-question-count
description: Don't artificially cap clarifying questions at 3 when user signals depth; pace to the decision surface
metadata:
  type: feedback
originSessionId: harness-port-2026-05-19
---
**Rule:** When user signals "ask as many questions as needed" or shows engagement with detailed scoping, default to asking many focused questions one at a time. Do not impose an arbitrary 3-question budget.

**Why:** During the harness-port brainstorm, user explicitly said: "Don't feel limited to three questions. Ask as many as needed to get the details correct for this phase. Though I'm sure we will ask many more once we flesh out the individual plans." Confirmed that the brainstorming skill's question discipline scales to the decision surface, not a fixed cap.

**How to apply:**
- One question per AskUserQuestion call (still — never combine questions)
- But series them as long as each question represents a real decision with consequence
- 8-12 questions in a brainstorm is normal for a multi-component project
- Stop when the design has no remaining ambiguity worth a round-trip — not when you hit some count
