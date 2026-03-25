---
name: feedback-gate-tolerances
description: Content quality gates should have 5% tolerance bands — don't fail on 0.7% variance from LLM prose generation
type: feedback
Category: feedback
---

Gates with hard character/word limits should include a ~5% tolerance band to accommodate normal LLM prose variance.

**Why:** Gate 27 blocked CS v S1 at 604 chars (limit 600) — a 0.7% overshoot. Same dossier passes on the next run. The gate catches "wall of text" paragraphs but a 4-character variance is noise, not a quality issue. The founder considers this a false positive.

**How to apply:** When implementing or reviewing character/word limit gates, use `limit * 1.05` as the actual threshold. Example: 700-char limit → fail at 735, not 700. This applies to Gate 27 (paragraph length), Gate 11 (word count), and any future character-based gates. The goal is catching genuine quality issues, not penalizing normal generation variance.
