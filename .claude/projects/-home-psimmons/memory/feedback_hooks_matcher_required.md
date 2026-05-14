---
name: PostToolUse hooks must always have a matcher
description: A PostToolUse hook with no matcher fires on every tool including Agent and MCP calls, causing silent hangs
type: feedback
originSessionId: 1dbc5cfd-00c3-4482-aafd-1db78696edcb
---
Always add a `matcher` field to every `PostToolUse` hook entry in settings.json. A hook block without a matcher fires after **every** tool use — including `Agent`, `mcp__*`, `Read`, everything. This causes 5-second hangs on every tool call while the hook script runs, and can produce silent rejections if the script fails.

**Why:** Discovered 2026-04-28 when `instinct-post-tool-use.sh` (timeout: 5s, runs `git remote get-url`) had no matcher and was firing on all MCP and Agent tool calls. Tools appeared to "sit there and do nothing" without prompting the user.

**How to apply:** When writing or auditing `PostToolUse` hooks, check every entry for a `matcher` field. For instinct/telemetry hooks that only care about write operations, use `"matcher": "Edit|Write|Bash|Agent"`. Never leave a PostToolUse hook block without a matcher unless you explicitly intend it to fire universally.
