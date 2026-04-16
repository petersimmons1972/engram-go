---
name: Graph-Driven Gap Discovery
description: Query structured knowledge graphs against your own coverage map to surface gaps a human operator cannot see directly
type: feedback
Category: pattern
---

When you have a structured knowledge graph containing mitigation/countermeasure/relationship data (MITRE ATT&CK, D3FEND, CAPEC, internal coverage models), query it against your own implementation's coverage map to find gaps you didn't think of.

**Why:** Humans design coverage based on what they can think of. Graphs contain relationships the designer never enumerated. The graph surfaces gaps the operator cannot see directly because the operator is working from the inside-out view of "what I built" instead of the outside-in view of "what the problem space demands." On 2026-04-08 the Locked Shields Bluesteel gap analysis found four real gaps (M1022 file permissions, M1027 password policies, M1041 data encryption at rest, M1052 UAC) that the operator had not identified when writing the Blue Team counter-prediction v1 — the graph found them in one query.

**How to apply:**
- When a project has access to a structured research DB with coverage/mitigation relationships, build a gap analysis tool early. Don't wait until v2.
- Pattern:
  1. Get the predicted/expected targets (techniques, threats, requirements)
  2. Query the graph for everything that "mitigates" or "counters" or "addresses" those targets
  3. Cross-reference against your own coverage map (static dict in code: target_id → {level, modules, notes})
  4. Categorize into: full / partial / detect-only / out-of-scope / real gap / unknown
  5. Output terminal table, markdown, and JSON
- Treat "real gap" and "unknown" results as signal. "Out of scope" and "full" are noise.
- Re-run periodically as the coverage map evolves.
- Reference implementation: `tools/research/gap_analysis.py` in the locked-shields repo. 300 lines of Python, reads `EXTERNAL_DATABASE_URL` via Infisical, outputs a Markdown report.
- The coverage map itself is the reusable asset. Keep it in-code as a dict with `level`, `modules`, `notes` fields. Update it when new modules ship.

**Do not:**
- Let the graph output drive new module development in the same session without operator review. Some "gaps" are correctly out of scope. The graph is a suggestion, not a mandate.
- Treat unknown (not in coverage map) and real gap (in map, marked uncovered) as the same category. Unknown means "figure out if this matters." Real gap means "this is a decision to close or defer."
