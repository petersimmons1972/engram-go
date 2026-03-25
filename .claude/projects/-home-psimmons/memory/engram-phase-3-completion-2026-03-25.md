---
name: Engram Phase 3 Completion - Docker Module Packaging
description: Phase 3 complete - Docker module fully packaged with GPU support, Ollama required, networking fixed
type: context
Category: active-work
---

# Engram Phase 3: Docker Module Packaging — COMPLETE ✅

**Date**: 2026-03-25
**Status**: COMPLETE
**Commit**: 6d81eee

## What Was Done

### 1. Rewrote docker-compose.yml — Ollama Required

**Changes**:
- Ollama service uncommented (was optional, now REQUIRED)
- OLLAMA_URL corrected from `http://localhost:11434` to `http://ollama:11434` (Docker internal DNS)
- Three required services: postgres, engram, ollama (all auto-start)
- Added explicit `container_name` for clean naming:
  - engram-postgres
  - engram-ollama
  - engram-app
  - engram-open-webui
  - engram-test-postgres
  - engram-ollama-init

**Key Fixes (GitHub Issues)**:
- ✅ #43 FIXED: Ollama is now required service
- ✅ #44 FIXED: OLLAMA_URL uses Docker internal networking (http://ollama:11434)

### 2. Added Model Initialization Service

**New Service**: ollama-init
- Profile: init (opt-in, not auto-run)
- Purpose: Pull nomic-embed-text model on first setup
- Usage: `docker compose --profile init run --rm ollama-init`
- Key feature: Only runs after ollama service is healthy

### 3. Created GPU Support Overlay Files

**docker-compose.nvidia.yml** (NVIDIA CUDA GPU support)
- GPU reservation via `deploy.resources.reservations`
- Count: 1 GPU (configurable)
- Usage: `docker compose -f docker-compose.yml -f docker-compose.nvidia.yml up -d`

**docker-compose.amd.yml** (AMD ROCm GPU support)
- Image override: ollama/ollama:rocm
- Device access: /dev/kfd, /dev/dri
- Usage: `docker compose -f docker-compose.yml -f docker-compose.amd.yml up -d`

**docker-compose.host-ollama.yml** (System/Host Ollama)
- For users running Ollama natively on host
- Sets OLLAMA_URL=http://host.docker.internal:11434
- Disables Docker Ollama services (profiles: ["disabled"])
- Usage: `docker compose -f docker-compose.yml -f docker-compose.host-ollama.yml up -d`

### 4. Deleted Non-Docker Files

Removed 5 files (systemd/script-based installation, no longer needed):
- ✅ engram-server.service
- ✅ engram.sh
- ✅ setup.sh
- ✅ setup-remote.sh
- ✅ ALTERNATIVE-INSTALL.md

**Reason**: Docker is now the single installation path. All other installation methods are deprecated.

## Verification Results

| Check | Status | Detail |
|-------|--------|--------|
| `docker compose config` | ✅ | Base compose valid, services: postgres, engram, ollama |
| NVIDIA override | ✅ | Valid, GPU reservation present |
| AMD override | ✅ | Valid, ROCm image + devices |
| Host Ollama override | ✅ | Valid, services: postgres, engram only |
| Build dry-run | ✅ | Image builds successfully |
| Deleted files | ✅ | All 5 files removed |

## Implementation Details

### Ollama Networking
- Service-to-service communication uses DNS resolution by service name
- `http://ollama:11434` is the correct Docker internal address (not localhost:11434)
- open-webui also updated to use `http://ollama:11434`

### Health Checks
- postgres: pg_isready check every 5s
- ollama: curl /api/tags check every 10s
- ollama-init: waits for ollama service_healthy before pulling model
- engram: waits for postgres service_healthy (ollamaconnection has built-in retry logic)

### Container Naming
All Docker objects now have explicit names (no auto-generated IDs):
- Services (container_name)
- Volumes (pgdata, ollama-data, open-webui-data)
- Network (implicit, named engram_default)

## Next Steps: Phase 4 (Memory as a Cohesive Thing)

**What Phase 4 Requires**:
1. Create `src/engram/markdown_io.py` — markdown serialization for memory dump/ingest
2. Add `memory_dump` MCP tool + CLI subcommand — export memories as .md files (UNINSTALL)
3. Add `memory_ingest` MCP tool + CLI subcommand — import .md files as memories (INSTALL)
4. Implement install-time snapshot zip: `memory-backup-YYYY-MM-DDTHH-MM-SSZ.zip`
   - Contains: original source files + manifest.json + memory snapshot
   - Provides proof of data capture at ingest time (trust factor)

**Why Phase 4 Matters**:
Without dump/ingest, Engram has lock-in risk. Users must be able to export all memories as markdown and walk away with their data.

## Files Modified This Session (Phase 3)

| File | Action | Status |
|------|--------|--------|
| docker-compose.yml | REWRITE | ✅ Committed (6d81eee) |
| docker-compose.nvidia.yml | CREATE | ✅ Committed |
| docker-compose.amd.yml | CREATE | ✅ Committed |
| docker-compose.host-ollama.yml | CREATE | ✅ Committed |
| .gitignore | MODIFY | ✅ Committed (e3d6a97) |
| engram-server.service | DELETE | ✅ Committed |
| engram.sh | DELETE | ✅ Committed |
| setup.sh | DELETE | ✅ Committed |
| setup-remote.sh | DELETE | ✅ Committed |
| ALTERNATIVE-INSTALL.md | DELETE | ✅ Committed |

## Plan Reference

Full implementation plan with all phases: `/home/psimmons/.claude/plans/iterative-yawning-lightning.md`

---

**Owner**: Claude Code
**Worktree**: .worktrees/phase-3-docker-packaging
**Branch**: phase-3-docker-packaging
**Next Phase**: Phase 4 (Memory as a Cohesive Thing)
