---
name: chart-rebuild-families
description: Tukhachevsky's analysis of 71 chart methods — 7 visual families, 8 base renderers, migration order by data acquisition pattern
type: project
Category: active-work
---

71 generate_* methods decompose into 7 visual families and 3 deployment contexts (EDR/Landscape/Market Map).

**Why:** Guides Phase 4 of Operation Chart Rebuild — determines whether we need 71 individual renderers or 8 family bases.

**How to apply:**
- 8 family base renderers: BarComparison (22), GridMatrix (14), Quadrant/Bubble (10), Timeline (7), Scorecard (6), Flow/Tree (4), Radar (2), Constellation (3), Multi-panel (3)
- Migration order by DATA pattern: Market Map (28, hardcoded) → Landscape (13, clean extractor) → EDR (30, chaotic inline)
- 2 dead code methods to delete: generate_detection_gap_map, generate_breach_cost_exposure
- 49 static prototypes need no migration
- EDR methods need extractor.get_*() methods created BEFORE rendering migration
- Full analysis at /tmp/chart-family-analysis.md (ephemeral)
