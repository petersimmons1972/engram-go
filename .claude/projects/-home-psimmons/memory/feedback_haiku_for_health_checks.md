---
name: Route health_check and mechanical turns to Haiku
description: Short operational turns (status acks, file-path transforms, cookie strings, yes/no checks) embedded in coding sessions are Haiku-appropriate — Sonnet is overkill
type: feedback
originSessionId: 4c60ba5a-c1a5-4dd6-a1de-8a915ae61eb0
---
**Rule:** When dispatching subagents or selecting model tier, explicitly route `health_check`, `mechanical_transform`, `formatting`, `classification`, and `bulk_judge` tasks to Haiku. Do not let these inherit the session's Sonnet default.

**Why:** A 14-day transcript audit (2026-05-11, Article 024) found 10% of turns were Sonnet on Haiku-appropriate tasks. All five flagged cases were short operational turns embedded in longer coding sessions — status acknowledgments ("the postgres is fine"), CSV file-path pass-throughs, skill directory lookups, cookie string transforms. None required Sonnet judgment. CLAUDE.md model floor rule already mandates this; the audit confirmed it's being missed in practice.

**How to apply:**
- Before every `Agent(...)` dispatch, ask: "Is this classification, formatting, bulk judge, mechanical transform, or health check?" If yes → `model: "haiku"`.
- The most common miss pattern: brief conversational status turns that precede or interrupt multi-file coding sessions (e.g., "FYI, I posted X", "Y is fine, continue"). These feel like chat but are functionally health_check.
- Homogeneous Sonnet agent teams are a smell — always ask if any workers could be Haiku.
