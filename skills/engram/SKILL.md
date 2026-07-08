---
description: Hidden Engram ops: bulk ingest from files/exports, memory consolidation/sleep/summarize, and episode lifecycle (start/end/list/recall).
---

# engram

Access hidden Engram MCP tools that are intentionally excluded from `tools/list`
to prevent accidental triggering during normal recall/store workflows. All calls
go directly to the MCP JSON-RPC endpoint via `xh`.

## Routing

Identify which operation category applies, then read the matching reference file:

- Bulk ingest from files, directories, or exports → [reference/ingest.md](reference/ingest.md)
- Memory consolidation, sleep, summarize, resummarize → [reference/consolidate.md](reference/consolidate.md)
- Episode start, end, list, recall → [reference/episodes.md](reference/episodes.md)

## Common Setup

All calls use:

```bash
xh POST "${ENGRAM_BASE_URL:-http://localhost:8788}/mcp" \
  "Authorization: Bearer ${ENGRAM_API_KEY}" \
  Content-Type:application/json \
  jsonrpc=2.0 id:=1 method=tools/call \
  params:='{"name":"<tool>","arguments":{...}}'
```

If `ENGRAM_API_KEY` is not set: halt and tell the user to set the variable
before retrying.

If the server is unreachable: stage a note in
`${ENGRAM_FALLBACK_PATH:-~/.claude/engram/fallback.md}` and retry when Engram
reconnects.
