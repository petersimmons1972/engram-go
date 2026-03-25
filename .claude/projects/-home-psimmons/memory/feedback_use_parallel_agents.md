---
Category: feedback
name: feedback_use_parallel_agents
description: User preference to use parallel agents for independent tasks instead of sequential work
type: feedback
---

Use parallel agents (via `superpowers:dispatching-parallel-agents`) whenever there are 2+ independent tasks — don't work sequentially when work can be parallelized.

**Why:** User explicitly instructed "Use multiple agents" when I was doing sequential work on updates/checks that could have run in parallel.

**How to apply:** Before starting any multi-step task, check if steps are independent. If so, dispatch agents in parallel rather than running them one at a time. Examples: checking status of multiple services, updating multiple components, investigating unrelated failures.
