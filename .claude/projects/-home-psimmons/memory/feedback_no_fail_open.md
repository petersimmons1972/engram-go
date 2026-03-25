---
Category: feedback
name: feedback_no_fail_open
description: Quality gates must NEVER fail open — if a gate can't run, the pipeline stops. No silent degradation.
type: feedback
---

Quality gates must NEVER "fail open" (silently pass when they can't actually check anything). If a gate cannot run, the pipeline MUST stop with a hard error.

**Why:** The visual-tester Stage 5.5 was designed to "gracefully degrade" to SVG unit tests when the K8s pod was unavailable. Those unit tests only checked utility functions on synthetic data — they never rendered actual charts. The fallback reported "PASSED" on every report, giving false confidence while chart rendering bugs shipped undetected. The founder spent hours manually finding 4-5 bugs per chart across dozens of charts that the system was supposed to catch. A quality gate that says "PASSED" when it checked nothing is worse than no gate at all.

**How to apply:** When building any validation gate, quality check, or automated review step:
1. If the checker is unavailable → HARD FAIL with CRITICAL severity
2. Never fall back to a weaker check that reports "passed" — that's lying
3. Log at ERROR level, not WARNING
4. Include actionable fix instructions in the error message (e.g., "Deploy with: kubectl apply -f ...")
5. This applies to ALL gates, not just visual-tester: if cert-manager can't validate, if a test runner can't start, if a reviewer persona can't load — STOP, don't degrade
