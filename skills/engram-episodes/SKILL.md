---
description: Start, end, list, and recall named work session episodes in Engram memory
---

# engram-episodes

Group memories into named episodes so you can recall everything that happened
during a discrete work session. Episodes give Engram a timeline structure —
instead of searching by topic, you retrieve by "what happened during session X".

These tools are hidden from the normal tools/list. This skill calls them
directly via HTTP.

## When to Use

- User says "start an episode", "begin a session", "open an episode for <project>"
- User says "end the episode", "close the session", "finish this work block"
- User says "list episodes", "what episodes exist", "show me recent episodes"
- User says "recall episode <id>", "what happened in episode X", "replay episode"
- Any time you want to bracket a discrete block of work for later retrieval

## How to Use

Follow the workflow below in order. Each step shows the exact command.

### 1. Start an episode — call this at the beginning of a named work block

```bash
xh POST "${ENGRAM_BASE_URL:-http://localhost:8788}/mcp" \
  "Authorization: Bearer $ENGRAM_API_KEY" \
  Content-Type:application/json \
  jsonrpc=2.0 id:=1 method=tools/call \
  params:='{"name":"memory_episode_start","arguments":{"project":"<project>","description":"<description>"}}'
```

The response includes an `id` field — save this episode ID. You will need it
to close the episode and to recall it later. Tell the user the episode ID so
they can reference it.

### 2. Store memories normally during the session

Use `memory_store` or `memory_quick_store` as usual. Memories stored while an
episode is open are automatically associated with it.

### 3. End an episode — call this when the work block is complete

```bash
xh POST "${ENGRAM_BASE_URL:-http://localhost:8788}/mcp" \
  "Authorization: Bearer $ENGRAM_API_KEY" \
  Content-Type:application/json \
  jsonrpc=2.0 id:=1 method=tools/call \
  params:='{"name":"memory_episode_end","arguments":{"id":"<episode-id>","project":"<project>","summary":"<optional-one-sentence-summary>"}}'
```

The `summary` field is optional but recommended — a single sentence describing
what was accomplished. If the user did not provide one, write a concise summary
from what was done during the session.

### 4. List recent episodes for a project

```bash
xh POST "${ENGRAM_BASE_URL:-http://localhost:8788}/mcp" \
  "Authorization: Bearer $ENGRAM_API_KEY" \
  Content-Type:application/json \
  jsonrpc=2.0 id:=1 method=tools/call \
  params:='{"name":"memory_episode_list","arguments":{"project":"<project>","limit":10}}'
```

Returns the most recent episodes. Increase `limit` if the user wants further
history. Present results as a table: episode ID, description, status, date.

### 5. Recall all memories from a specific episode

```bash
xh POST "${ENGRAM_BASE_URL:-http://localhost:8788}/mcp" \
  "Authorization: Bearer $ENGRAM_API_KEY" \
  Content-Type:application/json \
  jsonrpc=2.0 id:=1 method=tools/call \
  params:='{"name":"memory_episode_recall","arguments":{"id":"<episode-id>","project":"<project>"}}'
```

Returns memories in chronological order. Use this to reconstruct what happened
during a past session, brief a new agent, or produce a session summary report.

## Tools Available

| Tool | Arguments | Effect |
|------|-----------|--------|
| `memory_episode_start` | `project` (string), `description` (string) | Open a new episode; returns `id` |
| `memory_episode_end` | `id` (string), `project` (string), `summary` (string, optional) | Close the episode |
| `memory_episode_list` | `project` (string), `limit` (int, default 10) | List recent episodes |
| `memory_episode_recall` | `id` (string), `project` (string) | Retrieve all memories in the episode in order |

## Typical Full-Session Workflow

```
session start  → memory_episode_start  (note the id)
during session → memory_store / memory_quick_store (normal)
session end    → memory_episode_end    (include a summary)
later          → memory_episode_list   (find the id)
               → memory_episode_recall (replay what happened)
```

## Error Handling

If the episode ID is unknown: run `memory_episode_list` to find the correct ID
before retrying.

If the server is unreachable: stage a note in
`~/.claude/projects/-home-psimmons/memory/fallback.md` with the episode
description and timestamp. Open the episode when Engram reconnects.
