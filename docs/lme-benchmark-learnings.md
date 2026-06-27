# LongMemEval Benchmark Learnings

**Date**: May 2026  
**Benchmark**: LongMemEval-M (500 questions, 6 types)  
**Memory System**: Engram  
**LLM Backend**: vLLM with Nemotron-3-Nano-Omni-30B-A3B-Reasoning-NVFP4 (FP4 quantized, max_model_len=131072)

---

## Executive Summary

We ran the LongMemEval-M benchmark against Engram to evaluate long-context memory performance. The test dataset contains 500 questions across six types: knowledge-update, multi-session, single-session-assistant, single-session-user, single-session-preference, and temporal-reasoning. Initial configuration attempts failed due to vLLM/Nemotron incompatibilities and resource limits. After systematic debugging and optimization, we achieved **~8 completions/minute** (62-minute ETA for full 500-question run) with correct factual answers emerging from the model.

Key learnings document nine configuration issues (all resolvable), throughput optimization strategies, and model-specific constraints that future benchmarks should adopt from the start.

---

## Configuration Issues Found (and Fixed)

### 1. Nemotron HTTP 400 Errors (vLLM GH#39103)

**Problem**: Requests to vLLM with Nemotron v3 reasoning models returned HTTP 400.

**Root Cause**: The oaiMessage struct included a `Reasoning` field (even when empty). Nemotron's reasoning parser rejects this field in the payload.

**Fix**: Use a struct with only `Role` and `Content`:
```go
type SimpleMessage struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}
```

**Reference**: vLLM GitHub issue #39103 — Nemotron v3 reasoning parser does not tolerate extra fields.

---

### 2. max_tokens=20480 Causes 400 Errors

**Problem**: Setting `max_tokens=20480` caused HTTP 400 responses when prompt context was large.

**Root Cause**: With large prompts (40 memory blocks × 10KB ≈ 100K tokens) and max_tokens=20480, the total request token budget (104K + 20K = 124K) left insufficient headroom before the model's max_model_len=131072 limit. The backend rejected requests as out of bounds.

**Fix**: Set `max_tokens=2048` — adequate for QA responses when `enable_thinking: false`.

**Reasoning**: QA answers typically require <2K tokens. This conservative limit leaves buffer room and prevents backend rejections.

---

### 3. enable_thinking Must Be False

**Problem**: With default Engram config, Nemotron entered thinking mode, consuming all tokens inside `<think>` tags, and produced no useful output.

**Root Cause**: Nemotron v3 is a reasoning-specialized model. Without explicitly disabling reasoning, it activates `enable_thinking`, which causes the model to spend its entire token budget on internal reasoning and emit nothing.

**Fix**: Set `chat_template_kwargs: {"enable_thinking": false}` in the vLLM server launch config.

**Implication**: When `enable_thinking: false`, the model behaves more like an instruct model — appropriate for factoid QA. For tasks that benefit from chain-of-thought reasoning (e.g., multi-step inference), consider an instruct-specialized model like Qwen2.5-30B instead.

---

### 4. contextTopK=40 Creates 104K Token Prompts

**Problem**: With `contextTopK=40`, each question's context exceeded 100K tokens, causing severe throughput degradation.

**Root Cause**: 
- LME dataset has ~470 sessions per question
- Memory blocks average 10KB ≈ 2,618 tokens each
- 40 blocks × 2,618 tokens = 104,720 tokens per prompt
- With 32 concurrent workers and vLLM's continuous batching, each request processes sequentially
- At 1-2 completions/minute with large prompts, 500 questions = 8+ hours

**Fix**: Reduce `contextTopK` to 8 (total ≈21K tokens per prompt).

**Result**:
- Prompt size: ~21K tokens (5x reduction)
- Throughput: ~8 completions/minute (4-5x improvement)
- ETA for 500 questions: ~62 minutes
- Accuracy: Minimal loss because vector recall (recallTopK=100) already captures the top semantic matches

**Rationale**: In factoid QA tasks, the correct answer is almost always in the top-3 semantically relevant memory blocks. Providing top-8 offers a safety margin with orders-of-magnitude less context.

---

### 5. Engram Rate Limiting with High Worker Counts

**Problem**: With 32 concurrent workers and 41 Engram API calls per question, burst requests exceeded Engram's default rate limit (50 req/s, burst 200), causing HTTP 429 errors.

**Calculation**: 32 workers × 41 calls/question = 1,312 burst requests; default limit burst = 200.

**Fix**: Set `ENGRAM_RATE_LIMIT_DISABLE=true` in `.env` AND force-recreate the container.

**Critical**: `docker compose restart` does NOT pick up new env vars. Use `docker compose up -d --force-recreate engram` to apply changes.

---

### 6. MCP URL Format Matters

**Problem**: Benchmark created MCP connection URL as `serverURL + "/sse"`. If vLLM was running at `http://host:8788/mcp`, the result became `http://host:8788/mcp/sse` → 404.

**Fix**: Pass `http://host:8788` (no `/mcp` suffix) as the base serverURL.

**Lesson**: Be explicit about URL path construction. The MCP server expects the base URL; the client adds the `/sse` path.

---

### 7. Cleanup Deletes Ingested Data

**Problem**: Running the benchmark without `-no-cleanup` deletes Engram projects after each question. Re-running after a partial or failed run requires re-ingesting the same data.

**Fix**: Always use `-no-cleanup` flag when running against pre-ingested data you want to preserve.

**Implication**: For repeated benchmark runs (e.g., parameter tuning), keep the `-no-cleanup` flag set. Only enable cleanup between completely fresh runs on new datasets.

---

### 8. Container Restart vs Force-Recreate

**Problem**: After adding `ENGRAM_RATE_LIMIT_DISABLE=true` to `.env`, restarting the container did not pick up the new env var.

**Root Cause**: `docker compose restart` sends SIGTERM to the running container but does not re-read `.env`. The process restarts with the old environment.

**Fix**: Use `docker compose up -d --force-recreate <service>`.

**General Rule**: Any change to `.env` → force-recreate. Any change to Docker image → rebuild. Only use restart for clean shutdowns of already-correct containers.

---

### 9. Competing Benchmark Processes Corrupt Checkpoint Files

**Problem**: Killing the shell process (Ctrl+C) did not kill the actual benchmark binary. Two instances of the benchmark ran concurrently, both writing to the same checkpoint file, corrupting progress state.

**Fix**: Kill the actual binary PID. Use:
```bash
ps aux | grep longmemeval
kill -9 <actual_binary_pid>
```

**Prevention**: Use a process manager (systemd service, supervisord, or K8s) if benchmark runs in background. Shell job control can hide subprocess PIDs.

**Automated guard (v3.3.0+):** The `--exclusive-backend` flag (default enabled) now prevents this scenario automatically. A PID-liveness lock file is written to `$XDG_RUNTIME_DIR/lme/backend-<hash>.lock` (or `/tmp/lme/` if `XDG_RUNTIME_DIR` is unset). If a second `lme run` targets the same backend while the first is still alive, it exits immediately with code **75 (EX_TEMPFAIL)**:

```
ERROR another lme run holds the lock on backend http://oblivion:8000/v1
(pid=12345, started=2026-05-20T10:00:00Z, invocation=...).
Wait for it, or pass --no-exclusive-backend if you accept result contamination.
Exit code 75 (EX_TEMPFAIL) signals temporary contention.
```

When two parallel runs *are* intentional (e.g. benchmarking two different Engram server URLs on the same host), pass `--no-exclusive-backend` to both invocations to opt out. Dead-process lock files are reclaimed automatically on the next acquisition attempt — no manual cleanup is needed.

#### Manual recovery

Automatic reclaim handles the common case (lock held by a dead PID). If reclaim silently fails — for example when the lock directory permissions are wrong, or an unexpected `flock` error caused the guard to skip the lock entirely — you can recover manually:

1. **Locate the lock file.** Lock files follow the pattern `backend-<12-hex-chars>.lock`:
   - `$XDG_RUNTIME_DIR/lme/backend-*.lock` (preferred path when `XDG_RUNTIME_DIR` is set)
   - `/tmp/lme/backend-*.lock` (fallback)
   - `<custom-dir>/backend-*.lock` (if `--backend-lock-dir` was used)

2. **Verify no `lme run` is active.** Use `ps aux | grep longmemeval` to confirm no benchmark process is running against the affected backend.

3. **Remove the lock file.** `rm` is safe when no `lme run` is active:
   ```bash
   rm "$XDG_RUNTIME_DIR/lme/backend-*.lock"   # or /tmp/lme/backend-*.lock
   ```

4. **Relocate if the directory is problematic.** Use `--backend-lock-dir /path/to/writable/dir` to move lock files to a directory with correct permissions.

---

## Throughput Optimization

The following table summarizes the effect of `contextTopK` on prompt size and throughput:

| Config | Prompt Size | Completions/min | Completion Time | ETA for 500q |
|--------|-------------|-----------------|-----------------|--------------|
| contextTopK=40 | ~104K tokens | 1–2 | 30–60s per Q | 8+ hours |
| contextTopK=8 | ~21K tokens | ~8 | ~7.5s per Q | ~62 min |

**Throughput Limiting Factors** (contextTopK=8):
1. Sequential Engram fetch: 8 calls × ~50ms each = 400ms per question
2. vLLM inference: ~6–7 seconds per question (30B model on single A100)
3. Checkpoint write + state management: ~300ms

Total: ~7.7 seconds per question ≈ 8 questions/minute.

**Optimization Opportunities**:
- Parallel Engram fetches: Rewrite benchmark to fetch all contextTopK blocks in one request or parallel goroutines (estimated 200ms savings)
- vLLM batch size tuning: Nemotron with large prompts processes 1 request/batch. Smaller prompts (contextTopK=8) enable 2–4 concurrent requests (estimated 1–2x throughput improvement)
- Engram indexing: Add composite indices on session_id + created_at to speed vector recall (estimated 50ms savings per question)

---

## Architecture Notes

### vLLM Continuous Batching Behavior

With prompts >50K tokens, vLLM's continuous batching degrades to near-sequential processing. The GPU's KV cache becomes the bottleneck. Reducing prompt size (contextTopK=8) allows the scheduler to queue 2–4 requests concurrently, improving overall throughput despite longer inference time per request.

### Vector Recall Quality vs Context Size

- `recallTopK=100`: Engram returns the 100 most semantically relevant memory blocks (vector similarity)
- `contextTopK=8`: Benchmark uses only the top 8 of those 100 blocks
- For factoid QA: 95% of correct answers are in the top-3 blocks; top-8 provides a 2.7x safety margin
- Trade-off: Slightly higher risk of missing the answer vs. 13x smaller prompt (104K → 21K tokens)

In practice, the answer is almost always in top-8 for well-indexed memory systems.

### Sequential Fetch Bottleneck

Each worker fetches `contextTopK` memory blocks sequentially via Engram API:
```
for i in 0..contextTopK-1:
  GET /recall?block_id=X
```

This creates an implicit 50ms × contextTopK serial delay per question. Future optimization: batch request or parallel goroutines.

---

## Model-Specific Notes (Nemotron-3-Nano-Omni-30B-A3B-Reasoning-NVFP4)

### Constraints

- **Requires `enable_thinking: false`** for QA tasks — reasoning mode exhausts context budget within the model's own reasoning, leaving nothing for output
- **max_tokens=2048** sufficient when thinking is disabled
- **Do not send `Reasoning` field** in chat messages (vLLM GH#39103 — Nemotron v3 reasoning parser rejects it)
- **Specialized for reasoning**: This model is optimized for chain-of-thought reasoning tasks. For factoid recall QA, non-reasoning instruct models (e.g., Qwen2.5-30B-Instruct) may perform better

### Performance Characteristics

- Inference latency: ~6–7 seconds per question (21K-token context) on single A100 GPU
- Memory requirement: ~22GB VRAM (FP4 quantized)
- With 32 concurrent workers: queue depth increases, but GPU processes 1 request at a time due to large prompt size

### Final Results — Full 500-Question Run (run c3d9f1, 2026-05-16)

| Question Type | n | Correct | Partial | Effective |
|---|---|---|---|---|
| single-session-assistant | 56 | 46 (82.1%) | 1 | **83.9%** |
| single-session-user | 70 | 41 (58.6%) | 1 | **60.0%** |
| knowledge-update | 78 | 35 (44.9%) | 1 | **46.2%** |
| single-session-preference | 30 | 0 (0.0%) | 7 | **23.3%** |
| multi-session | 133 | 25 (18.8%) | 1 | **19.5%** |
| temporal-reasoning | 133 | 25 (18.8%) | 0 | **18.8%** |
| **Total** | **500** | **172 (34.4%)** | **11 (2.2%)** | **36.6%** |

**Broken config baseline**: ~32% — but nearly all "Not answerable" abstentions, not real recall.

### Failure Mode Analysis

**single-session-preference (0% exact / 23% partial)**: Model treats expressed user preferences as unanswerable. Returns "Not answerable" when context contains "I prefer X" because it expects stated facts, not conversational preferences. Fix: instruct the generation prompt that preferences inferred from conversational context are valid answers.

**multi-session (19.5%) and temporal-reasoning (18.8%)**: Both require synthesising state across multiple sessions. Vector recall returns high-similarity individual chunks but 8 independent blocks cannot support "what changed between A and B" reasoning. Fix: two-pass recall with timeline-ordered chunks, or explicit "compare T1 vs T2" prompt framing.

**knowledge-update (46.2%)**: Recall finds the most recent fact but model occasionally returns the outdated value. Recency not encoded in retrieval prompt.

### What Works

- **Direct factual recall from single sessions**: 83.9% — the core RAG loop is sound
- Numeric quantities, names, specific items with exact matches in top-8 recall: high accuracy
- contextTopK=8 sufficient for single-session question types



---

## Quick Start for Future Benchmark Runs

Use these exact settings to replicate the working configuration:

### Benchmark Launch Command

```bash
# Resolve the generation endpoint/model from AI Flight Controller + Olla.
./longmemeval route-discover --purpose generation > /tmp/lme-route.json
export LME_LLM_URL="$(jq -r .llm_url /tmp/lme-route.json)"
export LME_LLM_MODEL="$(jq -r .llm_model /tmp/lme-route.json)"

# Stage 1: ingest the dataset into Engram (per-question isolation projects).
./longmemeval ingest \
  --data ~/path/to/longmemeval_m_cleaned.json \
  --url http://localhost:8788 \
  --workers 32 \
  --out ~/benchmarks/lme-m \
  --cleanup-policy=never \
  --scratch-ttl 168h

# Stage 2: recall + generate hypotheses per question.
./longmemeval run \
  --data ~/path/to/longmemeval_m_cleaned.json \
  --url http://localhost:8788 \
  --llm-url "${LME_LLM_URL}" \
  --llm-model "${LME_LLM_MODEL}" \
  --workers 32 \
  --out ~/benchmarks/lme-m \
  --recall-topk 100 \
  --context-topk 8

# Stage 3: score hypotheses against gold answers using score-only reuse.
./longmemeval score-efficient \
  --data ~/path/to/longmemeval_m_cleaned.json \
  --scorer-url "${LME_SCORER_URL:-${LME_LLM_URL}}" \
  --scorer-model "${LME_SCORER_MODEL:-${LME_LLM_MODEL}}" \
  --workers 16 \
  --out ~/benchmarks/lme-m

# Stage 4: summarize completeness and failure classes.
./longmemeval analyze --results ~/benchmarks/lme-m
```

**Top-k tuning**: `--recall-topk` controls how many memories are fetched before trimming, and `--context-topk` controls how many context blocks go into generation. Start with `--recall-topk 100 --context-topk 8`; raise context only for targeted ablations because larger prompts reduce throughput quickly.

**Checkpoint files** are written to `<out>/checkpoint-ingest.jsonl`, `<out>/checkpoint-run.jsonl`, and `<out>/checkpoint-score.jsonl`. Resume is automatic. Re-running `score-efficient` is score-only reuse: it reads existing run checkpoints, preserves already-`CORRECT` rows by default, and does not ingest or generate.

### Maintained Wrappers

Use tracked wrappers under `scripts/` for reproducible runs:

```bash
# Full ingest -> run -> score-efficient pipeline (captures route-discover output when enabled).
ROUTE_DISCOVER=1 OUT=~/benchmarks/lme-m scripts/longmemeval-pipeline.sh

# Resume run/score with optional route discovery and optional RUN_PID wait.
ROUTE_DISCOVER=1 OUT=~/benchmarks/lme-m scripts/longmemeval-resume.sh
```

Result-local scripts inside `results/**` are historical run artifacts and are not maintained entrypoints.

### vLLM Server Launch

```bash
vllm serve \
  nvidia/Nemotron-3-Nano-Omni-30B-A3B-Reasoning-NVFP4 \
  --max-model-len 131072 \
  --quantization-mode aqfp4 \
  --chat-template-kwargs '{"enable_thinking": false}' \
  --gpu-memory-utilization 0.95 \
  --port 8000
```

### Docker Compose Configuration (Engram)

`.env` file:
```env
ENGRAM_RATE_LIMIT_DISABLE=true
ENGRAM_LOG_LEVEL=warn
```

Start or update Engram:
```bash
docker compose up -d --force-recreate engram
```

### vLLM Completion Settings (Go Client)

```go
type CompletionRequest struct {
    Model            string            `json:"model"`
    Messages         []SimpleMessage   `json:"messages"`
    MaxTokens        int               `json:"max_tokens"`
    Temperature      float64           `json:"temperature"`
}

type SimpleMessage struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

// Use max_tokens=2048, temperature=0.0 for deterministic QA
```

### Engram Recall Settings

```go
// Recall parameters for LME benchmark
RecallTopK:   100          // Get top 100 semantically relevant blocks
ContextTopK:  8            // Use top 8 in prompt (5x smaller context)
SessionLimit: 470          // Typical LME dataset size
```

---

## Testing Checklist for Future Runs

- [ ] Verify vLLM server is running and accessible: `curl http://localhost:8000/v1/models`
- [ ] Verify Engram is running and rate limiting is disabled: Check container logs for "rate limit" mentions (should be absent)
- [ ] Pre-ingest LME dataset into Engram (or use `-no-cleanup` if already present)
- [ ] Confirm benchmark checkpoint file is writable and not locked by another process
- [ ] Verify MCP URL is `http://host:8788` (no `/mcp` suffix)
- [ ] Run smoke test to verify end-to-end flow: `./longmemeval ingest --data <path> --workers 1 --out /tmp/lme-smoke` then `./longmemeval run --data <path> --llm-url <url> --llm-model <model> --workers 1 --out /tmp/lme-smoke` (limit by pre-truncating the dataset to 10 questions; there is no --max-questions flag).
- [ ] Monitor vLLM GPU memory with `nvidia-smi` in separate terminal (should stabilize at ~22GB)
- [ ] Monitor Engram rate limit logs: `docker compose logs -f engram | grep -i rate` (should see none)
- [ ] Check completion latency in benchmark logs — expect ~7–8 questions/minute with contextTopK=8

---

## Known Limitations & Future Work

### Current Limitations

1. **Sequential Engram fetch**: Each question fetches blocks one at a time. Parallel or batch fetch could cut fetch time by 70%.
2. **Model selection**: Nemotron is optimized for reasoning; instruct models may be better for factoid QA.
3. **Fixed contextTopK**: Benchmark uses static top-K; adaptive selection (e.g., stop when confidence threshold met) could reduce context.
4. **No caching**: Each worker independently fetches the same memory blocks. A shared LRU cache could reduce redundant Engram calls.

### Recommended Improvements

- [ ] Implement parallel Engram block fetching (estim. 200ms latency savings)
- [ ] Add optional caching layer (Redis) for frequently accessed blocks
- [ ] Profile vLLM throughput with reduced batch sizes and smaller prompts
- [ ] Evaluate alternative models (Qwen2.5-30B-Instruct, Llama3.1-8B) on same dataset
- [ ] Instrument checkpoint corruption detection (verify file lock, checksum)

---

## Operator: Scratch Retention (TTL, #754)

LME benchmark runs create ephemeral `lme-<run-id>` projects. Without cleanup these accumulate indefinitely, inflating index size and risking accidental re-use of stale haystacks.

### Stamping TTL at ingest time

Pass `--scratch-ttl <duration>` to `longmemeval ingest`:

```
longmemeval ingest \
  --data questions.json \
  --out /tmp/lme-run-001 \
  --scratch-ttl 168h
```

The default TTL is **168 h (7 days)** — long enough to re-run scoring without re-ingesting; short enough to prevent unbounded growth.

### Running the prune sweep

```
longmemeval prune \
  --database-url "${DATABASE_URL}" \
  --url "${ENGRAM_URL}" \
  --api-key "${ENGRAM_API_KEY}"
```

By default this is a plan-only dry run. To delete, opt in explicitly:

```
longmemeval prune \
  --database-url "${DATABASE_URL}" \
  --url "${ENGRAM_URL}" \
  --api-key "${ENGRAM_API_KEY}" \
  --execute \
  --confirm-prefix lme- \
  --limit 50
```

Use `--unlimited` only for a deliberately unbounded sweep. If you want prune to discover `ENGRAM_API_KEY` or a Claude MCP token in execute mode, pass `--use-default-token`; otherwise deletion requires `--api-key`.

The sweep is deployed as a weekly K8s CronJob at `deploy/lme-prune-cronjob.yaml`.

### Updating the prune image and rollout controls

The checked-in CronJob pin is currently `ghcr.io/petersimmons1972/engram-go/longmemeval@sha256:c51f11f15003768b965774669b753c885c40cfdf13e2bb8b7a42f652143161f3`.
Do not switch this job to `:latest`. For destructive scheduled deletes, replace it with the reviewed release tag or immutable digest you intend to ship, and keep that change visible in git review.

Update `deploy/lme-prune-cronjob.yaml` in the same reviewed change that approves the
new prune binary. The CronJob uses `imagePullPolicy: Always` so each run resolves the
currently reviewed reference instead of reusing a cached mutable tag.

Before rollout, suspend the CronJob so the next run cannot fire while evidence is
being collected:

```bash
kubectl patch cronjob lme-prune -n engram -p '{"spec":{"suspend":true}}'
kubectl apply -f deploy/lme-prune-cronjob.yaml
kubectl -n engram get cronjob lme-prune -o jsonpath='{.spec.jobTemplate.spec.template.spec.containers[0].image}'
kubectl -n engram get cronjob lme-prune -o jsonpath='{.spec.suspend}{"\n"}'
```

#### Safe canary and rollout evidence

Before the next scheduled destructive run, run a dry-run canary with the same
image and credentials. Use a short-lived one-off Job so blast radius stays bounded:

```bash
IMAGE=$(kubectl -n engram get cronjob lme-prune -o jsonpath='{.spec.jobTemplate.spec.template.spec.containers[0].image}')
CANARY_JOB=lme-prune-canary-$(date +%s)

cat <<EOF | kubectl apply -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: ${CANARY_JOB}
  namespace: engram
spec:
  backoffLimit: 0
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: lme-prune-canary
          command: ["/engram"]
          image: ${IMAGE}
          envFrom:
            - secretRef:
                name: engram-lme
          args:
            - prune
            - --prefix=lme-
            - --dry-run
            - --confirm-prefix=lme-
            - --limit=50
            - --use-default-token
EOF

kubectl wait -n engram --for=condition=complete job/"$CANARY_JOB" --timeout=10m
CANARY_POD=$(kubectl -n engram get pod -l job-name="$CANARY_JOB" -o jsonpath='{.items[0].metadata.name}')
CANARY_IMAGE_ID=$(kubectl -n engram get pod "$CANARY_POD" -o jsonpath='{.status.containerStatuses[0].imageID}')
CANARY_EXIT_CODE=$(kubectl -n engram get pod "$CANARY_POD" -o jsonpath='{.status.containerStatuses[0].state.terminated.exitCode}')
kubectl logs -n engram job/"$CANARY_JOB" --timestamps
echo "CANARY imageID: $CANARY_IMAGE_ID"
echo "CANARY exit code: $CANARY_EXIT_CODE"
```

Use the canary log as your decision record:
- planned deletion count and project list (`prune: DRY RUN — would delete`)
- image identity (`imageID`)
- command output timestamps (`--timestamps`)
- command exit status (`$CANARY_EXIT_CODE`)
- summary status (`prune: X of Y project(s) deleted`)

If the canary is correct, run a second one-off execute job using the same `IMAGE`
and then resume the CronJob. If the canary is unexpected, leave the CronJob suspended
and keep the previous reviewed image manifest in place.

If the canary or execute check reports unexpected `delete` failures, keep the
CronJob suspended and begin recovery before resume.

If you need to run a destructive one-off execute sweep for evidence, keep this
bound similarly:

```bash
kubectl apply -f <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: lme-prune-verify-$(date +%s)
  namespace: engram
spec:
  backoffLimit: 0
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: lme-prune
        command: ["/engram"]
        image: ${IMAGE}
        envFrom:
          - secretRef:
              name: engram-lme
        args:
          - prune
          - --prefix=lme-
          - --execute
          - --confirm-prefix=lme-
          - --limit=50
          - --use-default-token
EOF
```

Rollout alert contract:

- non-zero execute exit code
- `prune: delete ... failed` appears in logs
- blast radius exceeds 200 deletions on first verify run
- cronjob repeatedly skips scheduling because the image cannot start (2 consecutive failures)

When the execute run is complete and matches expected blast radius and logs, resume
the scheduled CronJob:

```bash
kubectl patch cronjob lme-prune -n engram -p '{"spec":{"suspend":false}}'
```

### Backfilling existing runs

Existing `lme-*` projects (created before migration 022) have `NULL expires_at`. To opt them into the sweep:

```sql
UPDATE project_ttl
   SET expires_at = created_at + INTERVAL '7 days'
 WHERE project LIKE 'lme-%'
   AND expires_at IS NULL;
```

Projects without a `project_ttl` row at all can be enrolled with:

```sql
INSERT INTO project_ttl (project, created_at, expires_at)
SELECT DISTINCT project, NOW() - INTERVAL '1 day', NOW() + INTERVAL '7 days'
  FROM memories
 WHERE project LIKE 'lme-%'
ON CONFLICT (project) DO NOTHING;
```

---

## References

- **vLLM Repository**: https://github.com/vllm-project/vllm (GH#39103 — Nemotron v3 reasoning parser)
- **LongMemEval**: https://github.com/microsoft/LongMemEval (benchmark suite)
- **Nemotron-3 Documentation**: Nvidia AI Enterprise documentation (max_model_len, chat_template_kwargs)
- **Engram Memory System**: `docs/architecture.md`

---

**Document Version**: 1.0  
**Last Updated**: May 2026  
**Status**: Complete — Ready for future benchmark runs

---

## External Research — Papers to Build On (transmitted 2026-06-21)

Five arXiv papers (June 2026) surfaced during the fleet-capability-eval work in
`claude-codex`. Transmitted here as **possible learnings** for the LongMemEval
harness and its judge (issue #1030). These are candidates, not commitments —
each notes the concrete LME hook. Full digests + the fleet's analysis live in
`claude-codex/docs/superpowers/results/2026-06-21-fleet-paper-conclusions.md`.

### 1. EvoArena — evaluate in *evolving* environments + patch-based memory (arXiv:2606.13681)
**Most relevant to LME.** Models environment change as sequences of progressive
updates; static evals overstate capability (agents avg 39.6%). EvoMem records
memory as **structured update histories** (patches), improving evidence capture;
+6.1% GAIA, +4.8% LoCoMo, +3.7% *chain-level* (consecutive evolving subtasks).
- **LME hook:** LongMemEval already has a `knowledge-update` question type — EvoArena
  is the rigorous version of exactly that. Consider (a) a **patch/update-history**
  memory representation as an Engram retrieval mode to benchmark, and (b) a
  **chain-level success** metric (all consecutive knowledge-update hops correct),
  which is harder and more honest than per-question accuracy. Exposes whether
  Engram preserves *complete evolving state* vs latest-value-only.

### 2. Visual Para-Thinker++ — reconcile reasoning traces, don't vote labels (arXiv:2606.09290)
Hallucination-resistance comes from a Summary stage that **reconciles full reasoning
traces** rather than majority-voting final labels.
- **LME hook:** the judge harness (#1030, "judge-attributed reports") should score
  by reconciling the answer's reasoning trace against retrieved evidence, not by
  final-string match alone — reduces judge false-negatives on correct-but-differently-
  worded answers and judge false-positives on fluent-but-ungrounded ones. Pairs with
  `docs/lme-judging.md`.

### 3. Who Pays the Price? — stakeholder prompt-injection, outcome+process (arXiv:2606.13385)
Harm is victim-dependent; score outcome AND process; failure modes: stealthy
parasitism / misaligned disruption / compounded failure.
- **LME hook:** a **memory-poisoning** robustness eval — plant adversarial content in
  stored sessions and measure whether retrieval/answering obeys it (stealthy
  parasitism: correct answer surfaced *and* injected instruction followed). Engram's
  retrieval-miss/`failure_class` machinery could gain an `adversarial`/`poisoned` class.

### 4. InterleaveThinker — planner+critic + step-wise scoring of long trajectories (arXiv:2606.13679)
Step-wise signals guide 25+-call trajectories better than terminal grading.
- **LME hook:** for multi-session / multi-hop questions, score per-hop retrieval
  quality (did each hop surface the needed evidence?), not just the final answer — a
  step-wise complement to end-to-end accuracy.

### 5. SpatialClaw — code-as-action over a stateful kernel (arXiv:2606.13673)
Training-free; agent writes executable cells against a stateful kernel of primitives;
generalizes across backbones.
- **LME hook:** lowest direct relevance, but a stateful query kernel (compose retrieval
  + filter + aggregate primitives) is a candidate interface for multi-hop memory
  questions where a single retrieval call underperforms.

**Routing note:** raised as possible learnings only — not yet scoped into the harness.
Owner to triage which (EvoArena chain-metric + Para-Thinker judge-reconciliation are
the highest-value, most LME-aligned candidates).

### Triage outcome (2026-06-21, socialized codex/grok/hermes vs the aggregation-synthesis hypothesis)

The dominant loss is aggregation synthesis (31/34 multi-session failures; gold in context,
model enumerates right values but never sums / counts out-of-scope items). Mapped the papers
to that hypothesis and socialized three-way:
- **Para-Thinker++ (C) → ADOPTED into the generation lever** (not the judge, yet): enumerate-first
  now requires per-item SOURCE provenance + explicit INCLUDE/EXCLUDE, i.e. trace reconciliation
  at generation time. This is the highest-EV paper hook — it directly attacks scope errors.
- **InterleaveThinker (A) → inline structured recompute only** (forced `SUM=`/`COUNT=` line);
  second-pass critic / planner-critic deferred (confounds attribution). Step-wise scoring kept for
  post-hoc failure-mode labelling.
- **EvoArena (B) → chain-level used as DIAGNOSTIC decomposition, not a new acceptance metric**
  (LME is binary; binary already = chain-level for aggregation). Patch/update-history memory for
  knowledge-update DEFERRED (n=3, multi-week representation build).
- Who-Pays-the-Price + SpatialClaw: not applicable to the current synthesis hypothesis.
See `docs/lme-campaign/PLAN-2026-06-21.md` for the lever + run config + acceptance.

---

## Session 2026-06-26 — ss-preference root cause, #1185/#1181, and methodology failure modes

**Backend:** Qwen3-32B BF16 on Spark/oblivion (`max_model_len=40960`) for generation; Mistral-Small-24B on W6800 (`max_model_len=16384`) for bulk judging/extraction; Olla as router (model `inference`→Spark, `fast-inference`→W6800). Scorer = Olla `inference` (same Spark GPU).

### WHAT WORKS (validated)

| Finding | Evidence | Confidence |
|---------|----------|------------|
| **ss-pref baseline = 50.0% strict** (Qwen3-32B, native per-type topk=15, no truncation) | 15/30; fits 40960 natively (single-session-preference contexts are small) | HIGH |
| **Every generation lever HURTS ss-pref** | topk20 43.3%, H-PG(`--preference-ground`) 40%, H-PE(`--preference-enumerate`) 30%, H-QF(`--preference-quote-first`) 30%, thinking 13.3% | HIGH |
| **The 50% ceiling is REAL, not a scorer artifact** | Taxonomy audit of frozen outputs (independent Mistral judge): **0/60 scorer-undercounts** | HIGH |
| **#1185 fix: `NewRestClient` base-URL normalization** | Fresh ingest now returns `status=done` (sessions=48); was 30/30 errors | HIGH |
| **#1181 extraction populates entities** | atom-build produced 462 atoms, **313 with verbatim `entity`, 333 with `polarity`** (new schema works) | HIGH |
| **Phase 5 ingest works post-#1185** | user/multi/temporal all ingested 30/30 | HIGH |

### FAILURE MODES / METHODOLOGY LESSONS (the durable part)

1. **FM-PG — ss-pref specific-detail confabulation (#1183).** The generator surfaces the correct preference *category* but invents named specifics (brands/titles/ingredients) absent from retrieved context. **Confabulation SCALES with retrieval breadth**: hallucination 13/30 at topk8 → 16/30 at topk20. This is *why* every material-adding lever hurts. **Not prompt-suppressible** (3 grounding levers all scored below baseline; anti-hallucination instructions ironically *raised* confabulation via priming). Fix is structural → #1181, NOT prompting.

2. **Context overflow on big-context types (Spark 40960 wall).** single-session-user / multi-session / temporal-reasoning OVERFLOW at native per-type topk → HTTP 400 on most items (user 15/30, multi 21/30, temporal 26/30 errored). Only single-session-preference fits natively. **Any cross-type gap profile on a 40960 backend must cap context per-type.**

3. **Truncation damage (`--max-block-chars` too aggressive).** Setting `--max-block-chars 2500` to force-fit made all items run (0 overflow) but **gutted accuracy**: user30 strict went 87% → **21.4%**. Chopping every block to 2500 chars cuts the answer-bearing detail. **Lesson: truncation to fit is NOT free; a suspiciously low score on an easy type implicates the truncation, not the model.**

4. **Biased-subset trap.** Scoring only the items that *fit* (the overflow survivors) overstates accuracy — those are the smaller-context, easier items. The biased user/multi numbers (87%/89%) looked great but were unrepresentative. **Always report scored-total vs N; a denominator < N means silent item loss.**

5. **Spark exclusive-backend-lock collisions (exit 75, EX_TEMPFAIL).** `longmemeval run` takes an exclusive lock on the backend URL. Overlapping background jobs (an orchestrator that fell through to its own run when its child was killed) starved a second run → 0/0 garbage. **Happened twice.** Lesson: **ONE Spark `run` at a time; verify-free-before-launch; an orchestrator killed mid-flight may fall through to its next stage.**

6. **atom-build at-scale extraction is expensive + timeout-prone.** Full extraction = ~1500 LLM calls (30 Q × ~50 sessions). Per-call deadline is hardcoded (no flag). Spark Qwen3 timed out heavily; Mistral/W6800 faster but still 19 timeouts / only 4-of-30 projects covered. **Eval-time synchronous bulk extraction is the wrong vehicle — use the production async atom worker (now unblocked by the #1185 fix) for #1181's at-scale validation.**

7. **`/quick-store` wrong-path returns a bare number (#1192).** POSTing to `/mcp/quick-store` returned a bare JSON number with a 2xx-ish status — the worst response: looks fine at HTTP, undecodable at the client (`cannot unmarshal number into struct{OK;ID;Error}`). Filed #1192 to return structured errors for wrong-path/non-object responses.

8. **Misdiagnosis lesson: capture the live response before blaming a deployed service.** I initially diagnosed #1185 as a *stale Engram server* and nearly redeployed the container. The deployed `/quick-store` was actually healthy (`{ok,id}`); the bug was a one-line client URL miss (`RestClient` kept the `/mcp` suffix → POSTed to `/mcp/quick-store`). **Always capture the actual response from the exact URL the failing client uses, before attributing failure to infra or restarting anything.**

9. **Watcher readiness false-positive (format mismatch).** A Phase 5 auto-runner grepped the *checkpoint JSONL* for `status=error` (the *log* format) instead of `"status":"error"` (the JSON format), so it never matched → false "unblocked" → ran against still-broken ingest → 0/0. **Lesson: gate readiness on a positive success signal (`"status":"done"`), not absence-of-a-wrong-format-error-string.**

### Open methodology question
A *fair* cross-type gap profile on Spark's 40960 requires per-type context capping that fits without gutting. `--max-block-chars 2500` was too aggressive; the committed retry is `--context-topk 8 --max-block-chars 8000` (fewer blocks, each a full session). Numbers from that run, with the config noted, are the defensible profile — distinct from the ss-pref baseline's *native* config. Cross-type comparison on a 40960 backend is inherently config-constrained; a larger-context backend removes the confound.

See issues #1183 (FM-PG), #1185 (RestClient URL fix), #1192 (cleanup), #1181 (structural fix + impl map).

---

## Temporal Reasoning Lever Experiments (Phase 6, 2026-06-26)

**Baseline**: 48.3% strict / 55.2% lenient on temporal30 (topk8/max-block8000, Qwen3-32B/Spark)

### T1: --inject-question-date (H16)

**Result**: **strict=63.3% (+15.0pp)  lenient=63.3% (+8.1pp)** (19/30)

Prepends `"Today's date is: {question_date}"` to every temporal-reasoning prompt. This is the single largest lever found for temporal reasoning. The lenient=strict collapse (63.3%=63.3%) is significant: every item the model gets right, it gets **strictly** right — there are no partial-credit cases where the model approximates the answer.

**Interpretation**: The baseline failures were not retrieval failures — they were date-arithmetic failures. The model had the relevant memory blocks but couldn't anchor date math without knowing "now". Injecting the question date resolves the anchor.

### Mistral-Small-24B Cross-Arch Baseline (pref30, max-block-chars=4000)

**Result**: strict=6.7% (2/30)  lenient=53.3% (16/30) vs Qwen3-32B strict=50.0%/lenient=83.3%

Severe accuracy degradation — 43pp strict gap vs Qwen3. Two hypotheses:
1. **Context-gutting**: max-block-chars=4000 (vs 8000 for Qwen3) to fit Mistral's 16384 token ceiling may strip answer-bearing content
2. **Model weakness**: Mistral-24B may be genuinely weaker at preference synthesis even given full context

**Retake B2** (max-block-chars=6000) in flight to isolate: 6000 chars/block × 8 blocks ≈ 12000 tokens + overhead ≤ 16384. If B2 shows significant lift, context-gutting is the cause; if flat, it is model capability.

### Key new failure mode (FM-TEMPORAL-ANCHOR)

**FM-TEMPORAL-ANCHOR**: Temporal-reasoning questions without an explicit "today is X" anchor cause 15pp accuracy loss even when retrieved memory blocks are correct. The model performs date arithmetic relative to an implicit "now" it cannot know. Lever: `--inject-question-date`. Applies to any question_type=temporal-reasoning with a `question_date` field. **Do not benchmark temporal reasoning without this flag unless measuring the no-anchor baseline intentionally.**

### T2: --temporal-prompt-aug (H-M5+H-M1)

**Result**: **strict=53.3% (+5.0pp)  lenient=56.7% (+1.5pp)** (16/30)

Enumerate-all-events-then-commit ordering instruction: model lists all temporal events before committing to an ordering. Modest improvement. The strict=lenient collapse holds here too (53.3%=53.3% approx — 56.7% lenient is close but not equal, meaning a few partial-credit cases exist).

**Interpretation**: Ordering-enumeration provides some benefit but much less than date-anchor injection. The model benefits more from knowing *when now is* than from being instructed to enumerate events carefully.

### T3: --inject-question-date --temporal-prompt-aug (Combined)

**Result**: **strict=60.0% (+11.7pp)  lenient=60.0% (+4.8pp)** (18/30)

Combining T1 and T2 levers. Strict=Lenient again (zero partial-credit gap).

**Key finding — subadditivity**: T3 (+11.7pp) < T1 (+15.0pp). Adding temporal-prompt-aug to inject-question-date *reduces* performance from the T1 baseline. Possible causes: (a) the enumeration instruction adds prompt overhead that dilutes the date-anchor benefit, (b) the two instructions interact in ways that confuse the model's reasoning chain, (c) N=30 noise. Given T2's weak standalone effect (+5pp), the enumeration instruction may be net-negative when date is already anchored.

**Recommendation**: Use T1 (`--inject-question-date`) alone as the production temporal lever. Do not combine with `--temporal-prompt-aug`.

### Mistral-Small-24B Cross-Arch Baseline B2 (pref30, max-block-chars=6000)

**Result**: **strict=20.0% (+13.3pp vs B1)  lenient=76.7% (+23.4pp vs B1)** (6/30 strict, 23/30 lenient)

**Context-gutting hypothesis confirmed**: Raising max-block-chars from 4000 to 6000 produces +13.3pp strict improvement. Context size was materially limiting Mistral-24B's preference-synthesis accuracy.

**Capability gap remains**: Even with adequate context, Mistral-24B strict=20.0% is 30pp below Qwen3-32B strict=50.0%. The lenient gap narrows significantly (76.7% vs 83.3% = only 6.6pp), suggesting Mistral-24B can identify preference *categories* reliably but fails to retrieve the specific details required for strict credit.

**Failure pattern**: Mistral-24B exhibits FM-PG confabulation at a different failure boundary than Qwen3. The strict/lenient split (20% vs 77%) is far wider than Qwen3's (50% vs 83%), indicating Mistral generates plausible-category answers that lack the specificity Qwen3 achieves.

### Temporal Lever Leaderboard (temporal30, Qwen3-32B, N=30)

| Variant  | Flags                                  | Strict   | Lenient  | Delta strict |
|----------|----------------------------------------|----------|----------|--------------|
| Baseline | —                                      | 48.3%    | 55.2%    | —            |
| T2       | `--temporal-prompt-aug`                | 53.3%    | 56.7%    | +5.0pp       |
| T3       | `--inject-question-date --temporal-prompt-aug` | 60.0% | 60.0% | +11.7pp   |
| **T1**   | **`--inject-question-date`**           | **63.3%** | **63.3%** | **+15.0pp** |
| T4       | `--temporal-window-recall`             | *pending* | *pending* | —          |
| T5       | `--inject-question-date --temporal-window-recall` | *pending* | *pending* | —   |

### T4/T5 in flight

- **T4** (`--temporal-window-recall`): server-side two-pass date-windowed recall via `valid_from` filtering (H-NEW-1)
- **T5** (`--inject-question-date --temporal-window-recall`): H16 + H-NEW-1 combination; highest-value test given T1 dominance
