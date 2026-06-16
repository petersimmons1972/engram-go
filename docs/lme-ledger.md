
## inject-question-date (lever 2) — REJECTED
vs det-baseline (deterministic temp=0.0):
- OVERALL 24.6% -> 21.5% (-3.1)
- temporal-reasoning 37.8% -> 28.9% (-8.9, 17->13)  [regressed its own target]
- multi-session 17.0% -> 14.9% (-2.1)
- preference 17.2% -> 16.7% (flat); small-n (ku/ssu/ssa) = single-item noise
VERDICT: reject. Prepending 'Today's date' distracts temporal reasoning, not anchors it.

## DIAGNOSTIC (2026-06-15) — retrieval is NOT the bottleneck
Ran existing `retrieval-metrics` subcommand on det-baseline (gold sessions vs retrieved):
- gold_in_context: OVERALL 99.3% (multi-session 100%, temporal 100%, preference 96.7%, ku 100%, ssu 100%)
- BUT AvgRank of gold is deep for precision cats: preference 41.1, single-session-user 30.4, temporal 12.8 (vs multi-session 4.1, ku 1.3)
- NDCG@5 low: preference 0.315, temporal 0.411
DECOMPOSITION: gold retrieved ~99.3%, end accuracy 24.6% => retrieval-miss <=0.7%;
~74.7% of questions are GOLD-RETRIEVED-BUT-WRONG = ranking + generation failures, NOT recall.
IMPLICATION: prompt-flag levers and recall/topk levers cannot move the needle (recall already ~100%).
The gap is (a) ranking — gold buried under distractors (reranker #1091 directly targets this), and
(b) generation/synthesis from noisy context (context-trim + structured memory).
PRIORITY REORDER: reranker #1091 = #1 (data-confirmed). Context dilution reduction = #2.
Structured write-time memory for synthesis = #3. Prompt flags = abandoned.

## temporal-prompt-aug (lever 3) — REJECTED (data, post-diagnostic)
OVERALL 24.6% -> 22.4% (-2.2); temporal-reasoning 37.8% -> 28.9% (-8.9).
Identical -8.9pp temporal regression as inject-question-date. TWO independent temporal prompt
levers both hurt temporal by the same amount. Conclusive: prompt-flag tuning is a dead end
(recall is ~100%; the deficit is ranking+generation). No further prompt-flag levers.

## SESSION 2026-06-15 — generation lever + reranker gate + preference fix (4 results)

### gen-Sonnet (lever: swap generator Nemotron -> Claude Sonnet) — ACCEPTED (the big lever)
Config: OFF retrieval (prod 8788), --generation-model sonnet (claude --print, subscription),
Nemotron judge (score-efficient --scorer-model inference), workers=3. Apples-to-apples vs det-baseline.
- OVERALL 24.6% -> 32.6% (+8.0)
- multi-session 17.0% -> 29.8% (+12.8); temporal 37.8% -> 46.7% (+8.9)
- preference 17.2% -> 16.7% (flat correct, but 63.3% PARTIAL — generation commits poorly here)
VERDICT: accept. Generator swap is the single largest lever measured. Confirms the diagnostic:
gold is retrieved ~100%, the loss was generation/synthesis from noisy context.

### gen-Opus (lever: generator = Claude Opus) — REJECTED as global generator
Same config, --generation-model opus.
- OVERALL 31.9% vs Sonnet 32.6% (tie, slightly behind) vs base 24.6%
- multi-session 23.4% (< Sonnet 29.8%); temporal 44.4% (~ Sonnet 46.7%)
- preference 23.3% (> Sonnet 16.7%, +6.6) — Opus naturally commits better on preference
VERDICT: reject as global generator (no overall gain over Sonnet, heavier on the 5h window).
Sonnet is at the generation ceiling. KEEP the preference signal: Opus +6.6pp on preference
triangulates with enumerate-prompt (+10pp) => preference loss is a generation-COMMITMENT problem.

### reranker answerability gate (lever: ENGRAM_ANSWERABILITY_RERANKER, server-side) — CONDITIONAL
Generator-free A/B: OFF=prod(8788) vs ON=cloned in-cluster pod(8790), identical prod image+flag.
Intersection of done items (135=135). Trust NDCG@5/Recall@5/GoldInCtx (AvgRank noisy — variable tail).
- OVERALL NDCG@5 0.438 -> 0.464 (+0.03, small net +)
- multi-session NDCG@5 0.533 -> 0.602 (helps); temporal 0.411 -> 0.462 (helps top-5)
- preference NDCG@5 0.315 -> 0.264, Recall@5 0.400 -> 0.300, GoldInCtx 100% -> 96.7% (HURTS — drops gold out of ctx)
VERDICT: category-specific, NOT a global win. Enable for multi-session/temporal; NEVER on preference.
Rerank pod torn down cleanly; prod pod untouched.

### preference-enumerate prompt (lever: GenerationPromptPreferenceEnumerate) — ACCEPTED (preference fix)
30 preference items, Sonnet generator, Nemotron judge. Anchors output ("The user prefers:") +
forces a specific named product/brand/feature/location.
- CORRECT 16.7% -> 26.7% (+10.0); PARTIAL 63.3% -> 33.3% (-30.0); INCORRECT ~20% -> 40% (+20.0)
VERDICT: accept with caveat. Net +10pp strict-correct on preference (the category nothing else moved),
by forcing commitment. Trades safety for commitment: hard-wrong rate ~doubles. Directional win.

### SESSION DECISION
Sonnet generator = the lever (+8.0pp, ship it). Opus = not worth the cost. Residual LME loss is
ranking+generation on preference: address with (1) preference-enumerate prompt on Sonnet (+10pp,
Opus-grade preference without Opus cost), (2) answerability reranker gated to multi-session/temporal only.

## SESSION UPDATE 2026-06-16 — socialization + ship decision (Path B)

Socialized the plan three-way (Codex implementer review + Hermes contrarian). Outcome:
the plan got SMALLER. Per-type reranker gating KILLED (Hermes: question_type is an
eval-only artifact absent in prod; aggregate +0.03 NDCG@5 hid preference Recall@5 -0.10).
Preference-enumerate kept for the SCORE (strict-correct +10pp is real under LME's
CORRECT-only metric; the +20pp incorrect is scoring-neutral) but flagged for a product-side
abstention gate. Run order flipped to stack-first.

SHIP DECISION (founder, Path B). "Sonnet generator" cannot ship server-side: engram's only
Claude path (internal/claude.Client -> Anthropic /v1/messages, x-api-key) needs an API key
that does not exist by policy; the eval's claude --print is a host CLI a pod cannot call.
But the eval generation step MODELS the calling agent — in real use the client's own Claude
IS the Sonnet generator. So: engram stays a memory backend; ship preference-enumerate as a
synthesis_directive attached to preference-intent recall responses, applied client-side.
- Gate on an OBSERVABLE lexical preference-intent signal, never question_type (FM-77).
- Directive carries an abstention clause to avoid confident-wrong (FM-76).
Packaged as Codex issue engram-go#1113 (p1, parallel-safe).

Registered failure modes from the red-team: FM-76 (forced-commitment confident-wrong),
FM-77 (aggregate metric masks per-segment collapse). Home repo commit 8ba2b68.

Stack-first validation (sonnet + preference-enumerate, reranker OFF, full 135): IN PROGRESS.
Expected ~34.8% overall (base 32.6% + preference 16.7%->26.7% over 30/135). Number to be
appended on completion.

## STACK VALIDATION RESULT (2026-06-16) — CONFIRMED
Sonnet + preference-enumerate, reranker OFF, full 135, Nemotron judge.
- OVERALL 32.6% -> 35.6% (+3.0pp); vs 24.6% baseline = +11.0pp.
- single-session-preference 16.7% -> 30.0% (+13.3pp) — the lever, confirmed (beat the
  isolated 30-item run's 26.7%).
- temporal +2.2, multi-session -2.1, single-session-user -20.0 (n=5, ONE item), ku flat.
CAVEAT (FM-77 discipline): non-preference categories are byte-identical prompts (the flag
only alters preference, code-proven), so the small non-pref wobble is claude --print
generation NON-DETERMINISM (single-item flips in small-n categories), NOT flag leakage.
Net +3.0pp is real and preference-driven. Scope-isolation holds.
VERDICT: stack-first validation PASSED. The +10pp preference lever survives stacking and
the gains are additive. Path B (engram-go#1113) ships this behavior client-side.
