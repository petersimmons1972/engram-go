---
name: validate-charts
description: Automated chart validation for security-intelligence-business reports. Use before manual review to catch visual issues (overlaps, cutoffs, sizing, contrast, rendering). Detects text overlap, false positives, OCR errors, geometric overlap, and color contrast violations. Runs OCR and geometric analysis on report HTML/screenshots. Outputs validation states (correct, false positive, needs tuning).
---

# Validate Charts

**Purpose:** Catch 80% of visual chart issues automatically before manual review.

**Invocation:** `/validate-charts report.html` or `/validate-charts screenshot.png`

## Context

Part of security-intelligence-business quality control loop. Reports contain 5-15 charts that need validation for:
- Text cutoffs (axis labels, titles, legends)
- Element overlaps (legend vs data, labels vs axes)
- Margin violations (too close to edges)
- Color contrast issues (readability)
- Font size minimums
- Chart sizing (too small/large)

This skill automates detection so manual review focuses on **business logic and data accuracy**.

## Validation Checks

### 1. Text Cutoff Detection
```python
# Check if text elements extend beyond SVG boundaries
- X-axis labels: bottom margin >= 40px
- Y-axis labels: left margin >= 60px
- Chart titles: top margin >= 30px
- Legends: all edges >= 10px from boundary
```

### 2. Overlap Detection
```python
# Check bounding boxes for intersections
- Legend vs plot area: no overlap
- Data labels vs axes: min 5px clearance
- Multiple data series labels: no overlap
- Title vs legend: min 10px separation
```

### 3. Margin Validation
```python
# Minimum spacing requirements
- Plot area margins: top 50px, right 20px, bottom 60px, left 70px
- Element spacing: min 10px between distinct elements
- Edge clearance: min 5px from SVG edges
```

### 4. Color Contrast (WCAG AA)
```python
# Contrast ratios
- Text on background: >= 4.5:1
- Data series colors: distinguishable by colorblind users
- Chart background vs plot elements: >= 3:1
```

### 5. Font Size Minimums
```python
# Readability thresholds
- Chart titles: >= 16px
- Axis labels: >= 12px
- Data labels: >= 10px
- Legend text: >= 11px
```

### 6. Sizing Validation
```python
# Chart dimensions
- Min width: 600px (readable)
- Max width: 1400px (not overwhelming)
- Min height: 400px
- Aspect ratio: 1.2:1 to 2:1 preferred
```

## Implementation

**Script:** `~/bin/validate-charts.py`

```bash
# Validate HTML report
~/bin/validate-charts.py ~/reports/report-v5.html

# Validate screenshot
~/bin/validate-charts.py ~/Pictures/Screenshot_2026-02-14_13-46-40.png

# Validate multiple files
~/bin/validate-charts.py ~/reports/batch-001/*.html
```

**Dependencies:**
```bash
pip install playwright pillow pytesseract
playwright install chromium
sudo apt-get install tesseract-ocr
```

## Implementation Details

**Option A: Python + Playwright (Full Automation)**
```python
from playwright.sync_api import sync_playwright
from PIL import Image
import pytesseract

def validate_chart(report_html):
    with sync_playwright() as p:
        browser = p.chromium.launch()
        page = browser.new_page()
        page.goto(report_html)

        # Extract chart SVGs
        charts = page.query_selector_all('svg.chart')

        for i, chart in enumerate(charts):
            bbox = chart.bounding_box()

            # Check 1: Text cutoff
            text_elements = chart.query_selector_all('text')
            for text in text_elements:
                text_bbox = text.bounding_box()
                if text_bbox['x'] + text_bbox['width'] > bbox['width']:
                    yield f"Chart {i+1}: Text cutoff detected"

            # Check 2: Overlaps
            legend = chart.query_selector('.legend')
            plot_area = chart.query_selector('.plot-area')
            if overlaps(legend, plot_area):
                yield f"Chart {i+1}: Legend overlaps plot area"

            # Check 3: Margins
            # Check 4: Color contrast
            # Check 5: Font sizes
            # Check 6: Sizing
```

**Option B: Image Analysis (Screenshot-based)**
```python
from PIL import Image
import pytesseract
import cv2

def validate_screenshot(screenshot_path):
    img = Image.open(screenshot_path)

    # OCR to find text elements
    text_data = pytesseract.image_to_data(img, output_type='dict')

    # Detect text near edges (likely cutoff)
    for i, text in enumerate(text_data['text']):
        x, y, w, h = text_data['left'][i], text_data['top'][i], \
                     text_data['width'][i], text_data['height'][i]

        if x + w > img.width - 10:  # 10px edge threshold
            yield f"Text cutoff detected: '{text}' at right edge"

        if y + h > img.height - 10:
            yield f"Text cutoff detected: '{text}' at bottom edge"

    # Color contrast analysis
    # Overlap detection via contour analysis
    # Font size estimation
```

**Option C: Hybrid (Both)**
- Use Playwright for structured HTML reports
- Use image analysis for screenshots/PDFs
- Combine results for comprehensive validation

## Output Format

```markdown
# Chart Validation Report - [timestamp]

## Summary
- **Charts validated:** 8
- **Issues found:** 5
- **Pass rate:** 62.5%

## Issues by Chart

### Chart 1: Timeline (PASS)
✓ No text cutoffs
✓ No overlaps
✓ Margins valid
✓ Color contrast OK
✓ Font sizes OK
✓ Sizing OK

### Chart 3: TCO Comparison (FAIL - 2 issues)
✗ Legend overlaps plot area (top-right corner)
  → Fix: Move legend outside plot, right edge
✗ X-axis label cutoff (bottom)
  → Fix: Increase bottom margin to 50px

### Chart 5: Deployment Timeline (FAIL - 1 issue)
✗ Color contrast violation (yellow on white background)
  → Fix: Darken yellow to #D4A574 (gold) for 4.5:1 ratio

## Auto-fixable Issues
- Chart 3: Legend positioning → CSS margin adjustment
- Chart 3: Bottom margin → SVG height adjustment
- Chart 5: Color → Palette swap

## Manual Review Required
- None (all issues are mechanical)

## Next Steps
1. Apply auto-fixes to chart generator
2. Re-generate report
3. Re-run validation
4. If pass, proceed to manual review for business logic
```

## Integration with Feedback Loop

```bash
# Workflow
1. Generate reports → ~/reports/batch-001/*.html
2. /validate-charts ~/reports/batch-001/*.html
3. If issues found:
   - Auto-fix mechanical issues (margins, positioning, colors)
   - Re-generate
   - Re-validate
4. If validation passes:
   - Manual review with /cf (focus on business logic)
5. /if integration (combine auto + manual feedback)
```

## Dependencies

```bash
# Python packages
pip install playwright pillow pytesseract opencv-python

# System packages
sudo apt-get install tesseract-ocr

# Playwright browsers
playwright install chromium
```

## Development Priority

**Phase 1: MVP (Quick Win)**
- Text cutoff detection (catches 40% of issues)
- Overlap detection (catches 30% of issues)
- Output: Simple pass/fail per chart

**Phase 2: Full Suite**
- All 6 validation checks
- Detailed reports
- Auto-fix suggestions

**Phase 3: Auto-Fix**
- Generate corrected chart configs
- Re-render automatically
- Iterate until pass

## Edge Cases

| Situation | Behavior |
|-----------|----------|
| Dynamic charts (JS-rendered) | Use Playwright, wait for render complete |
| PDF reports | Convert to images, use image analysis |
| Multiple chart types | Customize checks per type (bar, line, scatter) |
| False positives (tunable) | Adjust threshold, manual override |
| Overlapping labels | Geometric overlap detection, min 5px clearance |
| Small charts (<600px) | Flag, skip contrast checks, focus on readability |
| Dynamic content updates | Re-validate after state change |
| Text cutoff at edges | OCR detects edge proximity <10px |
| Chart width constraints | Validate min 600px, max 1400px, report width violations |

## Common False Positive Thresholds and Tuning

False positives occur when validation flags issues that aren't visual problems. **All thresholds are tunable:**

| Threshold | Default | Symptom | Adjustment |
|-----------|---------|---------|------------|
| Edge clearance | 10px | Text near edges flagged as cutoff | Increase to 15px if labels appear cut but aren't |
| Overlap tolerance | 5px | Legend flagged as overlapping when barely touching | Increase to 10px for tighter layouts |
| Margin minimum (bottom) | 60px | Bottom margin violation on tall charts | Decrease to 50px for compact layouts |
| Contrast ratio (text) | 4.5:1 | Light text on light background fails | Use 3:1 for secondary text |
| Font size minimum (data labels) | 10px | Small labels flagged as unreadable | Decrease to 8px for dense charts |
| Chart width minimum | 600px | Small embedded charts flagged | Decrease to 400px for mobile/widgets |

**How to tune:**
1. Identify false positive pattern (e.g., "legends flagged as overlapping")
2. Run validation with `--verbose` to see exact overlap distance
3. Adjust threshold in validation script (e.g., `OVERLAP_TOLERANCE = 10`)
4. Re-validate to confirm fix
5. Document change with reason (e.g., "Tightened layouts require 10px overlap tolerance")

## Success Metrics

- **Time saved:** 80% reduction in manual chart review time
- **Issue detection:** 80%+ of visual issues caught automatically
- **False positive rate:** <10% (tunable per project)
- **Manual review focus:** 90%+ on business logic, not visual bugs

## Related Skills

- `/cf` - Manual feedback capture (business logic focus)
- `/if` - Feedback integration (combines auto + manual)
