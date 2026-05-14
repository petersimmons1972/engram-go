---
name: ENGRAM_EMBED_DIMENSIONS=0 required for vLLM embedding models
description: Set ENGRAM_EMBED_DIMENSIONS=0 when using vLLM as embedding backend — vLLM rejects the dimensions parameter that engram sends when it's non-zero
type: feedback
originSessionId: e45f0d22-2a97-42f5-b66a-cd6c2fc0e80a
---
When pointing engram-go/engram-reembed at a vLLM embedding endpoint (as opposed to Ollama), set `ENGRAM_EMBED_DIMENSIONS=0`.

**Why:** engram sends `{"dimensions": N}` in the embedding API request when `ENGRAM_EMBED_DIMENSIONS > 0`. vLLM rejects this parameter with HTTP 400. With 0, engram skips sending the parameter entirely and vLLM returns the model's native output dimensions.

**How to apply:** Any time the embedding model is on vLLM (not Ollama), ensure `.env` has `ENGRAM_EMBED_DIMENSIONS=0`. If you see HTTP 400 floods in engram-reembed logs, this is the likely cause — it will also trip Olla's circuit breaker for that endpoint.
