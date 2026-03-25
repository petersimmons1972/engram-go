---
name: feedback-always-pull-xp
description: Always pull latest XP from GitHub generals repo before deploying or referencing any general's stats
type: feedback
Category: feedback
---

Always pull latest XP from the generals GitHub repo before deploying or referencing any general's stats. Never use cached/remembered XP values.

**Why:** During the data layer design session (2026-03-23), Slim was cited as 0 XP and Groves as 0 XP when they actually had 225 and 75 respectively. The bench-roster.md was stale. The founder caught it.

**How to apply:** Before ANY agent deployment or XP reference:
```bash
cd ~/projects/generals && git pull origin master
```
Then read the actual profile file for each general. Never trust the inline table in AGENTS.md or bench-roster.md — those are convenience summaries that drift. The profile YAMLs/MDs in `profiles/` are authoritative.
