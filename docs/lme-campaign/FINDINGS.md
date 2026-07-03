# LongMemEval Campaign — Findings (what we have learned)

**Last updated:** 2026-06-04 · **Repo:** engram-go · **Variant:** LongMemEval-M (500q, golden corpus)
**Companion doc:** `RUNBOOK.md` (exact steps to reproduce / continue).

## 0. Goal
Improve engram's LongMemEval scores and produce numbers comparable to competing memory
systems. Two sub-goals: (a) an *honest* single-system baseline (not the inflated
Haiku→Opus ensemble), (b) test whether the flag-gated retrieval features already merged
on `main` actually help.

## 1. Score hygiene / what the headline numbers mean
- The old **412/500 (82.4%)** figure is `Haiku-correct ∪ Opus-rescued` on **LME-M**. It is
  NOT comparable to anyone's published number (those are **LME-S**, single-system). Do not
  quote it externally.
- Honest single-system Wave-0 baseline:
  - **Qwen3-judged:** 327/500 = **65.4%** strict.
  - **GPT-5.4-judged (re-judge):** 319/500 = **63.8%** strict, 341/500 = **68.2%** lenient.
  - The two judges agree within ~2pp per type ⇒ **the 65.4% baseline is sound, not a judge
    artifact.**

## 2. Judge constraints (IMPORTANT — drove the whole back-half)
- **NO API keys.** Everything runs on subscriptions: generation via `claude --print`
  (Claude Code sub), GPT judging via `codex exec` (ChatGPT/Codex sub). Attempts to call
  OpenAI `/v1` directly (incl. the `AGENTGATEWAY_OPENAI_API_KEY` gateway) are dead ends.
- **gpt-4o is NOT selectable** on the Codex subscription — only **gpt-5.4** (effort `medium`).
  So competitor comparison (they judged with gpt-4o) is **indicative, not exact**. gpt-5.4
  is newer/stronger, so if anything it judges more precisely.
- The judge is **Codex itself** reasoning over each row (NOT Codex calling an endpoint).

## 3. Wave 0 per-type (GPT-5.4 judge, flags OFF) — the baseline
| type | strict | lenient | n |
|---|---:|---:|---:|
| single-session-assistant | 85.7% | 87.5% | 56 |
| knowledge-update | 75.6% | 79.5% | 78 |
| single-session-user | 68.6% | 72.9% | 70 |
| temporal-reasoning | 66.9% | 67.7% | 133 |
| multi-session | 54.1% | 59.4% | 133 |
| single-session-preference | 10.0% | 33.3% | 30 |
| **overall** | **63.8%** | **68.2%** | 500 |

**Weak types = multi-session (54%) and single-session-preference (10%).**
single-session-preference being ~10% under BOTH judges proves it is a **real
retrieval/extraction gap, not a grading artifact.**

## 4. Do the flag-gated retrieval features help? NO. (central result)
Two features were tested: **session-DCG** (LEVER-8, `ENGRAM_SESSION_NDCG_AGG`) and
**preference-MMR** (H-NEW-2, `ENGRAM_PREFERENCE_MMR`). All runs reuse the same corpus;
only the flags + reader change. Judge held constant (gpt-5.4).

- **Wave 1 (both flags ON, Haiku reader):** 306/500 = **61.2%** strict vs Wave-0 63.8% ⇒
  **−2.6pp**. Both target hypotheses refuted: multi-session −0.7pp (flat), preference −3.3pp.
- **Single-flag ablations (Sonnet reader):**
  | type | session-DCG | preference-MMR |
  |---|---:|---:|
  | multi-session *(DCG target)* | 50.4% / 68.4% | 47.4% / 53.4% |
  | single-session-preference *(MMR target)* | 10.0% / 70.0% | 6.7% / 66.7% |
  | overall strict / lenient | **60.6% / 71.0%** | **59.4% / 67.6%** |
  - **Neither flag lifts its own target type above the noise floor.** preference-MMR is even
    *lower* on preference than the DCG arm.
  - DCG ≈ MMR overall (within noise). Non-target swings (single-session-assistant,
    single-session-user, ±7-8pp) are **generation noise**, not signal.

**Conclusion: session-DCG and preference-MMR, as built, do NOT improve LME. Do not ship
them. The lever for single-session-preference is almost certainly ingest-time atomic
preference extraction, not retrieval-time re-ranking.**

## 5. Two methodology traps discovered (must respect going forward)
1. **Generation noise ≈ ±7pp/type, ±2-3pp overall.** Haiku/Sonnet regenerate different
   answers each run; types the flags don't touch swung that much between waves. **Single
   runs are noise-dominated** — a real flag effect must exceed this or be confirmed by
   replication.
2. **Reader × judge partial-label artifact.** Sonnet (stronger reader) scored *lower*
   strict (60.6/59.4%) than Haiku (63.8%) but *higher* lenient — because gpt-5.4 labels
   Sonnet's wordier answers PARTIALLY_CORRECT far more often (50ish partials vs ~22 for
   Haiku). **Strict comparisons across readers are not apples-to-apples.** A Sonnet
   flags-OFF baseline (NOT yet run) is needed to anchor the Sonnet arms.

## 6. Deployment reality
- Live `engram-go` (ns `engram`, 3 replicas, `:latest` = version **3aabf50**) **already
  contains the flag code** (`/engram --help` shows `-session-ndcg-agg`). No rebuild needed
  to toggle flags — just env vars + rolling restart.
- **CI does not build/deploy images** — deploy is manual; image drift is the default.
- Flags are **server-side config** (not per-request), so testing them means changing the
  live deployment's env (reversible; auto-revert after each run).

## 7. Infra / process learnings (filed)
- **Codex poller poison-pill bug** — one stale worktree (`set -euo pipefail` + `return 1`)
  crashes the whole `codex-poll.service` every tick, blocking the entire queue. Filed:
  **petersimmons1972/claude-codex#19** (fix: `continue` not `return 1`). Workaround: remove
  the stale worktree + `systemctl --user reset-failed codex-poll.service`.
- **git merge-base diff trap** — `git diff origin/main...<branch>` errors silently
  ("no merge base", empty output) on this repo's two disjoint histories; empty output
  mis-reads as "merged". Classify by PR/issue state instead. (Saved to memory.)
- **Branch cleanup done:** 47 local + 65 remote merged/superseded non-codex branches
  deleted (SHAs logged, reflog-recoverable). codex-owned branches left to claude-codex#1029.

## 8. Current state (as of 2026-06-04)
- Prod engram: **flags OFF (clean baseline), 3/3 healthy**.
- PR **#1031** (Codex judging harness) rebased on green main; CODEX REPORT with Wave-0
  gpt-5.4 table posted (comment 4617913429). Still draft.
- **Pending decision:** run the **Sonnet flags-OFF baseline** to anchor the Sonnet arms /
  settle the partial-inflation artifact.

## 9. Where the data lives
- Generation runs (main checkout): `results/wave0-rerun-haiku-20260603`,
  `results/wave1-mmr-dcg-20260603`, `results/wave1a-dcg-sonnet-20260603`,
  `results/wave1b-mmr-sonnet-20260603` (each: `checkpoint-run.jsonl`, `RUN_STATUS.json`).
- Judge outputs (in worktree `.codex-poll-worktrees/engram-go-issue-1030-lme-judge-harness/results/`):
  `wave0-gpt54-judge`, `wave1-gpt54-judge`, `wave1a-dcg-judge`, `wave1b-mmr-judge`
  (each: `checkpoint-score.jsonl`, `score_report.json`, `bundle.jsonl`).
- Original campaign plan: `~/.claude/plans/what-lessons-can-be-distributed-rossum.md`.
