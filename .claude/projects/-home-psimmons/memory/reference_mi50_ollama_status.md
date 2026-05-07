---
name: MI-50 ollama status — gfx906 unusable on stock images, custom container needed
description: MI-50 (Vega20/gfx906) cannot run ollama with GPU offload on either the host binary or the stock ollama/ollama:rocm Docker image; both fall back to CPU
type: reference
originSessionId: d78f1ef0-76f8-4af6-bca5-fb99755d3d8d
---
**Status as of 2026-05-07:** parked. The MI-50 (Instinct MI-50 / Vega20 / gfx906) on `precision.petersimmons.com:11436` falls back to CPU regardless of the ollama distribution.

**What was tried this session:**
1. Host binary `/usr/local/bin/ollama serve` with `HSA_OVERRIDE_GFX_VERSION=9.0.6` and `ROCR_VISIBLE_DEVICES=1` (later corrected to `=0` — MI-50 is GPU 0 per `rocm-smi`). Result: `load_backend: loaded CPU backend from libggml-cpu-skylakex.so`, `offloaded 0/49 layers to GPU`. CPU fallback.
2. Official `ollama/ollama:rocm` Docker image with `--device /dev/kfd --device /dev/dri/renderD129`, `--group-add 109 --group-add 44`, `ROCR_VISIBLE_DEVICES=0`, `HSA_OVERRIDE_GFX_VERSION=9.0.6`. Image *can* see the device (picks up GPU GUID `GPU-c6f2214172dc76bb`) but the runner crashes during HIP/ROCm backend load: `failure during GPU discovery ... runner crashed`. CPU fallback.

**Diagnosis:** gfx906 (Vega20) was deprecated from upstream ROCm rocBLAS/Tensile builds. Both the host ollama binary and the official `ollama/ollama:rocm` Docker image ship rocBLAS without gfx906 kernels compiled in. `HSA_OVERRIDE_GFX_VERSION` only spoofs the device version string — it doesn't make missing kernels appear.

**Path forward — easy:** the agent's last finding before being stopped was that **`ollama/ollama:0.6.8-rocm` (older image tag) detects gfx906 successfully** and reports 16 GB VRAM available. Older ollama ROCm images ship a rocBLAS that still includes gfx906 kernels. Pin the MI-50 service to that tag and skip the custom-container work entirely. Verification was not completed (agent was stopped before running an actual generation), so confirm tok/s + `size_vram > 0` before relying on it.

**Path forward — hard (only if `0.6.8-rocm` regresses):** build a custom container with rocBLAS+Tensile compiled targeting `gfx906`, plus a matching ollama/llama.cpp binary linked against that rocBLAS. Reference: `ROCm/rocBLAS` repo with `-DAMDGPU_TARGETS=gfx906`. Non-trivial — Tensile autotuning takes hours.

**VRAM correction:** earlier in this session the MI-50 was assumed to be 32 GB. The 0.6.8-rocm probe reported 16 GB. The MI-50 ships in both 16 GB and 32 GB HBM2 variants — confirm which is installed before sizing models. If 16 GB, qwen3-coder:30b (~18 GB footprint) does NOT fit cleanly and the MI-50 caps at 14B-class.

**Implication for routing:** until a custom container is built, the MI-50 is treated as **unavailable for inference**. Local-coder budget caps at what the W6800 (32 GB, gfx1030 — works) and RX 7900 XT (20 GB, gfx1100 — works on engram-ollama) can host with margin per the clean-fit rule. qwen3-coder:30b runs cleanly on the W6800 (precision:11435) at 39 tok/s with 100 % offload.

**Don't repeat:** do not retry stock ollama on the MI-50. Do not invest more than a few minutes proving CPU fallback again. If the topic comes up, point at this memory and at the custom-container path.
