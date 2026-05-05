# Changelog

All notable changes to engram-go are documented here.

---

## [Unreleased] — v3.2.0

### Documentation
- **Dual Docker Compose profiles:** Added `docker-compose.local.yml` for 100% local Ollama-only setup. Default profile remains hybrid (LiteLLM external). Both profiles share identical PostgreSQL backend and MCP tool set — swap without migration.
- **Deployment notes:** New `docs/deployment-notes.md` documents Ollama vs LiteLLM trade-offs, Infisical integration, embedding model selection, and scaling guidance.
- **Operations runbook:** New `docs/runbook.md` covers every Prometheus metric with interpretation, thresholds, diagnostics, and fixes.
- **Command reference:** New `cmd/README.md` documents six binaries (starter, engram, engram-setup, engram-eval, instinct, benchmark) with usage, frequency, and typical scenarios.
- **Contributing updates:** Added Rust toolchain section for `reembed-rs/` development. Generalized AI-PR policy to permit any AI-assisted development (Claude Code, Copilot, etc.) with labeling and depth-of-review as control, not prohibition.
- **README overhaul:** Rewritten Quick Start covering both Docker profiles, environment variable table, RFC1918 setup guidance, and clear section dividers.
- **VERSION file:** Added canonical version constant (3.1.0).

### Environment & Configuration
- **Makefile `init` target:** Now creates Docker volumes (`engram_pgdata`, `ollama_storage`) automatically, eliminating separate `docker volume create` step.
- **.env.example header:** Added namespace convention note documenting ENGRAM_*, LITELLM_*, ANTHROPIC_*, POSTGRES_* intentional coexistence.
- **docker-compose.yml:** Added inline documentation for RFC1918, LiteLLM endpoint requirement, ENGRAM_SETUP_TOKEN_ALLOW_RFC1918 behavior.

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
