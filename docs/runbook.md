# Operations Runbook

This page documents Engram's observable metrics and how to interpret them. Each section covers one metric, its meaning, when to investigate, diagnostic commands, and typical fixes.

---

## Metrics Overview

Engram exports Prometheus metrics on port 8788 at `/metrics`. The endpoint requires `Authorization: Bearer $ENGRAM_API_KEY`. The canonical set appears below. All metrics are prefixed `engram_`.

```bash
curl -H "Authorization: Bearer $ENGRAM_API_KEY" http://localhost:8788/metrics
```

---

## engram_chunks_pending_reembed

**Type:** Gauge

**Meaning:** Number of memory chunks waiting to be re-embedded. Normally zero or very small. Non-zero indicates:

- The re-embedder is busy or paused
- A new embedding model was just switched (re-embedding in progress)
- Ollama or LiteLLM is unreachable (embeddings are queued)

**When to investigate:**

- Stays >1000 for >10 minutes — your embedding service is too slow or down
- Grows continuously and never shrinks — re-embedder crashed or stalled

**Diagnostic commands:**

```bash
# Check the re-embedder container health
docker ps | grep engram-reembed
docker logs engram-reembed | tail -50

# Check if the embedding service is responding
curl -s http://localhost:11434/api/tags  # (Ollama local profile)
curl -s http://your-litellm:4000/v1/models  # (LiteLLM hybrid profile)

# Check database schema version
docker exec engram-postgres psql -U engram -d engram -c "SELECT version FROM schema_versions ORDER BY applied_at DESC LIMIT 1;"
```

**Typical fixes:**

- **Ollama not responding:** Restart Ollama — `docker restart engram-ollama`
- **LiteLLM not responding:** Verify network connectivity and URL in `.env`
- **Re-embedder container crashed:** Check logs and restart — `docker restart engram-reembed`
- **Model switching in progress:** Wait — re-embedding can take hours for large stores

---

## engram_episodes_ended_by_reaper_total

**Type:** Counter (cumulative, never resets)

**Meaning:** Total number of MCP episodes reaped (closed) by the automatic reaper. An episode represents one SSE connection from a Claude Code session. This counter increments every time a session is terminated (user closes IDE, network disconnects, timeout, etc.).

**When to investigate:**

- Counter increases rapidly (>5/minute) — indicates clients connecting and disconnecting frequently, unusual session churn, or network instability
- Steady baseline is fine; sudden spikes warrant investigation

**Diagnostic commands:**

```bash
# Get the current metric value
curl -s http://localhost:8788/metrics | grep engram_episodes_ended_by_reaper_total

# Check SSE connection logs
docker logs engram-go-app | grep "episode" | tail -20

# Monitor in real-time
watch -n 1 'curl -s http://localhost:8788/metrics | grep engram_episodes'
```

**Typical fixes:**

- **Normal operation:** Nothing to do. This is expected churn.
- **Abnormal churn:** Check Claude Code logs for connection errors. Network issues? Firewall dropping long-lived connections?

---

## engram_worker_panics_total

**Type:** Counter (cumulative, never resets)

**Meaning:** Total number of panics caught in background worker goroutines. Should be zero in production. A non-zero value indicates a bug was triggered and handled gracefully (the server did not crash, but something unexpected happened).

**When to investigate:**

- Any non-zero value in production — a bug was encountered

**Diagnostic commands:**

```bash
# Get the current metric value
curl -s http://localhost:8788/metrics | grep engram_worker_panics_total

# Get detailed panic logs
docker logs engram-go-app 2>&1 | grep -i panic
```

**Typical fixes:**

- **Report the panic:** File a GitHub issue with the panic stacktrace from the logs. Include your `.env` settings (no secrets).
- **Workaround:** If the panic is in background workers (reembed, consolidation), restarting engram-go-app may help. If it recurs, data corruption or memory corruption is possible — back up immediately.

---

## Health Checks

**Endpoint:** `GET /health`

**Contract:** Returns dependency health for PostgreSQL and the embedding router. No authentication required.

```bash
curl http://localhost:8788/health
curl http://localhost:8788/ready
```

**Status codes:**
- `GET /health` `200 OK` — dependencies are reachable, or the embedding router is in startup-degraded mode.
- `GET /health` `503 Service Unavailable` — PostgreSQL is unavailable, or a previously healthy embedding router is now unreachable.
- `GET /ready` `200 OK` — startup warmup is complete and the server is ready for traffic.
- `GET /ready` `503 Service Unavailable` — startup warmup is still in progress or embedding is not ready.

**When health is degraded:**

The server is starting up, migrations are running, or a component failed to initialize. In Kubernetes, start with `make status-k8s`; for local Docker, use `make status`. If it persists:

```bash
# Check startup logs
docker logs engram-go-app | tail -50

# Check database connectivity
docker exec engram-postgres pg_isready -U engram
```

---

## Setup-Token Endpoint

**Endpoint:** `GET /setup-token`

**Contract:** Returns the current bearer token, SSE endpoint URL, and server name. Localhost-only (RFC1918 addresses accepted in Docker). Requires `Authorization: Bearer <ENGRAM_API_KEY>`.

```bash
curl \
  --header "Authorization: Bearer $ENGRAM_API_KEY" \
  http://localhost:8788/setup-token
# {"token":"...","endpoint":"http://127.0.0.1:8788/sse","name":"engram"}
```

**Rate limit:** 3 requests per 5 minutes per IP. If you exceed it, you'll get a `429 Too Many Requests`.

**When to investigate:**

- Endpoint returns `403 Forbidden` — you're calling from outside localhost/RFC1918. This is intentional (security).
- Endpoint returns `429 Too Many Requests` — you've polled it too frequently. Call it once during setup, then use the token in your client config.

**Typical fixes:**

- **Inside Docker:** Use the container hostname or bridge IP (172.17.0.x)
- **Outside Docker on same machine:** Use 127.0.0.1 or localhost
- **Outside Docker on different machine:** Set `ENGRAM_SETUP_TOKEN_ALLOW_RFC1918=1` in the server's `.env`

---

## Database Connections

**Monitor:** Postgres `pg_stat_activity`

Engram opens a connection pool (default: 32 connections, configurable via `DATABASE_URL`). Heavy re-embedding can spike connection count. If connections exceed 100, either:

1. Increase the pool size in `docker-compose.yml` Postgres settings
2. Reduce `ENGRAM_REEMBED_CONCURRENCY` in `.env`

```bash
docker exec engram-postgres psql -U engram -d engram -c "SELECT count(*) FROM pg_stat_activity;"
```

---

## Memory Usage

**Monitor:** Docker container stats

```bash
docker stats engram-go-app engram-reembed
```

- **engram-go-app:** Typically 50–150 MB. Spikes during consolidation (memory_consolidate) or exploration (memory_explore).
- **engram-reembed:** Typically 100–200 MB. Spikes during bulk re-embedding.

Both have `mem_limit` set in `docker-compose.yml`. If either hits its limit, Docker kills the container. Check logs:

```bash
docker logs engram-go-app | grep -i "killed\|oom"
```

**Fix:** Increase `mem_limit` in `docker-compose.yml` and restart.

---

## Embedding Retry Exhaustion

**When to investigate:**

If embeddings fail after all retries, queries may degrade to BM25-only (no semantic vectors). This is graceful degradation, but search quality is lower.

**Diagnostic commands:**

```bash
# Check re-embedder logs for retry exhaustion
docker logs engram-reembed | grep -i "exhausted\|retries\|failed"
```

**Typical fixes:**

- **Ollama/LiteLLM is down:** Restart the service and monitor `engram_chunks_pending_reembed` until it returns to zero.
- **Network timeout:** Increase `ENGRAM_REEMBED_CONCURRENCY` or reduce `ENGRAM_REEMBED_BATCH_SIZE` to lighten the load.

---

## engram_db_pool_* (pgxpool saturation)

**A-2 sibling / #673**. The shared pgxpool exposes six gauges sampled every 5 seconds by `db.StartPoolMetricsSampler`:

- `engram_db_pool_acquired_conns` — currently in use
- `engram_db_pool_idle_conns` — sitting idle
- `engram_db_pool_total_conns` — acquired + idle
- `engram_db_pool_max_conns` — configured ceiling
- `engram_db_pool_acquire_count_total` — cumulative successful acquires
- `engram_db_pool_acquire_duration_seconds_total` — cumulative wall-time blocked acquiring

**Saturation signal**: `acquired_conns / max_conns` approaching 1.0 means callers will start blocking. The first thing to check when query latency rises.

**Diagnostic**:
```bash
# Live gauges
curl -s -H "Authorization: Bearer $ENGRAM_API_KEY" http://localhost:8788/metrics   | grep '^engram_db_pool_'

# Slow queries that may be holding connections
docker exec engram-postgres psql -U engram -d engram -c   "SELECT pid, now()-query_start AS age, state, substring(query,1,80) FROM pg_stat_activity WHERE state='active' ORDER BY age DESC LIMIT 10;"
```

If `acquired_conns` is near `max_conns` for sustained periods, the pool is too small or queries are too slow. Raise `MaxConns` in `internal/db/postgres.go:configureSharedPool` or fix the slow query (Postgres Diagnostics section below).

## engram_audit_drift_alerts_total

**#695**. Increments every time a canonical-query snapshot's RBO-vs-previous score drops below the configured `alert_threshold`. The companion slog.Error line gives the project + query + score; the metric gives a queryable signal for Grafana alerts.

**Investigate when**:
- Counter rate increases noticeably (sustained drift across multiple snapshots)
- A specific project's counter increments — recall ranking has shifted for that project

**Likely causes**:
- Embedding model changed (`memory_migrate_embedder` was run)
- Adaptive weights diverged (`memory_weight_tune` adjusted ranking inputs)
- Large bulk ingest changed the relative scoring of the audited query

**Diagnostic**:
```bash
# Recent snapshots for a specific canonical query
docker exec engram-postgres psql -U engram -d engram -c   "SELECT created_at, rbo_vs_prev, jaccard_at_5 FROM audit_snapshots WHERE query_id=<id> ORDER BY created_at DESC LIMIT 10;"
```

If drift is expected (post-migration), reset the baseline by deleting old snapshots for the affected queries. If unexpected, treat as a recall-quality regression and investigate via `memory_diagnose`.

## Embed Circuit Breaker State Transitions

**#676**. Every state transition is logged at INFO (HALF_OPEN, CLOSED) or WARN (OPEN). Search `journalctl -u engram-go` for `embed circuit breaker` to see the full transition history.

The companion gauge `engram_embed_circuit_state` (0=CLOSED, 1=HALF_OPEN, 2=OPEN) gives the current state for Prometheus alerts.

When OPEN: `memory_recall` degrades to BM25+recency until the next probe attempt. The log line includes `next_probe_at` so on-call knows when recovery will be tried automatically.

## Reembed Worker Healthchecks

**#672**. Each `engram-reembed-*` container now has a Docker HEALTHCHECK that probes Postgres reachability via `pg_isready`. `docker ps` shows healthy/unhealthy based on that probe.

**What this detects**:
- Network partition between reembed worker and engram-postgres
- Postgres unreachable / not accepting connections

**What this does NOT detect** (still — same caveat as before):
- A deadlocked worker that holds DB connections but no longer processes chunks. For that, watch `engram_chunks_pending_reembed` (see top of this runbook).

## Entity Extraction Drops

**Meaning:** When `ENGRAM_ENTITY_PROJECTS` is set and `ANTHROPIC_API_KEY` is present, entities are extracted from new memories asynchronously. If Claude API calls fail, extraction is skipped for that memory.

**Diagnostic commands:**

```bash
# Check for extraction errors in logs
docker logs engram-go-app | grep -i "entity\|extraction\|claude"

# Check if ENGRAM_ENTITY_PROJECTS is set
docker exec engram-go-app env | grep ENGRAM_ENTITY_PROJECTS
```

**Typical fixes:**

- **ANTHROPIC_API_KEY expired:** Rotate it in `.env` and restart
- **Rate limit hit:** Claude API is rate-limiting requests. Wait and retry.
- **Network issue:** Verify connectivity to api.anthropic.com

---

## Decay & Audit Workers

**Metrics:** Not directly exposed, but logged when they run.

**Frequency:**
- Importance decay: Every 8 hours (configurable: `ENGRAM_DECAY_INTERVAL`)
- Decay audit snapshots: Every 6 hours (configurable: `ENGRAM_AUDIT_INTERVAL`)
- Adaptive weight tuning: Every 24 hours (configurable: `ENGRAM_WEIGHT_INTERVAL`)

**When to investigate:**

```bash
# Check if workers are running
docker logs engram-go-app | grep -i "decay\|audit\|weight\|tuner"
```

These are background tasks. If they stall, memories will not age and retrieval weights will not adapt. Typically recovers on next container restart.

---

## Consolidation Cycle

**Meaning:** The `memory_consolidate` tool runs a near-duplicate detection pass across stored memories and merges them when appropriate. In large stores, this can take minutes.

**Monitor progress:**

```bash
docker logs engram-go-app | grep -i "consolidat"
```

**When to investigate:**

- Consolidation takes >30 minutes — your store is very large or system is slow
- Consolidation fails or reports errors — check logs

**Typical fixes:**

- **Let it finish:** Consolidation is safe to interrupt. If needed, restart the container and it will resume from where it left off.
- **Reduce memory store size:** Archive old memories or split into separate projects

---

## W6800 Canary

Use this sequence when validating the W6800-backed chat canary for LongMemEval or a small general share.

Verification:

1. Confirm the embedding backend is live and still probing `BAAI/bge-m3` successfully.
2. Confirm `llama3.1:8b` is installed in `engram-ollama`.
3. Run one short completion against `llama3.1:8b` and confirm it returns normally.
4. Expand traffic only after the canary remains stable under a small batch.

Rollback:

1. Stop sending canary traffic to the W6800 host.
2. Restore the previous chat backend.
3. Leave the embedding backend pinned on `BAAI/bge-m3`.
4. Recheck `ollama list` in the container and confirm the chat model you want is resident.

If the Ollama container gets tight on memory, let `qwen3-coder:30b` fall out first. The embed backend should stay on `BAAI/bge-m3`.

## Common Issues & Solutions

| Issue | Symptom | Root Cause | Fix |
|-------|---------|-----------|-----|
| Embedding backlog | `engram_chunks_pending_reembed` >1000 | Ollama/LiteLLM slow/down | Restart embedding service, check network |
| High latency on queries | `/sse` takes >5s | Postgres slow, embedding timeout, or network | Check postgres logs, verify embedding service, tune timeouts |
| Session disconnections | High `engram_episodes_ended_by_reaper_total` | Network instability, firewall dropping long-lived connections | Check network, increase keepalive timeout |
| Panic in logs | `engram_worker_panics_total` >0 | Software bug | File GitHub issue with logs and `.env` settings |
| Setup-token returns 403 | Can't get token from outside localhost | Not in RFC1918 or Docker bridge | Use correct host IP, set `ENGRAM_SETUP_TOKEN_ALLOW_RFC1918=1` |

---

## Postgres Backups

**A-2 / #658**. The `postgres-backup` sidecar container (defined in `docker-compose.yml`)
takes a nightly `pg_dump -Fc` of the `engram` database to `./backups/`, with
14-day retention. Combined with `synchronous_commit=on` on the primary, this
covers two failure modes:

- **Crash / OOM / power**: in-flight commits are durable (sync_commit=on).
- **Operator error / logical corruption / volume loss**: the most recent
  nightly dump is restorable.

**Verify a backup exists**:

```bash
ls -la backups/engram-*.dump | tail -5
```

If no recent file, check the sidecar logs:

```bash
docker logs engram-postgres-backup --tail 30
```

**Force a backup now** (e.g. before a migration or `memory_delete_project`):

```bash
make backup-now
```

**Restore drill** — do this ONCE before trusting the backup path. Restores into
a throwaway database; does not touch primary.

```bash
make backup-restore-drill
```

The drill creates `engram_restore_test`, restores the most recent dump, runs a
sanity query, and drops the test DB. If it fails, your backup chain is broken
and `synchronous_commit=on` is the only durability you actually have.

**What this does NOT protect against**: logical/semantic corruption that
propagates into the backup window before being noticed. If a bug writes garbage
embeddings or a bad `memory_correct` overwrites a real memory, the nightly
dumps will faithfully capture the corrupted state. Within 14 days every backup
contains the bug. Detecting silent semantic corruption requires either WAL
archiving with PITR (deliberately out of scope per A-2 advisory) or
application-level audit logging of mutations.

---

## Postgres Diagnostics

For deeper investigation:

> ⚠️ **Heredoc stdin warning:** `docker exec` without `-i` silently discards
> stdin — multi-statement heredocs appear to succeed (exit 0) but never run.
> Use `docker exec -i ...` for inline SQL, or preferably use
> `scripts/psql-exec.sh <file.sql>` which uses `docker cp` + `psql -f`
> (unambiguous, no stdin). See issue #646.

```bash
# Top 10 slowest queries
docker exec engram-postgres psql -U engram -d engram -c "
  SELECT query, calls, total_time, mean_time
  FROM pg_stat_statements
  ORDER BY mean_time DESC
  LIMIT 10;
"

# Table sizes
docker exec engram-postgres psql -U engram -d engram -c "
  SELECT schemaname, tablename, pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) as size
  FROM pg_tables
  WHERE schemaname NOT IN ('pg_catalog', 'information_schema')
  ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;
"

# Check for table bloat or missing indexes
docker exec engram-postgres psql -U engram -d engram -c "
  SELECT * FROM pg_stat_user_indexes
  WHERE idx_scan = 0
  ORDER BY pg_relation_size(relid) DESC;
"
```

See [Operations](operations.md) for backup, security, and data portability details.
