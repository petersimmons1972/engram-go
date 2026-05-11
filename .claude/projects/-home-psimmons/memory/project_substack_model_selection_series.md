---
name: Substack model-selection article series (Articles 024–025+)
description: User is writing a Substack series on AI model selection and cost optimization; Articles 024 and 025 are in progress as of 2026-05-11
type: project
originSessionId: 4c60ba5a-c1a5-4dd6-a1de-8a915ae61eb0
---
**Fact:** Peter is authoring a Substack series on "which model, when" for AI practitioners. Articles are numbered and posted sequentially; free content through Article 024 was posted 2026-05-11.

**Why:** The series teaches operators how to reduce AI compute costs by routing tasks to appropriately-tiered models. Article 024 (Prompt 1) audits transcript history for Opus/Sonnet overspend on Haiku-appropriate tasks. Article 025 extends the logic from "which model" to "which surface."

**How to apply:**
- When the user says "the next article" or references Article N, this is the Substack series.
- Article 024 audit result: user's 14-day window shows zero Opus usage; all sessions are `claude-sonnet-4-6`. Primary downgrade opportunity is health_check/mechanical turns (10% of sampled turns, 269 tokens). Audit scratch files at `~/.claude/experiments/2026-05-11-024-model-overspend/`.
- Prompt 2 of Article 024 = replay a flagged turn against Haiku to confirm quality parity.
- Article 025 = audit by surface (Claude Code vs ChatGPT vs API), not just model tier.
