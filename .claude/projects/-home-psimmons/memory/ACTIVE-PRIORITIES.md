---
name: active-priorities
description: Current work focus and pending action items
type: project
Category: active-work
originSessionId: f13be8e1-9cae-4933-afaa-71fe701071a8
---
# Active Priorities

**Last Updated**: 2026-04-14
**Current Focus**: Wave 3 QA campaign — Stage 7 feedback pipeline convergence

---

## Just Completed (2026-04-14)
- Removed 8 fabricated CyberArk acquisition entries from `dossiers/microsoft_defender_vs_paloalto_cortex.json` — closed #4509, #4502
- MD_v_PAN v123: A- / ships, 0 CyberArk references
- S1_v_PAN v099: A- / ships, circuit reset to 0
- S1_v_MD dossier: removed 2 "Defender for Business" name references (Gate 36 fix) — filed #4524
- Ran auto_triage on 6 CS_v_S1 v292 issues — 5 accepted, 1 rejected

## Pending (priority order)
- [ ] **#4517 (severity/blocker)** — CS_v_S1 DELIVERY_CHECKLIST_FAILED code bug. `FinalFormatValidator._check_citation_distribution_quality` fires CRITICAL when >20% of H2 sections lack citations, blocking reports that graded A-/ships. Fix before triggering v293.
- [ ] **#4524** — S1_v_MD Gate 36 dossier fix done; trigger v110 regen to confirm clean
- [ ] **Dispatcher run** — one pass picks up all queued accepted issues: CS_v_S1 (5), MD_v_PAN (~12), S1_v_PAN (5), S1_v_MD (1). Run AFTER #4517 is fixed so CS_v_S1 v293 doesn't re-fail delivery.
- [ ] **Visual QA cache invalidation** — spec at `docs/superpowers/specs/2026-04-10-visual-qa-cache-invalidation-design.md`. Unblocks #4234, #4235.
- [ ] **Go migration P2 — Stage 6 gates** — plan `~/.claude/plans/snoopy-seeking-dragonfly.md`. Batch 2 still stubbed (11 gates).

## Blocked/Deferred
| Task                       | Status                                      |
|----------------------------|---------------------------------------------|
| SCALE bugs (#2340-#2364)   | Deferred — 25 P2 concurrency bugs, non-blocking |
| NHI vendor data quality    | Open: #2904, #2923 — monitoring after RSAC  |
