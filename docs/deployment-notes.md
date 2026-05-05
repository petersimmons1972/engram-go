# Deployment Notes

This page documents optional infrastructure choices and how Engram integrates with them.

---

## Default: Hybrid Profile with LiteLLM

The default `docker-compose.yml` assumes you have a LiteLLM proxy running on an external machine or in another Docker compose stack. This decouples the memory server from the embedding/summarization infrastructure.

**When to use:**
- You want to use advanced embedding models (Qwen, BGE-M3) not yet available in Ollama
- You have a shared LiteLLM instance serving multiple projects
- You need fine-grained control over which model each project uses

**Configuration:**

```bash
# .env
LITELLM_URL=http://your-litellm-host:4000
LITELLM_API_KEY=<your-key-if-litellm-requires-auth>
ENGRAM_EMBED_MODEL=qwen3-embedding:8b
ENGRAM_EMBED_URL=http://your-litellm-host:4000/v1
```

The `engram-reembed` service (Rust re-embedder) is the only container that calls LiteLLM heavily. The main `engram-go` service uses it only for query-time embedding with a 2-second timeout and BM25 fallback.

---

## Local-Only: Ollama Profile

The `docker-compose.local.yml` profile brings Ollama into the Docker network. Nothing leaves your machine.

**When to use:**
- You want 100% local setup with no external dependencies
- You are on a single machine with >8 GB RAM available for Ollama
- You prefer to avoid managing a separate LiteLLM infrastructure

**Configuration:**

```bash
docker compose -f docker-compose.local.yml up -d
```

Ollama runs in a separate container with 8 GB memory limit. On first start, it downloads the configured model (default: `mxbai-embed-large`, ~600 MB). The container persists the model in the `ollama_storage` Docker volume, so subsequent starts are fast.

```bash
# .env (when using local profile)
ENGRAM_EMBED_MODEL=mxbai-embed-large
ENGRAM_SUMMARIZE_MODEL=llama3.2:3b
# LITELLM_URL and LITELLM_API_KEY are not used; Ollama serves both roles
```

---

## Personal Infrastructure: Infisical

The `Dockerfile` and `cmd/starter` assume you may be using [Infisical](https://infisical.petersimmons.com) (open-source secret management) to inject credentials at container boot time. This is optional.

**When to use:**
- You have multiple services that need shared secrets (API keys, database passwords)
- You want secrets to never appear in `.env` files or version control

**If you use Infisical:**

The `.env.machine-identity` file is a placeholder for Infisical machine-identity credentials. Populate it with your Infisical service account token:

```bash
# .env.machine-identity
CLIENT_ID=your-client-id
CLIENT_SECRET=your-client-secret
INFISICAL_API_URL=https://your-infisical-instance
```

The `cmd/starter` container entrypoint will authenticate to Infisical, fetch your configured secrets (POSTGRES_PASSWORD, ENGRAM_API_KEY, etc.), and inject them into the process environment before starting the engram server. Secrets never appear in `docker inspect` output or container logs.

**If you don't use Infisical:**

Leave `.env.machine-identity` empty. All secrets must be in `.env` or environment variables. `make init` generates them automatically.

---

## Embedding Model Selection

Engram's retrieval quality depends heavily on the embedding model. Here are canonical choices:

| Model | Dimensions | Container | Speed | Notes |
|-------|-----------|-----------|-------|-------|
| `mxbai-embed-large` | 1024 | Ollama | 2× (GPU) / 30× (CPU) | **Default for local profile.** Strong semantic quality, good open-source baseline. |
| `nomic-embed-text` | 768 | Ollama | 3× | Smaller memory footprint. Good for resource-constrained environments. |
| `bge-m3` | 1024 | Ollama | Slower | Strong multilingual support. Larger model, 6+ GB VRAM. |
| `qwen3-embedding:8b` | 1024 | LiteLLM | 1× (GPU) | **Default for hybrid profile.** State-of-the-art semantic quality. Requires external LiteLLM. |

See [docs/how-it-works.md](how-it-works.md) for how embeddings feed into Engram's retrieval pipeline.

To switch models:

```bash
# Update .env
ENGRAM_EMBED_MODEL=bge-m3

# Migrate existing embeddings
memory_embedding_eval(project="myapp", model_a="mxbai-embed-large", model_b="bge-m3")
memory_migrate_embedder(project="myapp", model="bge-m3")
```

The migration tool compares quality against your stored memories before committing. See [tools.md](tools.md) for the full tool reference.

---

## OpenAI-Compatible Endpoints

Both profiles accept OpenAI-compatible embedding endpoints (Ollama, LiteLLM, vLLM, text-embedding-3, etc.). Set `ENGRAM_EMBED_URL` to any `/v1`-compliant endpoint:

```bash
# Example: using a vLLM instance
ENGRAM_EMBED_URL=http://vllm-host:8000/v1
ENGRAM_EMBED_MODEL=Alibaba-NLP/gte-Qwen1.5-7B-instruct
```

Engram discovers embedding dimensions at startup. If your endpoint returns inconsistent dimensions, set `ENGRAM_EMBED_DIMENSIONS` explicitly:

```bash
ENGRAM_EMBED_DIMENSIONS=1024
```

---

## Backup & Recovery

Both profiles use the same PostgreSQL backend. Backups work identically:

```bash
# Backup (works for both profiles)
docker exec engram-postgres pg_dump -U engram engram | gzip > backups/engram-$(date +%Y%m%d).sql.gz

# Restore
gunzip -c backups/engram-20260101.sql.gz | docker exec -i engram-postgres psql -U engram engram
```

The backup captures all memories, vectors, graph edges, and schema. Embedding vectors are stored alongside the text — you can restore a backup to a different embedding model without re-embedding everything.

---

## Scaling

**Single-machine:** Both profiles are designed for one developer or small team on a single machine. Memory server uses <20 MB at idle. Ollama uses 4–8 GB depending on model. Postgres uses 2–10 GB depending on memory count.

**Multiple projects / environments:**

For separate memory namespaces (personal notes, work project, AI training logs), use Engram's project-scoped storage:

```python
memory_store(content="...", project="my-app")
memory_store(content="...", project="ai-training")
```

Everything is in one database but projects are isolated by the `project` field. No additional infrastructure needed.

For multiple machines or high-availability, fork the repo — Engram's local-first design is intentional. Cloud sync is not in scope.

---

## Troubleshooting

**Ollama won't download the model:**
- Check disk space: `df -h`
- Check network: `curl https://registry.ollama.ai/v2/_catalog`
- Logs: `docker logs engram-ollama`

**Embedding dimension mismatch:**
- Your endpoint returned different vector sizes. Set `ENGRAM_EMBED_DIMENSIONS` explicitly in `.env`.
- Re-embedding existing memories: `memory_migrate_embedder` handles this.

**LiteLLM not reachable:**
- Verify the URL: `curl -v $LITELLM_URL/health`
- Check hostname resolution from inside the container: `docker exec engram-go-app nslookup your-litellm-host`
- If using Docker Compose, ensure both compose stacks are on the same network or check your network setup.

**Postgres won't start:**
- Check logs: `docker logs engram-postgres`
- Verify volume: `docker volume ls | grep engram_pgdata`
- Ensure the volume is not corrupted: back up and reset if needed.

See [Operations](operations.md) for deeper diagnostics and security configuration.
