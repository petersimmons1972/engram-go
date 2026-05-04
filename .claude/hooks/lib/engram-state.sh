#!/usr/bin/env bash
# Shared state helper for the Engram hook pipeline.
# Provides update_state and read_state for ~/.claude/.engram-hook-state.json.
# Uses flock + atomic write so concurrent hook invocations never corrupt the file.
# Source this file at the top of any hook that needs to read or update state.
# engram-go#404

STATE_FILE="${ENGRAM_TEST_STATE_FILE:-$HOME/.claude/.engram-hook-state.json}"
STATE_LOCK="${STATE_FILE}.lock"

_state_defaults='{
  "last_session_start": null,
  "last_recall_results": 0,
  "last_flush_at": null,
  "fallback_entry_count": 0,
  "consecutive_auth_failures": 0,
  "last_auth_ok_at": null,
  "sessions_since_last_flush": 0
}'

update_state() {
    # $1: JSON key   $2: value (valid JSON — strings must be pre-quoted)
    local key="$1"
    local val="$2"
    python3 - "$STATE_FILE" "$STATE_LOCK" "$key" "$val" <<'PYEOF'
import json, sys, os, tempfile, fcntl

path, lock_path, key, val = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]

defaults = {
    "last_session_start": None,
    "last_recall_results": 0,
    "last_flush_at": None,
    "fallback_entry_count": 0,
    "consecutive_auth_failures": 0,
    "last_auth_ok_at": None,
    "sessions_since_last_flush": 0,
}

with open(lock_path, "w") as lf:
    fcntl.flock(lf, fcntl.LOCK_EX)
    try:
        with open(path) as f:
            state = json.load(f)
    except (FileNotFoundError, json.JSONDecodeError):
        state = defaults.copy()
    try:
        state[key] = json.loads(val)
    except json.JSONDecodeError:
        state[key] = val
    dir_ = os.path.dirname(path) or "."
    fd, tmp = tempfile.mkstemp(dir=dir_, prefix=".engram-state-tmp")
    try:
        with os.fdopen(fd, "w") as f:
            json.dump(state, f, indent=2)
            f.write("\n")
        os.replace(tmp, path)
    except Exception:
        os.unlink(tmp)
        raise
PYEOF
}

read_state() {
    # $1: key — prints the value or empty string on error
    python3 -c "
import json, sys
try:
    d = json.load(open('${STATE_FILE}'))
    v = d.get('${1}', '')
    print('' if v is None else v)
except:
    print('')
" 2>/dev/null || true
}

increment_state() {
    # $1: key — atomically increment an integer field by 1
    local key="$1"
    python3 - "$STATE_FILE" "$STATE_LOCK" "$key" <<'PYEOF'
import json, sys, os, tempfile, fcntl

path, lock_path, key = sys.argv[1], sys.argv[2], sys.argv[3]

defaults = {
    "last_session_start": None,
    "last_recall_results": 0,
    "last_flush_at": None,
    "fallback_entry_count": 0,
    "consecutive_auth_failures": 0,
    "last_auth_ok_at": None,
    "sessions_since_last_flush": 0,
}

with open(lock_path, "w") as lf:
    fcntl.flock(lf, fcntl.LOCK_EX)
    try:
        with open(path) as f:
            state = json.load(f)
    except (FileNotFoundError, json.JSONDecodeError):
        state = defaults.copy()
    state[key] = int(state.get(key) or 0) + 1
    dir_ = os.path.dirname(path) or "."
    fd, tmp = tempfile.mkstemp(dir=dir_, prefix=".engram-state-tmp")
    try:
        with os.fdopen(fd, "w") as f:
            json.dump(state, f, indent=2)
            f.write("\n")
        os.replace(tmp, path)
    except Exception:
        os.unlink(tmp)
        raise
PYEOF
}
