# Temporal-Reasoning Failure Class: Mechanism Analysis

**Date:** 2026-05-19
**Source experiments:** v9 (Qwen baseline), camp-09 (Sonnet), camp-10 (Sonnet+H1+H2)
**Scope:** All 45 temporal-reasoning items from `v9-failure-taxonomy.json`

---

## 1. Retrieval Check: The Null Hypothesis Is Killed

Every one of the 45 items had **identical retrieved_ids** in Qwen v9 and camp-10 (100 retrieved memories each, 1.00 overlap). Recovery differences are **entirely generative**, not retrieval. H1+H2 did not change what sessions were surfaced.

---

## 2. Cluster Table

Five clusters, defined by temporal arithmetic style at the question level:

| Cluster | Type | Items | Recovered | Recovery Rate |
|---------|------|------:|----------:|--------------:|
| **A** | Date arithmetic (subtraction/duration) | 16 | 10 | **63%** |
| **B** | Relative anchor ("N days/weeks ago", day-of-week) | 17 | 3 | **18%** |
| **C** | Event ordering / sequencing | 10 | 1 | **10%** |
| **D** | Boundary inclusion ("past 3 months") | 0 | 0 | — |
| **E** | Implicit preference/routine | 2 | 0 | **0%** |
| **Total** | | **45** | **14** | **31%** |

**Conclusion:** Sonnet's 31% recovery is almost entirely Cluster A. Clusters B, C, E are structurally resistant to the H1+H2 intervention.

---

## 3. Mechanism Taxonomy of the 31 Still-Incorrect Items

Each still-failing item was hand-classified into one of five root mechanisms:

| Mechanism | Count | Description |
|-----------|------:|-------------|
| **M1** | 11 | Date arithmetic correct, wrong event/entity at target session |
| **M2** | 6 | Gold session genuinely absent from retrieved context |
| **M4** | 3 | Arithmetic error — wrong anchor date or off-by-N counting |
| **M5** | 9 | Multi-session ranking/ordering failure |
| **M6** | 2 | Implicit preference — no temporal anchor in retrieved content |

---

## 4. Five Representative Item Walkthroughs

### Walkthrough 1 — Cluster A, M1 (RECOVERED): `982b5123`
**Q:** "How many months ago did I book the Airbnb in San Francisco?"  
**Gold:** Five months ago | **Question date:** 2023/05/23  
**Camp-10 (CORRECT):** Correctly found session dated 2022/12/23, computed 5 months back, answered "five months ago."  
**Qwen (INCORRECT):** Answered "three months in advance" — misread the question as forward-looking rather than backward-looking.  
**Mechanism:** Sonnet applied date subtraction correctly AND understood the question's direction. Qwen's generation failed on the relative direction of "ago."

---

### Walkthrough 2 — Cluster B, M1 (STILL FAIL): `gpt4_e414231f`
**Q:** "Which bike did I fix or service the past weekend?"  
**Gold:** road bike | **Question date:** 2023/03/21 (Tue)  
**Camp-10 (INCORRECT):** Computed "past weekend" = 2023/03/15 (Wed), found a session containing bike servicing, but extracted **mountain bike** (flat tire repair) instead of the gold **road bike**.  
**Session evidence:** Two bike events exist near that date — the model surfaced the mountain bike session and missed the road bike session on the correct weekend date.  
**Mechanism:** Date arithmetic is correct; the failure is entity disambiguation within a dense session cluster. Two bike events close in time, model picks the more salient (detailed flat tire narrative) over the correct one.

---

### Walkthrough 3 — Cluster B, M1 (STILL FAIL): `gpt4_468eb064`
**Q:** "Who did I meet with during the lunch last Tuesday?" | Date: 2023/04/18 (Tue)  
**Gold:** Emma  
**Camp-10 (INCORRECT):** "No information about a lunch meeting last Tuesday."  
**Sister question `gpt4_468eb063` (CORRECT):** "How many days ago did I meet Emma?" (from 2023/04/20) → correctly answered "9 days ago."  
**Same gold session:** `answer_9b09d95b_1` used for both. Session IS in context.  
**Mechanism:** Day-of-week anchor ("last Tuesday") fails while N-days anchor ("9 days ago") succeeds — even on identical context. The model resolves N-days arithmetic reliably but day-of-week backwards projection is fragile.

---

### Walkthrough 4 — Cluster C, M5 (STILL FAIL): `gpt4_f420262c`
**Q:** "What is the order of airlines I flew with from earliest to latest before today?"  
**Gold:** JetBlue, Delta, United, American Airlines | **Question date:** 2023/03/02  
**Camp-10 (INCORRECT):** Retrieved flights but returned American Airlines first instead of JetBlue.  
**Root cause:** All four airline sessions are present in the 100 retrieved memories, but the retrieval order presents sessions by relevance, not by chronological order. Sonnet synthesizes from this ranked (not chronological) list and reconstructs the wrong sequence.  
**Mechanism:** M5 — the model must impose a full global sort across n=4+ sessions. Without a chain-of-thought that explicitly lists all sessions by date and re-sorts, the generation follows retrieval rank not session date.

---

### Walkthrough 5 — Cluster A, M2 (STILL FAIL): `af082822`
**Q:** "How many weeks ago did I attend the friends and family sale at Nordstrom?"  
**Gold:** 2 weeks | **Question date:** 2022/12/01  
**Camp-10 (INCORRECT):** "The memory blocks provided do not contain any mention of attending a Nordstrom friends and family sale."  
**Qwen (INCORRECT):** Same — total context gap.  
**Mechanism:** M2 — pure retrieval failure. Neither model can answer because the Nordstrom event session was not surfaced in 100 retrieved memories. The event may be at a cosine-distance threshold that BM25+embedding retrieval misses. No generation intervention can fix this.

---

## 5. Targeted Hypothesis for the 31 Still-Incorrect Items

### Why did they fail despite H1+H2?

**H1** and **H2** appear to have improved **date arithmetic reasoning** (Cluster A: 63% recovery) — Sonnet uses the question date as an explicit anchor and subtracts durations. The H hints likely encourage the model to show its date math explicitly.

The 31 remaining items fail for three distinct reasons that H1+H2 do not address:

---

### Hypothesis H-M1: Session-level entity disambiguation (11 items)
**Claim:** When a date range contains multiple events of the same semantic category (two sports events, two bike sessions, multiple social media activities), Sonnet picks the most lexically salient one rather than the temporally precise one.

**Evidence:** `gpt4_e414231f` (mountain vs road bike), `9a707b82` (chicken fajitas vs chocolate cake), `gpt4_e061b84g` (volleyball vs soccer on same date), `gpt4_1e4a8aec` (herb harvesting vs tomato planting).

**Intervention:** A post-retrieval prompt step: "List ALL events of category X found at the target date range. Select the one that best matches the question's qualifier." This is a **within-session disambiguation pass** — add a step in the hypothesis chain that enumerates candidates before committing.

**LOC estimate:** ~30 lines in the prompt construction path. Files: `runner/` prompt template and any hypothesis builder. Falsifiable: M1 items should move from INCORRECT to CORRECT; items in other mechanisms should be unchanged.

---

### Hypothesis H-M5: Chronological sorting pass (9 items)
**Claim:** All n events exist in the retrieved context but are presented in retrieval-relevance order. Sonnet reconstructs sequence from this ranked list, producing wrong orderings.

**Evidence:** `gpt4_f420262c` (airlines), `gpt4_7abb270c` (6 museums), `gpt4_e061b84f` (3 sports events), `gpt4_2d58bcd6` (2 books), `gpt4_7f6b06d` (3 trips). Pattern: ordering questions with n≥2 items all fail.

**Intervention:** For question types detected as ordering questions (keywords: "order", "earliest to latest", "which first"), prepend a sorting instruction: "First, extract all instances of X from the memory blocks with their session dates. Sort by date ascending. Then answer." This is a **chain-of-thought forcing step** for chronological sort.

**LOC estimate:** ~20 lines — question-type detection + conditional prompt injection. Falsifiable: re-run on the 9 M5 items; if ≥6 move to CORRECT, hypothesis confirmed.

---

### Hypothesis H-M2: Retrieval gap — temporal keyword indexing (6 items)
**Claim:** 6 items have no relevant session in the top-100 retrieved memories. The missing events are low-frequency entities (Nordstrom sale, MoMA visit, smoker purchase, contract signing) that don't have strong semantic overlap with the question.

**Evidence:** `af082822` (Nordstrom), `gpt4_59149c77` (MoMA), `gpt4_8279ba03` (smoker), `eac54add` (contract), `gpt4_93159ced` (prior employment), `gpt4_d6585ce9` (music event).

**Intervention:** For M2 items, the only fix is retrieval-side: expand k (top-150), add date-range filtered retrieval (query within ±7d of computed anchor date), or add a second-pass retrieval after date arithmetic resolves the target window.

**LOC estimate:** ~50 lines in retrieval layer. Falsifiable: check whether gold session appears in top-150 retrieved for these 6 items.

---

### H-M4 and H-M6 (3+2 items)
- **M4 (arithmetic errors):** 3 items. Two of these have anchor identification errors — the model reads the wrong session as the anchor event. Fix: explicit anchor verification step ("Verify the anchor event date before computing the delta"). Low-value to implement — only 3 items.
- **M6 (implicit preferences):** 2 items. These questions have no temporal anchor in the haystack. `gpt4_2c50253f` (wake time) and `gpt4_2f56ae70` (streaming service) require inferring from implicit state. These are **not fixable with current context window** — would require a structured preference store.

---

## 6. Implementation Priority Matrix

| Mechanism | Items | Feasibility | Confidence | Priority |
|-----------|------:|------------|-----------|---------|
| H-M5: Chron-sort forcing | 9 | High — prompt-only | High | **P1** |
| H-M1: Entity disambiguation pass | 11 | Medium — extra prompt step | Medium | **P2** |
| H-M2: Retrieval expansion | 6 | Medium — retrieval change | Medium | **P3** |
| H-M4: Anchor verification | 3 | Low — small N | Low | P4 |
| H-M6: Preference store | 2 | Low — structural | Low | Defer |

---

## 7. Key Finding

**Recovery was NOT due to better retrieval** (retrieval was identical). Sonnet recovered Cluster A (arithmetic) because it applies explicit date subtraction more reliably than Qwen. The 31 remaining items are structurally different: M1 requires entity disambiguation within a retrieved session, M5 requires global chronological sorting across sessions, M2 requires retrieval expansion. No single intervention covers all three — they need separate hypothesis engineering steps.

The highest-yield next experiment: inject a **mandatory chronological sort step** for ordering questions (H-M5) + an **entity enumeration step** for "N ago" questions with ambiguous session content (H-M1). Together these cover 20 of the 31 remaining items (65%).

---

## 8. Post-Exp-14/16 Correction (2026-05-20)

### Cluster B mechanism — corrected

Section 5 named the cluster B failures as relative-time arithmetic problems and proposed H16 (question_date injection) as the targeted fix. Exp 16 falsified that mechanism. On the 17-item cluster B subset, H16 produced 3/17 CORRECT — identical to baseline. Spot-checking the still-INCORRECT items under H16 shows Sonnet now performs the date arithmetic correctly (e.g., "Today: 2022/04/15. In the 2022/03/21 session, 'yesterday' = 2022/03/20"). The remaining failure is **downstream entity disambiguation at the resolved date**, not arithmetic.

Updated M1 root cause: *date math is correct; the wrong session/entity is selected near the computed date.*

### H-M5 and H-M1 (Exp 14) — falsified

Exp 14 implemented H-M5 (chrono-sort forcing) + H-M1 (entity-enumeration pass) as prepended natural-language instructions to the Sonnet prompt. Prediction: 20/31 (65%). Result: 5/31 (16%). M1 and M5 named walkthroughs each went 0/5. Both hypotheses are falsified in their prompt-only form. The mechanism taxonomy in Sections 3–4 still stands; the **intervention surface was too weak** (a prepended instruction cannot reliably force the model to enumerate and chronologically order events). Subsequent work should test structural interventions: pre-computed chronological event lists injected into context, multi-step (enumerate → commit) generation pipelines, or retrieval-side date-windowed re-recall.

### Priority Matrix update

Section 6 listed H-M5 as P1 and H-M1 as P2. Both should now read FALSIFIED. New top candidates for cluster B / M1 / M5:

1. Retrieval-side **date-windowed re-recall** — resolve the question date first, then re-query memories filtered to a ±N-day window. Addresses M1 entity-disambiguation directly.
2. **Structural chrono-sort** — sort retrieved memories by `valid_from` before prompt assembly (not via instruction).
3. **Two-pass pipeline** — resolve date → re-recall with date filter → re-generate.
