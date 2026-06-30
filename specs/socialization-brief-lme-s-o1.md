# Socialization Brief: LME-S Phase 1–11 Results + Full 500Q Baseline — Next Hypothesis Set

**Branch:** `worktree-lme-preference-constraint`
**Date:** 2026-06-29 (amended same day)
**Author:** Chester William Nimitz (Claude Code, session 2966e7d4)
**Audience:** Hermes (consult), Review lane, Codex (impl)
**Prior brief:** `specs/socialization-brief-chonkie.md`

> **⚠ AMENDMENT — CORRECTED DIAGNOSIS (dispatched brief was incomplete)**
>
> The original dispatch (earlier today) stated H-P1 (phrasing mismatch) as HIGH confidence for the preference failure. That was wrong. After discovering 15 pre-existing pref30 experiments (Phase 7–11, run 2026-06-26/27), the diagnosis has been corrected: **context volume is the lever, not prompt engineering**. See §3 below for full correction. The P10D-500Q run to validate is now running (PID 3478267, launched 2026-06-29 19:13 UTC).


---

## 1. What We've Learned — Phases 1 and 2 Complete

### Phase 1: Overlap Sweep (KU-78, 78-item knowledge-update slice)

| Label | Config | Strict | Δ vs baseline |
|-------|--------|--------|---------------|
| KU-R2 | overlap 0 | 61.5% | — |
| KU-O1 | overlap 400 chars | 70.5% | +9.0pp |
| KU-O2 | overlap 800 chars | 69.86% | +8.3pp |
| KU-O3 | overlap 1200 chars | **72.73%** | **+11.2pp ← Phase 1 best** |

Overlap is monotonically beneficial up to 1200 chars, with a dip at 800 (likely noise, n=73 due to 1 run error).

### Phase 2: Turn-Boundary Chunking (KU-78)

| Label | Config | Strict | Δ vs KU-R2 |
|-------|--------|--------|------------|
| TB-0 | turn-boundary only | 71.43% | +9.9pp |
| TB-1 | TB + overlap 400 | 71.79% | +10.3pp ← Phase 2 best |
| TB-2 | TB + overlap 1200 | 64.10% | +2.6pp — **REGRESSION** |

**Key Phase 2 finding:** Stacking turn-boundary with overlap 1200 caused a 7.7pp regression vs turn-boundary alone. The compound interaction is destructive for the knowledge-update question type. Large overlap within variable-length turn-boundary chunks creates chunk redundancy that dilutes retrieval precision.

Phase 2 best config: `--turn-boundary --block-overlap-chars 400` (TB-1, 71.79%).

---

## 2. The Full 500Q Baseline: LME-S-O1

**Config:** overlap 400, no turn-boundary, bench engram port 8789, W6800 embed (BAAI/bge-m3 dims=1024)
**Result:** 70.39% strict / 74.85% lenient (347/493, 7 run errors)
**run_id:** ef0eaf8eae6e4be5

### By Question Type

| Question Type | Strict | Lenient | Strict/Lenient Gap | n |
|---------------|--------|---------|-------------------|---|
| **single-session-preference** | **16.67%** | **63.33%** | **46.7pp ← anomaly** | 5/30 |
| multi-session | 63.64% | 65.15% | 1.5pp | 84/132 |
| knowledge-update | 70.51% | 70.51% | 0pp | 55/78 |
| temporal-reasoning | 72.87% | 74.42% | 1.5pp | 94/129 |
| single-session-user | 84.06% | 86.96% | 2.9pp | 58/69 |
| single-session-assistant | 92.73% | 96.36% | 3.6pp | 51/55 |
| **Overall** | **70.39%** | **74.85%** | **4.5pp** | **347/493** |

---

## 3. The Critical Flaw: single-session-preference — CORRECTED DIAGNOSIS

The 46.7pp strict/lenient gap (16.67% strict / 63.33% lenient) was identified correctly in the initial dispatch. The diagnosis, however, was wrong.

### What Phase 7–11 pref30 experiments revealed

**15 experiments on a 30-question preference slice (pref30) were run 2026-06-26/27 and are registered in `results/benchmark-registry.jsonl`.** The full leaderboard:

| Label | Config | Strict | Δ vs baseline |
|-------|--------|--------|---------------|
| ORACLE (P12) | Perfect retrieval | **73.3%** | **+56.7pp ← ceiling** |
| P10D | 12k chars + topk12 | **43.3%** | **+26.7pp ← best non-oracle** |
| P10C | topk12 (8k chars) | 36.7% | +20.0pp |
| P10A | 12k chars (topk8) | 26.7% | +10.0pp |
| P7B | topic-anchor recall | 23.3% | +6.7pp |
| P10E | 12k + dual-pref | 23.3% | +6.7pp |
| P7C | dual-tab recall | 20.0% | +3.3pp |
| P11A | pref-enumerate | 20.0% | +3.3pp |
| Baseline (P7A) | topk8 + 8k chars | 16.7% | — |
| P11B | pref-enum + dual | 13.3% | **−3.3pp** |
| P11C | pref-stack | 13.3% | **−3.3pp** |
| P11D | 12k + pref-stack | 13.3% | **−3.3pp** |
| P7E | recall-repair | 10.0% | **−6.7pp** |

**Note:** P10B (16k chars) scored only 23.3% — diminishing returns beyond 12k.

### Corrected diagnosis

**H-P1 (phrasing mismatch) — REFUTED.** The generation prompt already says "Start your response with 'The user would prefer...'" and results are checked — the mismatch is not format but specificity. The strict judge counts items PARTIALLY_CORRECT when right substance but missing specific details. Prompt engineering (P11A-D: pref-enumerate, pref-stack) made things WORSE across the board, including the 12k variant.

**H-P2 (retrieval context volume) — CONFIRMED PRIMARY CAUSE.** The single biggest lever is how much retrieved context reaches the LLM:
- topk 8→12 alone: +20.0pp (P10C)
- 8k→12k chars alone: +10.0pp (P10A)
- Combined (P10D): +26.7pp

The preference questions in LME-S describe implicit preferences scattered across many conversation turns. The model needs to see MORE retrieved chunks to assemble the full picture.

**H-P3 (recall precision) — PARTIALLY CONFIRMED.** P7B (topic-anchor) gave +6.7pp vs baseline by improving which chunks are retrieved. But recall quality improvements are secondary to context volume.

**H-P4 (ground truth overconstrained) — PARTIALLY CONFIRMED.** The oracle at 73.3% strict (with PERFECT retrieval) shows 8/30 questions have inherently hard-to-match ground truth. These are a hard floor.

### Current status
**P10D-500Q is running** — full 500Q re-run with topk12 + 12k chars, using existing bench ingest checkpoint (no re-embed). PID 3478267, started 2026-06-29 19:13 UTC, expected ~8 hours. Will show whether pref30 gains (43.3%) transfer to the full distribution and what the cross-type effect of topk12 is.

---

## 4. What We Want to Know From Fleet (UPDATED)

The original dispatch questions about H-P1 vs H-P2 are now answered by data (P11A-D refuted H-P1; P10C-D confirmed H-P2). The current open questions are:

### Questions for Hermes (consult)

1. **P10D cross-type effects:** P10D raises topk to 12 globally and max-block-chars to 12k. This may HELP preference (+26.7pp on pref30) but could HURT other types by diluting recall precision with more chunks. Which types are most at risk? Knowledge-update (70.5%) is already strong; would topk12 introduce noise? Multi-session (63.6%) might benefit from seeing more chunks per question. What's your prior on directional effects per type?

2. **Preference-specific topk override vs global:** If P10D-500Q shows that topk12 hurts other types, the mitigation is a per-type topk override (topk=12 for single-session-preference, topk=8 for everything else). Is there an implementation path in the binary that supports this without a new flag? See `cmd/longmemeval/run.go` line 682: `contextLimit = longmemeval.ContextTopKForTypeWithBump(item.QuestionType, cfg.ContextTopKBump)` — `ContextTopKBump` is currently all-or-nothing. A per-type table would require code changes.

3. **Phase 3 semantic chunking relevance post-P10D:** If P10D gets preference to ~35-43% on 500Q, should Phase 3 (semantic chunking) still be pursued? The argument for: semantic chunking might group preference-bearing sentences, reducing the need for topk12. The argument against: we've shown the lift comes from retrieval volume, not chunk quality. Which is the better ROI path?

4. **Multi-session (63.6%):** P8A-E2 experiments tested multi-session on a 30-item slice; best was P8E at 65.5%. Turn-boundary was NOT tested for multi-session. TB-3 (full 500Q with TB-1 config) would add this signal. Should TB-3 run before or after P10D-500Q completes?

### Questions for Review lane

1. **P10D-500Q run validity:** I launched P10D-500Q by copying the LME-S-O1 ingest checkpoint (500 sessions already embedded at port 8789) and running with topk=12, max-block=12k. The run command reads `checkpoint-ingest.jsonl` to find project IDs and skips re-ingest. Is this methodology sound, or could the copied checkpoint cause issues (e.g., project ID collision, stale embeddings)?

2. **Compound regression analysis:** TB-2 regressed 7.7pp vs TB-0 when adding overlap 1200. We attributed this to chunk redundancy diluting retrieval. Is there a simpler explanation (e.g., total chunk count explosion causing ranking noise at topk8)? Would topk12 actually HELP TB-2 by allowing the higher chunk count to be useful?

3. **KU-78 proxy for non-KU types:** Phase 1+2 ran only on KU-78. We now have pref30, multi30, user30, temporal30 slices tested through P7-P11. But these are type-specific slices. The only full 500Q result is LME-S-O1. What's the confidence level that pref30 results predict 500Q preference behavior within ±10pp?

---

## 5. Next Hypothesis Set (UPDATED — based on Phase 7–11 data)

### Running now

**LME-S-P10D-500Q** — Full 500Q with topk12 + 12k chars. Uses existing bench ingest (no re-embed). Expected: preference ~35-43% strict (from 16.67%), other types uncertain. **This is the gate experiment for preference.**

### Tier 1: After P10D-500Q result (queue now, start after P10D completes)

**TB-3 (LME-S-P10D + turn-boundary)** — Run 500Q with `--turn-boundary --block-overlap-chars 400 --context-topk 12 --max-block-chars 12000`. Combines TB-1 (best KU-78 config) with P10D (best pref30 config). Expected stacking: +10pp KU from TB, +26pp pref from P10D. Multi-session unknown — TB has not been tested on multi-session.

**LME-S-O3-500Q** — Full 500Q with overlap 1200. KU-O3 = 72.73% on KU-78. Does overlap 1200 hurt other types as badly as it hurt turn-boundary? This answers whether O3 is safe to stack in Phase 3.

### Tier 2: Pending P10D cross-type analysis

**Preference-specific topk override (code change)** — If P10D-500Q shows topk12 hurts other types, implement per-type topk table (pref→12, others→8). Required code: modify `ContextTopKForTypeWithBump` to take a per-type map. Medium effort, TDD required (new flag + unit test).

**Phase 3 semantic chunking** — Gate met via TB-1 (+10.3pp). Defer until P10D result is in. If P10D gets preference to 40%+, the remaining gap is ~33pp to oracle; semantic chunking is likely NOT the next lever. If P10D gets preference only to 25-30%, semantic chunking becomes more relevant.

### Tier 3: Infrastructure / analysis

**Inspect 19 PARTIALLY_CORRECT preference items** (H7 from original brief) — These are lenient-correct but strict-wrong from LME-S-O1. What's the phrasing gap? This is a 30-minute manual analysis that could sharpen the prompt if needed (though P11A-D showed prompt changes alone don't help).

**Multi-session slice experiments** — P8A-E2 used a 30-item multi30 slice; best was P8E (65.5%, unknown config). Check P8E config and whether TB runs were included. If TB was not tested on multi30, run P8F = TB on multi30 as a fast check before TB-3.

---

## 6. Architecture Notes (unchanged from prior brief)

See `specs/socialization-brief-chonkie.md` §3 for ingest/run path and ChunkText signature. No architecture changes since Phase 1+2 implementation (PR #1253, PR #1255).

**Current binary flags available:**
- `--block-overlap-chars N` (Phase 1, PR #1253)
- `--turn-boundary` (Phase 2, PR #1255)
- `--context-topk N` (existing)
- `--max-block-chars N` (existing, prompt-assembly layer — NOT a chunker param)

**Bench engram:** port 8789, postgres 5433, W6800 embed localhost:8006, BAAI/bge-m3 dims=1024
**Production engram:** port 8788, `ENGRAM_API_KEY` env var (never pass `--api-key` explicitly)
**LLM:** `http://192.168.0.138:30411/olla/openai/v1`, model `inference` (Qwen3-32B), unauthenticated

---

## 7. Constraints (unchanged)

- No Sonnet for generating — Qwen3-32B via Olla
- No `--enable-thinking` flag (do NOT use with Nemotron v3)
- MI-50 (Radeon VII) = production, DO NOT TOUCH / DO NOT REEMBED
- W6800 = precision (bench embed OK), 7900 XT = leviathan (local)
- `--workers 2` max on KU-78 runs

---

## 8. Files Index

| File | Role |
|------|------|
| `results/benchmark-registry.jsonl` | All results: KU-R2, KU-O1–O3, TB-0–2, LME-S-O1 |
| `specs/chonkie-concepts-lme-lift-plan.html` | Full plan with Phase 1–4, amendments |
| `internal/chunk/chunker.go` | ChunkText, splitSentences, LazyChunkThreshold |
| `internal/chunk/chunker_turnboundary.go` | ChunkTextTurnBoundary (Phase 2) |
| `cmd/longmemeval/ingest.go` | ingestOne(), QuickStore call site |
| `cmd/longmemeval/run.go` | runOne(), prompt assembly, generation |
| `testdata/longmemeval/longmemeval_s.json` | LME-S 500Q dataset (25,112 sessions) |
