---
name: reference-rocklambros-harness-engineering
description: Public GitHub repo at rocklambros/harness-engineering documenting a multi-platform Claude Code harness build; source of QC.* + AP.* vocabulary
metadata:
  type: reference
originSessionId: harness-port-2026-05-19
---
**Pointer:** `https://github.com/rocklambros/harness-engineering` — public reference build, MIT-attribution mix (uses Superpowers MIT, sec-context CC-BY-4.0, SecureForge MIT). Thesis: "reasoning is the artifact."

**Structure:**
- `foundation/` — Quality Contract (QC.1-QC.5), Threat Model (T.1-T.7), Architectural Principles (AP.1-AP.10), Seed Evaluation, Research References. Cross-cuts every platform section.
- `mac/` `jetson/` `windows/` — three platform builds; mac is validated reference. Each has README, ARCHITECTURE, prompts/, harness/, evaluations/.
- `research/` — read-only source docs.
- Root `CLAUDE.md` is ~200 lines, organized into Role / Code standards / Security / Core constraints / Things-that-break / Operational / Writing rules / Status. Cited as a model for tight CLAUDE.md.

**What we ported (Project A):** QC.1-QC.5 verbatim (with QC.4a adapted), AP.1, AP.2, AP.4-AP.9 verbatim. Added local QC.6 (Engram memory), QC.7 (Container hardening), AP.11 (Cost asymmetry), AP.12 (Defects tracked). Source AP.3 (cross-platform parity) and AP.10 (born-public) preserved as numbered gaps for cross-reference.

**What we rejected:** Adopt-the-repo (it's personal-config by design); prose style rules (em dashes etc. — author's voice, not user's); commit-history rewrite as backfill (destructive, fabricates reasoning).

**How to apply:** When Project A or related work cites a QC or AP ID, check the source repo for verbatim text. When designing harness rules, prefer rocklambros's framing of "deterministic over advisory" (AP.1) — hooks for what matters, CLAUDE.md only for ambiguity weighting.
