---
description: Bulk-load files, directories, or exported chat histories into Engram; export memories to markdown
---

# engram-ingest

Load large volumes of content into Engram memory without hand-crafting
individual `memory_store` calls. Supports local files and directories, exported
chat histories from Slack, Claude, and ChatGPT, and full memory exports for
backup.

These tools are hidden from the normal tools/list. This skill calls them
directly via HTTP.

## When to Use

- User says "ingest this file", "load this directory into memory", "import my notes"
- User says "parse my Slack export", "import my Claude history", "ingest ChatGPT export"
- User says "export all memories", "back up Engram", "dump memories to files"
- User says "check ingest status", "how is the ingestion job doing", "is the import done"
- Any bulk load of existing documents or conversation history

## How to Use

### 1. Ingest a local file or directory

```bash
xh POST "${ENGRAM_BASE_URL:-http://localhost:8788}/mcp" \
  "Authorization: Bearer $ENGRAM_API_KEY" \
  Content-Type:application/json \
  jsonrpc=2.0 id:=1 method=tools/call \
  params:='{"name":"memory_ingest","arguments":{"path":"<file-or-directory-path>","project":"<project>"}}'
```

`path` can be an absolute path to a single file or a directory. When a
directory is given, Engram walks it recursively and ingests all supported
files. The response includes a `job_id` for async status checks.

**Supported file types**: Markdown, plain text, JSON, and other document
formats Engram knows how to parse.

### 2. Ingest an exported chat history

```bash
xh POST "${ENGRAM_BASE_URL:-http://localhost:8788}/mcp" \
  "Authorization: Bearer $ENGRAM_API_KEY" \
  Content-Type:application/json \
  jsonrpc=2.0 id:=1 method=tools/call \
  params:='{"name":"memory_ingest_export","arguments":{"path":"<export-file-path>","project":"<project>","source":"<slack|claude|chatgpt>"}}'
```

Set `source` to match the origin of the export:
- `slack` — Slack export zip or JSON
- `claude` — Claude conversation export
- `chatgpt` — ChatGPT conversation export JSON

The parser normalizes the format and extracts semantically meaningful
memories from the conversation history. Returns a `job_id`.

### 3. Check async ingestion job status

Ingest operations run asynchronously. Poll until `status` is `complete` or
`failed`.

```bash
xh POST "${ENGRAM_BASE_URL:-http://localhost:8788}/mcp" \
  "Authorization: Bearer $ENGRAM_API_KEY" \
  Content-Type:application/json \
  jsonrpc=2.0 id:=1 method=tools/call \
  params:='{"name":"memory_ingest_status","arguments":{"job_id":"<job-id>"}}'
```

Poll every 10–15 seconds for large jobs. Report progress to the user when
`status` changes. Stop polling when `status` is `complete` or `failed`.

### 4. Export all memories to markdown files

```bash
xh POST "${ENGRAM_BASE_URL:-http://localhost:8788}/mcp" \
  "Authorization: Bearer $ENGRAM_API_KEY" \
  Content-Type:application/json \
  jsonrpc=2.0 id:=1 method=tools/call \
  params:='{"name":"memory_export_all","arguments":{"project":"<project>","output_dir":"<absolute-directory-path>"}}'
```

Writes one markdown file per memory into `output_dir`. Creates the directory
if it does not exist. Use this for backups before major changes, or to hand
off memory content to another system.

## Tools Available

| Tool | Arguments | Effect |
|------|-----------|--------|
| `memory_ingest` | `path` (string), `project` (string) | Ingest local file or directory; returns `job_id` |
| `memory_ingest_export` | `path` (string), `project` (string), `source` ("slack"\|"claude"\|"chatgpt") | Parse exported chat history; returns `job_id` |
| `memory_ingest_status` | `job_id` (string) | Check async job status and progress |
| `memory_export_all` | `project` (string), `output_dir` (string) | Export all project memories to markdown files |

## Typical Ingest Workflow

```
1. memory_ingest / memory_ingest_export  → get job_id
2. memory_ingest_status (job_id)         → poll until complete
3. Report: N memories ingested, M failed
```

## Error Handling

If `path` does not exist: verify the path before calling. Use an absolute path.

If `job_id` is unknown: it may have expired. Re-run the ingest and use the
new `job_id`.

If `status` is `failed`: read the error field from the status response and
report it to the user. Common causes: unsupported file format, malformed
export, permission error on path.

If the server is unreachable: stage a note in
`~/.claude/projects/-home-psimmons/memory/fallback.md` with the path and
project, and retry when Engram reconnects.
