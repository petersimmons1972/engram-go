# Next LME Campaign Runbook — H8–H15 (2026-05-19)

**Generated**: 2026-05-19 22:52 UTC  
**Branch**: `lme-h8h12h15-2026-05-19` (commits `a439a50`, `eda7ce1`, `b860213`)  
**Baseline**: v9 failure taxonomy (135 items), Exp 10 (Sonnet at topK=100)  
**Source docs**: `next-campaign-hypotheses.md`, `in-context-all-failure-mechanics.md`, `v9-failure-taxonomy.json`

---

## Pre-Flight Checklist

- [ ] Engram service healthy: `curl -s http://localhost:8788/health | jq .status`
- [ ] Oblivion model loaded: `ollama list | grep qwen3-32b`
- [ ] Free disk: `df -h /home/psimmons/projects/engram-go/ | tail -1` must show >50GB available
- [ ] Branch verified: `git branch` shows `lme-h8h12h15-2026-05-19`
- [ ] No uncommitted changes: `git status --short` is empty
- [ ] Run `go test ./... -count=1 -race -timeout 10m` (expected: 0 failures)

---

## Experiment Matrix: Planned

| Exp# | ID | Hypothesis | Primary lever | Item subset | N items | Expected wall time | Flags |
|------|----|-----------|---------------|-------------|---------|-------------------|-------|
| 14 | H15 | Dual-query preference recall | Retrieval: subject-anchor NP union with generic preference query | h15-preference | 29 | ~35–40 min | `--dual-preference-recall` |
| 15 | H8 | Exhaustive aggregation recall | Retrieval: topK=500 sweep on object NP, merged with topK=100 baseline | h8-aggregation | 26 | ~30–35 min | `--exhaustive-aggregation` |
| 16 | H8+H12 | Stacked retrieval + prompt | H8 retrieval fix + enumerate-first generation instruction | h812-combo | 26 | ~30–35 min | `--exhaustive-aggregation --enumerate-first` |
| 17 | Stack (H15+H8+H12) | Full-stack candidate pool + generation | All three hypotheses combined | stacked-all | 135 | ~75–85 min | `--dual-preference-recall --exhaustive-aggregation --enumerate-first` |

---

## Post-Campaign Outcomes (2026-05-20)

The 2026-05-19/20 campaign executed Exp 13, 13s, 14, 15, 16. Results below override the speculative predictions above for those rows.

| # | Result | Action |
|:---|:---|:---|
| Exp 13 (Qwen + H6+H7, 135 items) | 28/128 CORRECT (21.9%) — H6+H7 zero uplift vs Exp 10 baseline. | H6/H7 NOT promoted to defaults. Env-var overrides remain available. |
| Exp 13s (Sonnet + H6+H7, 30 items) | 6/30 (20%) — identical to Exp 10 baseline. Confirms Exp 13 verdict. | Same. |
| Exp 14 (H-M5 + H-M1 prompts, 31 temporal residuals) | 5/31 (16%) vs predicted 20/31 (65%). **FALSIFIED.** | Drop H-M5/H-M1 from candidates. `--temporal-prompt-aug` flag retained as opt-in. |
| Exp 15 (H17 paraphrased BM25 union, 24 incidental) | 5/24 CORRECT (21%); **gold session in retrieved set: 24/24 (100%) vs 0% baseline. Retrieval objective fully achieved.** | **Promote `--query-paraphrase-passes 3` as new baseline default for next campaign.** Generation now the bottleneck for residual failures. |
| Exp 16 (question_date injection, 17 cluster B) | 3/17 — identical to baseline. **FALSIFIED.** Autopsy: Sonnet date arithmetic is correct; downstream entity disambiguation is the real bottleneck. | `--inject-question-date` flag retained as opt-in but not default. |

### New top candidates (post-campaign)

1. **Date-windowed re-recall** (retrieval-side, P1) — addresses M1 entity disambiguation surfaced by Exp 16's autopsy
2. **`--query-paraphrase-passes 3` as default** — Exp 15 retrieval primitive confirmed
3. **Structural chrono-sort** — sort by `valid_from` before prompt assembly, not via prepended instruction
4. **Two-pass pipeline** — resolve date → re-recall with date filter → re-generate

### Decision Tree status

The decision tree below is **historical** — it was written pre-results. The executed branch was "Exp 13 ≥ 8 gains". Subsequent campaigns should write their own tree, not extend this one.

---

## Experiment Execution Plan

### Decision Tree (Based on Exp 13 Result)

**If Exp 13 (H6/H7 in-flight) achieved ≥8 CORRECT gain on temporal-reasoning failures:**
1. Run Exp 14 (H15) first — orthogonal to H6/H7, high-EV standalone (10–16 item recovery expected)
2. Run Exp 15 (H8) second — orthogonal, well-characterized failure class
3. Optionally run Exp 16 (H8+H12) if H8 alone resolves ≥12 items; skip if H8 < 8 items (generation fix less likely to help)
4. **Skip Exp 17** (stacked) if two of H15/H8 have achieved target thresholds

**If Exp 13 achieved <8 CORRECT gain or results still pending:**
1. Run Exp 14 (H15) — highest EV hypothesis per ranking
2. Run Exp 15 (H8) — second-highest EV
3. Run Exp 16 (H8+H12) as a two-factor decomposition to isolate retrieval vs. generation
4. Run Exp 17 only if individual experiments show subadditive gains (synergy testing)

**Stopping rule**: If any single experiment recovers ≥20 items on its cohort, the hypothesis is confirmed sufficient; candidates for production are flagged immediately.

---

## Command Reference

### Exp 14: H15 — Dual-Query Preference Recall

**Test command (manual verification before full run):**
```bash
cd /home/psimmons/projects/engram-go

# Dry run on 2 items
bin/longmemeval run \
  --test-items testdata/longmemeval/next-campaign-h15-preference.json \
  --sample-size 2 \
  --dual-preference-recall \
  > results/exp-14-h15-dry-run.jsonl

# Verify output format
jq . results/exp-14-h15-dry-run.jsonl | head -50
```

**Full run:**
```bash
bin/longmemeval run \
  --test-items testdata/longmemeval/next-campaign-h15-preference.json \
  --dual-preference-recall \
  --output results/exp-14-h15-predictions.jsonl \
  --timeout 2400s

# Score with Sonnet (preference class requires stronger model per Exp 09)
bin/longmemeval score-efficient \
  --predictions results/exp-14-h15-predictions.jsonl \
  --test-items testdata/longmemeval/next-campaign-h15-preference.json \
  --model-name sonnet \
  --output results/exp-14-h15-scores.jsonl

# Tally results
jq -s 'map(select(.label == "CORRECT")) | length' results/exp-14-h15-scores.jsonl
# Expected: ≥6 items (falsification threshold), target 10–16
```

**Pre-registered falsification criteria:**
- Fewer than 6 CORRECT items recovered → falsify H15
- Zero improvement over Exp 10 baseline (3.3%) → falsify H15

---

### Exp 15: H8 — Exhaustive Aggregation Recall

**Full run:**
```bash
bin/longmemeval run \
  --test-items testdata/longmemeval/next-campaign-h8-aggregation.json \
  --exhaustive-aggregation \
  --output results/exp-15-h8-predictions.jsonl \
  --timeout 2400s

bin/longmemeval score-efficient \
  --predictions results/exp-15-h8-predictions.jsonl \
  --test-items testdata/longmemeval/next-campaign-h8-aggregation.json \
  --model-name haiku \
  --output results/exp-15-h8-scores.jsonl

# Tally
jq -s 'map(select(.label == "CORRECT")) | length' results/exp-15-h8-scores.jsonl
# Expected: ≥8 items, target 8–14
```

**Pre-registered falsification criteria:**
- Fewer than 5 CORRECT items recovered → falsify H8
- INCORRECT rate on aggregation items rises by ≥8 (noise from larger context) → falsify H8

---

### Exp 16: H8 + H12 — Stacked Retrieval + Prompt

**Full run (decomposition test):**
```bash
bin/longmemeval run \
  --test-items testdata/longmemeval/next-campaign-h812-combo.json \
  --exhaustive-aggregation \
  --enumerate-first \
  --output results/exp-16-h812-predictions.jsonl \
  --timeout 2400s

bin/longmemeval score-efficient \
  --predictions results/exp-16-h812-predictions.jsonl \
  --test-items testdata/longmemeval/next-campaign-h812-combo.json \
  --model-name haiku \
  --output results/exp-16-h812-scores.jsonl

jq -s 'map(select(.label == "CORRECT")) | length' results/exp-16-h812-scores.jsonl
# Expected: ≥12 items (H8 baseline + H12 additive gain ≥4)
```

**Comparison (isolate H12 contribution):**
```bash
# Extract H8-only CORRECT count from Exp 15
h8_baseline=$(jq -s 'map(select(.label == "CORRECT")) | length' results/exp-15-h8-scores.jsonl)

# Extract H8+H12 CORRECT count
h812_combined=$(jq -s 'map(select(.label == "CORRECT")) | length' results/exp-16-h812-scores.jsonl)

# Compute H12 additive effect
echo "H8 alone: $h8_baseline, H8+H12: $h812_combined, H12 additive: $((h812_combined - h8_baseline))"
```

**Pre-registered falsification criteria:**
- H12 additive gain < 4 items → falsify H12 (generation fix not binding constraint)
- H8+H12 combined < 12 items → both hypotheses insufficient for this class

---

### Exp 17: Full Stack (H15+H8+H12)

**Run only after Exp 14–16 show independent promise. This is a synergy test.**

```bash
bin/longmemeval run \
  --test-items testdata/longmemeval/next-campaign-stacked-all.json \
  --dual-preference-recall \
  --exhaustive-aggregation \
  --enumerate-first \
  --output results/exp-17-stack-predictions.jsonl \
  --timeout 5400s

bin/longmemeval score-efficient \
  --predictions results/exp-17-stack-predictions.jsonl \
  --test-items testdata/longmemeval/next-campaign-stacked-all.json \
  --model-name haiku \
  --output results/exp-17-stack-scores.jsonl

# Overall improvement
baseline_correct=$(jq -s 'map(select(.label == "CORRECT")) | length' results/exp-09-baseline-scores.jsonl)
stacked_correct=$(jq -s 'map(select(.label == "CORRECT")) | length' results/exp-17-stack-scores.jsonl)

echo "Exp 09 baseline (Sonnet, topK=100): $baseline_correct CORRECT"
echo "Exp 17 stack (all flags): $stacked_correct CORRECT"
echo "Net improvement: $((stacked_correct - baseline_correct))"
# Target: ≥10–20 net CORRECT gain across 135 items (7–15% absolute improvement)
```

---

## Monitoring & Failure Recovery

### Latency Check (During Dry Runs)

For H15 (dual-query) and H8 (exhaustive sweep), measure wall time per-item:

```bash
# After dry run
jq -s 'map(.duration_ms) | {mean: (add/length), max: max, min: min}' results/exp-14-h15-dry-run.jsonl

# If mean > 60s per item, latency is concerning; investigate HNSW ef_search tuning or embedding cache misses
```

### Engram Service Recovery

If the Engram API becomes unavailable during a run:

```bash
# Check service health
curl -v http://localhost:8788/health

# If 503: restart the service (graceful shutdown + start)
# Logs live in ~/.claude/daemon/engram.log

# Resume from checkpoint (if run supports --resume-from-checkpoint)
# Otherwise re-run with --skip-completed-items to avoid re-scoring
```

### Oblivion Scorer Crashes

If scoring hangs or crashes (GPU OOM, thermal throttle):

```bash
# Kill stuck scoring process
pkill -f "ollama serve" || true

# Restart Ollama
ollama serve > ~/.claude/daemon/oblivion.log 2>&1 &

# Resume scoring from the last successful batch
# (Scorer should implement checkpoint recovery)
```

---

## Expected Results by Hypothesis

| Hypothesis | Target class | Baseline CORRECT (Exp 10) | Expected recovery | Total expected | Confidence |
|-----------|--------------|---------------------------|-------------------|--------------------|-----------|
| **H15** | in_context_all / preference | 1/29 (3.3%) | 10–16 items | 11–17 CORRECT (38–59%) | High |
| **H8** | aggregation_failure | 3/26 (11.5%) | 8–14 items | 11–17 CORRECT (42–65%) | High |
| **H12** | aggregation (stacked w/ H8) | N/A | +4–12 items (additive) | 15–29 CORRECT (58–100%) | Medium |
| **H9** | wrong_rank (pool expansion) | N/A | 5–8 items | N/A (deferred) | High |
| **H10** | wrong_rank / temporal | N/A | 6–9 items | N/A (deferred) | High |

**Combined ceiling (H15+H8+H12)**: ~20–40 items recovered from 72 failures in the three target classes (in_context_all + aggregation_failure combined), a net +10–20 CORRECT-rate points on the 135-item corpus.

---

## Post-Campaign Steps

1. **After Exp 14–16 complete (est. 100 min):**
   - Compare per-hypothesis CORRECT rates
   - Measure class-level recovery: preference vs. aggregation vs. stacked
   - File summary analysis: `/home/psimmons/projects/engram-go/results/exp-14-16-analysis.md`

2. **Exp 17 decision gate (only if promised):**
   - If H15 alone ≥15 items AND H8 alone ≥12 items → run Exp 17 (expect synergy >20 items total)
   - If either H15 or H8 < 8 items → skip Exp 17 (subadditive return); focus on the next-ranked hypotheses (H9/H10)

3. **Hypothesis confirmation:**
   - H15 confirmed: implement in `cmd/longmemeval/run.go`, feature-flag it, test on live haystack
   - H8 confirmed: implement exhaustive sweep, test on full 500-item corpus before production rollout
   - H12 confirmed: bake into `GenerationPrompt()`, validate on real-world aggregation patterns

4. **Next campaign (deferred):**
   - H9 (vector candidate pool expansion topK×3→topK×5)
   - H10 (temporal recency weight 0.30→0.40)
   - H11 (precision weight null hypothesis)
   - H13/H14 (lower-confidence candidates, test only if budget permits)

---

## Item Subset File Reference

All files live in `/home/psimmons/projects/engram-go/testdata/longmemeval/`:

- **next-campaign-h8-aggregation.json** — 26 items (missing_recall class, H8 target)
- **next-campaign-h15-preference.json** — 29 items (in_context_all + single-session-preference, H15 target)
- **next-campaign-h812-combo.json** — 26 items (same as h8, used for H8+H12 factorial decomposition)
- **next-campaign-stacked-all.json** — 135 items (full v9 failure set, H15+H8+H12 combined test)

**Validation:**
- All files are valid JSON arrays of LME test objects
- Items verified by ID cross-reference with `v9-failure-taxonomy.json`
- Two spot-checks per file completed (first and last item)
- No missing IDs: 26 + 29 + 135 = all accounted for

---

## Branch & Commit Info

- **Branch**: `lme-h8h12h15-2026-05-19`
- **Base**: `main` (commit hash TBD, verified clean)
- **Feature commits**:
  - `a439a50`: Add `--dual-preference-recall` flag + subject-anchor NP extraction
  - `eda7ce1`: Add `--exhaustive-aggregation` flag + topK=500 sweep logic
  - `b860213`: Add `--enumerate-first` flag + conditional prompt injection

**Verification before run:**
```bash
git log --oneline lme-h8h12h15-2026-05-19 | head -10
git diff main..lme-h8h12h15-2026-05-19 --stat | tail -5
```

---

**Generated for**: Next session (2026-05-20 or later)  
**Estimated campaign runtime**: 3–4 hours (including scoring + analysis)  
**Approver**: (mark off when ready to dispatch)
