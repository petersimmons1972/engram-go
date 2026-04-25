# Design: Rewrite instinct consolidator in Go

**Date:** 2026-04-25  
**Status:** Approved

## Context

The `consolidator/` directory contains a Python batch job (`instinct`) that was migrated from a separate repo on 2026-04-24. It reads Claude Code tool-call events from a buffer file, writes episodes to Engram, and calls Claude Haiku to detect workflow patterns. It has no technical reason to be Python — the repo is Go, the HTTP calls are simple, and the Python dependency forces users to install `uv` and a venv just to use the hooks.

**Goal:** Replace `consolidator/` with `cmd/instinct/main.go`. Delete all Python. Same behaviour, single toolchain.

---

## Architecture

One binary: `cmd/instinct`. Batch job — run, process buffer, exit. No daemon.

Single file `cmd/instinct/main.go` (~250 lines), five logical sections:

| Section | Responsibility |
|---------|---------------|
| Config | Read env vars + `~/.claude/mcp_servers.json` for Engram URL/token |
| Buffer | Read `buffer.jsonl`, skip bad lines, rotate on success |
| Engram client | 6 HTTP functions wrapping MCP tool calls |
| Haiku client | Call `claude-haiku-4-5-20251001`, parse pattern JSON |
| Pipeline | `run()`: load → group by session → episodes → patterns → upsert |

Tests in `cmd/instinct/main_test.go`.

---

## Config

Two sources, in priority order:

1. `ENGRAM_BASE_URL` + `ENGRAM_API_KEY` env vars
2. `~/.claude/mcp_servers.json` → `mcpServers.engram.url` + `mcpServers.engram.headers.Authorization`

Additional env vars (same names as Python for drop-in compatibility):
- `INSTINCT_BUFFER` — path to buffer JSONL (default: `~/.local/state/instinct/buffer.jsonl`)
- `INSTINCT_MIN_EVENTS` — minimum events to trigger processing (default: `20`)
- `ANTHROPIC_API_KEY` — required for Haiku pattern detection

---

## Buffer

```
load_and_rotate(path string) ([]Event, error)
```

- If file missing or line count < `INSTINCT_MIN_EVENTS`: return empty slice, nil error (noop)
- Read all lines, parse each as JSON, skip malformed with `slog.Warn`
- Rename to `buffer.jsonl.YYYYMMDDTHHMMSSZ.processed` before returning
- On pipeline error: rename `.processed` back to `buffer.jsonl` for retry (only if `buffer.jsonl` not already present)

Event struct mirrors the JSONL schema written by `post-tool-use.sh`:
```go
type Event struct {
    Timestamp         string `json:"timestamp"`
    SessionID         string `json:"session_id"`
    ProjectID         string `json:"project_id"`
    ToolName          string `json:"tool_name"`
    ToolInputHash     string `json:"tool_input_hash"`
    ToolOutputSummary string `json:"tool_output_summary"`
    ExitStatus        int    `json:"exit_status"`
    SchemaVersion     int    `json:"schema_version"`
}
```

---

## Engram HTTP Client

Thin struct:
```go
type engramClient struct {
    baseURL string
    token   string
    http    *http.Client  // 30s timeout
}
```

All tool calls: `POST {baseURL}/message` with `Authorization: Bearer {token}` and JSON body `{"tool": "<name>", "arguments": {...}}`. Response parsed as `map[string]any`.

Six methods:

| Method | MCP Tool | Arguments |
|--------|----------|-----------|
| `episodeStart(sessionID, projectID)` | `memory_episode_start` | `title`, `project` |
| `ingest(content, projectID, sessionID)` | `memory_ingest` | `content`, `memory_type: "context"`, `project`, `importance: 0.2`, `tags` |
| `episodeEnd(episodeID)` | `memory_episode_end` | `episode_id` |
| `store(pattern, confidence, projectID)` | `memory_store` | `content`, `memory_type: "pattern"`, `project`, `importance`, `tags` |
| `recall(tagSignature, projectID)` | `memory_recall` | `query`, `project` |
| `correct(memoryID, confidence)` | `memory_correct` | `memory_id`, `importance` |

---

## Haiku Client

Reuse `internal/claude.Client` (existing custom HTTP wrapper). Call `Complete()` with:
- **Model:** `claude-haiku-4-5-20251001`
- **System prompt:** identical to Python version (pattern detection instructions for CORRECTION, ERROR_RESOLUTION, WORKFLOW types), with ephemeral prompt cache header
- **User message:** one line per event — `[{timestamp}] {tool_name} | {output_summary} | exit={status}`
- **Max tokens:** 1024

Response: JSON array. Parse, validate each object has `type`, `description`, `domain`, `evidence`, `tag_signature`. Skip invalid entries with `slog.Warn`. Return `[]Pattern` — empty on API error or no patterns (pipeline continues, patterns are best-effort).

```go
type Pattern struct {
    Type         string `json:"type"`
    Description  string `json:"description"`
    Domain       string `json:"domain"`
    Evidence     string `json:"evidence"`
    TagSignature string `json:"tag_signature"`
}
```

---

## Confidence Management

Constants (inline in `main.go`):
```go
var confidenceSteps = []float64{0.3, 0.5, 0.7, 0.9}
const promoteThreshold = 0.8
const confidenceTolerance = 0.01
```

`upsertPattern(ctx, engram, pattern, events)`:
1. Query existing: `recall(pattern.TagSignature, primaryProject)`
2. **New pattern:** store at 0.3
3. **Existing:** step up (non-correction) or step down (correction type); call `correct()` only if confidence changed
4. **Global promotion:** if new confidence ≥ 0.8 AND events span ≥ 2 distinct project IDs → `recall(tagSignature, "global")`; if absent, `store(pattern, confidence, "global")`

Primary project = most common `project_id` across events in the batch.

---

## Pipeline

```go
func run(ctx context.Context, cfg config) error {
    events, processedPath := loadAndRotate(cfg.bufferPath, cfg.minEvents)
    if len(events) == 0 { return nil }  // noop

    groups := groupBySession(events)  // map[(sessionID, projectID)][]Event

    engram := newEngramClient(cfg)
    for key, group := range groups {
        writeEpisode(ctx, engram, key.sessionID, key.projectID, group)
    }

    patterns := detectPatterns(ctx, cfg.anthropicKey, events)
    for _, p := range patterns {
        upsertPattern(ctx, engram, p, events)
    }
    return nil
}
```

On any error from `writeEpisode` or `upsertPattern`: log with `slog.Error`, continue processing remaining items (best-effort). Only re-queue buffer on catastrophic failure (engram unreachable for all episodes).

---

## Hook Changes

**`hooks/post-tool-use.sh`** — remove `CONSOLIDATOR` and `CONSOLIDATOR_MODULE` vars, replace invocation:
```bash
# remove:
CONSOLIDATOR="$HOME/projects/instinct/consolidator/.venv/bin/python"
CONSOLIDATOR_MODULE="$HOME/projects/instinct/consolidator"
# ...
PYTHONPATH="$CONSOLIDATOR_MODULE" "$CONSOLIDATOR" -m instinct.run >> "$LOG_FILE" 2>&1 &

# replace with:
instinct >> "$LOG_FILE" 2>&1 &
```

**`hooks/install.sh`** — remove the `uv sync` step, replace with:
```bash
echo "Building instinct binary..."
go build -o "$HOME/bin/instinct" "$REPO_DIR/cmd/instinct"
echo "  Binary installed at ~/bin/instinct"
```

**`consolidator/`** — deleted entirely.

---

## Testing

`cmd/instinct/main_test.go`:

| Test | What it covers |
|------|---------------|
| `TestLoadAndRotate_MinEvents` | Returns empty when count < threshold |
| `TestLoadAndRotate_SkipsMalformed` | Malformed JSON lines skipped, valid parsed |
| `TestLoadAndRotate_Rotates` | File renamed to `.processed` on success |
| `TestGroupBySession` | Events grouped correctly by (sessionID, projectID) |
| `TestConfidenceStepUp` | 0.3 → 0.5 → 0.7 → 0.9, capped |
| `TestConfidenceStepDown` | 0.9 → 0.7 → 0.5 → 0.3, floored |
| `TestGlobalPromotion` | Promoted when ≥ 0.8 and ≥ 2 projects |
| `TestEngramClient_Store` | HTTP roundtrip via `httptest.NewServer` |
| `TestEngramClient_Recall` | Parses recall response, returns nil on no match |
| `TestPatternParse_SkipsInvalid` | Missing fields → entry skipped |

---

## Deletion Checklist

- [ ] Delete `consolidator/` (all Python, pyproject.toml, tests)
- [ ] Remove `uv sync` from `hooks/install.sh`
- [ ] Remove `CONSOLIDATOR*` vars from `hooks/post-tool-use.sh`
- [ ] Add `cmd/instinct/` to `.github/workflows/ci.yml` coverage scope if needed
- [ ] Update `README.md` install instructions (no more Python/uv requirement)
