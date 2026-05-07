---
name: Local GPU model routing — Clearwatch / homelab
description: Authoritative map of which model runs on which GPU and why; covers MI-50, W6800, RX 7900 XT, Olla front-end, and pool isolation
type: reference
originSessionId: d78f1ef0-76f8-4af6-bca5-fb99755d3d8d
---
**Front-end:** Olla load balancer at `http://localhost:40114` (config: `~/projects/olla/config.local.yaml`, container `olla`). Path prefix for all proxied calls: `/olla/<provider>/...` — e.g. `/olla/openai/v1/embeddings`, `/olla/openai/v1/chat/completions`, `/olla/anthropic/v1/messages`. Olla does model-aware routing: a request for a model only available on one endpoint goes there; a model on multiple endpoints round-robins.

## Three-GPU local stack (final state 2026-05-07)

| GPU | Endpoint name in Olla | Backend URL | VRAM | Role | Models |
|-----|----------------------|-------------|------|------|--------|
| **W6800** (gfx1030) | `w6800` | `precision:11434` (Docker `ollama-w6800`) | 32 GB | Heavy coding + embedding | `qwen3-coder:30b` (25.3 GB loaded), `jina-embeddings-v4-Q8_0` (4.7 GB loaded) |
| **MI-50** (gfx906, Vega20) | `mi50` | `precision:11436` (Docker `ollama-mi50.service`) | 16 GB HBM2 | Coding only | `qwen2.5-coder:14b-instruct-q6_K` (13.4 GB loaded, ~30 tok/s, 100% offload) |
| **RX 7900 XT** (gfx1100, leviathan display card) | `leviathan-7900xt` | `engram-ollama:11434` (leviathan Docker) | 20 GB (display takes ~4 GB) | Backup embedding + small utility | `jina-embeddings-v4-Q8_0` (4.0 GB after KV tuning), `llama3.2:3b` (lazy-loaded) |

**Retired 2026-05-07:** the host-binary `ollama.service` on `precision:11435`. Stopped + disabled. Don't restart it — `precision:11434` Docker now serves all W6800 traffic.

## Pool isolation (per-GPU, never shared)

| Pool | Path | Owner |
|------|------|-------|
| W6800 | Docker volume `ollama_ollama_w6800` (`/var/lib/docker/volumes/ollama_ollama_w6800/_data`) | `ollama-w6800` container |
| MI-50 | `/var/lib/ollama-mi50/.ollama` on precision (Docker bind mount) | `ollama-mi50.service` container |
| 7900 XT | Docker volume `ollama_ollama_storage` on leviathan | `engram-ollama` container |

**Rule (load-bearing):** if a model is in a pool, it fits on that card. A delete on one endpoint only affects its own pool. Don't share pools — the failure mode is silent CPU fallback when a model in a shared pool happens to be requested via the wrong endpoint.

## Hard rules

1. **One pool per GPU, never shared.** See above. The 2026-05-07 incident: shared pool on precision meant deleting `qwen3-coder:30b` "from the MI-50" wiped it from the W6800 too. Per-pool storage prevents this.
2. **Jina pinned on W6800 and 7900 XT** — embedding traffic stays available even if one host is busy. MI-50 does NOT carry jina; it's coder-only.
3. **No CPU inference, ever.** Verify `size_vram > 0` before relying on a local model.
4. **Clean VRAM fit ≤80 %**, except MI-50 where pushing to ~90 % is acceptable (single-tenant by design).
5. **MI-50 needs `ollama/ollama:0.6.8-rocm` Docker image.** Newer ollama ROCm builds dropped gfx906 from rocBLAS; the 0.6.8-rocm tag is the known-good. Don't upgrade without re-verifying GPU offload.
6. **leviathan-7900xt KV tuning is intentional, not default.** `OLLAMA_NUM_PARALLEL=2`, `OLLAMA_FLASH_ATTENTION=1`, `OLLAMA_KV_CACHE_TYPE=q8_0`. Cut jina VRAM 8.9 GB → 4.0 GB. Defined in `~/projects/engram-go/docker-compose.local.yml` (committed `08a1f22`, merged engram-go PR #619 as `ff5d01bf`). Don't revert without confirming display headroom is still adequate.

## Task → GPU routing (default)

| Task class | GPU | Model |
|---|---|---|
| Trivial mechanical edits | 7900 XT | `llama3.2:3b` (small utility) |
| Medium code work (single-package, ≲5 files) | MI-50 | `qwen2.5-coder:14b-instruct-q6_K` |
| Heavy code work (multi-file synthesis) | W6800 | `qwen3-coder:30b` |
| Embeddings (Engram retrieval, semantic search) | W6800 OR 7900 XT (Olla round-robins) | jina-v4 |
| Multi-file architecture / security correctness | Cloud Sonnet | — |
| Strategic synthesis / architecture forks | Cloud Opus (rare, A1–A5 triggers) | — |

## Why this layout

- **W6800 (32 GB):** hosts qwen3-coder:30b cleanly + jina pinned. The biggest coder GPU.
- **MI-50 (16 GB):** caps at 14B-Q6 with ~84 % VRAM (just over the 80% rule, allowed because single-tenant). Below 14B isn't worth a dedicated GPU; above 14B exceeds VRAM.
- **7900 XT (20 GB):** display card with 4 monitors — minimal generation footprint by policy. KV-tuned jina + small utility leaves ~10 GB headroom.
- **Jina on two hosts:** embedding redundancy. If W6800 is doing 30B coder generation, jina traffic naturally lands on 7900 XT via Olla round-robin.

## Verifying state

```bash
# All three Olla endpoints + per-endpoint health
curl -s http://localhost:40114/internal/status/endpoints | python3 -m json.tool

# Olla model registry (which model is available where)
curl -s http://localhost:40114/olla/models | jq '.data[].id, .data[].olla.availability'

# What's loaded in VRAM right now on each backend
for ep in precision.petersimmons.com:11434 precision.petersimmons.com:11436 localhost:11434; do
  echo "=== $ep ==="
  curl -s http://$ep/api/ps | python3 -m json.tool
done
```
