---
name: LongMemEval Llama-3.1-8B run state
description: State of LongMemEval benchmark run using Llama-3.1-8B-Instruct on Oblivion — config, lessons, and resume instructions
type: project
originSessionId: 581488e2-acdb-468f-b1b3-2ebf215d0b73
---
**What:** Running LongMemEval-M (500 questions) using Llama-3.1-8B-Instruct on Oblivion instead of Claude Opus/Haiku. New `--llm-url` / `--llm-model` flags added to the longmemeval binary route generation and scoring through an OpenAI-compatible vLLM endpoint.

**State (stopped 2026-05-09):** 212/500 done, 40 errors. Checkpoint preserved. Resume with `resume.sh`.

**Resume:**
```bash
~/projects/engram-go/results/longmemeval-llama3-8b/resume.sh
```

**Key config that works:**
- `MAX_MODEL_LEN=131072` — LME-M prompts range 34k–67k tokens (p50=51k, max=67k). 65536 was too small for 3 questions.
- `max_tokens=256` in OAI request — without this, vLLM reserves full remaining context, causing extreme slowness
- `--workers 1` — concurrent prefill of 50k+ token prompts causes 3m timeouts even with 4 workers
- Rate: ~0.4 q/min → ~500 questions ≈ 20h total

**What failed (do not repeat):**
- `MAX_MODEL_LEN=8192/32768/65536` — all too small, hit VLLMValidationError
- `workers=4/8` without max_tokens — 3m timeouts from concurrent long prefill
- `workers=4` with max_tokens=256 — still timeouts on contested GPU

**Code:**
- Branch: `feat/lme-llama-backend` in engram-go (worktree at `.worktrees/lme-llama`)
- New functions: `GenerateOAI`, `ScoreOAI` in `internal/longmemeval/claude.go`
- Ingest checkpoint reused from `results/longmemeval-v8/` (run-id: c3d9f1, 500 questions, projects `lme-c3d9f1-*` exist in Engram with `--no-cleanup`)

**Lesson: survey prompt lengths first**
```python
tokens = [sum(len(t['content']) for s in item['haystack_sessions'][:20] for t in s)//4 for item in data]
```
This 5-line script answered 3 restart cycles of guessing. Always run before tuning MAX_MODEL_LEN.
