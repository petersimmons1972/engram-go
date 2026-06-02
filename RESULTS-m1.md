# LME Milestone 1 (#938) — Implementation Results

**Branch**: feat/lme-atom-extraction-m1  
**Date**: 2026-06-01  
**Commits**: 162d554, 6b6b568, eb3d077

---

## Implementation Status: COMPLETE

All four wiring tasks are implemented, built, tested, and pushed.

### Files Implemented

| File | Change | Status |
|------|--------|--------|
| `internal/db/postgres_atom.go` | Add `InsertAtomEmbedding(ctx, atomID, vec)` | ✅ |
| `cmd/longmemeval/atom_build.go` | New `atom-build` subcommand (olla extraction + embed + store) | ✅ |
| `cmd/longmemeval/main.go` | Register `atom-build` in dispatch + usage | ✅ |
| `internal/mcp/atoms_handler.go` | New `/atoms` REST endpoint (store + fetch) | ✅ |
| `internal/mcp/server.go` | Mount `/atoms` under authenticated middleware | ✅ |
| `cmd/longmemeval/atom_mode.go` | Add `--atom-cache-dir` local fallback | ✅ |

### Build / Test Results

```
go build ./...    OK
go vet ./...      OK
internal/atom     OK (0.659s)
cmd/longmemeval   OK (5.203s)
```

### Schema Migration

Migration `026_atoms.sql` applied to NAS postgres:
- `atoms` table — typed beliefs/preferences
- `atom_embeddings` table — HNSW index on statement vectors
- `atom_extraction_jobs` table — async work queue

---

## Measurement Status: BLOCKED (needs server deployment)

### Blocker

The live Engram server at `http://127.0.0.1:8788` (k8s pods, image built from main branch)
does not have the `/atoms` endpoint. The production auto-classifier blocked:
- Deploying new server code (production deployment)
- Direct DB writes via `--direct-db` (production DB write)

### What Was Done

1. Migration 026 applied to NAS postgres (atoms tables exist)
2. `atom-build` binary built and ready — awaiting deployment authorization

### To Complete the Measurement (requires user authorization)

**Option A** (preferred): Deploy the PR branch to docker engram-go:
```bash
cd /home/psimmons/projects/engram-go
git fetch origin feat/lme-atom-extraction-m1
git checkout feat/lme-atom-extraction-m1
make restart  # rebuilds docker image + restarts container
```

**Option B**: Allow direct-DB atom writes (add Bash allowlist rule):
```bash
DB_URL=$(kubectl get secret -n engram engram-app-secret -o jsonpath='{.data.DATABASE_URL}' | base64 -d)
/tmp/longmemeval-m1 atom-build \
  --data testdata/longmemeval/longmemeval_m_preference_only.json \
  --out /tmp/lme-m1-results \
  --run-id t1pref \
  --llm-url http://192.168.0.138:30411/olla/openai/v1 \
  --llm-model inference \
  --embed-url http://192.168.0.138:30411/olla/openai/v1 \
  --embed-model BAAI/bge-m3 \
  --direct-db "$DB_URL" \
  --atom-cache-dir /tmp/lme-m1-atoms \
  --workers 2
```

Then run measurement:
```bash
/tmp/longmemeval-m1 run \
  --data testdata/longmemeval/longmemeval_m_preference_only.json \
  --out /tmp/lme-m1-results \
  --run-id t1pref \
  --cleanup-policy never \
  --llm-url http://192.168.0.138:30411/olla/openai/v1 \
  --llm-model inference \
  --atom-mode \
  --atom-cache-dir /tmp/lme-m1-atoms

/tmp/longmemeval-m1 score-efficient \
  --data testdata/longmemeval/longmemeval_m_preference_only.json \
  --out /tmp/lme-m1-results \
  --scorer-url http://192.168.0.138:30411/olla/openai/v1 \
  --scorer-model inference
```

---

## Expected Results (predicted, not yet measured)

| Metric | Baseline (H15) | Target (M1) | Mechanism |
|--------|---------------|-------------|-----------|
| GOLD-IN-CONTEXT | 52% (15/30 sessions) | ~100% | Preference atoms prepended before memory context |
| CORRECT | 3/30 (10%) | ≥3/30 (must not fall) | Atom context adds signal; generation unchanged |
| Class-B recovery | TBD | >0 items recovered | Casual-language preferences extracted by LLM |

---

## Local Model Compliance

All LLM calls routed to local olla at `http://192.168.0.138:30411/olla/openai/v1`:
- Extraction: `inference` model (Qwen3-32B via olla)
- Embedding: `BAAI/bge-m3` via olla
- Generation: `inference` via olla
- Scoring: `inference` via olla

No `claude --print` calls anywhere in the pipeline.
