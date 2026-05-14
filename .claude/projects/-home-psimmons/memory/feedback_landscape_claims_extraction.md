---
name: feedback_landscape_claims_extraction
description: "Three bugs in Stage 3 claims extraction for landscape-format dossiers (devops_secrets, NHI) — and the fixes that resolved them"
metadata: 
  node_type: memory
  type: feedback
  originSessionId: b76f3233-5e64-472f-b3f5-6def1a9e65d8
---

Three distinct failures hit when running devops_secrets through the pipeline for the first time. Each required a targeted fix.

**Bug 1: Gate 1 validated wrong sections (EDR default never updated)**
`Stage1and2Processor` is instantiated at orchestrator `__init__` time with `segment="edr_xdr"`. The actual segment is detected later inside `process_complete()`. Fix: update the sub-attribute — `self.stage_1_2.section_validator = SectionValidator(segment=_segment)` — rather than replacing `self.stage_1_2` entirely. Replacing the whole object breaks test mocks that patch `orchestrator.stage_1_2`.

**Bug 2: `list[int]` and `list[str]` leaked into claims**
Landscape manifests have sibling keys under each section: `source_ids` (list[int]), `vendors` (list[str]), and `vendor_details` (list[dict]). `_get_section_claims` was blindly extending claims with the first list it found. Fix: filter to dict-only — `claims.extend(item for item in value if isinstance(item, dict))`. Also fix `_synthesize_executive_claims`: prefer `vendor_details` key explicitly; fallback uses first list whose first item is a dict.

**Bug 3: Text-only sections had 0 claims, triggering zero-tolerance fail**
Sections like `intro`, `methodology`, `legal_disclaimer` contain only a `framework` description string — no dict claims. Stage 3's zero-tolerance check skipped them and failed the pipeline. Fix: in `_get_section_claims`, when no dict claims are found but a `framework` string exists, synthesize one pseudo-claim carrying that string. The section context already has detailed writing instructions — the pseudo-claim just prevents the zero-tolerance gate from firing.

**Why:** All three bugs were latent for months because only pairwise EDR dossiers were running. The first devops_secrets landscape run hit all three in sequence.

**How to apply:** When adding a new landscape segment, run a smoke test through Stage 1 claims extraction and Stage 3 section claims before running the full pipeline. The three failure modes above recur whenever a new segment uses a non-EDR manifest structure.
