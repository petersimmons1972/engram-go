---
name: Local LLMs must fit cleanly on a working GPU
description: Two hard rules — never run inference on CPU (including silent fallbacks), and only use models whose weights+KV+compute total ≤80% of the target GPU's VRAM
type: feedback
originSessionId: d78f1ef0-76f8-4af6-bca5-fb99755d3d8d
---
**Rule 1 — No CPU inference, ever.** Never invoke a local model that runs on CPU. This includes silent fallbacks where ollama logs `offloaded 0/N layers to GPU` and loads `libggml-cpu-*.so` despite a GPU being configured.

**Rule 2 — Clean VRAM fit with margin.** Pick a model whose weights + KV cache + compute graph total ≲ 80% of the target GPU's VRAM. Tight fits (95%+) cause OOM at long contexts and are not "clean fit." If the only GPU that fits a chosen model is broken or unsupported, pick a smaller model — do not run on CPU and do not fight the GPU stack mid-task.

**Why:** CPU inference is 5–10× slower and burns wall-clock time the user is paying for. Worse, it produces *plausible-looking output* that masks a broken GPU stack — so the failure goes undiagnosed. Tight VRAM fits OOM at long contexts after working fine on short prompts, hiding capacity problems until production. Both modes paper over real stack failures with degraded compute.

**How to apply:**
1. Before any local-model call, verify GPU offload by either: (a) `/api/ps` `size_vram > 0`, or (b) ollama logs `offloaded N/M layers to GPU` with N==M.
2. Before picking a model, compute weights+KV+graph and check ≤80% of the smallest VRAM on any GPU you might route to.
3. If GPU offload didn't happen, or the model doesn't fit cleanly, **stop and report** — do not retry on CPU and do not pick a tighter model "to make it fit."
4. Specific case: ollama service `ollama-mi50.service` on `precision:11436` silently falls back to CPU — bundled ROCm libs don't support gfx906 (MI-50 / Vega20). Until that service is migrated to `ollama/ollama:rocm` Docker image, treat the MI-50 as **unavailable**, and cap local model size at what the working 16 GB cards (RX 7900 XT, W6800) can host with margin (~13 GB total footprint = 14B-class at Q5 or 20B-class at MXFP4).
5. Cloud models (Sonnet/Opus/Haiku) are always fine — this rule applies only to local inference.
