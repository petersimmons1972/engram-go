---
name: Gordon Ramsay reviews all graphics with text
description: All SVG/graphics containing text labels must be reviewed by gordon-ramsay agent before delivery
type: feedback
Category: feedback
---

All graphics we create that contain text must be approved by Gordon Ramsay (gordon-ramsay agent) before being delivered to the user.

**Why:** Text in SVG has cross-platform rendering issues (Unicode glyphs, font stacks, overflow, collision with geometric elements) that are easy to miss during creation but visible to readers. Ramsay catches geometric errors, text collisions, and readability failures.

**How to apply:** After any agent produces an SVG/graphic with text labels, dispatch gordon-ramsay to review before presenting to the user. Fix findings rated HIGH or CRITICAL before resubmission. This is a quality gate, not optional.
