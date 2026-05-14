---
name: Parallelization over sequential restarts
description: Always combine results instead of restarting processes; use parallel execution when multiple resources available
type: feedback
originSessionId: f3ae2e31-3715-47ec-a406-f213ad3ab159
---
**Rule:** When you have partial results from a process and additional resources become available, run both in parallel against the same checkpoint/output file instead of stopping and restarting.

**Why:** Restarting discards progress, restarts from checkpoints, and is slower. Parallelization preserves all completed work and adds to it.

**How to apply:** 
- If a process has completed N items and you want to add a second model/worker: start the second worker pointing to the same output file
- Both will process different items (checkpoint deduplication)
- Combined results are the sum of both workers' output
- Never say "restart" or "switch to" when you could instead say "and also run"

**Example:** LongMemEval v8 run with leviathan at 128 items + precision just back online → run precision in parallel against same checkpoint, not restart with precision alone.
