---
description: Compress and prune Engram memory after long sessions or when the store feels cluttered
---

# engram-consolidate

Run memory maintenance operations against the Engram MCP server. These tools
are hidden from the normal tools/list to keep context lean. This skill calls
them directly via HTTP so you can trigger consolidation without bloating the
active tool roster.

## When to Use

- After a long or busy work session where many memories were stored
- When `memory_recall` results feel noisy or redundant
- When you want to force a summary onto a single memory
- When you want to clear all cached summaries for a project and let them regenerate
- User says "consolidate memory", "sleep Engram", "clean up memory", "prune memory", "summarize memory for X"

## How to Use

Identify which operation the user wants, then issue the matching `xh` call
below. All calls go to the MCP JSON-RPC endpoint. Capture the response and
report the result (or error) to the user.

### 1. Quick consolidation — prune stale entries, decay edges, merge near-duplicates

```bash
xh POST "${ENGRAM_BASE_URL:-http://localhost:8788}/mcp" \
  "Authorization: Bearer $ENGRAM_API_KEY" \
  Content-Type:application/json \
  jsonrpc=2.0 id:=1 method=tools/call \
  params:='{"name":"memory_consolidate","arguments":{"project":"<project>"}}'
```

Use this first. It is fast (seconds to low minutes). Appropriate after any
normal session.

### 2. Deep consolidation — full cycle with relationship inference

```bash
xh POST "${ENGRAM_BASE_URL:-http://localhost:8788}/mcp" \
  "Authorization: Bearer $ENGRAM_API_KEY" \
  Content-Type:application/json \
  jsonrpc=2.0 id:=1 method=tools/call \
  params:='{"name":"memory_sleep","arguments":{"project":"<project>"}}'
```

Slower and more thorough than `memory_consolidate`. Runs relationship inference
across all memories in the project. Use when the quick consolidation is not
enough, or at end-of-week cleanup. Warn the user this may take a few minutes.

### 3. Summarize a single memory

```bash
xh POST "${ENGRAM_BASE_URL:-http://localhost:8788}/mcp" \
  "Authorization: Bearer $ENGRAM_API_KEY" \
  Content-Type:application/json \
  jsonrpc=2.0 id:=1 method=tools/call \
  params:='{"name":"memory_summarize","arguments":{"id":"<memory-id>"}}'
```

Use when the user has a specific memory ID they want summarized immediately,
rather than waiting for the background summarizer.

### 4. Clear all summaries for a project (they regenerate within 60 seconds)

```bash
xh POST "${ENGRAM_BASE_URL:-http://localhost:8788}/mcp" \
  "Authorization: Bearer $ENGRAM_API_KEY" \
  Content-Type:application/json \
  jsonrpc=2.0 id:=1 method=tools/call \
  params:='{"name":"memory_resummarize","arguments":{"project":"<project>"}}'
```

Use when summaries are stale or incorrect. The background summarizer will
regenerate all of them within approximately 60 seconds.

## Tools Available

| Tool | Arguments | Effect |
|------|-----------|--------|
| `memory_consolidate` | `project` (string) | Prune stale memories, decay graph edges, merge near-duplicates. Fast. |
| `memory_sleep` | `project` (string) | Full consolidation cycle with relationship inference. Slow but thorough. |
| `memory_summarize` | `id` (string) | Immediately summarize a single memory by ID. |
| `memory_resummarize` | `project` (string) | Clear all summaries for a project so they regenerate fresh. |

## Decision Order

1. User mentions a specific memory ID → use `memory_summarize`
2. User wants to regenerate all summaries → use `memory_resummarize`
3. User wants quick cleanup after a session → use `memory_consolidate`
4. User wants deep maintenance / end-of-week → use `memory_sleep`

## Error Handling

If the server is unreachable: report the error and stage a note in
`~/.claude/projects/-home-psimmons/memory/fallback.md` reminding the user
to run consolidation when Engram comes back online.

If `ENGRAM_API_KEY` is not set: halt and tell the user to set the variable
before retrying.
