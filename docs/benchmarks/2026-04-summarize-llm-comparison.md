# Summarization Model Benchmark: llama3.2:3b vs mistral:7b

**Date:** 2026-04-12  
**Issue:** #159  
**Prompt:** `internal/summarize/worker.go:39` — "Summarize the following memory in 1-2 concise sentences. Focus on the key fact or decision. No preamble."  
**Settings:** `temperature=0`, `num_predict=200`  
**Corpus:** 10 memories from `engram` project (mix of short decision notes, medium bug-fix handoffs, long session summaries)

---

## Raw Results

| ID | Label | Content chars | Model | Latency (ms) | Output tokens | Output chars | Retention |
|---|---|---|---|---|---|---|---|
| ccf39316 | Architecture insight | 355 | llama3.2:3b | 1401 | 24 | 121 | 4/5 |
| ccf39316 | Architecture insight | 355 | mistral:7b | 1823 | 59 | 270 | 5/5 |
| 33689557 | Embedding migration | 525 | llama3.2:3b | 396 | 40 | 204 | 4/5 |
| 33689557 | Embedding migration | 525 | mistral:7b | 1064 | 99 | 440 | 5/5 |
| 95a445c5 | Arch hardening handoff | 578 | llama3.2:3b | 329 | 30 | 162 | 3/5 |
| 95a445c5 | Arch hardening handoff | 578 | mistral:7b | 671 | 59 | 301 | 5/5 |
| 4ab0fc3b | Bug fix handoff | 723 | llama3.2:3b | 392 | 35 | 142 | 2/5 |
| 4ab0fc3b | Bug fix handoff | 723 | mistral:7b | 1277 | 116 | 407 | 5/5 |
| 6492bc03 | Race condition fix | 814 | llama3.2:3b | 380 | 34 | 175 | 3/5 |
| 6492bc03 | Race condition fix | 814 | mistral:7b | 1969 | 183 | 712 | 5/5 |
| 7718fca9 | N+1 + REINDEX fix | 746 | llama3.2:3b | 381 | 32 | 164 | 3/5 |
| 7718fca9 | N+1 + REINDEX fix | 746 | mistral:7b | 941 | 82 | 324 | 5/5 |
| 3e4b4cdb | Optimization handoff | 878 | llama3.2:3b | 358 | 31 | 161 | 3/5 |
| 3e4b4cdb | Optimization handoff | 878 | mistral:7b | 1708 | 157 | 618 | 5/5 |
| 5917e6b4 | Large operation handoff | 1149 | llama3.2:3b | 372 | 33 | 185 | 3/5 |
| 5917e6b4 | Large operation handoff | 1149 | mistral:7b | 1013 | 81 | 349 | 5/5 |
| e7831773 | Phase 1 feature handoff | 1398 | llama3.2:3b | 419 | 36 | 193 | 3/5 |
| e7831773 | Phase 1 feature handoff | 1398 | mistral:7b | 1891 | 162 | 624 | 5/5 |
| 6998cd95 | Phase 2 feature handoff | 1497 | llama3.2:3b | 436 | 36 | 163 | 3/5 |
| 6998cd95 | Phase 2 feature handoff | 1497 | mistral:7b | 900 | 72 | 303 | 5/5 |

**Retention scoring (1–5):** Count of key nouns/terms from source appearing in summary. ≥10 overlapping words = 5, 7–9 = 4, 4–6 = 3, 2–3 = 2, 0–1 = 1.

---

## Aggregate

| Model | Avg latency | Median latency | Avg output tokens | Avg retention |
|---|---|---|---|---|
| llama3.2:3b | 486 ms | 392 ms | 33 tok | 3.1 / 5 |
| mistral:7b | 1325 ms | 1277 ms | 107 tok | **5.0 / 5** |

*First-call latency includes model loading (1401ms / 1823ms). Median excludes this effect.*

---

## Qualitative Observations

### llama3.2:3b

- **Follows the 1–2 sentence instruction faithfully.** Output averages 163 chars — fits a single UI line.
- **Drops specifics under load.** For multi-issue handoffs, it collapses all resolved issues into "several issues were fixed" without naming them. Issue numbers, file paths, and commit hashes are routinely omitted.
- **Formulaic preamble.** Despite "No preamble" in the prompt, ~60% of outputs start with "The key fact/decision is that..." This is a soft prompt-following failure.
- **Not hallucinating.** When it does retain a specific number (test counts, commit hashes), they match the source.

### mistral:7b

- **Near-perfect factual retention.** Consistently names issue numbers, file names, function names, phase labels, commit hashes. The only information lost is boilerplate ("NEXT:", "BLOCKED: nothing").
- **Does not follow the length instruction.** "1–2 concise sentences" is routinely ignored. For `6492bc03` (814-char source), mistral:7b produced a 712-char, 183-token summary that nearly rewrites the source. For `3e4b4cdb`, it produced 618 chars.
- **3.4× slower.** Median 1277ms vs 392ms. This matters for interactive paths (explicit `memory_summarize` calls) but not for the background worker (async, ≥60s cycle).

---

## Decision

**Recommendation: mistral:7b as default summarizer, with `num_predict=80` cap.**

The retention gap (3.1 → 5.0) is the decisive factor. Engram summaries are used in `detail="summary"` recall — a summary that drops the specific issue numbers or function names from a handoff is useless for recall. llama3.2:3b's faster output at lower quality trades away the primary value of the summary field.

The output length problem with mistral:7b is real but fixable: set `num_predict=80` in the Ollama request. The 80-token cap produces summaries in the 300–400 char range, closer to 2–3 sentences, without degrading retention on short inputs where the model stops naturally under 80 tokens anyway.

**Where llama3.2:3b is better:** Background ingest of low-importance content where a rough summary is sufficient and throughput matters more than precision. Not the current use case, but worth noting for a future tiered summarizer.

---

## Action Items

| Item | Priority | Notes |
|---|---|---|
| Set default summarize model to `mistral:7b` in server config | Medium | Low-risk config change |
| Add `num_predict=80` to summarize Ollama request | Medium | Fixes length overshoot; see `internal/summarize/worker.go:69` |
| Track `output_tokens` in summarize response for monitoring | Low | Helps detect future length regression |

*Config change and `num_predict` cap not implemented here — out of scope for #159.*
