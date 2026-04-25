#!/usr/bin/env bash
# Check that instinct consolidator has run recently.
# Parses the ISO-8601 timestamp embedded in log output — not file mtime.
# Exit 0 = healthy, exit 1 = stale/missing
set -euo pipefail

LOG_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/instinct"
LOG="$LOG_DIR/run.log"
MAX_GAP_SECONDS="${INSTINCT_MAX_GAP:-7200}"  # 2 hours default

if [[ ! -f "$LOG" ]]; then
    echo "WARN: instinct run.log not found at $LOG"
    exit 1
fi

# Extract the most recent ISO-8601 timestamp from log lines of the form:
#   instinct: ... [2026-04-22T10:00:00Z]
last_ts=$(grep -oP '\[\K\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z(?=\])' "$LOG" 2>/dev/null | tail -1 || :)

if [[ -z "$last_ts" ]]; then
    echo "WARN: no timestamped instinct runs found in $LOG"
    exit 1
fi

last_epoch=$(date -d "$last_ts" +%s 2>/dev/null || date -j -f "%Y-%m-%dT%H:%M:%SZ" "$last_ts" +%s 2>/dev/null || echo 0)
now=$(date +%s)
gap=$(( now - last_epoch ))

if (( gap > MAX_GAP_SECONDS )); then
    echo "WARN: instinct last ran ${gap}s ago (threshold: ${MAX_GAP_SECONDS}s)"
    echo "Last timestamp: $last_ts"
    exit 1
fi

echo "OK: instinct last ran ${gap}s ago"
echo "Last: $last_ts"
exit 0
