# Night Protocols — Autonomous Overnight Operation

Reusable operating pattern for driving a campaign to completion while the founder is asleep. Established 2026-06-20; founder directed reuse.

## 1. State boundaries up front
Open each autonomous run by recording what authority is being exercised, what is deferred, and what stops-and-waits — a record the founder reads on waking. No surprises.

## 2. Authority model
- **Merge-on-green to main:** once the founder says "merge on green," that is standing authorization for the batch — no per-merge round-trips. Branch protection (green CI, no admin bypass) is the backstop.
- **Additive/reversible deploys** that advance the goal: authorized, each behind a build-proof gate.
- **Deferred/blocked unattended:** anything requiring handling the full secret blob (e.g. an atomic `ATTACH_TOKENS` edit), a prod fleet-dispatch rollout, the deferred leaked-token rotation, or any irreversible/data-loss op → **staged as a supervised batch** with a ready runbook, never executed alone.
- **Stop + document** on: the same error 3×, a ≥45-minute wall, or anything that could leave the live fleet broken overnight.

## 3. Bus-only fleet comms
Operational harness comms (work intake/report, consult, broadcast) go over fleet-dispatch **only**, never ssh. SSH is reserved for development, deploy, and break-fix — and is never disabled.

## 4. Build-proof gate on everything
"Done" = an independent validator + a founder-runnable live canary. Never accept 'done'/'green' on an implementer's word or an inferred clean exit — confirm with a live read. This caught both over-optimistic and over-pessimistic claims repeatedly.

## 5. Recon before build
Establish the real mechanisms first; diagnoses drift both optimistic and pessimistic. Read the actual code path before building a fix.

## 6. Protect the live embed/Engram path
The MI-50 live embed path is never touched during autonomous work.

## 7. Parallelism + cost discipline
Fan out multiple agents where non-overlapping (separate repos/hosts, no shared commits). Use the cheapest sufficient tier (Haiku/Sonnet); no Opus/Fable for execution. Cap fan-out to what genuinely helps — don't spawn while the next wave is gated. Worktree-isolate same-repo parallel implementers.

## 8. Harnesses as thinking partners
Once a harness is functional, use it via the bus to red-team plans, surface blockers, and offer alternatives.

## 9. Record failure modes
Record failure modes as discovered; have harnesses self-report their own (per-harness files).

## 10. Finish condition → QA
When the build is exhausted, run the 6-persona QA sweep → plan → fix (not just report). QA-style sweeps may also be used mid-stream to surface blockers.

## 11. Plan files persist the campaign
Across session/compaction boundaries, keep the overnight log, the staged supervised batch, and the maintenance runbook under `~/.claude/plans/`.

## 12. Wake-the-founder triggers still hold
>$5 compute, prod deploy, push to main without standing auth, data-loss, stuck ≥45 min, same error 3×.
