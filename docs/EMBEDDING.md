# Embedding Model — Source of Truth

**Status:** Authoritative  
**Maintained by:** Core team — update only via PR that also updates `internal/embedmodel` constants in the same commit.  
**Last updated:** 2026-05-30

---

## Canonical Model

| Field | Value |
|-------|-------|
| Canonical identifier | `BAAI/bge-m3` |
| Output dimension | `1024` |
| DB column type | `vector(1024)` (pgvector) |

## Accepted Alias Set

The following model identifiers are considered equivalent to `BAAI/bge-m3`. Any embed response whose reported `model` field matches one of these aliases is accepted for storage, provided `len(vec) == 1024`.

| Alias | Source |
|-------|--------|
| `BAAI/bge-m3` | Canonical (HuggingFace Hub ID) |
| `bge-m3` | Short form used by some olla/LiteLLM configurations |
| `bge-m3-Q8_0.gguf` | GGUF quantisation served via llama.cpp GPU hosts (`--alias BAAI/bge-m3`) |
| `bge-m3-Q4_K_M.gguf` | GGUF Q4 quantisation (lower VRAM, same dim contract) |

> The llama.cpp-based GPU host containers are started with `--alias BAAI/bge-m3`, which causes olla/LiteLLM to echo `BAAI/bge-m3` as the model ID. The raw GGUF filename aliases appear when the alias flag is absent or misconfigured — they are included in the accepted set as a safety net, but their presence in production logs should be investigated.

## Rejection Rule

**Anything other than bge-m3 is wrong.**

Reject any embed response where:
- The reported model ID is NOT in the alias set above, **OR**
- `len(vec) != 1024`

On rejection:
1. Do NOT write the vector to the DB. Leave `embedding` column NULL.
2. Increment `embed_validation_rejections_total` metric with the rejection class label (`wrong_model` or `wrong_dims`).
3. Emit `slog.Error` with model ID, dims, and chunk ID.
4. After 5 consecutive rejections from the same embedder, enter degraded hold (see gateway spec §4.3).

## Storage Layer Enforcement

The pgvector column `chunks.embedding vector(1024)` enforces dimension at the DB level — an INSERT of a 768-dim vector will fail at the Postgres layer. The gateway's in-process validation fires first (before the INSERT) to surface a clean error and trigger alarming rather than a raw DB error.

## Machine-Readable Constants

The Go package `internal/embedmodel` mirrors this table. When the alias set changes, update both this file and `internal/embedmodel/model.go` in the same commit. The gateway imports only from `internal/embedmodel` — never hardcodes strings inline.

```go
// internal/embedmodel/model.go (canonical values)
const (
    CanonicalBGEM3  = "BAAI/bge-m3"
    RequiredDims    = 1024
    PGNotifyChannel = "embed_queue"
)

var AcceptedAliases = []string{
    "BAAI/bge-m3",
    "bge-m3",
    "bge-m3-Q8_0.gguf",
    "bge-m3-Q4_K_M.gguf",
}
```

## Related

- Gateway design spec: `docs/superpowers/specs/2026-05-30-embedding-gateway-design.md`
- Embedder client configuration: `docs/configuration/embedders.md`
- Current alias table (to be superseded by `internal/embedmodel`): `internal/search/engine.go:embedderAliases`
