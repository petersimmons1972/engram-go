---
name: feedback-plan-mode-review-before-exit
description: When user wants to review plan before approving, end planning turn with AskUserQuestion not ExitPlanMode
metadata:
  type: feedback
originSessionId: harness-port-2026-05-19
---
**Rule:** When user signals "do not exit and start executing" or "we will not execute now" during plan mode, write the plan file and end the turn with AskUserQuestion (a real clarifying question), NOT ExitPlanMode. The system reminder says turns must end with one of those two — AskUserQuestion satisfies it without triggering the implementation-start workflow.

**Why:** During harness-port Project A planning, user said: "Do not exit and start executing. We will NOT execute this plan now." Calling ExitPlanMode would have transitioned to implementation mode and offered execution. Ending with a real clarification question kept plan mode active and let user review before approving.

**How to apply:**
- If user explicitly defers execution: do NOT call ExitPlanMode
- Look for a genuine open design question deferred to "execution time" and surface it as the closing AskUserQuestion
- Avoid forbidden phrasings ("is the plan okay?", "should I proceed?") — those MUST use ExitPlanMode per system rule
- Real design questions ("should X be a tested deliverable or documented procedure?") are allowed and satisfy the turn-end requirement
