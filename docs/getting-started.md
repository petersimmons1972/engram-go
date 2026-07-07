# Getting Started

When you finish this page, you will have a running memory server, a working IDE connection, and <!-- count:visible-default -->18<!-- /count --> tools visible to every AI assistant you use (<!-- count:visible-with-ai -->22<!-- /count --> with `ANTHROPIC_API_KEY` set). A handful of commands get you there (see below; previously this page claimed 'three' but the actual sequence is 4-6 depending on which profile you pick — #697). The whole thing takes about five minutes.

---

<p align="center"><img src="assets/svg/quick-start-flow.svg" alt="Quick start flow" width="900"></p>

---

## Prerequisites

Engram supports two local startup profiles:

- Hybrid (default `make up` profile): `postgres + engram-go`, with the embed/LLM backend running outside this compose stack and reachable via `ENGRAM_ROUTER_URL` (`LITELLM_URL` fallback)
- Local-only: `postgres + ollama + engram-go` via `docker-compose.local.yml`

For a fresh clone, use **local-only first** unless you already run an external
router.

Before you start, confirm you have what those profiles need:

- **Docker Engine 20.10 or newer** — check with `docker --version`
- **Docker Compose 2.0 or newer** — check with `docker compose version` (note: the subcommand, not `docker-compose`)
- **Go 1.26.3 or newer** (matches `go.mod` — kept in sync via CI doc-lint) — check with `go version`; download from [https://go.dev/dl/](https://go.dev/dl/)
- **4 GB RAM free** — local-only mode keeps the embedding model resident in Ollama
- **2 GB disk** — PostgreSQL volume plus the optional Ollama model download

Optional: an NVIDIA or AMD GPU. If you have one, embedding inference runs roughly 3× faster. Worth configuring if you plan to store large numbers of memories. Instructions are in the GPU section below.

---

## Step 1: Clone and Generate Credentials

```bash
git clone https://github.com/petersimmons1972/engram-go.git
cd engram-go
make init
```

`make init` generates two strong random credentials in `.env`:

- `POSTGRES_PASSWORD` — PostgreSQL authentication
- `ENGRAM_API_KEY` — bearer token that every MCP client must present

Both are required. The server refuses to start if either is missing. `make init` is idempotent — re-running it skips values that are already set.

Generating credentials first rather than shipping defaults means your memory store is protected from the first second it runs. Shared default passwords have ended careers. Yours will not.

---

## Step 2: Create Docker Volumes

Engram always uses one external PostgreSQL volume. The local-only profile also uses an Ollama model volume. External volumes survive `docker compose down` — your memories and model weights are never destroyed when containers are recreated.

```bash
docker volume create engram_pgdata
```

If you plan to use the local-only profile, create the Ollama model volume too:

```bash
docker volume create ollama_ollama_storage
```

Why external? Docker's default behaviour is to create and destroy volumes alongside the compose project. An accidental `docker compose down` would delete every memory you have stored. External volumes decouple data lifetime from container lifetime — the only way to lose data is to explicitly run `docker volume rm`.

---

## Step 3: Build the Postgres Image

Engram ships a custom Postgres image with `pgvector` pre-installed. Build it once before first start:

```bash
make build-postgres
```

You only need to run this on a fresh clone. `make up` will do it automatically if the image is missing, but running it explicitly first avoids any confusion about why the first `make up` takes longer than expected.

---

## Step 4: Start

```bash
docker compose -f docker-compose.local.yml up -d
```

For a predictable fresh-clone first run, use the local-only profile above.
`make up` is the hybrid profile and assumes an external embed/LLM backend
already exists at `ENGRAM_ROUTER_URL` (or legacy `LITELLM_URL`).

This local-only profile starts:

- `engram-postgres`
- `engram-ollama`
- `engram-go-app`

If your external backend is already configured, this starts the hybrid profile:

```bash
# Before `make up`, confirm your hybrid `.env` has:
# - `POSTGRES_PASSWORD` and `ENGRAM_API_KEY` from `make init`
# - `ENGRAM_ROUTER_URL` (or legacy `LITELLM_URL`) for your external router
# - `POSTGRES_HOST=postgres` and `POSTGRES_PORT=5432` for the bundled database
#   service, or override both for an external PostgreSQL server
make up
```

That hybrid profile starts:

- `engram-postgres` — PostgreSQL 16 with the pgvector extension installed
- `engram-go-app` — The MCP server, listening on port 8788 and routing embed/LLM traffic to `ENGRAM_ROUTER_URL`

**First local-only start takes 2–3 minutes.** Ollama downloads the configured embedding model before it reports healthy. Watch progress with:

```bash
docker compose -f docker-compose.local.yml logs ollama -f
```

Subsequent local-only starts are fast — the model is cached in the Ollama volume.

---

## Step 5: Connect Your IDE

For Claude Code, configure the MCP client once the server is healthy. `make setup` defaults to `http://localhost:8788`; pass `--url` to point at a remote host:

```bash
make setup
```

For local-only Docker, pass the local server URL explicitly:

```bash
go run ./cmd/engram-setup --url http://127.0.0.1:8788
```

This calls the `/setup-token` endpoint on the effective server, retrieves the bearer token, and writes it to `~/.claude/mcp_servers.json`. It also updates `~/.claude.json` when that file already has a `mcpServers` block. If `/setup-token` is unavailable during bootstrap, setup validates fallback credentials from `~/.config/engram/api_key` first and `~/projects/engram-go/.env` (`ENGRAM_API_KEY`) second before writing anything. Run `/mcp` in Claude Code afterward to activate the connection.

Re-run the same setup command any time the server restarts (if `ENGRAM_API_KEY` rotates) or after a fresh install.

For Cursor, VS Code, Windsurf, or Claude Desktop, see [Connecting Your IDE](connecting.md) — you will need to copy the token from `.env` manually for those clients.

---

## Verify It Is Working

This is the moment it clicks. Run these checks and watch the pieces confirm each other.

Check that the containers for your chosen profile are running and healthy:

```bash
docker compose -f docker-compose.local.yml ps
```

For the local-only profile, `engram-postgres`, `engram-ollama`, and
`engram-go-app` should all show `Up (healthy)`. If any service shows
`Up (health: starting)`, wait 30 seconds and check again — the health checks
run on a short interval.

If you started the hybrid profile with `make up`, rerun the same `ps` check
against the default compose file. That profile should show `engram-postgres`
and `engram-go-app` as `Up (healthy)`.

Confirm the MCP endpoint is reachable:

```bash
curl -s http://localhost:8788/sse
# Press Ctrl+C after a second or two
```

An SSE connection returns `data:` events on a keep-alive stream. If you get `Connection refused`, one of the containers is not up yet.

In Claude Code, confirm the tools loaded:

```
/mcp
```

You should see `engram` listed with <!-- count:visible-default -->18<!-- /count --> tools (<!-- count:visible-with-ai -->22<!-- /count --> if `ANTHROPIC_API_KEY` is set — <!-- count:ai-enhanced -->4<!-- /count --> optional AI-enhanced tools activate). If it shows fewer, restart Claude Code — IDE MCP clients cache the tool list at startup. See [MCP Tool Reference](tools.md) for the full callable surface (<!-- count:total-callable-default -->46<!-- /count --> default with <!-- count:hidden -->28<!-- /count --> hidden maintenance tools / <!-- count:total-callable-with-ai -->50<!-- /count --> with API key, including hidden maintenance tools).

When you see <!-- count:visible-default -->18<!-- /count --> tools (or <!-- count:visible-with-ai -->22<!-- /count --> with `ANTHROPIC_API_KEY`), you are done. The server is running, and your IDE has a persistent connection to your memory store. In local-only mode that also implies the embedding model is loaded in Ollama.

---

## Step 6: Optional — Install Bundled Skills

Engram includes four Claude Code skills for maintenance, consolidation, and diagnostics operations. These skills wrap advanced tools and provide a user-friendly interface to operations that are powerful but rarely needed during regular sessions.

To install the bundled skills:

```bash
make install-skills
```

This copies four skill directories to `~/.claude/skills/`:
- `/engram-consolidate` — memory consolidation and decay audits
- `/engram-episodes` — session and episode management
- `/engram-ingest` — import and export operations
- `/engram-diagnose` — health checks and analytics

After install, the skills appear in your Claude Code command palette (type `/` to see them). See [MCP Tool Profiles](tools.md#mcp-tool-profiles) for details on what each skill does and why you might need it.

For most work, you do not need these skills — the visible tools are sufficient. Install them if you plan to manage consolidation, ingest large document sets, or run maintenance operations.

---

## Configuration Reference

Most people never touch these. `make init` handles the two required values, and the defaults for everything else are sensible. But here is the full picture for when you need it:

```bash
# ============================================================
# Database — generated by 'make init'
# ============================================================
POSTGRES_PASSWORD=                         # Required: generated by make init
POSTGRES_HOST=postgres                    # Hybrid default: bundled Compose postgres service
POSTGRES_PORT=5432                        # Hybrid default: bundled Compose postgres service
                                          # Override both for an external PostgreSQL server

# ============================================================
# Authentication — generated by 'make init'
# ============================================================
ENGRAM_API_KEY=                            # Required: bearer token for all MCP connections
                                           # Generated by make init; clients configured by make setup

# ============================================================
# Embeddings
# ============================================================
OLLAMA_URL=http://ollama:11434             # Default: Ollama inside Docker
# OLLAMA_URL=http://host.docker.internal:11434  # Mac: native Ollama outside Docker

ENGRAM_EMBED_MODEL=mxbai-embed-large      # Ollama local-only path default; Infinity/olla path uses BAAI/bge-m3

# ============================================================
# Background summarization
# ============================================================
ENGRAM_SUMMARIZE_MODEL=llama3.2           # Ollama model for async summary generation

# ============================================================
# Server
# ============================================================
ENGRAM_PORT=8788                           # Change if 8788 conflicts with something else
ENGRAM_TRUST_PROXY_HEADERS=false           # Set to 1 when behind a trusted reverse proxy

# ============================================================
# Claude Advisor (all off by default — requires Anthropic API key)
# ============================================================
ANTHROPIC_API_KEY=                         # Set this to enable memory_reason and Claude features
ENGRAM_CLAUDE_SUMMARIZE=false             # Use Claude instead of Ollama for summaries
ENGRAM_CLAUDE_CONSOLIDATE=false           # Use Claude for consolidation analysis
ENGRAM_CLAUDE_RERANK=false                # Use Claude to rerank results (slower, better)
ENGRAM_CROSS_ENCODER_RERANK=false         # Use a cross-encoder on top dense hits before hybrid fusion
ENGRAM_CROSS_ENCODER_URL=                 # TEI-compatible /rerank endpoint, e.g. http://localhost:6006/rerank
```

| Variable                   | Default              | Required | Purpose                                               |
| -------------------------- | -------------------- | -------- | ----------------------------------------------------- |
| `POSTGRES_PASSWORD`        | *(none)*             | **Yes**  | PostgreSQL password — generated by `make init`        |
| `POSTGRES_HOST`            | `postgres`           | No       | Hybrid compose PostgreSQL host; override for an external database |
| `POSTGRES_PORT`            | `5432`               | No       | Hybrid compose PostgreSQL port; override for an external database |
| `ENGRAM_API_KEY`           | *(none)*             | **Yes**  | Bearer token for all SSE connections — generated by `make init` |
| `OLLAMA_URL`               | `http://ollama:11434`| No       | URL of the Ollama embedding service                   |
| `ENGRAM_EMBED_MODEL`       | *(none)*             | Yes      | Embedding model name (must be configured). `ENGRAM_OLLAMA_MODEL` is a deprecated alias retained for backward compat. |
| `ENGRAM_SUMMARIZE_MODEL`   | `llama3.2`           | No       | Ollama model for background summaries                 |
| `ENGRAM_PORT`              | `8788`               | No       | Port the MCP server binds to                          |
| `ANTHROPIC_API_KEY`        | *(empty)*            | No       | Enables `memory_reason` tool and Claude-backed features |
| `ENGRAM_CLAUDE_SUMMARIZE`  | `false`              | No       | Use Claude for async summaries instead of Ollama      |
| `ENGRAM_CLAUDE_CONSOLIDATE`| `false`              | No       | Use Claude for graph consolidation                    |
| `ENGRAM_CLAUDE_RERANK`     | `false`              | No       | Use Claude to rerank search results                   |
| `ENGRAM_CROSS_ENCODER_RERANK` | `false`           | No       | Enable dense-leg cross-encoder reranking before BM25/recency fusion |
| `ENGRAM_CROSS_ENCODER_URL` | *(empty)*            | No       | Full TEI-compatible `/rerank` endpoint used when cross-encoder reranking is on |
| `POSTGRES_DB`              | `engram`             | No       | Database name (rarely needs changing)                 |
| `POSTGRES_USER`            | `engram`             | No       | Database user (rarely needs changing)                 |
| `ENGRAM_TRUST_PROXY_HEADERS` | `false`              | No       | Trust `X-Forwarded-For` / `X-Real-IP` for rate limiting. Set to `1` only when a trusted reverse proxy is in front. |
| `ENGRAM_RECALL_MAX_TOP_K`  | `500`                | No       | Hard cap on results returned by `memory_recall`. Increase for bulk export use cases. |
| `ENGRAM_EMBED_GW_ENABLED` | `false`              | No       | Enable the unified embedding gateway. Default-off; when enabled it owns background embedding through a dedicated DB pool. |
| `ENGRAM_EMBED_GW_BATCH_SIZE` | `100`             | No       | Chunks processed per embedding gateway batch. Falls back to `ENGRAM_REEMBED_BATCH_SIZE` if unset. |
| `ENGRAM_REEMBED_BATCH_SIZE`| `100`                | No       | Chunks processed per legacy GlobalReembedder iteration. Also used as gateway batch-size fallback during rollout. |
| `ENGRAM_REEMBED_INTERVAL`  | `10s`                | No       | Delay between GlobalReembedder polling iterations. Accepts Go duration strings (`10s`, `1m`). |

---

## Secret Management: Infisical or Direct Environment Variable

The `engram-go` container uses a small `starter` binary as its entrypoint. Its job is to inject secrets before the server process starts. It supports two modes:

### Option A: Direct environment variable (simplest)

Set `ENGRAM_API_KEY` directly in `.env` (which `make init` does for you). The starter detects that `INFISICAL_CLIENT_ID` is absent and skips the Infisical fetch entirely, using the key already in the environment:

```bash
# .env — generated by make init
ENGRAM_API_KEY=your-key-here
POSTGRES_PASSWORD=your-password-here
```

This is the default path. No additional configuration needed.

### Option B: Infisical machine identity

If you manage secrets via Infisical, add a `.env.machine-identity` file (created empty by `make init`) with your machine identity credentials:

```bash
# .env.machine-identity — never commit this file
INFISICAL_CLIENT_ID=your-client-id
INFISICAL_CLIENT_SECRET=your-client-secret
# Optional overrides (defaults shown):
# INFISICAL_DOMAIN=https://infisical.yourcompany.com
# INFISICAL_PROJECT_ID=your-project-id
# INFISICAL_ENV=prod
# INFISICAL_SECRET_PATH=/apps/engram
```

When `INFISICAL_CLIENT_ID` is set, the starter authenticates to Infisical and fetches `ENGRAM_API_KEY` and `POSTGRES_PASSWORD` at container startup. The credentials are injected into the environment and the machine identity credentials are scrubbed before `engram` starts.

If neither `INFISICAL_CLIENT_ID` nor `ENGRAM_API_KEY` is set, the starter exits with a clear error explaining both options.

---

## GPU Acceleration

If you store a lot of memories, this matters. Every `memory_store` call runs an embedding pass through Ollama. On CPU, that is fast enough for normal use. Under heavy ingest — bulk imports, frequent stores across many projects — GPU acceleration makes the difference between a snappy tool and one that makes you wait.

### NVIDIA

Edit `docker-compose.yml` and uncomment the `deploy` block under the `ollama` service. It looks like this:

```yaml
    # deploy:
    #   resources:
    #     reservations:
    #       devices:
    #         - driver: nvidia
    #           count: 1
    #           capabilities: [gpu]
```

Remove the `#` characters. Then ensure you have the [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html) installed on the host.

### AMD (ROCm)

Change the Ollama image to `ollama/ollama:rocm` and uncomment the `devices` block in `docker-compose.yml`. Your user must be in the `render` and `video` groups:

```bash
sudo usermod -aG render,video $USER
```

Log out and back in for the group change to take effect.

### Mac (M-series)

Docker Desktop on Mac does not pass Metal GPU through to Linux containers. The practical solution is to run Ollama natively:

```bash
brew install ollama
ollama serve
```

Then in `.env`, change the Ollama URL:

```bash
OLLAMA_URL=http://host.docker.internal:11434
```

And comment out the `ollama` service in `docker-compose.yml` so Docker does not start the container Ollama. The `engram-go-app` container will use your native Ollama instead.

---

## Common Problems

**Port 8788 is already in use.**
Something else claimed that port before Engram did. Set `ENGRAM_PORT=8789` (or any free port) in `.env`, restart the same profile you started, and update the port in your IDE config. For local-only, use `docker compose -f docker-compose.local.yml up -d`. For hybrid, re-run `make up`.

**Embedding model not found / connection errors on first start.**
Ollama is still downloading the model — it can look like a crash when it is really just slow. Watch it finish:

```bash
docker compose -f docker-compose.local.yml logs ollama -f
```

Wait for a line like `llm server listening`. Then the `engram-go-app` container will become healthy.

**IDE says connection refused.**
Either a container is not up yet, or `engram-go-app` is waiting on its dependencies. Confirm the state:

```bash
docker compose -f docker-compose.local.yml ps
```

If `engram-go-app` shows `Up (health: starting)`, it is waiting for Postgres or Ollama. Give it 30 seconds. If it shows `Exit`, the process crashed — check the logs to see why:

```bash
docker compose -f docker-compose.local.yml logs engram-go-app
```

The most common causes are a missing `POSTGRES_PASSWORD` or `ENGRAM_API_KEY`. Run `make init` to generate both.

**`POSTGRES_PASSWORD must be set` error before containers start.**
Docker Compose requires `POSTGRES_PASSWORD` — there is no default, and this error fires before any container starts (not inside the logs). Run `make init` to generate it, or set it manually in `.env`.

**Ollama SSRF protection rejects your `OLLAMA_URL`.**
If you see `ollama URL resolved to private IP` in the logs, your `OLLAMA_URL` hostname is resolving to a private address that does not match the configured host. The configured host is always allowed. This error means the hostname in `OLLAMA_URL` resolves differently at dial time than expected — check for DNS misconfiguration or a mismatched `OLLAMA_URL` value.

---

**Previous:** [How It Works](how-it-works.md) — the full technical story.  
**Next:** [Connecting Your IDE](connecting.md) — exact config for Cursor, VS Code, Windsurf, and Claude Desktop.
