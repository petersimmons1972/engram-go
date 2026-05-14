---
name: Clearwatch quality campaign process lessons
description: What to do differently when running clearwatch-quality-campaign — pre-analysis, triage, and report gating
type: feedback
originSessionId: 4d9b0c22-6135-4b07-a686-4f8a13d472fa
---
Lessons from running the clearwatch-quality-campaign skill on 2026-05-08.

**Fix classes, not instances — but verify the class diagnosis first**
The campaign identified "banned chart types in v263" as a root cause. Exploration showed v263 had zero retired charts — all 3 filtering layers were working. The actual issue was financial jargon in prose (60+ ARR/FY violations). Don't accept the founder's problem description as a precise technical diagnosis; always verify against actual output before designing fixes.

**Why:** Saves a whole design track pointed at the wrong root cause.

**Issue triage must happen before running reports**
The first instinct was to run reports immediately after Phase 3 implementation. Triage first: close every issue the new validators cover, then run reports. Starting reports while 15+ issues are open obscures which failures are pre-existing vs. new regressions.

**Why:** Report failures when 15 issues are open = can't tell signal from noise. Report failures when 3 issues are open = you know exactly what's new.

**Gate addition without prompt update = guaranteed batch failure**
When Stage 6 gets a new BLOCKING gate (F2.a, jargon, F1), the upstream prose generation prompt MUST be updated simultaneously. The W2 gates were wired 3 weeks before the prompt was updated — every report generated in between failed on gates it had no instructions to satisfy.

**Why:** Stage 3 reads domain-knowledge/*.md only. CLAUDE.md rules are invisible to it. Any new blocking rule must be mirrored in domain-knowledge AND in annotation_format.py on the same day it's gated.

**Retry exhaustion = silent acceptance, not hard failure**
Stage 5 reaches max_attempts and returns sections as-is. "Stage 5 PASSED" means the stage completed without exception — it does NOT mean all validations passed. Sections with uncited percentages can propagate to Stage 6 after Stage 5 gives up. When Stage 6 catches them, surgical retries must not compound multiple error types in the same feedback message.

**How to apply:** Before any new campaign, verify: (1) all new gates have corresponding prompt updates, (2) the retry prompt contains no bypass phrases, (3) Stage 3b surgical retry handles report-level errors, (4) feedback is decoupled by error type.
