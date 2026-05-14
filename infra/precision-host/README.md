# precision-host — Systemd Unit Files

Infrastructure-as-code for `precision.petersimmons.com` GPU services used by the engram embed pipeline.

## GPU Layout

| Device        | GPU                        | Architecture | VRAM  | Role                                |
|---------------|----------------------------|--------------|-------|-------------------------------------|
| card0 / renderD128 | AMD Radeon PRO W6800  | gfx1030      | 32 GB | Ollama inference (port 11435) + Infinity embedding (port 8005) |
| card1 / renderD129 | AMD Radeon VII (MI50) | gfx906       | 16 GB | Ollama inference only (port 11436)  |

## Services

### `ollama-w6800.service` (drop-in)

Modifies the native `ollama.service` install to run the W6800 on port 11435.

**Deployed path:** `/etc/systemd/system/ollama.service.d/mi50.conf`

> **Naming note:** The on-disk filename `mi50.conf` is legacy — this drop-in configures the W6800, not the MI50. The name is retained to avoid breaking systemd unit state.

- `ROCR_VISIBLE_DEVICES=0` → card0 = W6800 (gfx1030)
- `OLLAMA_HOST=0.0.0.0:11435`
- `OLLAMA_NUM_PARALLEL=8` (W6800 has enough VRAM for concurrent requests)

### `ollama-mi50.service` (standalone Docker unit)

Runs the MI50 (Radeon VII) in a Docker container for GPU isolation.

**Deployed path:** `/etc/systemd/system/ollama-mi50.service`

- Uses `--device /dev/dri/renderD129` → card1 = MI50 (gfx906)
- `ROCR_VISIBLE_DEVICES=0` inside the container (only one GPU exposed)
- `HSA_OVERRIDE_GFX_VERSION=9.0.6` — required for ROCm gfx906 compatibility
- `OLLAMA_NUM_PARALLEL=1` — MI50 has 16 GB VRAM; single request avoids OOM
- Port 11436 on host → 11434 in container
- Model pool isolated to `/var/lib/ollama-mi50/.ollama`
- Image: `ollama/ollama:0.6.8-rocm` (pinned; 0.6.8 fixed gfx906 GPU offload regression)

## Applying Changes

```bash
# Preview diff (no writes)
infra/precision-host/apply.sh --dry-run

# Apply with confirmation prompt
infra/precision-host/apply.sh

# Apply without confirmation (CI/automation)
infra/precision-host/apply.sh --no-confirm

# Target a different host
PRECISION_HOST=my-other-host.example.com infra/precision-host/apply.sh
```

`apply.sh` diffs remote vs local, creates a timestamped backup before overwriting, and automatically rolls back if any service fails to start.

## Engram Pipeline Role

```
engram-go-app (query)
    → LITELLM_URL=http://olla:40114/olla/openai
         → olla load balancer (leviathan:40114)
              → Infinity bge-m3 on leviathan:8004     (RX 7900 XT)
              → Infinity bge-m3 on oblivion:8003       (GB10 Grace-Blackwell)
              → Infinity bge-m3 on precision:8005      (W6800)

engram-reembed-w6800 → precision:8005  (Infinity, W6800)
engram-reembed-7900xt → leviathan:8004 (Infinity, RX 7900 XT)
engram-reembed-oblivion → oblivion:8003 (Infinity, GB10)

Ollama inference (coding/chat) — NOT in engram embed path:
    precision:11435 (W6800, native ollama.service)
    precision:11436 (MI50, Docker ollama-mi50.service)
```

## Version History

| Date       | Change |
|------------|--------|
| 2026-05-03 | Initial tracking in engram-go (issue #413) |
| 2026-05-07 | MI50 re-enabled after ollama/ollama:0.6.8-rocm fixed gfx906 GPU offload |
| 2026-05-07 | MI50 jina embedding removed — embedding traffic stays on W6800/7900XT only |
| 2026-05-08 | Switched embed model to BAAI/bge-m3 via Infinity (14x throughput vs vLLM/jina-v5) |
| 2026-05-13 | Unit files added to version control (issue #413) |
