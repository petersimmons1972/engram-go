#!/usr/bin/env bash
# Engram hook shim (#396) — forwards a Claude Code hook event to the long-running
# `engram hook-daemon` over a Unix socket and relays the response back to Claude
# Code. Replaces the per-event shell scripts when ENGRAM_HOOK_DAEMON=1.
#
# Usage (from a settings.json hook registration):
#   engram-hook-shim.sh <EventName>
#
# The event name is passed as $1; the raw Claude Code hook stdin JSON is read
# from stdin and forwarded as the payload. The `engram hook` subcommand is a
# pure-Go client — no socat dependency — and lazy-starts the daemon on the
# first event of a session (Option C: lazy-start + idle-timeout).
#
# Fallback: when ENGRAM_HOOK_DAEMON is not set to 1, this shim is a no-op and the
# legacy per-event scripts (still registered) handle the event. This lets the
# daemon roll out behind a flag until it is proven stable.
set -euo pipefail

EVENT="${1:-}"
[[ -z "$EVENT" ]] && exit 0

# Feature flag: daemon path is opt-in until stable.
if [[ "${ENGRAM_HOOK_DAEMON:-0}" != "1" ]]; then
  # Drain stdin so the producer never blocks, then defer to legacy scripts.
  cat >/dev/null 2>&1 || true
  exit 0
fi

# Locate the engram binary: PATH first, then the repo build output.
ENGRAM_BIN=""
if command -v engram >/dev/null 2>&1; then
  ENGRAM_BIN="engram"
elif [[ -x "$HOME/projects/engram-go/bin/engram" ]]; then
  ENGRAM_BIN="$HOME/projects/engram-go/bin/engram"
else
  # No binary available — never block the session.
  cat >/dev/null 2>&1 || true
  exit 0
fi

# Forward stdin to the daemon via the binary shim. `engram hook` reads stdin,
# dials the socket (lazy-starting the daemon if needed), prints the daemon's
# stdout, and exits with the daemon's exit code. Never blocks the session.
exec "$ENGRAM_BIN" hook "$EVENT"
