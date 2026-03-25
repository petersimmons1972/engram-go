---
name: chart-regression-2026-02-06
description: Case study — 3 compounding bugs caused chart count to drop from 10 to 0
type: reference
Category: active-work
---
# Chart Rendering Regression - 2026-02-06

**Symptom**: 10 charts in v030-v050 → 0 charts in v054+. Pipeline reported "charts_loaded: 16" but HTML had 0 visible.

## Root Causes (3 compounding issues)

1. **Section ID mismatch** — `SECTION_CHART_MAP` had hardcoded slugs that didn't match generated HTML section IDs. Prevention: validate map keys exist in HTML.

2. **Duplicate method definition** — `_abbreviate_vendor_name()` defined twice in chart_generator.py. Second definition silently overwrote first. Callers passing `max_chars=18` crashed with TypeError.

3. **BeautifulSoup destroys SVG** — `_ensure_text_density_breaks()` used BeautifulSoup which lowercases `viewBox` → `viewbox`. ALL parsers do this. Fix: extract-process-restore pattern (see html-processing-patterns.md).

## Key Takeaway
`charts_loaded` ≠ `charts_rendered` — verify what the customer sees, not intermediate metrics. Post-processing can destroy injected content. Add counters at each pipeline stage.
