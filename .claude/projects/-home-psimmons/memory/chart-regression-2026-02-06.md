---
Category: reference
---
# Chart Rendering Regression - 2026-02-06

**Symptom**: 10 charts in v030-v050 → 0 charts in v054+. Pipeline reported "charts_loaded: 16" but HTML had 0 visible.

## Root Causes (3 issues)

1. **Section ID mismatch** — `SECTION_CHART_MAP` had hardcoded slugs that didn't match generated HTML section IDs. Fix: updated map. Prevention: validate map keys exist in HTML.

2. **Duplicate method definition** — `_abbreviate_vendor_name()` defined twice in chart_generator.py. Second definition (no `max_chars` param) silently overwrote first. Callers passing `max_chars=18` crashed with TypeError. Prevention: search for existing `def method_name` before adding.

3. **BeautifulSoup destroys SVG** — `_ensure_text_density_breaks()` used BeautifulSoup which lowercases `viewBox` → `viewbox` and mangles xmlns attributes. ALL parsers (html.parser, lxml, html5lib) do this. Fix: rewrote with pure regex. **RULE: NEVER use BeautifulSoup on SVG-containing HTML.**

## Key Lessons
- `charts_loaded` ≠ `charts_rendered` — verify what customer sees, not intermediate metrics
- Post-processing can destroy injected content — add counters at each stage
- Hardcoded section maps decay as prose evolves — need dynamic matching or validation
- Python duplicate method definitions silently overwrite (last wins, no warning)

## Resolution
v074+ ships with 8 charts. Files fixed: `stage_5.py` (map + regex rewrite), `chart_generator.py` (duplicate removed).
