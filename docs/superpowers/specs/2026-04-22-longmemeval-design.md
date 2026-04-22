# LongMemEval Integration Design

**Date:** 2026-04-22  
**Status:** Approved  
**Dataset:** `longmemeval_m_cleaned` (full scale — ~500 sessions/question, 500 questions)

---

## Goal

Run the [LongMemEval](https://github.com/xiaowu0162/LongMemEval) benchmark against the live engram-go MCP server to get an objective retrieval and end-to-end QA score. Produce two output artefacts:

1. `hypotheses.jsonl` — compatible with LongMemEval's `evaluate_qa.py` (GPT-4o scoring)
2. `score_report.json` + `retrieval_log.jsonl` — Claude Code self-scoring, usable immediately without Python/OpenAI

---

## Commands

```
longmemeval ingest  --data longmemeval_m_cleaned.json --workers 4 --run-id <hex>
longmemeval run     --data longmemeval_m_cleaned.json --workers 4 --run-id <hex>
longmemeval score   --data longmemeval_m_cleaned.json --workers 4 --run-id <hex>
longmemeval all     --data longmemeval_m_cleaned.json --workers 4
```

`all` generates a `--run-id` (6-char hex) automatically and chains the three stages. `--no-cleanup` skips Engram project deletion (debugging). `--retry N` (default 1) controls `claude --print` retry count on timeout.

---

## Data Flow

### Ingest stage
- Worker pool reads questions from a channel
- Per question: create Engram project `lme-<run-id>-q<NNN>`
- Per haystack session: `memory_store` with content = concatenated user turns, tags = `["lme", "sid:<session_id>"]`
- Checkpoint: `checkpoint-ingest.jsonl` — `{question_id, project, session_count, memory_ids, status}`
- Projects remain alive until run stage completes

### Run stage
- Reads ingest checkpoint for `project` + `memory_id → session_id` map
- `memory_recall(query=question, project=lme-..., top_k=50)`
- Fetches content of top-10 memories for generation context
- Calls `claude --print` with system prompt + context + question → `hypothesis`
- Checkpoint: `checkpoint-run.jsonl` — `{question_id, hypothesis, retrieved_ids, status}`
- Deletes Engram project after checkpoint written

### Score stage
- Calls `claude --print` as judge: reference answer + hypothesis → `CORRECT / PARTIALLY_CORRECT / INCORRECT` + one-line explanation
- Checkpoint: `checkpoint-score.jsonl`
- Final outputs:
  - `hypotheses.jsonl` — `{question_id, hypothesis}` per line
  - `retrieval_log.jsonl` — `{question_id, retrieval_results: {metrics: {session: {recall_all@5, ndcg_any@5, recall_all@10, ndcg_any@10}}}}`
  - `score_report.json` — accuracy breakdown by `question_type`

---

## Project Isolation

Each question gets a dedicated Engram project: `lme-<run-id>-q<NNN>`. The run-id ensures no collision between a re-run and an incomplete prior run. Cleanup (project deletion) happens in the run stage after hypothesis and retrieved IDs are checkpointed.

---

## Package Structure

```
cmd/longmemeval/
  main.go        — flag parsing, subcommand dispatch, run-id generation
  ingest.go      — ingest subcommand, worker pool
  run.go         — run subcommand (recall + generation)
  score.go       — score subcommand + output file writing
  all.go         — chains ingest → run → score

internal/longmemeval/
  types.go       — LongMemEvalItem, HaystackSession, CheckpointEntry structs
  checkpoint.go  — append-safe JSONL checkpoint (writer goroutine, channel-fed)
  engram.go      — MCP client wrappers (connect, memory_store, memory_recall, memory_fetch, delete_project)
  claude.go      — exec.Command("claude", "--print") wrapper with timeout + retry
  metrics.go     — RecallAny@k, RecallAll@k, NDCG@k; builds retrieval_log entries
```

`internal/eval/` (existing) is unchanged. `internal/longmemeval/metrics.go` imports and reuses its `NDCG` function.

---

## Error Handling

| Failure | Behaviour |
|---------|-----------|
| `claude --print` timeout (90s) | Retry once (configurable). On second failure: write `status=error` to checkpoint, skip question in outputs, increment error tally. |
| Score response parse failure | Default to `PARTIALLY_CORRECT`, log parse warning. |
| `memory_store` failure | Retry once with 5s backoff. On second failure: abort question (don't partially ingest), write error checkpoint. |
| `memory_recall` failure | Same retry policy. On second failure: skip generation (no context), write error checkpoint. |
| Worker panic | Recovered, logged as error — pool continues. |

**Concurrency:** A single goroutine owns each checkpoint file. Workers send completed entries over a channel to the writer goroutine. No file locking.

**Worker count:** Default 4. Tunable via `--workers`. Run with `--workers 1` for single-question debugging.

---

## Checkpoint Resume

Each subcommand builds a `map[question_id]bool` from its checkpoint file on startup. Workers skip questions where the entry exists with `status=done`. Re-running `ingest` after partial completion picks up from where it left off.

---

## Metrics

- **Retrieval:** `recall_any@k`, `recall_all@k`, `ndcg_any@k` at k=5,10 (session level). Abstention questions (`_abs` suffix in question_id) are excluded per LongMemEval convention.
- **QA accuracy:** % CORRECT, % PARTIALLY_CORRECT, % INCORRECT, broken down by `question_type` (5 types).
- **Error rate:** questions skipped due to Engram or generation failures.

---

## Prerequisites

**`memory_delete_project` MCP tool** — engram-go has no project-level bulk deletion. `memory_forget` only deletes one memory by ID. Adding a `memory_delete_project` tool (one SQL `DELETE FROM memories WHERE project = $1` + weight_config cleanup) is required before cleanup can be implemented efficiently. This is a small, bounded addition to `internal/mcp/tools.go` and must be the first implementation task.

Without this, cleanup would require paginated `memory_list` + 500 individual `memory_forget` calls per question — 250k round-trips for a full run. `memory_delete_project` must be TDD'd (happy path, empty project, unknown project) and covered before the eval harness is built.

---

## Out of Scope

- Turn-level granularity (session-level only for this run)
- Index expansion (key expansion, time-aware query) — pure recall first
- `longmemeval_s` or `longmemeval_oracle` variants (can be added by passing a different `--data` file)
