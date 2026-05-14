---
name: Agent tool must be explicitly in permissions allowlist
description: Without Agent in the allowlist, subagent spawns are silently auto-blocked in plan mode without showing a permission prompt
type: feedback
originSessionId: 1dbc5cfd-00c3-4482-aafd-1db78696edcb
---
Add `"Agent"` to `permissions.allow` in `~/.claude/settings.json`. Without it, Agent tool calls in plan mode (defaultMode: plan) are silently auto-rejected at the framework level — no prompt appears, the call just disappears.

**Why:** Discovered 2026-04-28. `mcp__engram__memory_recall` was in the allowlist but `Agent` was not. Both appeared to "sit there and do nothing" — actually the Agent was being auto-blocked silently and the Engram call was hanging on the unmatched PostToolUse hook.

**How to apply:** When setting up a new Claude Code environment or after adding new agent-dispatching workflows, verify `Agent` is in the allowlist. It is not included by default.
