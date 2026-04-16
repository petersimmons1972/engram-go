---
name: Stop checkpointing — run on auto-pilot and delegate to Haiku (Sonnet if Haiku clearly insufficient)
description: When executing a multi-step plan the user has already approved, keep going through independent steps without stopping for confirmation between each. Offload grinding work (test runs, container builds, parallel handler additions) to Haiku (Sonnet if Haiku clearly insufficient) subagents so Opus cycles are reserved for synthesis and real decisions.
type: feedback
Category: feedback
---

Stop treating every task boundary in an approved plan as a checkpoint.

**Why:** 2026-04-07 during clearwatch chart-quality-gate Phase 2. The user approved a 5-item next-action list ("Yes to 1-4, no to 5") and then called out the fact that I was stopping and reporting status after each individual sub-step of the work instead of driving through. Each stop cost a turn and slowed the whole run down. Opus thinking is not the bottleneck at execution-layer tasks — grinding is.

**How to apply:**

1. **If the plan is approved, execute until a real decision appears.** A decision = ambiguity in requirements, unrecoverable risk, a destructive operation not covered by prior approval, or a genuine fork where the right answer isn't knowable from context. Everything else is grinding — do it.

2. **Delegate grinding to Haiku (Sonnet if Haiku clearly insufficient) subagents.** Test runs, container builds, log mining, mechanical handler implementation with a clear spec, file-pattern edits across many files, long-running validation loops. Use `subagent_type=general-purpose` with `model=sonnet` when the task is "apply a known pattern" rather than "decide what the pattern should be". Opus cycles stay on synthesis, diagnosis, architecture, founder communication.

3. **Batch status reports at natural milestones, not sub-step boundaries.** A milestone = a phase completed, a blocker encountered, a real decision needed, or the overall plan is done. Not "I finished step 2.1.a".

4. **Parallelize independent work.** Kick off the long-running thing in the background immediately, then do the fast thing inline. Don't serialize just because each step feels discrete.

5. **Pre-authorize the likely next moves.** When the user approves a list, treat that as blanket approval for the sub-steps inside each item unless one introduces new risk. Don't come back and ask "OK to proceed with the sub-steps of item 2?".
