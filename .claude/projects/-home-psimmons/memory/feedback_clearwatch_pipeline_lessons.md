---
name: Clearwatch pipeline failure patterns and diagnosis rules
description: How to diagnose Stage 5/6 failures — validator bypass paths, retry loop failures, dossier data issues
type: feedback
originSessionId: 4d9b0c22-6135-4b07-a686-4f8a13d472fa
---
When Clearwatch Tier 1 reports fail Stage 6 after pipeline changes, use this checklist before writing any code.

**Why:** The 2026-05-08 campaign found that most failures came from 4 structural patterns, not from incorrect dossier data or LLM quality.

**Pattern 1 — Validator retry prompt teaches bypass**
Check: does the validator's retry message suggest any phrasing that matches a validation exemption? The `_validate_uncited_percentages()` retry message previously said "rephrase as approximately X%" — the exact phrase in the context_exempt pattern. The validator was actively training the LLM to evade its own check.
Fix: read the retry message text and cross-check every phrase against exemption patterns.

**Pattern 2 — Report-level errors silently dropped by surgical retry**
Check: `pipeline/pipeline_v2.py` `_attempt_surgical_retries()` — does it handle `section="report"` errors? These are document-level errors (jargon gate, F1/F2/F3) that don't map to a section name. If the retry loop does `current_sections[section]` without handling "report", they're silently skipped.
Fix: pop "report" key before per-section lookup, broadcast report-level feedback to all sections being retried.

**Pattern 3 — Compound retry whack-a-mole**
Check: if Stage 5's retry loop appends both citation errors AND insight/recommendation errors to the same feedback message, the LLM can only hold one constraint at a time across attempts.
Fix: citation feedback must be isolated. Only add insight/recommendation feedback when citations already pass on that attempt.

**Pattern 4 — FY/jargon in dossier source titles vs claim text**
Check: `FY\d{4}` in dossier JSON may appear in source titles (bibliographic — exempt from jargon gate) or in claim text (must sanitize). Grep both `sources[].title` and `manifest.*claim` fields separately.
Fix: load-time sanitization strips FY prefix from claim text only; jargon gate exempts endnotes/bibliography HTML blocks.

**How to apply:** Run `grep -n "rephrase\|approximately\|around\|up to" pipeline/compiler/validator.py` before shipping any validator change. Any phrase in the retry message that matches an exemption pattern is a bug waiting to happen.
