---
Category: feedback
name: chart-readability-rules
description: Founder feedback on chart design — no text overlaps, no wasted whitespace, accessible font sizes, CISO decision focus
type: feedback
---

## Chart Design Rules (from market-map prototype review 2026-03-15)

### Non-Negotiable
1. **No text inside small shapes.** Labels go OUTSIDE circles/dots, adjacent with clear space. Text overlapping shapes creates a "traffic accident" effect — the brain is involuntarily drawn to the collision, pulling attention from the chart's message.
2. **12px minimum font size.** No exceptions. The target buyer has poor vision.
3. **No excessive whitespace.** If data occupies less than ~60% of the SVG canvas, the chart needs to be tighter. Large empty areas mean fonts and labels are too small.

### Design Principles
4. **Label every axis meaningfully.** "Narrow → Deep" fails. Use concrete labels with units: "Weighted Rubric Score (40%-90%)" succeeds.
5. **Color must be distinguishable.** Multiple shades of the same hue (yellow/gold/amber) are indistinguishable. Add text labels inside colored cells if colors are similar. Use distinct hue families (blue vs purple vs gold, not orange vs amber vs gold).
6. **State the insight.** If the visual pattern IS the finding (e.g., colors converging = market converging), add an explicit callout stating it. Don't make the buyer interpret.
7. **Decision charts > capability charts.** The buyer cares about "which one is for me?" more than "what does the landscape look like?" Decision-layer charts (archetypes, blind spots, TCO iceberg, wrong-vendor cost) drive purchase confidence.

### Process
8. **Prototype first, code second.** Build all chart concepts in a static HTML gallery, get founder approval, THEN implement generator methods.
9. **Start with 2x the charts you need.** 28 concepts for ~8-10 final slots gives room to cut and combine.

**Why:** The founder has poor vision and represents the target buyer (1-3 person IT team, $50-150K decision, not data viz experts). If the chart isn't instantly readable, it fails.

**How to apply:** Before finalizing any chart prototype or generator method, audit for: (a) text-on-shape overlaps, (b) font sizes, (c) data-to-canvas ratio, (d) axis label clarity, (e) color distinguishability. Run the mental test: "Would a non-technical CISO understand this in 5 seconds?"
