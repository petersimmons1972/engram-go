---
name: engram-go multi-model embedding architecture
description: Future requirement to support multiple embedding models simultaneously in same chunks table at scale (billions of rows)
type: project
originSessionId: 6c6c81cb-73f5-4f0a-b6dd-dd1527971204
---
All models will be forced to the same vector dimensions (likely 1024) but may use different models (e.g. mxbai-embed-large vs a future model). Current architecture uses one model per project stored in project_meta.embedder_name. At billions of rows, need to handle:

**Why:** Encountered during April 2026 LME experiment — switching from mxbai-embed-large (1024 dims, 512 token ctx) to qwen3-embedding:8b (1536 dims, 8192 token ctx) broke the single-model assumption. Chunks created with large-context model became un-embeddable when switching back to small-context model.

**Current correct config:** qwen3-embedding:8b + ENGRAM_EMBED_DIMENSIONS=1024. The model natively outputs 1536 dims; MRL truncation via `"dimensions":1024` in the Ollama request produces 1024-dim vectors that fit in the existing vector(1024) column. Large context window (8192 tokens) handles long chunks. Do NOT set ENGRAM_EMBED_DIMENSIONS=1536 — that disables MRL truncation and 1536-dim vectors fail to insert.

**How to apply:** When designing any embedding migration, indexing strategy, or reembedder changes — account for the fact that different chunks in the same table may have been embedded with different models. The per-project embedder_name in project_meta is insufficient at scale; may need per-chunk model tracking. MRL truncation is the correct strategy for fitting large-dim models into smaller pgvector columns — this is already used in production.
