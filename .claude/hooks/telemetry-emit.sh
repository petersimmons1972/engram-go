#!/usr/bin/env bash
# claude-telemetry event hook. Must exit 0 unconditionally.
# See docs/design.md for contract.
#
# SECURITY FIXES (FM-99):
#   1. umask 0077 before mkdir: event files were created mode 664 (world-readable),
#      exposing tool_response payloads from mcp__infisical__get-secret calls.
#   2. Payload scrubbed before logging: tool_response (MCP return values) and
#      tool_input.content/new_string/old_string (file content) are stripped.
#      Error metadata (is_error, stderr) is preserved for diagnostics.

set +e
trap 'exit 0' ERR EXIT

EVENTS_DIR="${EVENTS_DIR:-$HOME/.claude/events}"
PROJECT_ROOTS_CONF="${PROJECT_ROOTS_CONF:-$HOME/.claude/hooks/project-roots.conf}"

# Mode 0700 for dir, 0600 for files — events may contain partial secret payloads
umask 0077
mkdir -p "$EVENTS_DIR"

input="$(cat)"
[ -z "$input" ] && exit 0

ts="$(date -u +%Y-%m-%dT%H:%M:%S.%3NZ)"
today="${ts:0:10}"

# Single jq invocation: extract all needed fields, one per line.
# Order: event_type, tool, payload_session_id, cwd, error_text
mapfile -t fields < <(echo "$input" | jq -r '
    (.hook_event_name // "unknown" | ascii_downcase),
    (.tool_name // ""),
    (.session_id // ""),
    (.cwd // ""),
    (
        if (.tool_response.is_error == true) then
            (.tool_response.stderr // .tool_response.stdout // "unknown-error")
        elif (.tool_response.error) then .tool_response.error
        else ""
        end | .[0:200] | gsub("\n"; " ")
    )
' 2>/dev/null)

event_type="${fields[0]:-unknown}"
tool="${fields[1]:-}"
payload_sid="${fields[2]:-}"
cwd="${fields[3]:-}"
error_text="${fields[4]:-}"

# Session ID precedence: env var → payload → .current-session file → UNKNOWN-pid
if [ -n "${CLAUDE_SESSION_ID:-}" ]; then
    session_id="$CLAUDE_SESSION_ID"
elif [ -n "$payload_sid" ]; then
    session_id="$payload_sid"
elif [ -f "$EVENTS_DIR/.current-session" ]; then
    session_id="$(cat "$EVENTS_DIR/.current-session")"
else
    session_id="UNKNOWN-$$"
fi

# SessionStart writes session_id to the fallback file
if [ "$event_type" = "sessionstart" ] && [[ ! "$session_id" =~ ^UNKNOWN- ]]; then
    echo "$session_id" > "$EVENTS_DIR/.current-session"
fi

# Error signature (sha1 of first 200 chars) if error present
error_signature=""
if [ -n "$error_text" ]; then
    error_signature="$(printf '%s' "$error_text" | sha1sum)"
    error_signature="${error_signature%% *}"
fi

# Derive project from cwd using the config file (pure bash, no fork)
project=""
if [ -n "$cwd" ] && [ -f "$PROJECT_ROOTS_CONF" ]; then
    while IFS=$'\t' read -r prefix name; do
        [[ "$prefix" =~ ^#.*$ ]] && continue
        [ -z "$prefix" ] && continue
        if [[ "$cwd" == "$prefix"* ]]; then
            project="$name"
            break
        fi
    done < "$PROJECT_ROOTS_CONF"
fi

# Scrub secret-bearing fields from payload before logging:
#   - tool_response: MCP get-secret calls return the secret value here
#   - tool_input.content, .new_string, .old_string: file content may embed secrets
# Preserve structural fields needed for diagnostics.
scrubbed="$(echo "$input" | jq '{
    hook_event_name: .hook_event_name,
    tool_name: .tool_name,
    session_id: .session_id,
    cwd: .cwd,
    tool_input: (if .tool_input then
        (.tool_input | del(.content, .new_string, .old_string, .command))
    else null end),
    tool_response_is_error: (.tool_response.is_error // null)
}' 2>/dev/null || echo '{}')"

# Build the output line in a single jq invocation
line="$(jq -nc \
    --arg ts "$ts" \
    --arg session_id "$session_id" \
    --arg event_type "$event_type" \
    --arg tool "$tool" \
    --arg error_signature "$error_signature" \
    --arg project "$project" \
    --argjson payload "$scrubbed" \
    '{
        ts: $ts,
        session_id: $session_id,
        event_type: $event_type,
        tool: (if $tool == "" then null else $tool end),
        error_signature: (if $error_signature == "" then null else $error_signature end),
        project: (if $project == "" then null else $project end),
        payload: $payload
    }')"

# Serialize appends with flock (100ms timeout — never block the session)
lockfile="$EVENTS_DIR/.${today}.lock"
(
    if flock -w 0.1 -x 9; then
        echo "$line" >> "$EVENTS_DIR/${today}.jsonl"
    else
        echo "$ts flock-timeout" >> "$EVENTS_DIR/.hook-errors.log"
    fi
) 9>"$lockfile"

exit 0
