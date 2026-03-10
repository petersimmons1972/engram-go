---
name: adversarial-source-check
description: Use when reviewing claims in Clearwatch reports for source integrity — pricing accuracy, detection rates, labor costs, ROI figures, or any factual assertion that traces to a citation. Triggers on source verification requests, claim audits, or when investigating potential fabrication, false attribution, or vendor-commissioned bias.
---

# Adversarial Source Check

## Overview

Systematic methodology for verifying report claims trace to legitimate, relevant sources. The key value is **completeness** — applying the same 6 challenge tests to every claim, every time, instead of only the ones that come to mind in the moment.

## When to Use

- Reviewing any claim category for source integrity (pricing, labor, detection rates, ROI)
- After generating a new report version with updated data
- When user says "verify sources", "check claims", "audit citations", or "source check"
- Proactively on high-stakes claim categories before shipping

## Methodology

**Input:** claim category + report version (or "all claims in section X")
**Working directory:** `~/projects/clearwatch`

### 1. Extract (systematic — do this BEFORE evaluating anything)

Find every claim in the specified category. For each one, record:
- Exact claim text
- Section location in the report
- Citation number `[N]`

Output as a numbered list. Do not skip this step or combine it with evaluation — you need the complete inventory first to spot patterns (e.g., all claims sourced from a single vendor study).

### 2. Trace

For each citation `[N]`, read `endnotes.json` in the report output directory and find the actual source. Classify each:

| Type | Definition | Example |
|------|-----------|---------|
| `independent` | Third-party research, no vendor funding | Gartner MQ, MITRE evaluations, BLS data |
| `vendor-commissioned` | Vendor paid for the research | Forrester TEI, vendor-sponsored IDC reports |
| `internal_analysis` | Our own calculations or estimates | Staffing models, composite pricing |
| `uncited` | No source provided | Missing endnote or dead link |

**Key trap:** Forrester/Gartner name does NOT mean independent. TEI studies are always vendor-commissioned. Check who paid.

### 3. Challenge (apply ALL 6 tests to EVERY claim)

Do not cherry-pick which tests to apply. Run all six — any single failure downgrades the claim:

| # | Test | What to check |
|---|------|--------------|
| 1 | **Scope mismatch** | EDR-only claim applied to full platform? Or vice versa? |
| 2 | **Scale mismatch** | Study covers 10K+ endpoints but our target market is 500–2,500? |
| 3 | **Managed vs self-managed** | MDR pricing compared to self-managed deployment? |
| 4 | **Selection bias** | TEI methodology surveys only the vendor's happiest customers |
| 5 | **False attribution** | Claim says "Forrester found..." but the actual report doesn't say this |
| 6 | **Stale data** | Source >2 years old in a fast-moving market |

### 4. Grade

Use these exact grades — each maps to a specific action:

| Grade | Criteria | Action |
|-------|----------|--------|
| **VERIFIED** | Independent source, relevant scale, scope matches | Keep as-is |
| **DIRECTIONAL** | Vendor source, right direction, wrong precision | Add disclosure footnote |
| **UNVERIFIABLE** | No source exists for this specific claim | Reframe as qualitative |
| **FABRICATED** | Citation points to nothing, or source contradicts claim | Delete immediately |

### 5. Recommend

Use consistent remediation language per grade:

- **VERIFIED** → keep, no changes
- **DIRECTIONAL** → add footnote: *"Based on a vendor-commissioned [Forrester/IDC] study; directional estimate for the composite organization modeled"*
- **UNVERIFIABLE** → reframe: *"Industry practitioners report..."* or use structural honesty: *"No independent benchmark exists for this metric; we estimate based on [methodology]"*
- **FABRICATED** → delete the claim entirely. File as a defect for investigation.

### 6. Output

Produce a graded claim table:

```
SOURCE INTEGRITY AUDIT — {category} — {pair} v{version}

| # | Claim (abbreviated) | Source Type | Grade | Issues | Action |
|---|---------------------|------------|-------|--------|--------|
| [N] | ... | independent/vendor/etc | VERIFIED/etc | ... | ... |

Summary: X/Y claims VERIFIED, X DIRECTIONAL, X UNVERIFIABLE, X FABRICATED
Recommendation: {SHIP / SHIP WITH FIXES / BLOCK}
```

## Common Mistakes

- Evaluating claims as you find them instead of extracting the full list first — you'll miss patterns
- Applying only 2-3 challenge tests instead of all 6 — scope and scale mismatches are easy to miss
- Treating "Forrester" or "Gartner" as automatically independent — always check who commissioned the study
- Using ad-hoc grading instead of the 4-grade system — inconsistent grades produce inconsistent actions
- Skipping the output table — without structured output, findings get lost between sessions
