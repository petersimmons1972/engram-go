---
name: Substack PNG delivery rule
description: Substack images must be PNG — SVG source must always be converted before delivery
type: feedback
originSessionId: acb87967-add4-42ad-9cd0-1e7d1ae13504
---
Substack only accepts PNG uploads. Never deliver a raw SVG as a Substack header.

**Why:** Substack's upload UI does not accept SVG files. PNG is the required format.

**How to apply:** After every header SVG passes XML validation and Ramsay review, convert immediately with cairosvg:
```bash
python3 -c "import cairosvg; cairosvg.svg2png(url='header.svg', write_to='header.png', output_width=1200, output_height=800)"
```
Commit both `.svg` (source) and `.png` (deliverable) together. The rule is now codified in `/home/psimmons/projects/substack/CLAUDE.md`.
