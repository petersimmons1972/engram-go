![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white) ![License](https://img.shields.io/badge/License-MIT-green) ![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker&logoColor=white) ![MCP](https://img.shields.io/badge/MCP-SSE-purple)

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

**What makes this different from a notes file:**

- **Finds what you mean, not just what you typed.** BM25 keyword search and 768-dimensional semantic vectors run simultaneously. Searching "database lock timeout" finds your note about "WAL mode contention under load" — no shared words, close meaning.
- **Weights by recency automatically.** Exponential decay at 1% per hour. Yesterday's decision outranks one from six months ago. Nothing is deleted; old memories step back.
- **Surfaces connected memories without being asked.** A knowledge graph links decisions to the bugs they caused and the patterns they require. Recall one; get its neighborhood.

---

## Quick Start

```bash
git clone https://github.com/petersimmons1972/engram-go.git && cd engram-go
make init     # generates POSTGRES_PASSWORD and ENGRAM_API_KEY in .env
make up       # starts postgres, ollama, and engram-go
make setup    # configures Claude Code MCP client (run after container is healthy)
```

The server starts on port 8788. Cold start: under 200ms. Memory at idle: 18 MB.

`make setup` calls the `/setup-token` endpoint to fetch the bearer token and writes it to `~/.claude/mcp_servers.json` automatically. Run `/mcp` in Claude Code after setup to connect.

---

## Architecture

<p align="center"><img src="docs/architecture.svg" alt="Engram Architecture" width="900"></p>

Your AI client speaks MCP over SSE. Engram exposes 31 tools — store, recall, connect, correct, diagnose, episode management, cross-project federation, and lightweight safety verification wrappers for constraint checks before acting. PostgreSQL with pgvector stores everything. Ollama (local) runs the embeddings. When Ollama is unavailable, search falls back to BM25 and recency. All tools stay functional.

---

## Documentation

| | |
|---|---|
| [Why Engram?](docs/why-engram.md) | The problem with AI agent memory and how Engram solves it |
| [How It Works](docs/how-it-works.md) | Four-signal search, knowledge graph, context efficiency |
| [Getting Started](docs/getting-started.md) | Install and connect in 5 minutes |
| [Connecting Your IDE](docs/connecting.md) | Claude Code, Cursor, VS Code, Windsurf, Claude Desktop |
| [All 31 Tools](docs/tools.md) | MCP tool reference with usage examples |
| [Claude Advisor](docs/claude-advisor.md) | AI-powered summarization, consolidation, re-ranking |
| [Operations](docs/operations.md) | Backup, security, data portability |

---

## v3.0 vs v2 vs v1

v1 was Python. v2 rewrote in Go. v3.0 adds required authentication, auto-episode starts on every SSE connection, and 31 tools.

| | v1 (Python) | v2 (Go) | v3.0 (Go) |
|---|---|---|---|
| Container size | 200 MB | 10 MB | 10 MB |
| Cold start | ~3 seconds | ~200ms | ~200ms |
| Idle memory | 120 MB | 18 MB | 18 MB |
| Base image | python:3.12-slim | Chainguard static | Chainguard static |
| MCP transport | stdio | SSE | SSE |
| Authentication | optional | optional | required |
| Auto-episode | no | no | yes |
| Tool count | 19 | 19 | 28 |

---

MIT — see [LICENSE](LICENSE)
