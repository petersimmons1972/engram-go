#!/usr/bin/env bash
set -u

# Minimal connectivity check for claude-codex communication infrastructure.
# Keep non-blocking so SessionStart can continue even if CLI/auth/network is
# unavailable in the current environment.

# Write log to private dir — /tmp/claude-codex-session-start.log was mode 664
# (world-readable), exposing any agent-status output to other local users. (FM-95)
mkdir -p "$HOME/.claude/logs"
if /home/psimmons/bin/agent-status --agent codex --brief >"$HOME/.claude/logs/codex-session-start.log" 2>&1; then
  echo "claude-codex: communication infrastructure reachable (agent/codex status checked)."
else
  # Keep startup resilient; this hook is advisory only.
  :
fi

exit 0
