---
name: Army Team Up — Predefined Agent Team Configurations
description: User wants a way to predefine named multi-agent team configurations that can be spawned as a unit with one command
type: feedback
Category: feedback
---

User wants "Army Team Ups" — predefined, named team configurations where each role is specified in advance and the team can be invoked by name.

**Why:** Avoids re-specifying team composition each time. Like calling "bring in the creative problem-solving team" and knowing exactly who shows up.

**First defined team — Creative Problem-Solving:**
- Field Marshall (strategic/executive coordinator) → Marshall or Eisenhower
- QA General (holds the standard, rejects mediocrity) → gordon-ramsay
- Researcher (data synthesis, multi-source intelligence) → edwin-layton
- Unconventional Thinker (lateral, creative, challenges assumptions) → rommel or yamamoto
- Neutral Observer (fresh eyes, zero prior context) → zero-context-reviewer

**How the team works:**
1. Each agent works independently on the same problem
2. Each produces their own recommendation from their role's perspective
3. Coordinator synthesizes the final result from all inputs

**How to apply:** When user says "spawn the creative team" or similar, map to this 5-role configuration. In the future, consider storing team configurations as named YAML/JSON presets in `~/.claude/agents/teams/` so they can be referenced by name.
