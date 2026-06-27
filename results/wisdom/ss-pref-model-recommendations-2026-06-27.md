# SS-Pref Model Recommendations for LongMemEval

**Date:** 2026-06-27  
**Issue:** #1221  
**Hardware target:** DGX Spark GB10, 128 GiB, vLLM v0.20.0  
**Scope:** `single-session-preference` ("ss-pref") benchmarking with 100-500 memory blocks in play

---

## Current Evidence

| Model | Strict | Notes |
|---|---:|---|
| Sonnet 3.5 | 53.3% | API ceiling, not a local deployment target |
| Qwen3-32B BF16 | 50.0% | Current best open model on this workload |
| Nemotron 120B NVFP4 | 37.9% | Strong baseline, but clearly below Qwen on ss-pref |
| Mistral 24B (6k) | 20.0% | Better than 4k, still 30 points behind Qwen |
| Mistral 24B (4k) | 6.7% | Context starvation |

The local evidence already answers the first-order question: `Qwen3-32B BF16` is the incumbent until another local model beats it on the same ss-pref slice.

Memory estimates below are approximate and should be read as:

- `weights-only`: raw parameter storage at the named precision
- `practical serving`: weights plus typical runtime overhead for vLLM, excluding pathological KV-cache peaks

For this workload, leave another 10-20 GiB of headroom beyond the "practical serving" estimate whenever possible because long prompts, allocator fragmentation, and parallelism spikes are real.

## Ranked Candidates

| Rank | Model | Why it belongs here | Estimated memory footprint |
|---|---|---|---|
| 1 | **Qwen3-32B BF16** | Best open result already observed on the target benchmark. Strongest control model for every future ablation because it is proven on the exact workload, not just plausible on paper. | ~60 GiB weights-only; ~66-74 GiB practical serving |
| 2 | **Llama 3.3 70B NVFP4** | Best upside under the memory budget. The extra capacity should help preference disambiguation when many near-duplicate personal-preference memories compete in context. Quantized, but still large enough to be a real challenger rather than a side-grade. | ~33 GiB weights-only; ~40-48 GiB practical serving |
| 3 | **Qwen2.5-72B NVFP4** | Similar budget class to Llama 3.3 70B NVFP4, with a stable instruct family and good long-context prior. Slightly lower priority than Llama 3.3 only because the repo's current winning line is already Qwen3, so this is more of a "bigger older sibling" test than a new family bet. | ~34 GiB weights-only; ~41-50 GiB practical serving |
| 4 | **Gemma 3 27B BF16** | Worth testing, but not the first challenger. It is likely competitive on formatting and instruction fidelity, yet the current evidence does not justify expecting it to beat Qwen3-32B on long-context preference tracking. Treat it as a lower-cost challenger, not the favorite. | ~50 GiB weights-only; ~56-64 GiB practical serving |
| 5 | **Qwen3-32B NVFP4** | Good screening surrogate for fast benchmark iteration. Same family and prompt behavior as the incumbent, so it is the least risky quantized proxy. Still, do not let it replace BF16 as the canonical scoreboard without calibration. | ~15 GiB weights-only; ~19-24 GiB practical serving |
| 6 | **Phi-4-reasoning BF16** | Lower-confidence candidate. Reasoning-specialized models often spend budget on verbose internal synthesis that does not directly help ss-pref answer extraction. Test only if you can disable or constrain extra reasoning behavior. | ~26-28 GiB weights-only; ~32-38 GiB practical serving |
| 7 | **Mistral 24B BF16** | Deprioritized. The observed 4k and 6k runs are too far below Qwen to justify more benchmark time before better challengers are tested. Revisit only after retrieval/context packing changes materially. | ~45 GiB weights-only; ~50-58 GiB practical serving |

### Direct answers to the issue questions

1. **Gemma 3 27B vs Qwen3-32B**: plausible, but not yet competitive enough to displace Qwen as the default bet. Run it after the 70B/72B NVFP4 challengers, not before.
2. **Qwen3-32B NVFP4 vs BF16**: tolerable for screening, not for final claims. Use it to rank candidate configs quickly, then confirm any winner on BF16.
3. **Best other sub-60 GiB candidates**: `Llama 3.3 70B NVFP4`, `Qwen2.5-72B NVFP4`, then `Gemma 3 27B BF16`.
4. **Phi-4-reasoning**: test only as a side branch; it is not a top-three ss-pref candidate from the current evidence.

## Quantization Guidance

`Qwen3-32B NVFP4` is acceptable if the goal is throughput-efficient screening. It is not acceptable as the only run used for a benchmark headline.

Recommended calibration policy:

1. Fix a stratified ss-pref slice of 100 items.
2. Run `Qwen3-32B BF16` and `Qwen3-32B NVFP4` with identical retrieval and prompt flags.
3. Compare strict score deltas.

Interpret the delta this way:

- `<= 2 points`: acceptable for model/config screening
- `> 2 and <= 4 points`: acceptable for exploratory work only; rerun finalists in BF16
- `> 4 points`: not acceptable; use BF16 for any decision you plan to publish

For the 70B/72B class, NVFP4 is still worth testing because it is the only practical way to fit those models comfortably while preserving context headroom. Just interpret wins conservatively until they beat the BF16 Qwen control on the same slice.

## Config Levers to Prioritize

These are the highest-value repo-native levers for ss-pref work, ordered by expected impact:

1. **Keep `--dual-preference-recall` enabled.** This is the cleanest retrieval-side protection against generic "what do I like?" queries collapsing onto the wrong preference cluster. The code path already unions a subject-anchor recall pass with the baseline recall pass.
2. **Turn on `--topic-anchor-boost` for ss-pref campaigns.** This specifically targets the multi-preference-session distraction problem by boosting on-topic preference memories whose content matches the domain tokens extracted from the question.
3. **Keep `--query-paraphrase-passes=3` unless latency becomes dominant.** The repo already treats three paraphrase passes as the P0 default. For preference questions, paraphrases help when the user's wording and the stored preference wording diverge.
4. **A/B the server-side `PreferenceMMR` pass next.** It exists specifically to surface domain-specific preference sessions buried under a dominant topic cluster. That is close to the failure shape expected in ss-pref over large memory sets.
5. **Do not expect `session-ndcg-agg` to move ss-pref.** The engine intentionally skips session-DCG aggregation when a preference query is active, so this lever is for other question classes.
6. **Do not spend ss-pref time on temporal levers such as `--inject-question-date`.** Those are useful for temporal-reasoning recovery, not for the preference-tracking bottleneck described here.

## Recommended Run Order

1. Keep `Qwen3-32B BF16` as the control on every new ss-pref campaign.
2. Run `Llama 3.3 70B NVFP4` next with `--dual-preference-recall --topic-anchor-boost`.
3. Run `Qwen2.5-72B NVFP4` on the same slice and compare directly.
4. Run `Gemma 3 27B BF16` only if the two large NVFP4 challengers fail to beat Qwen.
5. Use `Qwen3-32B NVFP4` for rapid config sweeps, then replay finalists on BF16.

## Bottom Line

If the goal is to maximize ss-pref strict accuracy with the least wasted benchmark time:

- keep `Qwen3-32B BF16` as the baseline and likely winner
- try `Llama 3.3 70B NVFP4` first as the highest-upside challenger
- try `Qwen2.5-72B NVFP4` second
- treat `Gemma 3 27B BF16` as a worthwhile but lower-priority challenger
- use `Qwen3-32B NVFP4` only as a calibrated screening proxy, not as the final published number

That ordering follows the evidence already present in this repo: preference benchmarking is currently bottlenecked more by long-context retrieval/disambiguation behavior than by raw prompt formatting, so larger-capacity challengers are a better bet than lateral 24-32B swaps unless they bring a meaningfully different retrieval-following profile.
