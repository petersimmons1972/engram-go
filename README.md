![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white) ![License](https://img.shields.io/badge/License-GPL%20v3-blue) ![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker&logoColor=white) ![MCP](https://img.shields.io/badge/MCP-SSE-purple) ![Local](https://img.shields.io/badge/Local--Only-No%20Cloud-ff6b6b)

<p align="center"><img src="docs/hero.svg" alt="Engram — Persistent Memory for AI Agents" width="100%"></p>

Every time you close your AI coding session, it forgets everything. The JWT library you chose. The expiry bug you spent an afternoon on. The pattern you explicitly rejected. Gone. Next session, the agent starts from zero and you start explaining.

```python
# Session start — before touching any code
memory_recall("session handoff recent decisions", project="myapp")

# After settling on a technical choice
memory_store(
    "Chose RS256 over HS256: the API gateway needs to verify tokens without
     holding the signing secret. HS256 would require distributing the key to
     every service. Do not change this without updating the gateway config.",
    memory_type="decision",
    project="myapp"
)
```

---

## 100% Local. No exceptions.

> **Your memories never leave your machine.**

Most memory tools send your code context, architectural notes, and decision logs to a third-party API. Engram doesn't. Your PostgreSQL stores every memory. Your Ollama instance runs every embedding. Nothing leaves your infrastructure unless you push it yourself.

- No account to create
- No API key to manage
- No vendor terms governing your codebase notes
- No data leaving your network

```bash
make init && make up && make setup
# Done. Memory server at localhost:8788.
```

---

## What makes this different

**Finds what you mean, not just what you typed.** BM25 keyword search and 768-dimensional semantic vectors run simultaneously. Searching "database lock timeout" finds your note about "WAL mode contention under load" — no shared words, close meaning. When Ollama is unavailable, search degrades gracefully to BM25+recency. Your results never disappear because an external service went down.

**Weights by recency automatically.** Exponential decay at 1% per hour. Yesterday's decision outranks one from six months ago. Nothing is deleted; old memories step back. Six-month-old memories are still there if nothing more recent matches.

**Surfaces connected memories without being asked.** A knowledge graph links decisions to the bugs they caused and the patterns they require. Recall one; get its neighborhood. Store a bug report, store the architectural pattern that caused it, connect them with a `causes` edge. Now any query about the pattern automatically surfaces the bug — you don't have to remember to ask for both.

**Stores documents, not just notes.** `memory_store_document` handles up to 500,000 characters. Engram chunks at sentence boundaries and embeds each chunk independently. A 20,000-word architecture document is searchable at the paragraph level — a query about authentication surfaces the auth section, not the whole document.

---

## Quick Start

```bash
git clone https://github.com/petersimmons1972/engram-go.git && cd engram-go
make init     # generates POSTGRES_PASSWORD and ENGRAM_API_KEY in .env
make up       # starts postgres, ollama, and engram-go
make setup    # writes bearer token to ~/.claude/mcp_servers.json
```

The server starts on port 8788. If you prefer to author `.env` by hand rather than using `make init`, `.env.example` at the repo root documents every available variable with its default and purpose. Cold start: under 200ms. Memory at idle: 18 MB.

> **Docker users:** `docker-compose.yml` now sets `ENGRAM_SETUP_TOKEN_ALLOW_RFC1918=1` automatically. If you run engram outside Docker and need `/setup-token` accessible from RFC1918 addresses (e.g. a LAN host), add this variable to your environment. Without it, `/setup-token` only accepts loopback (127.0.0.1 / ::1).

Run `/mcp` in Claude Code after setup to connect. All 38 core tools are available immediately. Five optional AI-enhanced tools (`memory_ask`, `memory_reason`, `memory_explore`, `memory_query_document`, `memory_diagnose`) activate when `ANTHROPIC_API_KEY` is set.

---

## Architecture

<p align="center"><img src="docs/architecture.svg" alt="Engram Architecture" width="900"></p>

Your AI client speaks MCP over SSE. Engram exposes 43 tools — 38 run entirely locally (store, recall, connect, correct, episode management, cross-project federation, aggregate analysis, decay audit, adaptive weight tuning, embedding evaluation, and more) plus 5 optional AI-enhanced tools that activate when `ANTHROPIC_API_KEY` is set. PostgreSQL with pgvector stores everything. Ollama (local) runs the embeddings.

---

## New in v3

### RAG Queries: `memory_ask`

Ask a natural-language question against your stored memories. Returns a synthesized answer with citations — not a list of chunks, a direct response.

```python
memory_ask(
    question="What did we decide about authentication and why?",
    project="myapp"
)
# → "You chose RS256 JWT with 24h expiry stored in httpOnly cookies.
#    The decision was driven by the need to verify tokens in the API gateway
#    without distributing the signing secret. localStorage was explicitly
#    rejected due to XSS risk. (memories: auth-decision-001, security-pattern-003)"
```

### Document Storage: `memory_store_document`

Store up to 500,000 characters — architecture documents, meeting transcripts, entire codebases. Engram chunks at sentence boundaries and makes every paragraph individually searchable.

```python
memory_store_document(
    content=entire_architecture_document,  # up to 500k chars
    memory_type="architecture",
    project="myapp"
)
# Later: search surfaces the specific section, not the whole document
memory_query_document(doc_id=doc_id, query="authentication flow")
```

### Aggregate Queries

Query the shape of your memory store without reading individual memories.

```python
memory_aggregate(by="memory_type", project="myapp")
# → [{label: "decision", count: 47}, {label: "error", count: 12}, ...]

memory_aggregate(by="failure_class")
# → [{label: "vocabulary_mismatch", count: 8}, {label: "stale_ranking", count: 3}, ...]
```

### Retrieval Miss Tracking

When `memory_recall` returns nothing useful, log the miss with a failure class. This feeds the retrieval quality benchmark and makes future recall better.

```python
memory_feedback(
    event_id="<id from recall>",
    memory_ids=[],
    failure_class="vocabulary_mismatch"  # or: aggregation_failure, stale_ranking,
                                          #     missing_content, scope_mismatch, other
)
```

---

## New in v3.1

### Decay Audit System

Track retrieval drift over time with canonical query snapshots. Register a set of reference queries, run them on a schedule, and measure how results shift between runs using RBO (rank-biased overlap) and Jaccard similarity. Alerts when drift exceeds threshold.

```python
# Register a reference query
memory_audit_add_query(project="myapp", query="deployment procedures", description="CI/CD runbook recall")

# Take a snapshot and see drift vs previous run
memory_audit_run(project="myapp")
# → [{query: "deployment procedures", rbo_vs_prev: 0.94, additions_count: 1, removals_count: 0, alert: false}]

# Browse snapshot history for a query
memory_audit_compare(query_id="cq-abc123", limit=10)
```

Five new tools: `memory_audit_add_query`, `memory_audit_list_queries`, `memory_audit_deactivate_query`, `memory_audit_run`, `memory_audit_compare`.

### Adaptive Weight Tuning

Failure-class events now feed a background tuner that adjusts the four search weights per project. Dominant `vocabulary_mismatch` events shift weight toward BM25; dominant `stale_ranking` shifts toward recency. Adjustments fire at most once per 7 days after ≥ 50 failure events, within hard per-weight guardrails.

```python
memory_weight_history(project="myapp")
# → {current_weights: {vector: 0.40, bm25: 0.35, recency: 0.10, precision: 0.15},
#    history: [{applied_at: "...", weights: {...}, trigger_data: "..."}]}
```

### Expanded Relation Types

Knowledge graph now supports 11 typed edges (up from 7):

| Type | Meaning |
| ---- | ------- |
| `caused_by` | This memory exists because of that one |
| `relates_to` | Adjacent context, no causal direction |
| `depends_on` | This memory requires that one |
| `supersedes` | This memory replaces that one |
| `used_in` | This memory is applied in that context |
| `resolved_by` | Problem resolved by the referenced memory |
| `contradicts` | Conflict or tension |
| `supports` | Evidence or reinforcement |
| `derived_from` | Citation chain — memory derived from another |
| `part_of` | Hierarchical containment |
| `follows` | Temporal or sequential ordering |

### Pluggable Embedder Interface + Eval Tool

Compare any two Ollama embedding models against your actual stored memories before committing to a migration. Auto-pulls models not yet present in Ollama.

```python
# See what models are installed and recommended
memory_models()

# Compare nomic-embed-text against mxbai-embed-large on your real queries
memory_embedding_eval(project="myapp", model_a="nomic-embed-text", model_b="mxbai-embed-large")
# → {model_a_stats: {...}, model_b_stats: {...}, overlap_scores: [...], recommendation: "..."}
```

### `other` Failure Class

`memory_feedback` now accepts `failure_class="other"` for misses that do not fit the four specific categories. Tracked separately in `memory_aggregate(by="failure_class")`.

---

## Documentation

| | |
|---|---|
| [Why Engram?](docs/why-engram.md) | The problem with AI agent memory and how Engram solves it |
| [How It Works](docs/how-it-works.md) | Four-signal search, knowledge graph, context efficiency |
| [Getting Started](docs/getting-started.md) | Install and connect in 5 minutes |
| [Connecting Your IDE](docs/connecting.md) | Claude Code, Cursor, VS Code, Windsurf, Claude Desktop |
| [All 43 Tools](docs/tools.md) | MCP tool reference with usage examples |
| [Claude Advisor](docs/claude-advisor.md) | AI-powered summarization, consolidation, re-ranking |
| [Operations](docs/operations.md) | Backup, security, data portability |
| [Document Storage Strategy](https://www.petersimmons.com/engram/document-storage-strategy/) | Four-tier ingest architecture, chunking, retrieval paths |

---

## v3.0 vs v2 vs v1

v1 was Python. v2 rewrote in Go. v3.0 adds required authentication, auto-episode starts on every SSE connection, 35 tools, document storage, RAG queries, and aggregate analysis. v3.1 adds decay audit, adaptive weight tuning, expanded relation types, embedder eval, and 8 new tools.

| | v1 (Python) | v2 (Go) | v3.0 (Go) | v3.1 (Go) |
|---|---|---|---|---|
| Container size | 200 MB | 10 MB | 10 MB | 10 MB |
| Cold start | ~3 seconds | ~200ms | ~200ms | ~200ms |
| Idle memory | 120 MB | 18 MB | 18 MB | 18 MB |
| Base image | python:3.12-slim | Chainguard static | Chainguard static | Chainguard static |
| MCP transport | stdio | SSE | SSE | SSE |
| Authentication | optional | optional | required | required |
| Auto-episode | no | no | yes | yes |
| Tool count | 19 | 19 | 35 (30 local + 5 AI-enhanced) | 43 (38 local + 5 AI-enhanced) |
| Max memory size | 50k chars | 50k chars | 500k chars | 500k chars |
| Document mode | no | no | yes — chunked at sentence boundaries | yes — chunked at sentence boundaries |
| RAG queries | no | no | yes — `memory_ask` | yes — `memory_ask` |
| Aggregate queries | no | no | yes — `memory_aggregate` | yes — `memory_aggregate` |
| Relation types | — | — | 7 | 11 |
| Decay audit | no | no | no | yes — snapshot-based drift detection |
| Adaptive weights | no | no | no | yes — failure-class driven, per-project |
| Embedder eval | no | no | no | yes — compare any two Ollama models |
| Cloud required | no | no | **no** | **no** |

---

## Credits

v3.1 features were developed in dialogue with [open-brain-template](https://github.com/wefilmshit/open-brain-template) by Myles Bryning. The decay audit concept, `supports` / `derived_from` / `part_of` / `follows` relation types, and pluggable embedder registry are derived from open-brain-template's architecture, adapted to engram-go's local-only, Ollama-first constraint. The BM25+vector+recency blend, failure-class taxonomy, and knowledge-graph-based retrieval originated in engram-go and were independently incorporated by open-brain-template.

---

GPL v3 — see [LICENSE](LICENSE)
