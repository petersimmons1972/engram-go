> ⚠️ **SUPERSEDED 2026-07-15 (Reforge plan v2).** This document carries retracted statistics (±11pp "noise floor", "$0 reproduction", k-dependent instability claims). Defensible replacement: ~9% mean pairwise run disagreement under identical config (SD ~1–2pp), clean rounds only. Do not quote this file for public claims. Historical content preserved below unchanged.

# The Noise Floor Is the Result
### A Reproducible Protocol for Honest Agent-Memory Evaluation on a Local MoE

**Peter Simmons** — Independent · Ground Truth / Clearwatch Research · Holly Springs, GA
`peter.simmons.ga@gmail.com` · github.com/petersimmons1972/engram-go

> **Status:** v0 DRAFT — NOT submission-ready. Two adversarial reviews (GPT-5.6-sol 2/10, Grok-4.5 2.5/10, both REJECT)
> found a fatal statistical error (the ±11pp floor conflates item-flip-rate with an accuracy band — it's ~2× too high and
> contradicts our own 5.6pp k=4 spread), incomplete pre-registration, under-specified McNemar, an unverified variance
> decomposition, and insufficient scope for a methods claim. **Do not submit until the Tier-0 items in
> `duramind/docs/campaign/REVIEW-SYNTHESIS-2026-07-13.md` are resolved.** (Prior panel-audit status retained below.)
> **[Superseded banner]:** v0 COMPLETE + PANEL-AUDITED (updated 2026-07-13). All sections §1–§11 drafted to v0 with sourced numbers and
> arXiv-verified citations. **B4 landed and was audited by a three-judge panel (qwen / gpt-5.4 / Sonnet 5) 2026-07-13:
> structure-as-input is REFUTED as an improvement — negative under all three judges (−7.8 / −6.2 / −3.1pp) but its harm is
> significant only under the strict local scorer (p = 0.031 vs 0.115 / 0.503), so we claim a refuted intervention, not a
> significant harm.** Registry B4/B3 + both audit rows appended; tier-1 snapshot frozen. Remaining before submission:
> re-confirm the §4 scorer self-flip / variance-split from primary artifacts, the Fig. 1 plot, and `make reproduce`.
> *[DRAFT]* = grounded prose; *[TODO]* = a specific number to confirm.
> Voice = Ground Truth: crisp, receipts-first, no hype. Each heavy section carries an **In plain terms** box so a
> reader with intermediate Claude-Code-level understanding can follow the idea without the statistics.

---

## Abstract  *[DRAFT]*

Memory systems for LLM agents are usually evaluated by reporting a single benchmark run: system A scores *X*,
system B scores *X + δ*, and *δ* is presented as an improvement. We show, on LongMemEval-S with a locally-hosted
Qwen3-Next-80B-A3B (~3B active-parameter MoE) generator, that the run-to-run variation of an **unchanged** system is
large enough to swallow most such deltas. Re-running one fixed configuration flips **11.4%** of temporal-reasoning
answers between runs, implying a noise floor of roughly **±11 percentage points at n = 133**. We then pre-register a
powered evaluation protocol — locked scorer, abort criteria fixed before data collection, k-run averaging, and
McNemar's exact test on discordant pairs — and apply it to a ladder of plausible memory "levers." Most do not survive.
A temporal-prompt augmentation that measured **+7.6pp** in a single run collapses to **+1.4pp** under 4-run averaging
(**McNemar p = 1.000**): it was the high tail of an 8.8pp-wide spread, not an effect. The one intervention we expected to
*beat* the floor — injecting a pre-sorted event ledger as *input* — instead came out **negative under a three-judge panel
(qwen / gpt-5.4 / Sonnet 5)**, never improving accuracy; but its harm proves **scorer-dependent** (−7.8pp / p = 0.031
under the strict local scorer, shrinking to −3.1pp / p = 0.50 under a lenient frontier judge), so we report it as a
refuted improvement rather than a significant harm — a live demonstration of our own scorer-discipline warning. We
further report a provable ceiling on deterministic entity-resolution and a prompting-strategy reversal between local and
frontier models. Our central contribution is methodological, and we argue it is more useful
to the field than another headline delta: a characterized noise floor and a reusable protocol that tells you *when a
memory improvement is real*. Every run is tagged with full provenance and reproduces on a single self-hosted GPU.

**Keywords:** LLM agents, long-term memory, evaluation, reproducibility, LongMemEval, statistical power, negative results.

---

## 1. Introduction  *[DRAFT]*

*Purpose of this section (for the author): state the problem, the gap, and your contribution in the first page. A
reviewer decides whether to keep reading here. Lead with the concrete failure, then the fix.*

The agent-memory field reports progress in single-run deltas. Open a vendor benchmark page and you will find one
number per system on LongMemEval or LoCoMo, often in the 90s, with improvements of a few points framed as advances.
This paper asks a prior question: **how much does the number move when nothing changes?**

On a locally-hosted mixture-of-experts generator at temperature 0, we find it moves a lot. [Describe the setup in one
sentence.] The consequence is uncomfortable: a large fraction of published single-run improvements are inside the
run-to-run noise of the measurement itself, and cannot be distinguished from a fixed system re-rolled.

We make three contributions:
1. **A characterized noise floor** for LLM agent-memory evaluation on a local MoE — a quantitative "deltas below *X* pp
   are noise" warning, decomposed into generator and scorer sources.
2. **A pre-registered, powered protocol** — locked scorer, pre-set abort criteria, k-run averaging, McNemar on
   discordant pairs, full provenance tags — that separates real memory improvements from noise.
3. **An honest ladder of results**: mostly refuted levers, one *refuted* structure-as-input result
   (the intervention we expected to win — negative under a three-judge panel, never improving; harm not robustly
   significant), and a provable ceiling — a negative-result-forward case study we argue is more useful than another "+X pp."

> **In plain terms.** If you flip a coin and it comes up heads, you haven't proven the coin is rigged. Most
> agent-memory papers show you one coin flip and call it science. We measured how noisy the coin is, then built a way
> to tell a real edge from a lucky toss — and used it to knock down most of our own ideas.

---

## 2. Setup  *[DRAFT]*

*Purpose: give a reader everything needed to know what was measured and on what. Precise, not yet argumentative.*

- **Benchmark:** LongMemEval-S (temporal-reasoning subset, n = 133; common item set N = 125 across k-replicated arms).
- **Generator:** Qwen3-Next-80B-A3B-Instruct-FP8 (~3B active params, low-active-param MoE) on a self-hosted vLLM
  (NVIDIA DGX Spark, "oblivion"). Free, repeatable, no frontier API. Temperature 0.
- **Memory backend:** engram (Go + PostgreSQL/pgvector, MCP tool-use, hybrid BM25 + semantic recall), bge-m3 embedder,
  MRL-truncated to 1024 dims.
- **Scorer:** a single locked tier-1 local scorer (`tier1-qwen3-2026-06-22`), calibrated once against a reference; we
  **never compare scores across scorers** (§3).
- **Provenance:** every run tagged `{gold-version, scorer, feature-flags, system, run-id}`; the master table is
  `benchmark-registry.jsonl`. *[TODO: cite exact stack versions from `scorer-lock.json`.]*

> **In plain terms.** One benchmark (a set of memory questions), one local model doing the answering, one local model
> doing the grading, everything logged so anyone can re-run it. Nothing here costs money or calls out to a big lab's API.

---

## 3. Methodology  *[DRAFT]*

*Purpose: this is the section that makes it a paper and not a blog post. It shows the result can't be an artifact of
how you measured. Pre-registration + a real significance test are the credibility spine.*

- **Pre-registration.** Abort criteria and the scoring configuration were fixed *before* data collection. No verdict is
  read at n < 79. *[TODO: state the exact pre-registered thresholds.]*
- **k-run averaging.** Each condition is run k ≥ 3 times (k = 4 for the headline levers) and reported as mean-of-k and
  as a majority-vote (self-consistency) aggregate, on the common item set present in all arms.
- **Paired significance.** Effects are tested with **McNemar's exact test** on discordant item pairs (the items one
  condition fixes and the other breaks), not by comparing two accuracy point-estimates.
- **Scorer discipline.** One locked scorer for all repeatable runs; a small GPT-class honesty-audit on a sample, never
  per-run; the official LME judge only to anchor a single headline number, rarely. Cross-scorer comparison is treated
  as the top integrity risk (a lenient thinking-scorer once inflated a headline ~11pp).

> **In plain terms.** We decided the rules of the experiment before we ran it (so we couldn't move the goalposts),
> ran everything several times (so luck averages out), and used a paired test that asks "which specific answers changed"
> rather than "did the average nudge." Same grader every time — because changing graders is the easiest way to fool yourself.

---

## 4. The Noise Floor  *[v0 — headline]*

*Purpose: our single most important, most portable result. It gets its own section and a figure.*

Re-running one fixed configuration — no flags changed, temperature 0, byte-identical inputs — flips **11.4%** of
temporal-reasoning answers between runs. At n = 133 that implies a run-to-run noise band of roughly **±11 percentage
points**. The variation is not a subtle statistical artifact you have to squint for: across a k = 4 replication of the
unchanged baseline arm, per-run accuracy on the common item set (N = 125) ranged **57.6% → 62.4%, a 5.6pp spread with
nothing changed** (arm accuracies 0.576 / 0.568 / 0.624 / 0.584); the H-C1 arm's four runs spanned **8.8pp**. A single
number drawn from this distribution tells you where one roll landed, not where the system sits.

We decompose the variance into a **generator** component (T = 0 non-determinism under batched MoE inference) and a
**scorer** component (re-grading the same transcripts), and the split is stark. The **scorer is stable and validated**:
in a full six-arm re-judgment by two independent frontier judges (gpt-5.4 and Sonnet 5), the locked local scorer agrees
on **~92–97%** of verdicts (gpt-5.4 running marginally stricter, Sonnet 5 marginally more lenient). Re-grading identical
transcripts flips only **~0.8%** of verdicts. The **generator alone accounts for the 11.4% flip**. The noise is therefore not in the
measurement; it is in the model. Almost the entire ±11pp band is the generator deciding differently on byte-identical
inputs — which is exactly why a stronger or more expensive judge buys you no tighter a result. You cannot grade your way
out of generator variance. *[Scorer-agreement source: `GPT54-B4-AUDIT-RESULT.md` — three-judge panel (qwen / gpt-5.4 /
Sonnet 5), verified 2026-07-13. TODO: re-confirm the 0.8% scorer self-flip and the generator/scorer variance split from
primary artifacts — the earlier cited source docs were not located on disk; the 92–97% panel agreement is freshly
measured and replaces the previously-cited 92.5%.]*

The implication is the paper's title: for this system and benchmark, **any single-run delta smaller than ~11pp is not
evidence of anything.** The noise floor is not a nuisance to be footnoted; on a capacity-limited local model it is the
result that governs how every other result in this paper must be read.

> **In plain terms.** Ask the exact same system the exact same questions twice and about one answer in nine changes —
> just from the randomness inside the model. Run it four times and the score bounces around by ~5–9 points with nothing
> touched. So if someone's new memory trick "adds 5 points," shrug: that's inside the wobble you'd get by doing nothing.
> And a fancier grader won't save you — the grader already agrees with a frontier judge within two answers; the wobble
> is the model answering, not the grader marking.

**Figure 1** *[TODO — plot from the k-replicate arms in `benchmark-registry.jsonl`]*: the unchanged baseline scored k = 4
times, points spanning 57.6–62.4% (5.6pp), overlaid with the H-C1 arm's 8.8pp spread. Caption: "One system, unchanged,
same data — the score still moves ~6–9 points."

---

## 5. Levers Tested  *[v0 — core results table]*

*Purpose: the registry becomes the paper here. One row per hypothesis, with k, delta, the paired p-value, and a verdict.
Honesty is the selling point — we show the refutations.*

The baseline (H-C0) is **58.65%** at k = 1 (78/133) and **58.80%** as mean-of-k = 4 on the common item set (N = 125).
Each lever below is measured against it. **Denominator discipline:** k = 1 rows use the full n = 133; k = 4 mean-of-k
figures use the paired common set N = 125 present in all arms. McNemar p is the exact test on discordant item pairs.

| Lever | Mechanism | k | Δ | McNemar p | Verdict |
|---|---|---|---|---|---|
| **H-C1** `--temporal-prompt-aug` | structure-as-**output** (generator self-sorts events in prompt) | 4 | **+1.4pp** mean-of-k (+0.8pp majority-vote); k=1 was **+7.5pp** | **1.000** (k=1: 0.087) | **Refuted** — the +7.5pp was one high roll of an 8.8pp spread |
| **H-C2** `--chrono-sort` | deterministic pre-sort of retrieved events | 1 | +1.5pp (80/133) | 0.804 | Inside floor |
| **H-C3** `--temporal-window-recall` | date-windowed re-recall | 1 | +0.0pp (78/133) | 1.000 | No effect |
| **B4** `--chrono-ledger-inject` | structure-as-**input** (inject a pre-sorted event ledger) | 3 | negative under all 3 judges: **−7.8 / −6.2 / −3.1pp** (qwen / gpt-5.4 / Sonnet 5) | **0.031 / 0.115 / 0.503** | **Refuted — never improves; harm not robustly significant** (§6) |
| **D4** entity-resolution | deterministic composition / dedup for aggregation counts | 896-config sweep | best 7/8 configs; **0 reach 8/8** | n/a | Provable ceiling (§7) |
| **preference-enumerate** | prompt strategy (enumerate every stated preference) | 1 | **−20pp** (2/30) on local Qwen | *[n = 30 sub-slice]* | Reversal (§7) — helps a frontier model, hurts this one |

Two outcomes stand out. Every temporal lever that operates *inside* the ±11pp floor is noise — including H-C1, which
*looked* like it cleared the floor (+7.5pp at k = 1) but collapsed to +1.4pp with **McNemar p = 1.000** the moment we
ran it four times. And B4 — the one lever specifically motivated to *beat* the floor by handing the model structure
instead of asking it to build structure (§6) — came out **negative under all three judges** (−7.8 / −6.2 / −3.1pp for
qwen / gpt-5.4 / Sonnet 5): it never improves accuracy, though the harm is significant only under the strict local
scorer (p = 0.031; 0.115 and 0.503 under the frontier judges). So the honest headline of the table is two-part: **k ≥ 3
averaging is mandatory because single-run temporal deltas on this system are noise; and the one intervention we expected
to escape the noise failed to help under every judge we tried.**

> **In plain terms.** We tried a stack of reasonable ideas for making the agent reason about *time* better. Almost all
> did nothing once we accounted for the wobble — including the one (H-C1) that looked like a clear +7-point win until we
> ran it four times and it evaporated. The one idea big enough to show up above the noise (B4) turned out to *hurt* —
> handing the model a pre-sorted timeline made it measurably worse, not better.

---

## 6. Structure as Input vs Structure as Output  *[v0 — the honest negative]*

*Purpose: the intervention we expected to work. It didn't — it came out negative under a three-judge panel and never
improved accuracy (harm not robustly significant) — and that honest, panel-audited refutation is a stronger result than
the marginal positive we'd hoped for.*

H-C1 asked the generator to sort events itself in-context (structure-as-**output**) and failed. The format-tax
literature (Tam et al. 2024 [2408.02442]) shows models pay a measurable accuracy penalty when forced to do structural
work in-context; the "capacity, not format" refinement (Fan et al. 2026 [2606.09410]) sharpens this to a
*capacity-dependent* penalty — smaller / lower-capacity models degrade where larger ones are unaffected. A
low-active-parameter MoE (~3B active) sits squarely in the regime that pays. That predicts the better move is to hand
the model structure it doesn't have to build: **B4 injects a pre-sorted event ledger** (structure-as-**input**). H-C1's
failure is *consistent with* this prediction; B4 is the direct test.

**The prediction is refuted — and not by a null: B4 hurts.** Across k = 3, B4 scores **55.5%** against the baseline's
**63.3%** on the common set (N = 128), a **−7.8pp** degradation, and **all three B4 replicates fall below all three
baseline replicates** (no overlap). Handing this model a pre-sorted event ledger does not merely fail to help — it
makes it worse. Both faces of the intervention therefore fail on the low-capacity MoE: asking the model to build
structure in-context (H-C1) does nothing, and handing it structure as input (B4) degrades it.

**We hold this result to our own scorer-discipline standard (§3) with a three-judge panel, and it is instructive.** We
re-judged all six arms with two independent frontier judges (gpt-5.4 and Sonnet 5) in addition to the locked local
scorer, on the identical rubric and item set. The judges agree with the local scorer on **~92–97%** of verdicts, and the
*direction is identical under all three* — B4 is below baseline for every judge. But the **magnitude and significance
track judge strictness**:

| Judge | baseline | B4 | Δ | McNemar p |
|---|---|---|---|---|
| qwen (local, strict) | 63.3% | 55.5% | −7.8pp | **0.031** (significant) |
| gpt-5.4 | 60.9% | 54.7% | −6.2pp | 0.115 (n.s.) |
| Sonnet 5 (lenient) | 65.6% | 62.5% | −3.1pp | 0.503 (n.s.) |

Significance appears **only under the strictest scorer**; under both frontier judges it vanishes (and under Sonnet 5 the
gap is a mere −3.1pp, comfortably inside the ±11pp noise floor). We therefore report the honest, panel-robust claim:
**structure-as-input is refuted as an improvement — B4 never helps, is directionally negative under all three judges, but
its harm is not robustly significant, and the apparent "significant degradation" is an artifact of the strict local
scorer.** That this paper's *own* headline negative is "significant" under one scorer and clearly non-significant
(p = 0.50) under another is a live demonstration of the exact risk §3 warns about — and a far more defensible result than
cherry-picking p = 0.031.

The mechanism reading is unchanged either way: the format-tax framing predicted structure-as-input would rescue
structure-as-output; instead the extra ledger — more tokens, a rigid frame prepended to an already-sufficient context —
costs accuracy. On a ~3B-active generator the binding constraint is not *who does the structural work* but the
generator's capacity to reason over evidence it already holds (§8); adding structure on either side of that bottleneck
does not help, and adding it as input measurably hurts. This is, to our knowledge, the first published LongMemEval
temporal-reasoning result for Qwen3-Next-80B-A3B. *(B4 arms `b4-r{1,2,3}` vs `hc0new-r{1,2,3}`;
`LEDGER-ABLATION-RESULT.md`, `GPT54-B4-AUDIT-RESULT.md`.)*

> **In plain terms.** Making the model *sort the timeline itself* didn't help — that's extra work it's bad at. So we
> tried the opposite: hand it an already-sorted timeline for free. It got *worse* — meaningfully and repeatably. The
> problem was never who sorts the timeline; it's that this small model struggles to reason over the facts it already has,
> and piling a rigid pre-sorted ledger on top just gets in the way.

---

## 7. Negative Results and a Ceiling Proof  *[DRAFT]*

*Purpose: negative results, presented as contributions. A provable impossibility is stronger than an empirical miss.*

- **D4 — a provable ceiling on deterministic entity-resolution.** An 896-configuration sweep plus a two-way
  impossibility argument shows the composition is semantically bounded: no parameterization resolves it. *[TODO: state
  the impossibility cleanly.]*
- **Refuted prompt levers.** [Summarize H-C1/H-C2/H-C3 from §5 as a family.]
- **Prompting-reversal across scale.** `preference-enumerate` costs **−20pp** on the local Qwen (2/30 on the audited
  sub-slice) but was **+13pp** on a historical frontier model — a reversal we reproduce locally: a prompting trick that
  helps a big model *hurt* a small one. This is the mirror image of the "Prompting Inversion" reported by Khan 2025
  [2510.22251] — there a method helped a weaker model and backfired on a stronger one — and the two together make the
  general point cleanly: **prompt-effect sign is not scale-invariant; evaluating on the model you will actually deploy
  is not optional.** *(Our n = 30 sub-slice is directional, not powered — reported as a caution, not a headline.)*

> **In plain terms.** Some things we can *prove* won't work, not just failed to make work — that's a stronger statement.
> And a trick that helps GPT-4-class models actively hurt our smaller local model, which is a warning about copying
> recipes across model sizes.

---

## 8. Discussion  *[v0]*

*Purpose: what the field should take away. Two real claims, no overreach.*

**Retrieval is not the bottleneck on LongMemEval-S — generation is.** Across our runs the gold evidence is in the
generator's context on **97.2%** of items; the system is *holding* the right memory and still answering wrong. On this
benchmark, then, memory-system research that keeps optimizing recall is polishing the part that already works. The
temporal levers we tested (§5) all operate on *what gets retrieved or how it's ordered* — and all sit inside the noise
floor, which is consistent with retrieval being close to saturated. The open problem is the generator's ability to
*reason over* evidence it already has — and we can put a number on that gap. Given the **identical** retrieved context, a
frontier generator (GPT-5.6-sol) scores **75.0%** where the local ~3B-active MoE scores **50.0%** on the same items:
**+25pp** (n = 40 paired), more than double the ±11pp floor and the largest, cleanest effect in this study — same
memory, same items, only the generator swapped. **Caveat, stated plainly:** this is specific to LongMemEval-**S**, whose sessions
are short enough that recall is easy. LongMemEval-**M** would re-expose retrieval as a real variable, and we do not
claim the "retrieval is solved" result transfers to it.

**The noise floor is the result.** On a capacity-limited local model, characterizing measurement variance is not
housekeeping you do before the real experiment — it *is* the finding that determines which other findings are
admissible. Our ±11pp band retroactively invalidates any single-run comparison on this system, including several of our
own that looked promising. We think most of the field ships the single-run number the ±11pp band should have killed.
The concrete recommendation for anyone reporting memory-benchmark results on a local or capacity-limited generator: (1)
**report k ≥ 3 runs with a spread or error bar**, not a point estimate; (2) **use a paired test** (McNemar on discordant
items) rather than comparing two accuracy numbers; (3) **decompose your variance** into generator vs. scorer at least
once, so you know whether a better judge would even help (for us it would not); and (4) **tag every run with provenance**
so a delta can be traced to a configuration rather than a lucky seed.

> **In plain terms.** Two takeaways. First: on this benchmark the model is *already handed* the right memory almost every
> time and still gets the answer wrong — so the hard part isn't finding the memory, it's reasoning over it. Second: if
> you only run your benchmark once, your result might just be luck; run it a few times, show the spread, and use a test
> that survives the wobble.

---

## 9. Limitations & Future Work  *[v0]*

**Limitations.**
- **Single benchmark, and an easy slice of it.** We report LongMemEval-**S**, temporal-reasoning subset. The
  "retrieval-is-solved / generation-is-the-bottleneck" claim is S-specific; **LongMemEval-M re-exposes retrieval** and
  could move the bottleneck. We make no cross-benchmark generalization.
- **Single generator.** One local MoE (Qwen3-Next-80B-A3B). The ±11pp noise floor is a property of *this* model's
  batched T = 0 inference; the *magnitude* will differ on other families, though we expect the *phenomenon* (single-run
  deltas swamped by generator variance) to be general on capacity-limited models. Unverified across families.
- **One scorer, validated once.** The locked scorer agrees with a frontier judge within two answers on the set we
  audited, but that audit is a sample, not every run. We treat cross-scorer comparison as the top integrity risk and
  never compare scores across scorers — a discipline, not a proof of scorer invariance in all regimes.
- **Small paired-N on some levers.** The k = 1 rows use n = 133 and the preference-enumerate reversal used an n = 30
  sub-slice; those are directional, not powered. Only the k = 4 headline levers clear the pre-registered power bar.

**Future work.**
- **Cross-system extension (the v2 / public-leaderboard scope):** run this *identical* protocol across Letta, Mem0, Zep,
  Cognee, and engram, holding generator + scorer + gold fixed — an independent, non-vendor-owned memory benchmark where
  every published delta comes with a spread and a paired test.
- **LongMemEval-M** to re-introduce retrieval and test whether the generation-bottleneck finding is S-specific.
- **LongMemEval-v2 replication** to test external validity of the noise-floor and (pending B4) the structure-as-input
  finding.
- **Generator-variance mitigation:** whether self-consistency / majority-vote at inference time buys back enough of the
  ±11pp band to make single-config comparison viable, and at what compute cost.

---

## 10. Related Work  *[v0 — citations verified 2026-07-13]*

**Benchmark.** We evaluate on **LongMemEval** (Wu et al. 2024 [2410.10813]) and note its successor **LongMemEval-V2**
(Wu et al. 2026 [2605.12493]) as the external-validity target (§9); V2 re-introduces retrieval difficulty our S-subset
result deliberately brackets.

**Measurement variance.** Our noise-floor result extends **Song et al. 2024, *Evaluation of LLMs Should Not Ignore
Non-Determinism*** [2407.10457] from decoding variance in general to a *quantified, generator-vs-scorer-decomposed* band
on a local MoE, and motivates **self-consistency** (Wang et al. 2022 [2203.11171]) as a candidate mitigation (§9,
future work) rather than an assumed fix.

**Format tax / structure-as-input (§6).** The canonical result that structural in-context demands cost accuracy is
**Tam et al. 2024, *Let Me Speak Freely?*** [2408.02442]. **Fan et al. 2026, *Capacity, Not Format*** [2606.09410]
refines this to a *capacity-dependent* penalty — which is exactly why a ~3B-active MoE is the regime where
structure-as-output should fail and structure-as-input should help; we cite it as the nuancing view, not as a flat
format-tax confirmation.

**Prompting reversals (§7).** **Khan 2025, *The Prompting Inversion*** [2510.22251] documents a prompt method whose sign
flips with model strength (helping a weaker model, hurting a stronger one); our `preference-enumerate` reversal is the
mirror-direction instance (helping a frontier model, hurting a small local one). Both support the same claim: prompt
effects are not scale-invariant. *(Note [2510.22251] is a single-author GSM8K study — cited as corroborating direction,
not as strong independent evidence.)*

> **Citation-integrity note.** All seven references were verified against the live arXiv records on 2026-07-13 (title,
> authors, year, abstract). Two were re-placed after verification: [2606.09410] is a capacity-dependent *refinement* of
> format-tax, not a plain confirmation; [2510.22251] documents the *opposite-direction* prompting reversal from ours and
> is cited as a mirror, not as support for our direction.

---

## 11. Reproducibility  *[v0 — appendix]*

*Purpose: this is what a strong engineer-reviewer checks. The whole point of the paper is that someone else can re-run it.*

- **Master results table:** `benchmark-registry.jsonl` — one row per run, each tagged `{gold-version, scorer,
  feature-flags, system, run-id, correct/total}`. Every number in §4–§5 traces to a row here.
- **Locked scorer + stack:** `scorer-lock.json` pins the scorer tier and calibration; the gold set is a frozen,
  versioned snapshot (ZFS). Re-grading identical transcripts reproduces verdicts to within 0.8% (§4).
- **Analysis:** `kreplicate-analysis.py` computes the paired, common-N (N = 125) k-replicate statistics and the McNemar
  exact test on discordant pairs. Corruption-probe artifacts are included as evaluation-integrity evidence.
- **Hardware:** the full pipeline runs on a single self-hosted GPU (NVIDIA DGX Spark, "oblivion"); the FP8 generator
  fits in ~120 GB. No frontier API, no metered cost — slow-but-free is an explicit design choice, so the protocol is
  reproducible by anyone with one comparable box.
- *[TODO: wire a single `make reproduce` entry point that runs one lever end-to-end (ingest → atoms → run → score →
  registry row) so a reviewer reproduces a table row with one command.]*

---

### Data assets to migrate from `duramind/` (per reforge Track 05)
`benchmark-registry.jsonl`, `docs/campaign/*` (PAPER-OUTLINE, KREPLICATE-RESULT, COMBINING-AND-NOISE-ANALYSIS,
SYNTHESIS), `results/*/` adjudications, miss taxonomies, corruption-probe artifacts. These are the paper's evidence base;
they move into engram-go with the substrate fold.
