---
name: Engram Fallback Staging
description: Temporary store for memories written while Engram is unavailable. Flush to Engram on reconnect.
type: reference
originSessionId: 0fc43d74-ceaf-4d5b-86c9-7a6e25ca0fc2
---
# Engram Fallback

This file is a staging area. When Engram is unreachable, store entries here in the format below.
On reconnect, call `memory_store` for each entry then delete it from this file.

## Pending Entries

<!-- Add entries below when Engram is down. Format:
## [YYYY-MM-DD] <title>
**Project:** <project>
**Type:** <decision|error|pattern|context>
**Tags:** [tag1, tag2]

<content>
-->
