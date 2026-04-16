---
name: SVG with text pre-flight checklist
description: Mandatory additions to any graphics agent dispatch brief when the SVG contains text labels
type: feedback
category: graphics
---

When dispatching any graphics agent (cassandre, mucha, toulouse-lautrec, rand, etc.) to create SVG that contains text, append these rules to the brief. These come from 3 rounds of Ramsay review on the Hermes security diagram (2026-04-05).

**Why:** SVG text is the #1 source of review failures. Agents cannot render fonts, so they estimate text width wrong, use font sizes that shrink below readability at viewing width, pick Unicode glyphs that don't render cross-platform, and create paint-order bugs where geometry occludes text.

**How to apply — add to every text-SVG dispatch brief:**

1. **State the effective viewing width** alongside the viewBox: "viewBox 1200x600, will render at ~680px effective on Substack." Agent must design for viewing size, not authoring size.
2. **Minimum font-size is 11px** at the viewBox scale. No exceptions. State this explicitly.
3. **No Unicode special characters in text** — plain ASCII only. Build custom markers from SVG circle + text elements.
4. **Paint order: backgrounds → geometry → strokes → text.** State this explicitly. Agents write top-to-bottom and lose track of z-order.
5. **All polygon vertices must be computed from a single stated center point.** No eyeballed coordinates. Include the formula in the brief if the shape is complex.
6. **Text containers must have 25% margin.** Agent cannot measure font metrics, so a text element expected to be 200px must sit in a 250px container. State this rule.
7. **After creating the SVG, verify:** (a) every text element is ≥11px, (b) no element drawn later with opaque fill occludes an earlier element, (c) all polygons are centered on their stated center.
