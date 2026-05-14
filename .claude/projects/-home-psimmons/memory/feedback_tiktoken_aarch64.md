---
name: tiktoken-rs cache path fix for aarch64 vLLM
description: TIKTOKEN_RS_CACHE_DIR must point to /root/.cache/tiktoken-rs-cache (XDG default) for openai-harmony to find vocab files on ARM64
type: feedback
originSessionId: e45f0d22-2a97-42f5-b66a-cd6c2fc0e80a
---
On aarch64 (ARM64) Linux with vLLM and openai-harmony: `TIKTOKEN_RS_CACHE_DIR` must be set to `/root/.cache/tiktoken-rs-cache` (the XDG default subdirectory), NOT a custom path like `/root/.tiktoken-rs-cache`.

**Why:** Known ARM64 bug in openai-harmony v0.0.8 — the Rust extension ignores non-default cache paths in some process contexts. The XDG path (`~/.cache/tiktoken-rs-cache`) is the only path that works reliably.

**How to apply:** Whenever configuring vLLM with gpt-oss models on ARM64 (GB10, Raspberry Pi, etc.), set:
```
TIKTOKEN_RS_CACHE_DIR=/root/.cache/tiktoken-rs-cache
TIKTOKEN_ENCODINGS_BASE=/root/.cache/tiktoken-rs-cache
```
And mount/seed the named volume with `o200k_base.tiktoken` and `cl100k_base.tiktoken` from `https://openaipublic.blob.core.windows.net/encodings/`. The `docker exec` test may pass even if the server process fails — they are not equivalent test environments.
