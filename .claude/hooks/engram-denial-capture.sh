#!/usr/bin/env bash
# PostToolUse hook: capture CC's synthesized "user rejected" tool denials
# with full context. Fires on any mcp__engram__* tool call result.
#
# When CC's runtime times out an MCP call, it synthesizes a tool result of
# "User rejected tool use" — even when the user took no action. This hook
# captures the tool_input + engram /health snapshot at the moment of denial,
# appending a structured JSON line to denial-log.md.
#
# Filed as petersimmons1972/engram-go#605.

set -euo pipefail

DENIAL_LOG="${ENGRAM_TEST_DENIAL_LOG:-$HOME/.claude/projects/-home-psimmons/memory/denial-log.md}"

raw_input="$(cat)"

# All work delegated to a single Python script — avoids bash heredoc/quote complexity.
# Python emits {"systemMessage":"..."} to stdout on denial; pass it through to CC.
printf '%s' "$raw_input" | python3 "$HOME/.claude/hooks/engram-denial-capture.py" "$DENIAL_LOG" 2>/dev/null || true

exit 0
