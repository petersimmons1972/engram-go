---
name: engram-go cutover
description: engram = engram-go v2.0 exclusively. Python repo archived. Stack managed from engram-go/. 166 memories intact.
type: project
Category: project
originSessionId: 845e03b0-6ff5-4144-85b9-20d8619236a6
---
# engram-go is canonical (2026-04-10)

**"engram" means engram-go v2.0.** Python project archived.

## Current state
- `engram-go-app` runs on `localhost:8788`
- Stack managed from `~/projects/engram-go/docker-compose.yml` (postgres + ollama + engram-go-app)
- Volumes external: `engram_pgdata`, `ollama_ollama_storage`
- Python repo `petersimmons1972/engram` archived on GitHub
- Active repo: `petersimmons1972/engram-go`
- 166 memories confirmed intact

## Architecture
- `internal/embed/ollama.go` — Ollama client
- `internal/search/engine.go` — Store/Recall/Connect/Consolidate/Verify/MigrateEmbedder
- `internal/search/score.go` — CompositeScore (0.50 cosine + 0.35 BM25 + 0.15 recency × ImportanceBoost)
- `internal/summarize/worker.go` + `internal/reembed/worker.go` — background tickers
- `internal/mcp/` — EnginePool (lazy per-project engines), SSE server, timing-safe auth, 18 tool handlers
- `Dockerfile` — Chainguard multi-stage (go:latest → static:latest)

## If it breaks
- Pre-migration backup: `engram-go/backups/pre-migration-20260410-121245.sql.gz`
- Rollback path no longer exists — Python repo archived
- Restore via `docker compose exec postgres pg_dump -U engram engram | gzip > /tmp/engram-backup-$(date +%Y%m%d-%H%M%S).sql.gz`
