---
name: project_clearwatch_stage3_parallel
description: Enhancement
metadata: 
  node_type: memory
  type: project
  originSessionId: b76f3233-5e64-472f-b3f5-6def1a9e65d8
---

Stage 3 generates sections sequentially at ~8 min/section. For devops_secrets (8 sections) that's ~64 min; for edr_xdr (10 sections) ~80 min. All sections run on Opus even though only 2 require synthesis.

**Filed as:** GitHub issue #4803

**Design (not yet specced for implementation):**
- Phase 1: data sections in parallel via ThreadPoolExecutor
- Phase 2: synthesis sections (executive_verdict, executive_summary) after Phase 1 completes
- Per-section model: Haiku for boilerplate (legal_disclaimer, methodology, about_clearwatch_research), Sonnet for data sections, Opus for synthesis only
- edr_xdr is primary target — highest volume (5 Tier-1 pairs, regenerated regularly per #4711)

**Precedent:** Stage 7 (grading) already runs on Haiku. Stage 3 is the natural extension.

**Gaps before implementation:**
- Thread safety audit: `self.section_metrics`, `self.skipped_count`, `self.api_client`, token budget tracking all have shared mutable state
- Error handling model: what happens if a Phase 1 section fails?
- Phase 2 dependency: exec_summary reads from the *manifest* (pre-Stage-3), not from Phase 1 prose — verify this before assuming hard dependency
- Model maps for saas_identity_security and identity_security_market_map not yet written

**Why:** Wall time drops 5× (80 min → ~15 min). Cost drops — Opus touches only 20% of sections instead of 100%.

**How to apply:** Run brainstorming → writing-plans before touching stage_3.py. Thread safety is the non-obvious failure mode.
