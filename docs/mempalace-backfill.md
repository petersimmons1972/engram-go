# MemPalace Back-fill & Benchmark Guide (LME Experiment #9)

## Overview

Migration 028 adds a 2-level hierarchical recall path (MemPalace). The schema is in place
after `go run ./cmd/engram-setup` or server startup, but **existing memories have
`cluster_id = NULL` and gain no benefit until back-fill runs.**

Flag: `ENGRAM_MEMPALACE_HIERARCHICAL_RECALL=true` (default: false, opt-in).

---

## Back-fill Step (run AFTER ingest, BEFORE benchmark)

Back-fill assigns each memory to a cluster by:
1. Pulling all embedded chunk vectors for the project.
2. Running k-means (k=√N, max k=50) to produce cluster centroids.
3. Inserting centroids into `memory_clusters`.
4. Setting `memories.cluster_id` to the nearest centroid for each memory.

**Do NOT run back-fill against prod DB.** This is a benchmark-only operation.

```bash
# Build the back-fill binary (once):
go build -o bin/mempalace-backfill ./cmd/mempalace-backfill

# Run back-fill for a specific LME project (replace lme-<run_id> with your run):
DATABASE_URL="${DATABASE_URL}" \
  bin/mempalace-backfill \
    --project lme-<run_id> \
    --clusters 20 \
    --max-iter 100

# Or back-fill all lme-* projects at once:
DATABASE_URL="${DATABASE_URL}" \
  bin/mempalace-backfill \
    --pattern "lme-%" \
    --clusters 20 \
    --max-iter 100
```

> NOTE: `cmd/mempalace-backfill` does not exist yet — it is the next step after this
> experiment's recall path lands. The back-fill binary should:
> 1. Query `SELECT id, embedding FROM chunks WHERE project=$1 AND embedding IS NOT NULL`
> 2. Run k-means on the embedding matrix (use `gonum.org/v1/gonum/mat` + Lloyd's algorithm)
> 3. INSERT INTO memory_clusters (centroid per cluster)
> 4. UPDATE memories SET cluster_id = nearest_centroid WHERE project=$1

---

## Benchmark Command (after back-fill)

```bash
# 1. Resolve routes
./longmemeval route-discover --purpose generation > /tmp/lme-route.json
export LME_LLM_URL="$(jq -r .llm_url /tmp/lme-route.json)"
export LME_LLM_MODEL="$(jq -r .llm_model /tmp/lme-route.json)"

# 2. Ingest (skip if already ingested with --no-cleanup from a prior run)
./longmemeval ingest \
  --data ~/path/to/longmemeval_m_cleaned.json \
  --url http://localhost:8788 \
  --workers 32 \
  --out ~/benchmarks/lme-m-mp9 \
  --cleanup-policy=never \
  --scratch-ttl 168h

# 3. Back-fill clusters (after ingest, before recall)
DATABASE_URL="${DATABASE_URL}" bin/mempalace-backfill \
  --pattern "lme-%" \
  --clusters 20

# 4. Run recall+generate with hierarchical flag ON
ENGRAM_MEMPALACE_HIERARCHICAL_RECALL=true \
  ./longmemeval run \
    --data ~/path/to/longmemeval_m_cleaned.json \
    --url http://localhost:8788 \
    --llm-url "${LME_LLM_URL}" \
    --llm-model "${LME_LLM_MODEL}" \
    --workers 32 \
    --out ~/benchmarks/lme-m-mp9 \
    --recall-topk 100 \
    --context-topk 8

# 5. Score
./longmemeval score-efficient \
  --data ~/path/to/longmemeval_m_cleaned.json \
  --scorer-url "${LME_SCORER_URL:-${LME_LLM_URL}}" \
  --scorer-model "${LME_SCORER_MODEL:-${LME_LLM_MODEL}}" \
  --workers 16 \
  --out ~/benchmarks/lme-m-mp9

# 6. Analyze — compare to golden baseline (run_id 7a87fd, 367/566 CORRECT)
./longmemeval analyze --results ~/benchmarks/lme-m-mp9
```

---

## Expected Improvement (from MemPalace paper, d3a98aa4)

Target failure classes where cluster pre-filtering should help:
- **multi-session (19.5%)** — off-topic sessions will be filtered by cluster boundary
- **temporal-reasoning (18.8%)** — same-topic sessions grouped in cluster
- **knowledge-update (46.2%)** — most recent cluster centroid closest to query

Baseline: 367/566 CORRECT (run_id 7a87fd, main 4bf268c).
Expected: ~490/566 if ~34% retrieval gain from MemPalace translates (upper bound).

---

## Ablation

To compare hierarchical vs. flat on the same ingested data:

```bash
# Baseline (flat path):
ENGRAM_MEMPALACE_HIERARCHICAL_RECALL=false \
  ./longmemeval run ... --out ~/benchmarks/lme-m-baseline

# Hierarchical:
ENGRAM_MEMPALACE_HIERARCHICAL_RECALL=true \
  ./longmemeval run ... --out ~/benchmarks/lme-m-mp9
```

---

## Tuning TopClusters

The number of coarse clusters searched per recall is controlled by:
- `RecallOpts.TopClusters` (programmatic, default 3)
- The back-fill `--clusters` parameter controls total cluster count K

Ablation values to try: TopClusters ∈ {1, 3, 5, 10}.
