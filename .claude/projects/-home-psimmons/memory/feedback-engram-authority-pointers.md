---
name: Engram indexes files, does not replace them
description: After ingesting file-based memories into Engram, add an index note but KEEP all content intact — never truncate
type: feedback
---

After ingesting file-based memories into Engram, add a small note that Engram indexes the file — but KEEP all content intact. Never truncate files to authority pointers.

**Why:** Engram is a search acceleration layer (vector + graph + BM25), not a replacement for the file system. Files are zero-dependency, crash-proof, auditable, git-tracked. Engram has moving parts (Python venv, SQLite, Ollama, MCP) that can break. Truncating files converts graceful degradation into cliff-edge failure.

**How to apply:** After ingesting a file into Engram, add this note near the top:
```
> Also indexed in Engram (project: `<project>`, tags: `<relevant,tags>`)
```
Keep ALL original content below it. If Engram is down, agents read the file directly.
