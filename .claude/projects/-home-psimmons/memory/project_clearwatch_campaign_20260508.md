---
name: Clearwatch Quality Campaign 2026-05-08 outcomes
description: Root causes, fixes, and pipeline lessons from the campaign that unblocked v264-v298
type: project
originSessionId: 4d9b0c22-6135-4b07-a686-4f8a13d472fa
---
Quality campaign run 2026-05-08 on branch `quality-campaign/2026-05-08`. Reports v264–v298 were all failing after v263. Campaign is in progress — reports running at session close.

**Why:** W2 gates (F1/F2/F3) were wired into Stage 6 but upstream prose generation prompts were never updated to meet the new gate requirements. This caused systemic F2.a citation failures across all 5 Tier 1 pairs.

**Root causes fixed:**

1. Validator retry prompt taught the LLM its own bypass — `validator.py` retry message said "rephrase as approximately X%", the exact phrase exempted from citation checks. Fixed commit `138ed38c`.

2. Stage 6 → Stage 3b retry silently dropped report-level errors — `section="report"` errors weren't found in `current_sections`, so jargon/F1/F2 gate errors were silently skipped. Fixed `965d68b5`.

3. Compound retry whack-a-mole — when citation AND insight errors coexist in one section, combined retry feedback causes LLM to fix one and break the other across attempts. Fixed `80f43429` by decoupling: citation-only feedback until citations pass, then insight check.

4. FY fiscal codes in dossier prose — `FY2025` appeared in both source titles (bibliographic, exempt) and claim text (must sanitize). Two-layer fix: load-time sanitization in `input_validator.py` + bibliographic endnote exemption in `enforcer.py`. Fixed `f5ce5b7f`.

5. v263 "banned chart types" was actually 60+ financial jargon violations (ARR, year-over-year, Q4 FY codes) — no gate enforced rule #4383. Fixed with Pass 5 BLOCKING gate `2929d5b1`.

6. 37 dead lessons with `stage: 'stage_5'` string — `Lesson.stage` is typed `int`, string values silently never inject. Normalised in `24bb8d4d`.

7. MITRE detection scores (100%, 88%) cited in separate sentences from `<<claim:N>>` — sentence-scoped validator catches them but LLM keeps repeating the pattern. Fixed with explicit MITRE example in annotation prompt `25e6a703`.

**How to apply:** When Clearwatch reports fail Stage 6 after W2 gate changes, check these first: (1) is the retry prompt teaching bypass phrases, (2) are report-level errors flowing back to Stage 3b, (3) is feedback compounding citation+insight in the same retry message.

**Open at session close:** Branch `quality-campaign/2026-05-08` has 8600 passing tests, third report batch in progress. Issues #4711, #4718 close on report green. Issues #4716, #4720 are design-debt follow-ons.
