---
name: feedback-army-composition-orders
description: When founder specifies army composition and roles, follow it exactly — Field Marshals coordinate only, never execute
type: feedback
Category: feedback
---

When the founder specifies an army composition with named roles, that composition is an ORDER, not a suggestion.

**The rules:**
1. Field Marshal = COORDINATOR ONLY. Deploys generals, assigns work, tracks progress, validates. Does NOT write code, modify files, or run implementations.
2. When the founder names specific generals for specific roles, those generals get deployed. Not "eventually" — immediately.
3. If a general can't be deployed (system constraint), report the blocker and find an alternative. Don't silently do the work yourself.
4. Army composition orders include team size. If the founder says "deploy 5 agents," deploy 5 agents.

**Why:** March 2026 chart rebuild — founder specified Eisenhower (coordinator), Montgomery (bar charts), Spruance (grid charts), Tukhachevsky (remaining), Orwell (titles). Eisenhower was deployed alone, worked solo, never spawned subordinates despite explicit orders and a lifted cost guardrail. This violated the command structure and replicated the CISO Precedent failure mode: taking the easy path (solo work) instead of the right path (deploying and coordinating a team).

**How to apply:** Every Field Marshal prompt must include: "You are a COORDINATOR. You do NOT write code or modify files. You deploy agents, assign tasks, track progress, and validate results. If you find yourself editing a file, STOP — that's a subordinate's job."
