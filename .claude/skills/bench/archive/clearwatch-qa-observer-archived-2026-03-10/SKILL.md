---
name: clearwatch-qa-observer
description: Use when running a Clearwatch smoketest and need an adversarial quality observer agent to visually inspect report output, identify chart and layout defects at scale, and file strategic GitHub Issues with fix ideas. Triggers on smoketest runs, report quality reviews, or "deploy Shewhart".
---

# Clearwatch QA Observer (Walter Shewhart)

## Overview

Launches an adversarial outside analyst agent — modeled on Walter A. Shewhart, the father of Statistical Process Control — to monitor a report generation run, visually inspect the output via Playwright screenshots, and file strategic GitHub Issues. Primary focus is charts/graphs, secondary is overall layout. Thinks at industrial scale — every defect recurs across all vendor pairs.

## When to Use

- After kicking off any Clearwatch smoketest
- When you want an outside-analyst perspective on report quality
- Before promoting a report version to `output/latest/`

## Required Inputs

Fill in these four values before launching:

| Variable | Example | Description |
|---|---|---|
| `{VERSION}` | `174` | Report version number |
| `{VENDOR_PAIR}` | `CrowdStrike_v_SentinelOne` | Output directory name |
| `{SMOKETEST_LABEL}` | `smoketest-174` | GitHub label for this run |
| `{LOG_FILE}` | `/tmp/clearwatch-run-174.log` | Path to generation log |

## How to Launch

Copy the prompt from `prompt-template.md` in this directory, fill in the four variables, then launch as a **background agent**:

```
Agent tool → subagent_type: general-purpose → run_in_background: true
```

## What Shewhart Does

1. **Polls** the log until Stage 7 completes and HTML appears
2. **Reads** CLAUDE.md, design-system/, domain-knowledge/ while waiting
3. **Renders** the HTML via Playwright at 1400px width — full page + every SVG chart individually
4. **Visually inspects** every screenshot (not HTML source)
5. **Files GitHub Issues** with 4-5 strategic fix ideas each, labeled `{SMOKETEST_LABEL}`
6. **Tags** architectural ideas with `needs-opus-planning`
7. **Reports** a summary table of all filed issues

## Quality Standards Shewhart Enforces

- Min 12px font size (per CLAUDE.md)
- No overlapping labels or elements
- SVG root must have `width="100%"`
- Silent tie = FAIL: scores within 10% need PARITY callout or chart removed
- No stacked/obscured multi-series charts
- No empty whitespace gaps in tier_staircase

## Known Failure Mode (RED → GREEN)

**Don't** launch Shewhart without Playwright in the prompt. HTML-source-only review misses:
- Font size rendering at actual viewport
- Label collision at rendered dimensions
- SVG clipping at container boundaries
- Color contrast on actual dark backgrounds

The prompt template in this directory has Playwright baked in. Use it as-is.
