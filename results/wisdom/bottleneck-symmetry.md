# Bottleneck Symmetry — Pattern from LME Campaign 2026-05-19/20

## The principle

When you fix one side of a two-stage system (retrieval + generation), score does not move if the **other** side is the bottleneck for the failure class you targeted. Both Exp 15 and Exp 16 demonstrated this in opposite directions.

## Evidence

### Exp 15 — retrieval fix, generation bottleneck

H17 paraphrased multi-pass BM25 union on 24 incidental-mention failures.

- **Retrieval objective**: 100% achieved. Gold session in union-retrieved set = 24/24, up from 0% with single-pass BM25.
- **Score uplift**: 5/24 CORRECT (21%). Modest — because for the 19 still-failing items, gold session is now in context but Sonnet cannot extract the answer.
- **Diagnosis**: retrieval was the binding constraint; once removed, generation becomes the new binding constraint.

### Exp 16 — generation fix, retrieval bottleneck

H16 question_date + per-block session_date injection on 17 cluster-B relative-anchor failures.

- **Generation objective (arithmetic)**: achieved. Spot-checking still-INCORRECT items confirms Sonnet now resolves "5 days ago" / "yesterday" / day-of-week to the correct concrete date.
- **Score uplift**: 0 absolute (3/17 → 3/17). Because at the resolved date, the wrong session/entity is retrieved — Sonnet cannot disambiguate which "yesterday" event is meant when retrieval returned events from multiple dates near the target.
- **Diagnosis**: generation arithmetic was not the binding constraint; entity disambiguation at the resolved date is, and that's a retrieval-side problem (insufficient date-window filtering).

## Consequences for hypothesis selection

1. **Classify the dominant mechanism before designing the fix.** A class-specific mechanism analysis (see `failure-mechanism-traces.md` and `temporal-reasoning-mechanism-analysis.md`) tells you which side to attack. Cross-side fixes will null-result.
2. **Necessary-but-not-sufficient ≠ failure.** Exp 15 was the campaign's clean retrieval win even though score barely moved. The retrieval primitive ships as a baseline default.
3. **Score is the wrong metric for proving a retrieval fix.** Measure retrieval coverage (gold-in-context %) separately from end-to-end score.
4. **Combined fixes** are the way to make score move for classes where both sides are bottlenecks. Single-side experiments are useful for diagnosis but rarely lift the headline number alone.

## Cross-references

- `failure-mechanism-traces.md` — Patterns 2 (preference distraction) and 3 (single-pass BM25 missing incidental) are the L1 data supporting Exp 15.
- `temporal-reasoning-mechanism-analysis.md` Section 8 — post-campaign correction confirming Exp 16's autopsy.
- `lme-camp-final-report.md` lines 136-138 — the seed paragraph this doc expands.
- Engram decisions: `019e44fb-a3ce-7263-84db-a706fac7fbab` (Exp 15), `019e44f3-c799-73cf-a7a0-37567cd0f36c` (Exp 16).
