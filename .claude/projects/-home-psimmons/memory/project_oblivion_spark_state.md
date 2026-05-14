---
name: Oblivion DGX Spark deployment state
description: Current state of oblivion.petersimmons.com (DGX Spark GB10) — services, GPU budget, and decisions as of 2026-05-09
type: project
originSessionId: 581488e2-acdb-468f-b1b3-2ebf215d0b73
---
**Running services on oblivion (as of 2026-05-09):**
- `vllm` (port 8000, vLLM, 60% GPU): `meta-llama/Llama-3.1-8B-Instruct` — inference for LongMemEval benchmark
- `infinity` (port 8003, Infinity): `BAAI/bge-m3` — sole embedding source for fleet (~10% GPU)
- `jina-v5` RETIRED — stopped 2026-05-09, port 8002 freed

**GPU budget (119.6 GiB total):** vLLM 60% + Infinity ~10% → ~36 GiB headroom ✅

**vLLM config (~/projects/spark/services/vllm/.env on oblivion):**
- `MAX_MODEL_LEN=131072` — covers all LME-M prompts (max observed: ~67k tokens)
- `GPU_MEMORY_UTILIZATION=0.60`
- `DTYPE=bfloat16`
- Manage via: `cd ~/projects/spark && just restart vllm`

**Why:** jina-v5/vLLM retired after migration to Infinity/bge-m3 (14× throughput). vLLM now dedicated to LongMemEval benchmark inference using Llama 3.1 8B.

**How to apply:** Adding another service requires reducing vLLM GPU utilization first. KV cache at 131072 tokens uses ~16 GiB per sequence — single-worker inference is fine at 60% utilization.

**LongMemEval benchmark state (stopped mid-run 2026-05-09):**
- 212/500 questions done, checkpoint preserved at `~/projects/engram-go/results/longmemeval-llama3-8b/`
- Resume: `~/projects/engram-go/results/longmemeval-llama3-8b/resume.sh`
- Branch: `feat/lme-llama-backend` (worktree at `.worktrees/lme-llama`)
- flags: `--llm-url http://oblivion.petersimmons.com:8000/v1 --llm-model meta-llama/Llama-3.1-8B-Instruct --workers 1 --no-cleanup`
