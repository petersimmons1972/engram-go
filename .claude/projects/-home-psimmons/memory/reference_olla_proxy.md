---
name: Olla local LLM proxy
description: OpenAI-compatible local model gateway at localhost:40114 with qwen3-coder and embedders; user prefers it for medium-tier coding work
type: reference
originSessionId: e43a9d4f-1d9c-483a-8828-b09b33374056
---
Olla is the local LLM proxy at `http://localhost:40114` (deployed on leviathan.petersimmons.com:40114). OpenAI-compatible API. Config at `~/projects/olla/olla-config.yaml`.

Models keyed via `id` field on `/olla/models`:
- `qwen3-coder:30b` — qwen3moe, 30.5B q4km, on `w6800` endpoint (priority 100). Use for code generation, code completion, programming.
- `qwen2.5-coder:14b-instruct-q6_K` — on `mi50` endpoint. Lighter coding fallback.
- `mxbai-embed-large`, `nomic-embed-text`, `snowflake-arctic-embed2`, `jina-embeddings-v4-text-retrieval-Q8_0` — embedders on `leviathan-7900xt` / `w6800`.
- `llama3.2:3b` — small text-gen on `leviathan-7900xt`.

Endpoints (priority order): precision (100, ollama on :11434), precision-mi50 (75, :11436), leviathan (50, :11434).

**User preference:** route medium-tier coding work (judgment-required, not pure transcription) through Olla rather than burning Anthropic compute on Sonnet. Direct shell-out: `xh POST localhost:40114/v1/chat/completions ...` or any OpenAI-compatible client.

**Caveat:** Claude Code's subagent dispatch is locked to Anthropic models — Agent tool can't route to Olla directly. Either shell out via Bash, or use Haiku as the cheapest Anthropic-tier fallback when subagent dispatch is required.
