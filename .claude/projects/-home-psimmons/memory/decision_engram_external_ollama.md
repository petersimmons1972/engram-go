---
name: Engram uses external Ollama
description: Engram switched from bundled Docker Ollama to external Ollama on leviathan with AMD GPU (2026-04-09)
type: project
Category: project
originSessionId: f3bd39be-e957-4489-afa7-cb9309dc7a88
---
Engram's docker-compose.yml no longer runs its own Ollama container. It points to `http://leviathan.petersimmons.com:11434` (set in `.env`).

**Why:** Leviathan already runs Ollama natively with AMD RX 7900 XT (20 GiB VRAM) via ROCm. Running a second Ollama inside Docker was redundant and was CPU-only (no GPU passthrough configured).

**How to apply:** If Engram embedding fails, check that the external Ollama is running on leviathan and has `nomic-embed-text` loaded. To re-enable the bundled container, see the commented-out block in `~/projects/engram/docker-compose.yml`.

**Files changed:** `~/projects/engram/.env`, `~/projects/engram/docker-compose.yml`
