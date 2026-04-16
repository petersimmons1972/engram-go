---
name: Terse affirmatives ("Go", "Yes", "Do it") require scope confirmation, not execution
description: When user sends a one-word affirmative after a long planning turn, confirm what they're affirming before taking destructive or creating action
type: feedback
Category: feedback
---

When the user sends terse affirmatives like "Go", "Yes", "Do it", or "Sure" — especially after a long research/planning turn with multiple recommendations — do NOT assume which recommendation they're green-lighting. Ask which one, or summarize the single most likely interpretation and wait for confirmation.

**Why:** On 2026-04-07, after a ~500-line pi-vs-cc landscape scan with seven patterns + six design questions + a recommended "lowest-friction starting point" (three YAML pipelines), the user sent "Okay. Go" which I read as "build the three pipelines." I started pre-flight. The user then said "Wait. I didn't have anything cooking in this window" — they had not been intending to trigger implementation. The "Go" was either a stale buffer, a continuation thought, or meant something different. I had to stop mid-execution.

**How to apply:** After any multi-recommendation response, if the user replies with ≤3 words of affirmation, treat it as *confirm which thing I mean*, not *execute now*. Exception: if I explicitly asked a binary yes/no question in the immediately preceding turn, treat it as the answer to that question. Default to one clarifying sentence before running Write/Edit/Bash. The cost of a one-sentence confirmation is small; the cost of writing files the user didn't want is real friction and context waste.
