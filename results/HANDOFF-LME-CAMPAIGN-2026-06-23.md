# LongMemEval Campaign — Session Handoff (2026-06-23)

**Purpose:** Pick up the engram-go LongMemEval (LME) benchmark campaign in the next session.
The campaign decides: **(a) fix engram toward the ~95% SOTA bar** vs **(b) rebuild clean**.
That decision needs TWO gate inputs. One is now in (GO); the other waits for Wednesday.

---

## TL;DR — where we are
- ✅ **Gate #2 (events-index retrieval pivot): GO.** Done, analyzed, saved. See below.
- ⏳ **Gate #1 (LME-S baseline number): pending Wednesday Jun 24, 6am ET** (Sonnet subscription resets).
- ✅ Scorer correctness bugs fixed and merged to `main` (#1169 + #1170).
- 🟢 Infra healthy: MI-50 live embed path untouched all session; no reembed anywhere; precision
  load was the events-index run (now finished → fans releasing).

---

## Gate #2 — Events-as-index experiment: ✅ GO
Full analysis: `results/events-index-experiment/GO-NOGO-ANALYSIS.md` (+ `gonogo.py`, `results.json`).

- Embedding-alone gets **0/47** deep items' gold fully into top-15; event-index re-rank gets **10/47**.
- Stricter production filter (gold within candidate window, `max_gold_embed_rank<=60`):
  **9/20 pool-eligible**, threshold ≥8 → **GO**, **zero regressions**.
- Lexical lower bound (Jaccard re-rank) — semantic would be ≥ this.
- **Strategic read:** the events-as-index pivot rescues exactly engram's weakest class
  (aggregation/multi-session). Points toward **fix** over rebuild. Confirm once gate #1 lands.

---

## Gate #1 — LME-S baseline: PAUSED, resume Wednesday
**Why paused:** generation uses `claude --print` with Sonnet; the Sonnet subscription hit its
cap. Resets **Wednesday Jun 24, 6:00am America/New_York**. Founder directive: "We will pick it
up Wednesday morning."

**Checkpoint state (`results/lme-s-baseline-0622/checkpoint-run.jsonl`):**
- 367 lines total: **293 done / 74 error** (the 74 errored against the capped Sonnet — regenerate them).
- Checkpoint-resumable (`--cleanup-policy=never`); do NOT discard it.

**Wednesday resume procedure:**
1. **Rebuild the LME-S binary from current `main`** — the merged scorer fixes (#1169/#1170) are NOT
   in the old `/tmp/longmemeval-s` binary. Rebuild: `go build -o /tmp/longmemeval-s ./cmd/longmemeval`
   from a clean checkout of `main` (HEAD `f47c13f`).
2. **Resume generation** with `--generation-model sonnet` against the existing checkpoint to clear
   the 74 errors. Low parallelism (founder: "Don't use huge parallelism on this LME-S").
3. **PAUSE the events-index** before judging — N/A now (events-index is DONE), but the rule was:
   both compete on olla, never judge while another olla job runs.
4. **Judge** with the local Nemotron `inference` model (vLLM on oblivion, `--max-model-len 65536`)
   or Haiku `score-batch`. The scorer now tail-keeps + budgets hypothesis length (see fixes below).
5. Compute the **gate #1 number** = engram's LME-S score vs the ~95% SOTA bar.

---

## Scorer fixes merged this session (on `main`)
The Nemotron judge was returning HTTP 400 — root cause was **caller-side**: verbose hypotheses +
`max_tokens 2048` overflowed the 65536 context. Fixed in `internal/longmemeval/claude.go`:
- **#1169** (`b1c852f`): `DefaultScorerMaxTokens` 2048→512; guard against long hypotheses.
- **#1170** (`f47c13f`): **tail-keep** truncation (the graded answer sits at the END for
  `--enumerate-first`; head-keep silently corrupted the gate number — caught in post-merge QA),
  dynamic budget = `65536 - maxTokens - overheadTokens`, `maxHypChars<0` clamp prevents panic.
  Added `Truncated bool` to `ScoreEntry` (types.go) for reproducibility.
- **Known nice-to-have (not blocking):** the 4-chars/token ratio can under-count tokens for dense
  text → consider a safety margin. File as a follow-up issue if it bites.

---

## Infrastructure state (all healthy, HARD rules held)
- **MI-50** (precision:8007, `ai-fleet-embed-mi50`, bge-m3) = Engram LIVE embed ONLY — untouched
  all session. Never batch/reembed/experiment on it.
- **W6800** (precision:8005) + **7900XT** = reembed/batch only. **No reembed ran** (and per standing
  directive, none should — single stored identity `BAAI/bge-m3`, no routing change triggers mass reembed).
- **fast-inference** (Mistral-Small-24B-AWQ, precision:8008) = the events-index generator; that 99%
  GPU load + high fans was this run, now finished.
- **Nemotron `inference`** (vLLM, oblivion, max-model-len 65536) = the LME judge.
- Olla = network service only (k8s `ai-fleet` NodePort 30411), controller-owned; do not hand-edit.

---

## Loose ends (deferred, none blocking)
1. **Codex throttle-fix clean PR to `main`.** The 401-throttle resilience is LIVE in the running
   `~/bin/codex-attach.sh` on the codex host, but its commit is on a side branch — needs a clean PR.
   (Codex auth = `chatgpt`/OAuth, `prolite` plan, intermittent 401s under load, token valid until Jul 2.)
2. **Grok #308** (`add-failure-mode.sh` missing-script fix) — in grok's `impl-grok` queue (`fe478e06`). Watch.
3. **codex-guard false-positives** — `truncate` (file util) flagged as `database_destruction`; `find ... *`
   flagged as `wildcard_delete`. Patch ready at `~/codex-guard-truncate-fix.md`. Blocked by the
   self-modification gate on the security hook → needs a founder hand to apply + rebuild
   (`cd ~/projects/codex && cargo build --release --bin codex-guard`).
4. Follow-up issue: scorer token-ratio safety margin (see Scorer fixes).

## Standing constraints (still in effect)
- "Ignore the engram key issues for now" (leaked ENGRAM_API_KEY remediation waived).
- Never `git push` from an agent brief; pushes are founder-gated (explicit "merge on green" only).
- MI-50 live path: protect above all. No reembed for any reason.
- Token efficiency overnight; **avoid Sonnet** until Wednesday reset.
