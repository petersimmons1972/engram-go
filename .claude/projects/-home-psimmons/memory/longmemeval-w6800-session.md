---
name: longmemeval-w6800-session-2026-05-05
description: LongMemEval run on W6800 produced 4.9% accuracy due to snowflake-stored chunk vectors vs jina-embedded queries — embedder mismatch is silent because both are 1024-dim
type: error
---

# LongMemEval W6800 attempt — 2026-05-05

**Result:** 77/500 generated, 66 scored, 3 CORRECT (4.9%), 60/61 hypotheses were "I don't know".

## Root cause

The lme-c3d9f1-* Engram projects (v9 ingest, 2.27M chunks) were embedded with `snowflake-arctic-embed2` on May 1. Current `ENGRAM_EMBED_MODEL=diqiuzhuanzhuan/jina-embeddings-v4-text-retrieval-Q8_0.gguf:latest`. Both are 1024-dim so the dimension check passes silently. Cosine similarity of a stored vector against fresh embeddings:
- vs snowflake: 1.000
- vs jina-v4: 0.019

Result: every retrieval returns 150 random chunks → model says "I don't know".

## Diagnostic that worked

```sql
SELECT chunk_text, embedding::text FROM chunks WHERE project='<proj>' LIMIT 1
```
Then call each candidate embedder via `/api/embeddings` on Ollama and compare cosine. The matching model wins.

## How to apply

Before running ANY benchmark or quality test against an Engram project older than the current `ENGRAM_EMBED_MODEL` setting: verify chunk vectors match the active embedder. There is no per-chunk model tracking. The `engram-reembed` worker migrates lazily (~280 chunks/min observed → 5 days for 2.27M chunks).

## Related infra changes (uncommitted)

- `~/projects/olla/config.local.yaml`: MI50 endpoint disabled (circuit breaker stuck), load_balancer changed from `least-connections` to `priority` (precision=100, leviathan=50) — least-connections inverts under load because instant-503 backends look idle.
- `~/projects/engram-go/.worktrees/feat-longmemeval/internal/longmemeval/claude.go:216` Score() patched to use Opus instead of Haiku (Opus was the only judge that ran reliably).

