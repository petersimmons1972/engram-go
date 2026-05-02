# Embedder Configuration

engram-go uses Ollama to compute embeddings for every memory. The embedder model
is selectable at startup via the `ENGRAM_OLLAMA_MODEL` environment variable.
All stored memories must share a single embedding model at a given time — the
migration tool is used to switch.

> **Note:** The operational contract is dimension-first: the configured model
> must produce 1024-dimensional vectors for the primary index. Use
> `memory_embedding_eval` before migrating in production.

---

## Available Models

Pull any of the supported models with `ollama pull <name>` and set
`ENGRAM_OLLAMA_MODEL` to switch.

| Model | Dim | Size | Best for |
|---|---:|---:|---|
| `nomic-embed-text` | 768 | 274 MB | Smallest footprint, solid English baseline |
| `mxbai-embed-large` | 1024 | 669 MB | Strong English retrieval quality |
| `bge-m3` | 1024 | 1200 MB | Multilingual corpora — 100+ languages |

---

## `ENGRAM_OLLAMA_MODEL`

Selects the embedder at server startup (`cmd/engram/main.go:46`).

```
ENGRAM_OLLAMA_MODEL=bge-m3               # multilingual corpus
ENGRAM_OLLAMA_MODEL=mxbai-embed-large    # English, higher recall
ENGRAM_OLLAMA_MODEL=nomic-embed-text     # legacy 768-dim option
```

A new server reads this at startup; changing the value has no effect on an
already-running process.

---

## Switching Models on a Populated Store

**Never** change `ENGRAM_OLLAMA_MODEL` on a store that already has memories
without running the migration tool. The stored embeddings are dimension-locked
in the pgvector column; mixing dimensions causes `INSERT` failures at runtime.

### Procedure

1. Pull the new model locally: `ollama pull bge-m3`.
2. Call the MCP tool:
   ```
   memory_migrate_embedder(project="default", new_model="bge-m3")
   ```
3. The tool performs a **dimension pre-flight** before nulling any existing
   embeddings. If the new model outputs a different vector width than the
   current stored dimension, migration is refused — no destructive action taken
   (see GitHub issue #251).
4. If pre-flight passes, all chunks are re-embedded with the new model.
   Expect this to take minutes-to-hours depending on corpus size; the process
   streams progress.
5. After migration completes, update `ENGRAM_OLLAMA_MODEL` and restart the server.

### When dimension pre-flight fails

pgvector columns in engram's schema are dimension-sized at table creation time.
A 768 → 1024 change requires schema-level work, not just re-embedding. That is
tracked as a separate concern — see `internal/db/migrations/` and the
`memory_migrate_embedder` refusal path for the current state.

---

## Performance Tradeoffs

| Change | Latency | Vector storage | Recall quality |
|---|---|---|---|
| `nomic` → `mxbai-embed-large` | ~2-3x slower embed | 33% larger | Higher English recall (MTEB) |
| `nomic` → `bge-m3` | ~2-3x slower embed | 33% larger | Higher multilingual recall |

Run `memory_embedding_eval` on a representative sample of your corpus before
committing to a migration.

---

## When to Consider Each Option

- **Stay on a 768-dim model like `nomic-embed-text`**: English-only corpus,
  latency-sensitive workloads, smallest deployment footprint.
- **Move to a 1024-dim model like `mxbai-embed-large`**: English-only corpus,
  recall quality matters more than embedding latency, have the extra 400 MB to
  spare.
- **Move to a multilingual 1024-dim model like `bge-m3`**: Any non-English
  content in the corpus, or expect to mix languages over time. Multilingual
  models are structural, not tuning — a 768-dim model cannot recover
  multilingual recall regardless of prompt or weight adjustment.

---

## The "default" Project

When no `project` argument is provided to any MCP tool, Engram uses `"default"` as the implicit project name. This applies to `memory_store`, `memory_recall`, and all other tools that accept a `project` parameter. To keep memories isolated, always pass an explicit project name in production use. The `"default"` project is a convenience for quick experiments and single-project setups.

---

## References

- Migration tool handler: `internal/mcp/tools.go` (`handleMemoryMigrateEmbedder`)
- Dimension pre-flight rationale: GitHub issue #251
- Startup env read: `cmd/engram/main.go:46`
