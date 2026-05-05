# Operations Runbook

This page documents Engram's observable metrics and how to interpret them. Each section covers one metric, its meaning, when to investigate, diagnostic commands, and typical fixes.

---

## Metrics Overview

Engram exports Prometheus metrics on port 8788 at `/metrics`. The canonical set appears below. All metrics are prefixed `engram_`.

```bash
curl http://localhost:8788/metrics
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

**Contract:** Returns `{"status":"ok"}` when the server is ready. No authentication required.

```bash
curl http://localhost:8788/health
```

**Status codes:**
- `200 OK` — Server is healthy and ready
- `503 Service Unavailable` — Server is initializing or degraded

**When health is degraded:**

The server is starting up, migrations are running, or a component failed to initialize. Typically resolves within seconds. If it persists:

```bash
# Check startup logs
docker logs engram-go-app | tail -50

# Check database connectivity
docker exec engram-postgres pg_isready -U engram
```

---

## Setup-Token Endpoint

**Endpoint:** `GET /setup-token`

**Contract:** Returns the current bearer token, SSE endpoint URL, and server name. Localhost-only (RFC1918 addresses accepted in Docker). No authentication required.

```bash
curl http://localhost:8788/setup-token
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

## Common Issues & Solutions

| Issue | Symptom | Root Cause | Fix |
|-------|---------|-----------|-----|
| Embedding backlog | `engram_chunks_pending_reembed` >1000 | Ollama/LiteLLM slow/down | Restart embedding service, check network |
| High latency on queries | `/sse` takes >5s | Postgres slow, embedding timeout, or network | Check postgres logs, verify embedding service, tune timeouts |
| Session disconnections | High `engram_episodes_ended_by_reaper_total` | Network instability, firewall dropping long-lived connections | Check network, increase keepalive timeout |
| Panic in logs | `engram_worker_panics_total` >0 | Software bug | File GitHub issue with logs and `.env` settings |
| Setup-token returns 403 | Can't get token from outside localhost | Not in RFC1918 or Docker bridge | Use correct host IP, set `ENGRAM_SETUP_TOKEN_ALLOW_RFC1918=1` |

---

## Postgres Diagnostics

For deeper investigation:

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
