---
name: Deterministic Python beats LLMs for structured transcription
description: When mapping is total and the input is structured, write the script — don't route to a coding model
type: feedback
originSessionId: e43a9d4f-1d9c-483a-8828-b09b33374056
---
For total, structured data transformations (e.g., transcribing a Python dict literal into Go struct literals, or a flat config into a typed map), use a Python script with `json.dumps`/`ast.literal_eval` — not a coding-model subagent or Olla/Qwen routing.

**Why:** During the Clearwatch Go port (#4708), the user explicitly suggested routing medium-tier coding to Olla (qwen3-coder:30b at localhost:40114). The strict-transcription parts (446-row vendor_facts seed from Python VENDORS, vendor → keyword search-config map) ran in milliseconds with zero hallucinations via deterministic Python. Routing them to a model would have introduced both latency and risk of silent field omission. The model-tier decision should be based on **whether judgment is required**, not on token cost.

**How to apply:** Reserve LLM routing (Haiku, Olla/Qwen, or higher) for tasks that need code-shape decisions, API integration, error handling, or multi-file synthesis. When the input is a typed structure and the output is another typed structure with a 1:1 mapping (or a flatten/unflatten), use Python — `ast.parse` + `json.dumps` covers most cases. Persist the generator script alongside the output (e.g., `scripts/gen_*_seed.py`) so it's reproducible if the source changes.
