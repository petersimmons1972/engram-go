> **SUPERSEDED** — Implemented in PR #803 (merged 2026-05-20). This document retained for design context only.

# Issue #748 — lme: taxonomy classifier misclassifies aggregation/counting questions as missing_recall

**Severity:** nice-to-have
**Area:** lme-tooling
**Status:** Design only — not yet implemented

## Root cause

The failure taxonomy classifier in `results/v9-failure-taxonomy.json` (generated post-run) classifies items by computing word-overlap between the gold answer and the retrieved hypothesis. When the gold answer is a bare digit ("2", "3", "4"), word-overlap is near-zero with any natural-language block. The classifier's branch `"Gold answer is empty or too short to analyze"` fires (threshold: ≤3 chars), assigning `missing_recall`. However, `results/research-notes-failure-patterns.md` §Section 2 (Pattern A) documents that **all 26 affected items have their gold session IDs present in the retrieved set** — the failure is aggregation, not missing data.

The classifier logic is not in a standalone Go file; based on the research notes, it was applied as a post-processing analysis step. The taxonomy JSON labels 26 `multi-session` items as `missing_recall` that should be `aggregation_failure`.

## Repro

```bash
# Open results/v9-failure-taxonomy.json and filter missing_recall items
python3 - <<'EOF'
import json
with open("results/v9-failure-taxonomy.json") as f:
    d = json.load(f)
missing = [i for i in d["items"] if i["class"] == "missing_recall"]
print(f"missing_recall count: {len(missing)}")
for m in missing[:5]:
    print(m["question_id"], repr(m["evidence"]))
EOF
# All 26 will show: "Gold answer is empty or too short to analyze."
# Cross-reference against longmemeval_m_cleaned.json — the gold answer is a
# single digit (e.g. "4") representing a count across sessions.
```

## Proposed patch

This is a classifier script fix. The taxonomy generation logic needs a third branch: if the gold answer matches `^\d+$` AND the item's gold `answer_session_ids` are present in the retrieved set, classify as `aggregation_failure` rather than `missing_recall`.

```diff
--- a/results/taxonomy_classify.py   (hypothetical — logic currently inline)
+++ b/results/taxonomy_classify.py
@@ -1,0 +1,15 @@
+import re
+
+NUMERIC_RE = re.compile(r'^\d{1,4}$')
+
 def classify_item(item, retrieved_session_ids, gold_session_ids):
-    if len(item["gold"]) <= 3:
-        return "missing_recall", "Gold answer is empty or too short to analyze."
+    if NUMERIC_RE.match(item["gold"].strip()):
+        gold_present = bool(gold_session_ids & retrieved_session_ids)
+        if gold_present:
+            return "aggregation_failure", (
+                "Gold answer is a count/aggregate. Gold sessions retrieved "
+                f"({len(gold_session_ids & retrieved_session_ids)}/{len(gold_session_ids)}) "
+                "but model failed to aggregate across sessions."
+            )
+        else:
+            return "missing_recall", "Numeric gold; gold sessions not in top-K."
     overlap = word_overlap(item["gold"], item["hypothesis"])
```

For the existing JSON taxonomy, a one-time re-classification migration is also needed — change the 26 existing `missing_recall` entries with `evidence` matching "empty or too short" to `aggregation_failure` where the gold session IDs were retrieved.

## TDD scenarios

1. **numeric_gold_sessions_retrieved → aggregation_failure** — Given a question with gold "4" and all gold session IDs in the retrieved set, when classify_item is called, then the class is `aggregation_failure` and the evidence mentions "Gold sessions retrieved."
2. **numeric_gold_sessions_missing → missing_recall** — Given a question with gold "4" and no gold session IDs retrieved, when classify_item is called, then the class remains `missing_recall`.
3. **non-numeric_short_gold → missing_recall unchanged** — Given a question with gold "yes" (3 chars, non-numeric), when classify_item is called, then the existing short-answer path applies unchanged.
4. **regression: 26 existing items reclassified** — Given the v9 taxonomy JSON loaded as a fixture, when the re-classification script runs, then exactly 0 items have `missing_recall` + `evidence="Gold answer is empty or too short"` remaining (all should become `aggregation_failure`).

## Risk notes

- Backwards compat: the JSON taxonomy format is unchanged; only the `class` field value changes for the 26 items. Existing scripts that pivot on `missing_recall` counts will see lower numbers — this is correct.
- The actual Go benchmark pipeline is unaffected; this is a post-processing analysis tool only.
- The `aggregation_failure` class string matches `types.FailureClass` in `internal/types/types.go:47` — verify before using.

## Rollout

No deploy steps. Re-run taxonomy generator script against `results/v9-failure-taxonomy.json` (or existing run directories) after applying the patch. Update `lme-camp-report.md` with corrected distribution.

## Out of scope (followups)

- H1 prompt fix (arithmetic instruction in generation prompt) — separate concern that addresses the root failure mode that this classifier now correctly identifies. See `results/research-notes-failure-patterns.md` §Section 3, H1.
- Exp 13 re-run with corrected aggregation prompt.
