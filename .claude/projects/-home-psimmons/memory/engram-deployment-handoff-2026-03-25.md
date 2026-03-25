---
name: Engram Deployment Quality & Cohesion Implementation Handoff
description: Progress on Engram deployment plan — phases 1-2 complete, phases 3-5 pending
type: context
Category: active-work
---

# Engram Deployment Quality & Cohesion — Session Handoff

**Date**: 2026-03-25
**Status**: Phase 1-2 COMPLETE, Phase 3-5 pending
**Plan**: `/home/psimmons/.claude/plans/iterative-yawning-lightning.md`

## Completed Work

### Phase 1: TDD for Deployment ✅

**GitHub Issues Filed:**
- #42: httpx missing from requirements.txt → silent NullEmbedder fallback in Docker
- #43: Ollama should be required service in docker-compose.yml (currently commented out)
- #44: OLLAMA_URL=http://localhost:11434 incorrect for Docker networking (should be http://ollama:11434)

**Test Suite Created:**
- `tests/test_embedder_resolution.py` — comprehensive tests for embedder factory
  - `test_create_embedder_ollama_returns_ollama_type` — verifies no silent fallback
  - `test_null_embedder_not_returned_for_named_provider` — catches dependency issues
  - `test_null_embedder_only_for_none_provider` — correct behavior for BM25-only mode
  - All tests PASSING in Docker after fix

**Bug Fix:**
- Added `httpx>=0.24` to requirements.txt
- Tested in Docker container: OllamaEmbedder now instantiates correctly
- Commit: 92ff386

### Phase 2: Linting Strategy ✅

**Linting Configuration:**
- Expanded ruff rules in pyproject.toml: added B (bugbear), UP (pyupgrade), RUF (ruff-specific)
- Created `.pre-commit-config.yaml` to enforce ruff on every commit
- Added mypy type checking config to pyproject.toml
- Added pytest-cov to dev dependencies for coverage measurement

**Development Tools:**
- Created `Makefile` with quality targets:
  - `make lint` — run ruff checks
  - `make typecheck` — run mypy type checking
  - `make test` — run pytest
  - `make test-postgres` — run tests against Docker Postgres
  - `make quality` — run all checks in sequence

**Commit:** d0c23bd

## Current Repository State

**Git Log:**
```
d0c23bd chore: expand linting, add pre-commit hooks, add mypy type checking, add Makefile
92ff386 fix: add httpx to requirements.txt to fix silent Docker deployment failure
```

**Staging Area:** Clean (all changes committed)

**Git Branch:** main (ready for next phase)

## Remaining Work (Phases 3-5)

### Phase 3: Docker Module Packaging (NEXT)

**Docker Compose Rework:**
- Make Ollama REQUIRED service (currently commented out in docker-compose.yml)
- Fix OLLAMA_URL from `http://localhost:11434` to `http://ollama:11434` (Docker internal DNS)
- Add `ollama-init` service (profile: init) to pull nomic-embed-text on first run
- Add explicit `container_name` attributes for clean naming
- Structure: three required services (engram, postgres, ollama) + optional services

**GPU Override Files (to create):**
- `docker-compose.nvidia.yml` — NVIDIA CUDA with GPU reservation
- `docker-compose.amd.yml` — AMD ROCm with /dev/kfd and /dev/dri devices + rocm image
- `docker-compose.host-ollama.yml` — for users running system Ollama (uses host.docker.internal:11434)

**Non-Docker Files (to delete):**
- engram-server.service
- engram.sh
- setup.sh
- setup-remote.sh
- ALTERNATIVE-INSTALL.md

Related issues: #43, #44

### Phase 4: Memory as a Cohesive Thing

**Install/Uninstall Story** — the foundation of product trust:
- Create `src/engram/markdown_io.py` — markdown serialization for memory dump/ingest
- Add `memory_dump` MCP tool + CLI subcommand — export memories as .md files (UNINSTALL)
- Add `memory_ingest` MCP tool + CLI subcommand — import .md files as memories (INSTALL)
- Create **install-time snapshot zip** — proof that data was captured at ingest moment
  - Saves all ingested source files + manifest.json + memory snapshot in one zip
  - Dated: `memory-backup-2026-03-25T10-15-30Z.zip`
  - Users see this in their directory — tangible proof of data preservation
  - Even though it's stale immediately, the psychological impact is huge

**Commands Users Will See:**
```bash
engram ingest --project my-app --dir ./docs/
# Output:
# ✅ Ingested 47 files as memories
# ✅ Created snapshot: memory-backup-2026-03-25T10-15-30Z.zip
```

### Phase 5: Documentation

- **README.md** — complete narrative rewrite (problem → solution → architecture → tools → installation → limitations)
- **docs/installation.md** — GPU-specific guides (NVIDIA, AMD, Mac M-series, host Ollama)
- Both follow the approved "Engram Documentation Plan"

## Critical Insights

1. **Install/Uninstall Story**: Without `memory_dump`/`memory_ingest`, Engram has lock-in risk. Users MUST be able to export all memories as markdown and walk away with readable data.

2. **Safety Snapshot Zip**: The dated zip file is a "placebo for trust" — users feel safer knowing their starting data was captured. It gets stale immediately but that's OK; the perception matters.

3. **Named Docker Objects**: All Docker resources need meaningful names:
   - Containers: `engram-postgres`, `engram-ollama`, not auto-generated
   - Volumes: `pgdata`, `ollama-data` (already good)
   - Networks: explicit named network if creating one

4. **Docker as the Only Installation Path**: User directive: "we are dropping the non-Docker version." This simplifies the product story significantly.

## Files Modified This Session

| File | Action | Status |
|------|--------|--------|
| requirements.txt | Add httpx>=0.24 | ✅ Committed |
| pyproject.toml | Expand ruff, add mypy, add dev deps | ✅ Committed |
| tests/test_embedder_resolution.py | NEW comprehensive test suite | ✅ Committed |
| .pre-commit-config.yaml | NEW pre-commit hooks | ✅ Committed |
| Makefile | NEW quality targets | ✅ Committed |

## Next Session Instructions

1. **Start Phase 3**: Docker module packaging
   - Read the plan file: `/home/psimmons/.claude/plans/iterative-yawning-lightning.md`
   - Rewrite docker-compose.yml (Ollama required, fix OLLAMA_URL)
   - Create GPU override files (nvidia, amd, host-ollama)
   - Delete 5 non-Docker files

2. **Verify Quality**:
   ```bash
   cd ~/projects/engram
   make quality    # run lint, typecheck, test
   git log --oneline -10
   ```

3. **After Phase 3**: Move to Phase 4 (memory_dump/memory_ingest + snapshot zip)

4. **After Phase 4**: Move to Phase 5 (documentation)

## GitHub Issues Status
- ✅ #42: FIXED (httpx added, commit 92ff386)
- ⏳ #43: PENDING (docker-compose.yml rework, Phase 3)
- ⏳ #44: PENDING (OLLAMA_URL fix, Phase 3)

## Plan Reference
Full implementation plan with all details: `/home/psimmons/.claude/plans/iterative-yawning-lightning.md`

---

**Owner**: Claude Code
**Session**: 20260325-030508 → continuing
**Next Action**: Phase 3 Docker Module Packaging
