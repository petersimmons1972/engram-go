---
description: Inspect a memory for conflicts and evidence, aggregate memory counts by category, or force-summarize a memory
---

# engram-diagnose

Inspect the internal state of specific memories and understand what is in the
store by category. Useful when a `memory_recall` result seems wrong, when you
want to understand the confidence and source evidence behind a memory, or when
you need a high-level count of what the store contains.

These tools are hidden from the normal tools/list. This skill calls them
directly via HTTP.

## When to Use

- User says "diagnose this memory", "why does Engram think X", "what's the evidence for memory <id>"
- User says "how many memories do I have", "aggregate by tag", "break down memory by type"
- User says "summarize memory <id>", "force summarize this entry"
- A `memory_recall` result looks wrong or contradicts what you know to be true
- Before correcting a memory — understand its evidence first

## How to Use

### 1. Diagnose a single memory — get its evidence map

```bash
xh POST "${ENGRAM_BASE_URL:-http://localhost:8788}/mcp" \
  "Authorization: Bearer $ENGRAM_API_KEY" \
  Content-Type:application/json \
  jsonrpc=2.0 id:=1 method=tools/call \
  params:='{"name":"memory_diagnose","arguments":{"id":"<memory-id>"}}'
```

Returns:
- **conflicts** — other memories that contradict this one
- **confidence** — numeric confidence score
- **invalidated_sources** — sources that have been marked as no longer reliable
- **evidence** — the raw source entries that contributed to this memory

Read the response carefully. If `conflicts` is non-empty, report each
conflicting memory ID and its content to the user. If `confidence` is low
(below 0.4), flag it as uncertain.

### 2. Aggregate memory counts by group

```bash
xh POST "${ENGRAM_BASE_URL:-http://localhost:8788}/mcp" \
  "Authorization: Bearer $ENGRAM_API_KEY" \
  Content-Type:application/json \
  jsonrpc=2.0 id:=1 method=tools/call \
  params:='{"name":"memory_aggregate","arguments":{"project":"<project>","group_by":"<tag|type|failure_class>"}}'
```

`group_by` options:
- `tag` — count by tag label
- `type` — count by memory type (decision, error, pattern, context, etc.)
- `failure_class` — count by failure classification (useful for error memories)

Present the result as a table with group name and count. This is the fastest
way to understand what a project's memory store contains without reading every
entry.

### 3. Summarize a specific memory

```bash
xh POST "${ENGRAM_BASE_URL:-http://localhost:8788}/mcp" \
  "Authorization: Bearer $ENGRAM_API_KEY" \
  Content-Type:application/json \
  jsonrpc=2.0 id:=1 method=tools/call \
  params:='{"name":"memory_summarize","arguments":{"id":"<memory-id>"}}'
```

Forces the summarizer to run immediately on this memory, rather than waiting
for the background batch. Useful after `memory_diagnose` reveals a memory
with missing or stale summary text.

## Tools Available

| Tool | Arguments | Effect |
|------|-----------|--------|
| `memory_diagnose` | `id` (string) | Return evidence map: conflicts, confidence, invalidated sources |
| `memory_aggregate` | `project` (string), `group_by` ("tag"\|"type"\|"failure_class") | Count memories by group across the project |
| `memory_summarize` | `id` (string) | Force-generate a summary for a single memory |

## Decision Order

1. User has a specific memory ID and wants to understand it → `memory_diagnose`
2. User wants a breakdown of what the store contains → `memory_aggregate`
3. User wants a memory summarized immediately → `memory_summarize`
4. After diagnosing a conflict → consider using `mcp__engram__memory_correct` (visible tool) to fix it

## Diagnosing a Bad Recall — Full Workflow

```
1. Run memory_recall to get the suspicious memory ID
2. Run memory_diagnose <id> — read conflicts and confidence
3. If conflicts exist: diagnose each conflicting ID
4. Report the conflict chain to the user
5. Use memory_correct (visible MCP tool) to fix the winner
6. Use memory_forget (visible MCP tool) to remove the loser if needed
```

## Error Handling

If the memory ID does not exist: confirm the ID from a `memory_recall` or
`memory_list` result before retrying.

If `group_by` returns an empty result: the project may have no memories of
that type. Try a different `group_by` value or confirm the correct project name.

If the server is unreachable: report the error. Diagnostic operations cannot
be staged offline — they require a live server response.
