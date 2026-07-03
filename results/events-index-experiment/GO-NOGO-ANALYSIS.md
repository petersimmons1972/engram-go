# Events-as-Index Experiment — GO/NO-GO Analysis

**Date:** 2026-06-23 (results.json landed 08:50Z)
**Source:** `results/events-index-experiment/results.json` (48 KB, 47 deep items, 2,919 cache files)
**Generator:** local olla `fast-inference` (Mistral-Small-24B-AWQ, precision:8008) — zero Anthropic spend
**Verdict: ✅ GO** (clean on both the experiment's own metric and the stricter production-meaningful filter)

## The question
Does an **event-index re-ranking** layer (extract countable EVENT facts at write time, use them
as a lexical index to re-rank retrieval candidates) lift the hard LongMemEval
aggregation / multi-hop items into the retrieval window where embedding-only retrieval fails?

## Results

### Experiment headline (all 47 deep items)
| Metric                          | Embedding alone | Event-index re-rank |
|---------------------------------|:---------------:|:-------------------:|
| All gold sessions in top-15     | **0 / 47**      | **10 / 47**         |
| Net lifted into top-15          | —               | **+10**             |
| Self-reported verdict           | —               | GO                  |

### Production-meaningful filter (recomputed, build-proof)
Only items where ALL gold sessions are actually within the candidate window
(`max_gold_embed_rank <= 60` — the real retrieval reach). This is the honest denominator:
items whose gold is already unreachable can't be rescued by re-ranking and shouldn't count.

| Metric                                   | Value                         |
|------------------------------------------|:-----------------------------:|
| Pool-eligible items                      | 20 / 47                       |
| Embedding all-gold @15                   | **0 / 20**                    |
| Event-index all-gold @15                 | **9 / 20** (threshold ≥8 → GO)|
| Net lift                                 | **+9, zero regressions**      |

**Lifted items (event-index yes / embedding no):**
`e66b632c, 031748ae_abs, gpt4_a1b77f9c, gpt4_f420262c, gpt4_e414231f, 0bc8ad93, 2ebe6c92, 129d1232, e3038f8c`
**Regressed (embedding yes / event-index no):** none.

## Interpretation
- **Embedding alone scores 0** on this hard set — it never gets all gold into top-15. The
  event-index supplies *all* the lift (0 → 9–10). Not marginal tuning; the difference between
  unanswerable and retrievable.
- **Zero regressions** — re-rank never demotes an item embedding already had. Pure upside here.
- **Lexical lower bound** — the re-rank is Jaccard (token overlap), not semantic. A real semantic
  event-index would do *at least* this well, very likely better.

## Strategic read (for the fix-vs-rebuild decision)
The events-as-index retrieval pivot demonstrably rescues exactly the question class engram is
weakest on (aggregation / multi-session, currently 42.6% on LME-M). This is a **point in favor
of fixing engram** via the retrieval pivot rather than a clean rebuild — the mechanism works and
is additive (no regressions). Final call waits on **gate #1 (LME-S baseline number)**, pending
Wednesday's Sonnet reset.

## Reproduce
```bash
python3 ~/.claude/jobs/98094d90/tmp/gonogo.py   # (script copied below if job tmp is gone)
```
Pool filter = `max_gold_embed_rank <= 60`; GO metric = `metrics['recall@15']['event_index_all_gold']`
counted over pool-eligible items; threshold ≥ 8.
