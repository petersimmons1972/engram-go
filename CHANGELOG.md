# Changelog

All notable changes to engram-go are documented here.

---

## [Unreleased] — v3.3.0

### Reliability (PR #611-fix2)

- **Async embed on store (architecturally enforced):** `memory_store` and `memory_store_batch` return after the DB write completes (~10ms), regardless of embed pool state. Chunks are stored with NULL embeddings and the existing reembed worker backfills them asynchronously. New Prometheus counter `engram_store_embed_async_total` tracks call volume on the async path. Rollback: set `ENGRAM_STORE_EMBED_MODE=sync` to restore inline embedding without redeploying.
- **Configurable recall embed timeout:** The bounded timeout on the embed query during recall is now configurable via `ENGRAM_EMBED_RECALL_TIMEOUT_MS` (default: 500ms). On expiry, recall falls through to BM25+recency immediately — degraded but fast. New Prometheus counter `engram_recall_embed_timeout_total` tracks how often this fallback fires. **Ops note:** the embed-failure log line was downgraded from WARN to INFO (expected behavior under saturation); if you have alert rules matching `"embed query failed"` at WARN level, update them to watch `engram_recall_embed_timeout_total` instead.
- **Observability:** Two new Prometheus counters (`engram_store_embed_async_total`, `engram_recall_embed_timeout_total`) enable operators to distinguish async-store throughput from recall-embed pressure in dashboards.

---

## [Unreleased] — v3.2.0

### Security (PR #594)
- **Bearer auth on `/setup-token` and `/metrics`:** Added Authorization header requirement to prevent token enumeration and metrics exposure from unauthenticated clients.
- **SSRF expansion with DNS resolution:** Implemented `netutil.ValidateUpstreamURL()` to block private/reserved IP ranges not only by address but by hostname resolution, preventing attacker-controlled DNS to bypass SSRF guards (#549).
- **Session API key hash isolation:** Changed session storage from plaintext bearer to HMAC-SHA-256 digest, reducing exposure window if database is dumped (#433).
- **X-Real-IP spoof regression test:** Added regression test for header spoofing — correctly honours X-Forwarded-For only when `ENGRAM_TRUST_PROXY_HEADERS=1` is set (#255).
- **0.0.0.0 startup warning:** Added security notice when binding to all interfaces; operators must confirm reverse proxy authentication is in place (#550).
- **ALLOW_RFC1918 env removed:** Replaced with `ENGRAM_SETUP_TOKEN_ALLOW_RFC1918` — explicit opt-in for Docker deployments on private bridge IPs.
- **LITELLM_API_KEY scrubbed:** Secret unset from process environment after read, reducing /proc/self/environ exposure window (#139, #141, #250, #549).

### Reliability (PR #599, #596)
- **Worker panic recovery + metric:** Background workers (audit, weight tuner, entity extraction) now recover from panics and log with structured error context; new `background_worker_panics_total` Prometheus metric tracks frequency (#559).
- **Request-ID propagation:** Added per-request correlation ID middleware; all logs and tool responses carry `request_id` for end-to-end tracing (#140).
- **TouchSession coalescing:** Implemented 30-second coalescing window for session DB writes — prevents unbounded goroutine spawning when clients make rapid requests to the same session (#553).
- **Graceful shutdown WaitGroup:** Added WaitGroup to wait for background workers to exit before closing database connection; 15-second timeout prevents hanging on unresponsive workers (#559).
- **embedDegraded atomic flag:** Upgraded from startup-only to real-time detection — `/health` probe loop every 30s updates atomic flag so embedding can recover without restart (#565).
- **Exponential-backoff retry on transient embed failures:** Memory store/recall now retry embedding with exponential backoff (3 attempts, 100ms→1.6s) when LiteLLM returns transient errors (5xx, timeout), with fallback to BM25+recency.

### Hygiene (PR #593)
- **pprof side-effect import gated:** Build-tagged `_ "net/http/pprof"` so import has no effect unless explicitly enabled with `ENGRAM_PPROF=1` env var; `--healthcheck` honors `--port` flag (#544).
- **html/template for SVG:** Switched from `text/template` to `html/template` in memory_export_all to prevent XSS when SVG metadata contains user-controlled content.
- **safepath relative resolution:** Enhanced `safepath.Resolve()` to block `..` traversal and symlinks; validates all paths stay within `--data-dir` (#530).
- **SQL builder comment guards:** Added `--` comment markers to prevent SQL injection via user-controlled strings in WHERE clauses.

### CLI (PR #595, #598)
- **`--healthcheck` honors `--port`:** Fixed healthcheck probe to use parsed `--port` flag, not hardcoded environment variable (#544).
- **`--dry-run` redacts bearer:** Preview mode now shows `Bearer ***` instead of plaintext API key in stdout (#136).
- **`--rate-limit-rps` precedence:** When both `--rate-limit` (deprecated) and `--rate-limit-rps` are set, `--rate-limit-rps` wins; warning logged to clarify precedence (#560).
- **instinct flag.Parse:** Ensured instinct CLI correctly parses all defined flags; fixed flag registration order issues.
- **`--format=json` on engram-setup:** Added JSON output mode for scripting; `--offline` mode works without network access (#541).
- **`--offline` mode:** New flag disables network requests (no LiteLLM probe, no Ollama fetch); used for dry-run, health checks, and scripting without infrastructure.
- **`--healthcheck` Authorization header:** Probe now includes `Authorization: Bearer <ENGRAM_API_KEY>` when key is set, preventing denial-of-service via authenticated endpoint probe failures.

### Documentation (PR #597, #600, #601)
- **Dual Docker Compose profiles:** Added `docker-compose.local.yml` for 100% local Ollama-only setup. Default profile remains hybrid (LiteLLM external). Both profiles share identical PostgreSQL backend and MCP tool set — swap without migration.
- **Deployment notes:** New `docs/deployment-notes.md` documents Ollama vs LiteLLM trade-offs, Infisical integration, embedding model selection, and scaling guidance.
- **Operations runbook:** New `docs/runbook.md` covers every Prometheus metric with interpretation, thresholds, diagnostics, and fixes.
- **Command reference:** New `cmd/README.md` documents six binaries (starter, engram, engram-setup, engram-eval, instinct, benchmark) with usage, frequency, and typical scenarios.
- **Architecture documentation:** New `docs/architecture.md` covers goroutine lifecycle, context propagation, shutdown sequences, and failure modes.
- **Contributing updates:** Added Rust toolchain section for `reembed-rs/` development. Generalized AI-PR policy to permit any AI-assisted development (Claude Code, Copilot, etc.) with labeling and depth-of-review as control, not prohibition.
- **README overhaul:** Rewritten Quick Start covering both Docker profiles, environment variable table, RFC1918 setup guidance, and clear section dividers.
- **Generalized AI-PR policy:** Permits any AI-assisted development with `ai-generated` label; three rounds of adversarial review (correctness, coverage, structural) before merge. `severity/blocker` findings block merge; `severity/nice-to-have` tracked as issues.
- **Go 1.25 standardization:** Updated all `.github/workflows` and `go.mod` to target Go 1.25; set `GOFLAGS=-v` in CI to surface build details.
- **`/setup-token` endpoint contract:** Documented request/response format, rate limiting (3 calls per 5 min), bearer auth requirement, and RFC1918 exemption.
- **Environment namespace convention:** Documented ENGRAM_* / LITELLM_* / ANTHROPIC_* / POSTGRES_* intentional coexistence and why they're separate.

### Validation (PR #600)
- **handleQuickStore/Recall input limits:** Enforced maximum content size (100 KB), project name length (255 chars), and tag count (50) to prevent unbounded allocation.
- **ENGRAM_CLAUDE_TOOL_TYPE env:** New validation for tool type selection (summarize/consolidate/rerank); invalid values logged as warnings with fallback to safe default.

### Runtime Configuration (PR #557)
- **SIGHUP runtime config reload:** New signal handler reloads feature flags without restart: `ENGRAM_CLAUDE_SUMMARIZE`, `ENGRAM_CLAUDE_CONSOLIDATE`, `ENGRAM_CLAUDE_RERANK`, `ENGRAM_LOG_LEVEL`. Changes atomically visible to all tool handlers via `RuntimeConfig` struct with atomic fields. Logged with changed keys.

---

## [3.1.0] — 2026-04-20 (Previous Release)

### Added
- **`POST /quick-store`** — sessionless REST endpoint that stores a memory directly using Bearer auth, without requiring an active SSE session. Enables hook scripts (Claude Code `PreCompact`, shell scripts, CLI tools) that cannot perform the SSE handshake. Accepts `{"content","project","tags","importance"}`, returns `{"ok":true,"id":"..."}`.

### Fixed
- **docker-compose postgres startup** — Chainguard image ENTRYPOINT already includes `postgres`; the duplicate `command: postgres -c max_connections=300` was causing unhealthy restarts on container recreate. Fixed to `command: -c max_connections=300`.

### Performance
- Raised MCP rate limit and Postgres `max_connections` to 300 for bulk operation scenarios.

---

## [3.1.0] — 2026-04-20

### Added
- **Open-brain integration** — new relation types, structured failure class tracking, embedder registry, and evaluation harness. Enables pluggable embedding backends and richer graph relationships between memories.

### Changed
- Documentation refresh — full narrative prose rewrite across all doc pages, updated SVGs, GPL v3 license footer.

---

## [3.0.0] — 2026-04-20

### Added
- **`memory_ask`** — RAG tool that answers questions directly from stored memories using Claude, returning a grounded prose response alongside the source memory IDs (`feat: add memory_ask RAG tool`, closes #180).
- **`memory_aggregate`** — query lane for structured failure class analysis. Counts retrieval misses by class (`vocabulary_mismatch`, `aggregation_failure`, `stale_ranking`, etc.) to feed retrieval benchmarking (#197).
- **GraphRAG improvements** — entity extraction graph, simplified `memory_explore` front door, evaluation harness for graph recall quality (Tasks 1–6, #259).
- **Document query** — `memory_query_document` + `RecallWithinMemory` for searching within a single stored document (Phase A5).
- **Handle mode default** — `detail=handle` is now the default return mode, reducing context token usage by returning lightweight references instead of full content (A6).
- **CI postgres service** — integration tests now run against a real PostgreSQL service container in CI (closes #205).
- **Explicit CI permissions** — `permissions: contents: read` on all workflows (closes code-scanning alert #1).

### Security
- Replaced `math/rand` with `crypto/rand` in minhash (closes #237).
- Validated argv allowlist before `syscall.Exec` to prevent argument injection (closes #236).
- Gated `/debug/pprof` behind `ENGRAM_PPROF=1` env var — no longer exposed by default (closes #239).
- Added `USER nonroot` to Dockerfile — container no longer runs as root (closes #238).
- Allow-listed configured Ollama host in SSRF guard to prevent host header attacks (#242).
- Upgraded `mmds.org` doc links to HTTPS (#240).

### Fixed
- `rowToChunk` panicked on NULL embedding during Ollama outage recovery; now handled gracefully.
- Re-embed worker now activates at startup when chunks have NULL embeddings from a prior Ollama outage.
- Entity backend goroutine leak; extraction enqueue refactored to be non-blocking.
- False-positive contradiction edges in `memory_sleep` consolidation eliminated.
- `tsvector` codec registration so `memory_diagnose` works correctly.
- 14 adversarial-review blockers across boundary conditions, error handling, and UTF-8 safety (#213–226).
- QA Army campaign — 16 findings remediated (#258).
- `charCap` enforced as a hard limit in `QueryDocument` (closes #195).
- TTL and mutex on upload registry; atomic write paths throughout.

---

## [2.0.0] — 2026-04-10

### Added
- **Claude advisor strategy** — Claude now participates in consolidation decisions: near-duplicate merge, contradiction detection, and `memory_sleep` cycle improvements use Claude for higher-quality judgment rather than pure heuristics.
- Infisical machine-identity secret injection via `cmd/starter` — no shell, no external HTTP dependency at container boot.

---

## Version Policy

`MAJOR.MINOR.PATCH`

- **Major** — breaking changes to the MCP tool interface or database schema requiring migration.
- **Minor** — new tools, new endpoints, or significant capability additions that are backwards compatible.
- **Patch** — bug fixes, security patches, and performance improvements.

Versions are tagged in git (`v3.1.0`, etc.) and the `engram-go:latest` Docker image is rebuilt on each release.
