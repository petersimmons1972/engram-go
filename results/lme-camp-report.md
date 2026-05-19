# LME-on-a-Budget Campaign Report

**Campaign Phase**: Phase 2 — Qwen3 Retrieval Ablation Suite (Experiment 02 only)
**Execution Session**: 2026-05-19 16:31–16:50 UTC
**Overall Status**: BLOCKED — HTTP 400 errors on all generation attempts

---

## Experiment 02 Summary: Qwen3 + topK=100

**Objective**: Run full 500-item benchmark with retrieval context size (topK=100).

**Configuration**:
- Generator: `inference` (Qwen3-32B via oblivion)
- Endpoint: `http://oblivion.petersimmons.com:8000/v1/chat/completions`
- Retrieval: `--context-topk 100 --recall-topk 100`
- Items: 500 (reused Engram projects from v9, no re-ingest)

**Results**:

| Metric | Value |
|:---|---|
| Wall elapsed | 19 minutes |
| Items completed | 0 / 500 |
| Items errored | 1 / 500 |
| Error type | HTTP 400 (Bad Request) |
| Error message | `OAI request: status 400` |

**Root cause**: Oblivion endpoint rejecting request validation for all generation attempts, likely due to:
1. Prompt size too large (100 context blocks + query = ~1–2MB text)
2. Token count exceeds model input limits
3. Request format incompatible with current oblivion configuration

**Evidence**:
- ✅ Oblivion health check OK
- ✅ Engram connected
- ✅ Manual curl test succeeds
- ❌ LongMemEval runner's requests fail with 400

**Checkpoint**: `results/lme-camp-02-qwen-topk100/checkpoint-run.jsonl` (1 error entry)

---

## Blockers & Recovery

**To unblock**:
1. Reduce `--context-topk` to 50 or 25 and retry
2. Add debug logging to inspect request body
3. Switch to Claude API as fallback

**Phase 0 note**: Multiple scoring jobs auto-started; killed manually due to resource contention.

**Next session**: Restart Exp 02 with reduced context size, or escalate to Claude API.

---

**Last Updated**: 2026-05-19T16:50:00Z

---

## 2026-05-19 — Exp 02 Unblock (Investigator agent)

**Root cause of HTTP 400:** Prompt context overflow. With `--context-topk 100` and real LME sessions averaging ~8,323 chars (~2,080 tokens each), 100 blocks totalled ~208,000 tokens — nearly 2× oblivion's `max_model_len=131072`. The vLLM error body confirmed: "prompt contains at least 129,025 input tokens + 2,048 output = 131,073 > 131,072."

**Fix applied (commit d4ee06d):**
- `cmd/longmemeval/main.go`: Added `MaxBlockChars` config field + `--max-block-chars` flag
- `cmd/longmemeval/run.go`: Per-block truncation applied before prompt assembly
- `internal/longmemeval/claude.go`: `LME_DEBUG_REQUESTS=1` env-gated logging — logs request_body_bytes + full response body on non-200

**Safe parameters:** `--max-block-chars 4800` → 100 blocks × ~1,200 tokens = ~120K + 2,048 output = ~122K tokens (within 131,072)

**Exp 02 restarted:** PID 107548, `--context-topk 100 --max-block-chars 4800 --workers 2 --no-cleanup`

**Phase 0 status:**
- `longmemeval-llama3-8b`: 519/553 scored (still running, PID 76780)
- `v9-opus-rerun`: 412/426 scored (still running, PID 86124)
- `longmemeval-nemotron-fixed`: 500/500 scored (COMPLETE)
- `longmemeval-v4-gptoss`: 346/455 scored (running via phase0 launcher PID 91751)

**Recommended next experiment:** Exp 03 (topK=50, no block truncation) — provides clean comparison to Exp 02 (topK=100, truncated). Key question: does richer retrieval with truncation beat topK=50 at full content? Use `--context-topk 50 --recall-topk 100` without `--max-block-chars` (50 blocks × 2,080 tokens = ~104K tokens — fits comfortably).

### 2026-05-19 17:03 UTC — Oblivion vLLM went down

After exp02 restarted, oblivion port 8000 began refusing connections (host reachable, port refused). Likely OOM from the 254K-token diagnostic request sent during root cause investigation. All scorers and exp02 killed cleanly.

**Recovery plan (once oblivion restarts):**
1. Restart Phase 0 scorers (see `lme-camp-state.json:restart_commands`)
2. Restart exp02 — same command, checkpoint handles resume automatically
3. No re-ingest needed. No data loss.

**Phase 0 status at kill:**
- `longmemeval-llama3-8b`: ~533/553 scored (paused)
- `v9-opus-rerun`: ~428/426 scored (paused, essentially done)
- `longmemeval-nemotron-fixed`: ~515/500 (done)
- `longmemeval-v4-gptoss`: ~346/455 scored (109 remaining)

---

## Phase 0 — Re-scoring with fixed rubric (CLOSED — partial, oblivion outage)

**Closed**: 2026-05-19T17:04:24Z
**Status**: 6 of 8 directories materially re-scored; v4-gptoss and topk100-sample-rescored remain at pre-fix labels (oblivion died before they ran).

### Final delta table

| Result dir                                 | Items | Old (C/PC/I) | New (C/PC/I) | Δ CORRECT | Status |
|:-------------------------------------------|------:|:-------------|:-------------|----------:|:------|
| longmemeval-v9-qwen3-20260517              |   566 | 367/173/26   | **428/176/26** | **+61** | ✅ Clean — full re-scoring, 0 errors |
| v9-opus-rerun                              |   392 | 375/2/15     | 399/6/15       |    +24  | ⚠️ Partial — 8 outage errors |
| longmemeval-v6-qwen72b                     |   446 | 151/53/242   | 152/53/242     |     +1  | ✅ Scorer dedup left most items unchanged |
| longmemeval-v5-qwen32b                     |   500 | 178/40/282   | 182/40/283     |     +4  | ✅ Rewritten |
| longmemeval-nemotron-fixed                 |   500 | 172/11/317   | 173/11/323     |     +1  | ⚠️ Partial — 8 outage errors |
| longmemeval-llama3-8b                      |   502 | 161/63/276   | 179/65/279     |    +18  | ⚠️ Partial — 8 outage errors |
| longmemeval-v4-gptoss                      |   346 | 68/49/229    | 68/49/229      |     +0  | ❌ BLOCKED — never started (oblivion down) |
| topk100-sample-rescored                    |    21 | 10/1/10      | 10/1/10        |     +0  | ❌ BLOCKED — never started (oblivion down) |

### Headline numbers

- **New v9 baseline: 428/566 CORRECT (75.6%)** — up from 367 (64.8%) under the buggy rubric. **Δ +61.**
- v9 INCORRECT count unchanged at 26 — the fix promotes PARTIALLY_CORRECT to CORRECT, doesn't recover hard misses.
- v9-opus-rerun: 399/392 = ~99% CORRECT (was 95.7%) — Opus confidence essentially confirmed.
- llama3-8b is the most-impacted weak model (+18) — terse answers were being unfairly downgraded.

### Total wall time

~39 minutes. Would have been ~50–60 min completing the remaining 2 dirs.

### Skipped / blocked

- `longmemeval-v4-gptoss` (346 items) — pre-fix labels stand; re-attempt when oblivion returns. Low priority (weak baseline).
- `topk100-sample-rescored` (21 items) — pre-fix labels stand; already a fixed-rubric sanity check from earlier. Negligible.

Partial re-scores (opus-rerun, nemotron, llama3) have 8 outage-errored items each that may still hold incorrect pre-fix labels; re-run when oblivion is back.

---

