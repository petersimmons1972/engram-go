# Grok Self-Reported Failure Modes

**Harness:** Grok (xAI Grok model, fleet-dispatch `impl-grok` / `comment_only` lane consumer)
**Date:** 2026-06-20
**Method:** Dispatched via `fleet-enqueue.sh --capability impl-grok` (bus-only, no ssh)
**Fleet-dispatch item ID:** `c43de81d-a6ba-4ac5-928e-3196b7ffb95d`
**Item state:** `done` | `result_bytes`: 3843 | `retries`: 0

---

## Verbatim Self-Report

### 1. Hallucination from training data / temporal cutoff

I state as fact details about software versions, APIs, CVEs, library behaviors, or external events after the April 2026 training cutoff. This occurs on queries phrased as "current", "latest", or "what does X do now" when I do not immediately call web_search, open_page, or run_terminal_command to ground the claim. Manifests as confident assertions like "Node 22.5 fixed Y" or "the current Terraform provider has field Z" that are simply stale or invented.

---

### 2. Missing or inaccessible ref/context from the dispatch item

I overlook hard constraints, file paths, environment notes, or acceptance criteria present in the user_query tag, attached logs, or referenced files because I only read a subset of the dispatch or treated a large file as "mostly read." Triggered by multi-paragraph instructions, "see task-0-1/..." references, or context that appears after an initial summary. Result: solutions that satisfy the headline request but violate unstated scope, path, or non-functional requirements.

---

### 3. Output format contract violations

I emit the wrong structure for tool calls or final artifacts — e.g., using absolute paths when relative are required, malformed search_replace old_string that does not match exactly, missing code fences, or producing extra prose when the dispatch said "return only the patch." Triggered by rapid iteration, user-specified "exact format" rules, or switching between file-edit tools and free text.

---

### 4. Silent omission under breadth pressure

Given a wide-scope request ("update the whole module", "handle all edge cases", "review the 9 services"), I implement the obvious core path and drop secondary files, tests, or error paths without any note that they were skipped. Common when the task spans >4-5 files or when a single grep finds many matches. The response looks complete because the implemented parts are polished.

---

### 5. Overconfidence on code generation with no execution

I write or edit scripts, build files, or automation logic that is syntactically plausible but fails on first run due to untested assumptions about paths, permissions, env vars, package availability, or shell differences (/bin/sh vs bash). Triggered whenever a dispatch says "add this feature" or "fix the script" and I stop after the edit without running the command, the test, or the build step.

---

### 6. Reasoning chain truncation under length pressure

During extended debugging or analysis of large files/stack traces, I perform the first 2-3 diagnostic steps correctly then jump to a conclusion once the remaining context feels long, silently dropping later causal links or alternative hypotheses. Triggered by very long source files, multi-turn investigation, or when the accumulated tool output + history approaches practical limits. The final answer therefore rests on an incomplete trace.

---

### 7. Tool/search result over-trust

I treat the first page of web_search, open_page, or grep results as authoritative and directly quote or code against them without cross-checking dates, provenance, or local reality. Includes believing misleading snippets, hallucinated function signatures in docs, or tool output that was itself stale or partially truncated. Triggered by time pressure or when the result "looks right" and matches my internal expectation.

---

### 8. Polished-sounding wrong answers that mask uncertainty

I deliver fluent, well-structured explanations or recommendations on topics where the evidence is weak, partial, or absent, using authoritative phrasing ("the correct approach is…", "always use…") instead of hedging or surfacing the actual confidence level. Most common on architectural advice, "best practice" questions, or integration details outside the immediately verified files or tool results. The surface quality hides the lack of grounding.

---

## Coordinator Assessment

**Genuinely novel findings vs. known catalog:**

- Items 1 (hallucination from temporal cutoff) and 8 (polished wrong answers masking uncertainty) map to FM-70 and FM-87. Known; confirmed from a different model's perspective, which is useful cross-model validation.
- Item 2 (missing/inaccessible ref/context) is an important Grok-specific variant: Grok receives dispatch items where the ref payload is the GitHub issue body, and fleet-enqueue warns when issue #0 yields an empty body (as it did in this dispatch). Grok's self-report that it "only reads a subset" of a large dispatch and proceeds against wrong targets aligns with the FM-87 warning about result_bytes truncation — Grok is aware of its own context-windowing failure mode. **This is operationally useful:** briefs to Grok must front-load the critical constraints, not bury them.
- Item 3 (format contract violations, specifically `search_replace old_string` that does not match exactly) is **novel and concrete**. No existing FM captures the specific failure mode of a code-editing tool use where the `old_string` parameter does not exactly match the file content. This is a Grok-specific implementation detail that warrants a catalog note (akin to FM-84 but at the tool-parameter level).
- Item 5 (code generation without execution) maps to FM-30 (handoff to executor lacking toolchain) from the producer side, but Grok's framing is from the consumer's own cognition — it knows it ships unexecuted code. **This is the most valuable self-report:** Grok is confirming that its `impl-grok` output should be treated as a draft requiring a smoke test before merge, not a verified implementation.
- Item 6 (reasoning chain truncation) is a new failure mode name for a known pattern. Not in the catalog as a named entry. The concrete trigger (accumulated tool output + history approaching limits) is operationally actionable for brief-writing: keep Grok briefs below the context pressure point.
- Item 7 (tool/search result over-trust) maps to FM-87 and the general observation-vs-conclusion class. Known.

**Net new signal:** Items 3 (search_replace exact-match failure), 5 (code generation is a draft requiring smoke test), and 6 (reasoning chain truncation under context pressure) are the most actionable new findings. Item 5 in particular changes how `impl-grok` output should be consumed: it is a code draft, not a verified commit. Every `impl-grok` result should be smoke-tested before merge.

**Deferred harnesses:** Codex (review path mid-fix) and OpenCode (not deployed) — will self-report during their capability-proof stages.
