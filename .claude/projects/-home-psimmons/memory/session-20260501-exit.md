---
name: LME Benchmark Session 2026-05-01 Exit State
description: End-of-session state for LongMemEval benchmark run — what's running, what's broken, what's next
type: project
---

Embeddings running overnight via engram-reembed container (batch mode, 16 texts/request, snowflake-arctic-embed2, W6800+7900XT dual backend via LiteLLM). ~3% done, ~31hr to complete at 279 chunks/min.

**Why:** direct postgres import of 500 LME items (237K memories, run-id c3d9f1) bypassed Engram API after accidental deletion of prior LME data from postgres.

**How to apply:** Before next benchmark run: (1) fix Engram API key via `make setup` in engram-go — current .env key gets 401; (2) check embedding %; (3) run benchmark with --run-id c3d9f1 --no-cleanup.

**MI50 unresolved:** llama-server-mi50-embed exits code 0 with ROCR_VISIBLE_DEVICES=1, no error. Service disabled. Worth debugging separately.

**Rule enforced:** No DELETE FROM memories/chunks without explicit named confirmation per operation.
