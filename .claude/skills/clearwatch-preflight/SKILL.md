---
name: clearwatch-preflight
description: Use when a Clearwatch report has been generated and needs quality verification before declaring it ready. Triggers on report generation completion, before Stage 7 review, or when checking report quality for visual defects, stray markdown, missing verdicts, or chart rendering issues.
---

# Clearwatch Preflight

## Overview

Pre-ship quality gate for Clearwatch reports. Runs automated gates first (fast, objective), then visual inspection, then content spot-checks. Order matters: don't waste time on manual checks if automated gates fail.

## When to Use

- After any report generation (HTML/PDF), before declaring a version ready
- Before Stage 7 review handoff
- When user says "check the report", "is this ready?", "preflight", or "QA this version"

## Preflight Checklist

**Input:** vendor pair + version number
**Working directory:** `~/projects/clearwatch`

### 1. File Check (fastest — do first)

Verify all required files exist at `output/{pair}/{version}/`:
- `{Vendor1}_v_{Vendor2}_{Month_Year}.html`
- `{Vendor1}_v_{Vendor2}_{Month_Year}.pdf`
- `summary.json`
- `prose.json`
- `endnotes.json`

If any missing → **PREFLIGHT FAIL** immediately. Stop here.

### 2. Stage Gate Check (automated — do before any manual work)

Read `summary.json` and check the `stages.gates` object:
- Every gate (0A, 0B, 0C, 0D, etc.) must have `"pass": true`
- All 14 GOLD gates must pass
- Any gate failure → **PREFLIGHT FAIL** with gate ID + `problems` array

Then run the test suites:
```bash
cd ~/projects/clearwatch
pytest tests/unit/test_svg_chart_quality.py -q
pytest tests/qa/ -k "not slow" -q
```
Any test failure → **PREFLIGHT FAIL** with test output.

### 3. Visual Inspection (Playwright screenshots)

Use the chart screenshot extractor — do NOT try to visually inspect raw HTML:

```bash
cd ~/projects/clearwatch
python3 pipeline/validators/chart_screenshot_extractor.py output/{pair}/{version}/{report}.html
```

Then read each generated chart screenshot with the Read tool. Check:
- Fonts ≥12px, no text overlap between labels/values
- `width="100%"` on SVG containers (not fixed pixel widths)
- No dark-on-navy unreadable text (contrast issue)
- All charts have visible data — no blank/empty/zero-height bars
- No clipped or truncated axis labels
- Correct vendor names on each chart

### 4. Content Spot-Check (manual — specific known defects)

Read the HTML and check these specific recurring defects:

| Check | What to look for | Why |
|-------|-----------------|-----|
| Verdict placement | Exec summary opens with verdict, not context-setting | Reports that bury the verdict fail CISO review |
| Stray markdown | No `>` characters in body text | BeautifulSoup sometimes leaks blockquote markers |
| Bold spacing | No missing spaces around `**bold**` text | Markdown-to-HTML conversion artifact |
| Pricing accuracy | Figures match dossier source data in `dossiers/` | Pricing has been wrong before ($180 vs $179.99) |
| Tier scoping | No NGAV-only tiers mentioned | Report scope is EDR+ only |
| Endnote integrity | Every `[N]` in body has matching endnote; no orphans | False citations are a blocking defect |

### 5. PDF Parity Check

Read the PDF and spot-check 3-4 data points against the HTML:
- Overall grades/scores
- One pricing figure
- One chart renders correctly (not blank)
- No blank pages where content should be

### 6. Result

Output a structured preflight report:

```
PREFLIGHT {PASS|FAIL} — {pair} v{version}

Files:        ✅|❌ (list missing if any)
Stage Gates:  ✅|❌ (list failing gates)
Tests:        ✅|❌ (failure count)
Charts:       ✅|❌ (list defective charts)
Content:      ✅|❌ (list defects)
PDF Parity:   ✅|❌ (list mismatches)

{If FAIL: specific defect list + fix recommendations}
{If PASS: "Ready for Stage 7 review"}
```

## Common Mistakes

- Skipping to manual content checks before running automated gates — wastes time if gates fail
- Trying to visually inspect HTML by reading the raw file — use `chart_screenshot_extractor.py` for charts
- Inventing your own pass/fail criteria instead of checking the GOLD gates in `summary.json`
- Skipping PDF check because HTML looks fine — PDF rendering can introduce layout breaks
- Assuming pricing hasn't changed between versions — always verify against dossier
