#!/usr/bin/env bash
# instinct-shadow.sh — Phase 2 shadow consolidation runner (15-min cron)
#
# PURPOSE: Runs the Go consolidator against real buffer traffic in a SEPARATE
# *-shadow Engram namespace so we can compare Go vs Python verdicts without
# touching the live hook or the authoritative Python path.
#
# LLM_BACKEND TOGGLE SCHEDULE
# ─────────────────────────────────────────────────────────────────────────────
# Even-numbered hours (0,2,4,6,...,22) → LLM_BACKEND=anthropic
# Odd-numbered  hours (1,3,5,7,...,23) → LLM_BACKEND=olla
#
# With 15-min granularity over 48h this guarantees each backend is exercised
# at least 24 windows per 48h, regardless of when the soak starts.
# ─────────────────────────────────────────────────────────────────────────────
#
# IDEMPOTENCY: files already copied to shadow-inbox are never reprocessed;
# the inbox basename is the discriminator.
#
# SAFETY: set -uo pipefail (NOT -e) — failures are logged; cron exit is always 0.
#         flock prevents overlapping runs.

set -uo pipefail

# ── Constants ──────────────────────────────────────────────────────────────
INSTINCT_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/instinct"
INBOX_DIR="$INSTINCT_DIR/shadow-inbox"
LOG_FILE="$INSTINCT_DIR/shadow.log"
CONSOLIDATE_BIN="$HOME/bin/instinct-consolidate"
LOCK_FILE="/tmp/instinct-shadow.lock"
# Engram base URL — WITHOUT the /sse suffix (the Go binary appends it)
export ENGRAM_BASE_URL="http://127.0.0.1:8788"
# Engram API key — read from .env if not already set in environment
if [[ -z "${ENGRAM_API_KEY:-}" ]]; then
    _env_file="$HOME/projects/engram-go/.env"
    if [[ -r "$_env_file" ]]; then
        export ENGRAM_API_KEY
        ENGRAM_API_KEY=$(grep -E '^ENGRAM_API_KEY=' "$_env_file" | cut -d= -f2- | tr -d '\n')
    fi
fi
# Seen-list: persistent record of source files already shadowed (survives inbox rotation)
SEEN_FILE="$INSTINCT_DIR/shadow-seen.txt"

mkdir -p "$INBOX_DIR"

# ── Logging ────────────────────────────────────────────────────────────────
log() {
    local ts
    ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    echo "${ts} instinct-shadow: $*" >> "$LOG_FILE"
}

# ── LLM backend toggle ─────────────────────────────────────────────────────
current_hour=$(date +%H | sed 's/^0*//' || echo 0)
current_hour=${current_hour:-0}
if (( current_hour % 2 == 0 )); then
    export LLM_BACKEND="anthropic"
else
    export LLM_BACKEND="olla"
fi
log "LLM_BACKEND=${LLM_BACKEND} (hour=${current_hour})"

# ── Flock: prevent overlapping runs ────────────────────────────────────────
exec 9>"$LOCK_FILE"
if ! flock -n 9; then
    log "SKIP another instance is running (lock held)"
    exit 0
fi

# ── Find most-recent .processed file not yet in inbox ─────────────────────
# Python consolidator rotates buffer.jsonl → buffer.jsonl.<TS>.processed
# Go consolidator also produces .processed — we look for files from either.
# We scan ALL .processed files and pick the newest one not yet shadowed.

latest_processed=""
latest_mtime=0

touch "$SEEN_FILE"
for f in "$INSTINCT_DIR"/buffer.jsonl.*.processed; do
    [[ -f "$f" ]] || continue
    fname=$(basename "$f")
    # Skip if already recorded in the seen-list (survives inbox rotation by Go binary)
    if grep -qxF "$fname" "$SEEN_FILE"; then
        continue
    fi
    mtime=$(stat -c %Y "$f" 2>/dev/null || echo 0)
    if (( mtime > latest_mtime )); then
        latest_mtime=$mtime
        latest_processed="$f"
    fi
done

if [[ -z "$latest_processed" ]]; then
    log "SKIP no unprocessed .processed files found"
    exit 0
fi

# ── Copy to inbox with timestamp prefix for uniqueness ────────────────────
src_fname=$(basename "$latest_processed")
inbox_ts=$(date -u +%Y%m%dT%H%M%SZ)
inbox_file="$INBOX_DIR/${inbox_ts}-${src_fname}"

if ! cp "$latest_processed" "$inbox_file"; then
    log "ERROR failed to copy $latest_processed to $inbox_file"
    exit 0
fi
echo "$src_fname" >> "$SEEN_FILE"
log "COPIED $latest_processed → $inbox_file"

# ── Rewrite project_id to add -shadow suffix ──────────────────────────────
# The Go binary reads project_id FROM the buffer events. Since INSTINCT_PROJECT_SUFFIX
# is not implemented, we mutate the inbox copy to append -shadow to each project_id.
if ! python3 - "$inbox_file" <<'PYEOF'
import json, sys, os, tempfile

path = sys.argv[1]
out_lines = []
with open(path, "r", encoding="utf-8") as f:
    for line in f:
        line = line.strip()
        if not line:
            continue
        try:
            ev = json.loads(line)
            pid = ev.get("project_id", "")
            if pid and not pid.endswith("-shadow"):
                ev["project_id"] = pid + "-shadow"
            out_lines.append(json.dumps(ev))
        except json.JSONDecodeError:
            out_lines.append(line)  # preserve malformed lines as-is

tmp = path + ".tmp"
with open(tmp, "w", encoding="utf-8") as f:
    f.write("\n".join(out_lines) + "\n")
os.replace(tmp, path)
PYEOF
then
    log "ERROR failed to rewrite project_ids in $inbox_file"
    exit 0
fi
log "REWRITTEN project_ids → *-shadow in $inbox_file"

# ── Run Go consolidator ────────────────────────────────────────────────────
if [[ ! -x "$CONSOLIDATE_BIN" ]]; then
    log "ERROR Go consolidator not found at $CONSOLIDATE_BIN"
    exit 0
fi

# Resolve API key (env var → known key file)
_api_key="${ANTHROPIC_API_KEY:-}"
if [[ -z "$_api_key" ]]; then
    _key_file="$HOME/.config/gmail-job-tracker/anthropic_api_key"
    [[ -r "$_key_file" ]] && _api_key=$(tr -d '\n' < "$_key_file")
fi

start_ts=$(date +%s)

if INSTINCT_BUFFER="$inbox_file" \
   ANTHROPIC_API_KEY="$_api_key" \
   "$CONSOLIDATE_BIN" >> "$LOG_FILE" 2>&1; then
    rc=0
else
    rc=$?
fi

end_ts=$(date +%s)
runtime=$(( end_ts - start_ts ))

log "DONE file=${src_fname} backend=${LLM_BACKEND} rc=${rc} runtime=${runtime}s"

# Script always exits 0 — cron chain must not break on shadow failure
exit 0
