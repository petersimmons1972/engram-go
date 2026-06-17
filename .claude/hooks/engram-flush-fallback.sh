#!/usr/bin/env bash
. ~/.claude/hooks/lib/timing.sh 2>/dev/null || true
# SessionStart hook: flush pending fallback.md entries to Engram via /quick-store.
# Runs after engram-token-refresh.sh so the server is confirmed up and token is fresh.
set -euo pipefail

# Load centralized endpoint
# shellcheck source=engram-endpoint.conf
source "$HOME/.claude/hooks/engram-endpoint.conf" 2>/dev/null || ENGRAM_BASE_URL="http://127.0.0.1:8788"

FALLBACK="${ENGRAM_TEST_FALLBACK:-$HOME/.claude/projects/-home-psimmons/memory/fallback.md}"
BASE="$ENGRAM_BASE_URL"

# shellcheck source=lib/engram-state.sh
source "$HOME/.claude/hooks/lib/engram-state.sh" 2>/dev/null || true

[[ -f "$FALLBACK" ]] || exit 0

# Quick exit if no pending entries
if ! grep -q "^\*\*Project:\*\*" "$FALLBACK" 2>/dev/null; then
    exit 0
fi

# Short-circuit: if Engram is known-degraded, skip flush — leave entries intact
# Degraded state expires after 20 minutes.
DISCONNECT_STATE="$HOME/.claude/.engram-disconnect-state"
if [[ -f "$DISCONNECT_STATE" ]]; then
  AGE_DISCONNECT=$(( $(date +%s) - $(date -r "$DISCONNECT_STATE" +%s 2>/dev/null || echo 0) ))
  if [[ "$AGE_DISCONNECT" -lt 1200 ]]; then
    echo "⚠️  engram-flush-fallback: degraded-skip — leaving fallback.md intact" >&2
    exit 0
  fi
  rm -f "$DISCONNECT_STATE"
fi

# Server must be up (engram-token-refresh.sh already ensured this, but double-check)
if ! curl -sf --max-time 2 "${BASE}/health" > /dev/null 2>&1; then
    echo "⚠️  engram-flush-fallback: Engram not reachable — leaving fallback.md intact" >&2
    exit 0
fi

# Resolve Bearer token in priority order:
# 1. ~/.config/engram/api_key  — written by `make init`, most reliable
# 2. ENGRAM_API_KEY in ~/projects/engram-go/.env — lower trust
# 3. /setup-token unauthenticated (TOFU one-time bootstrap only)
# The TOFU grant is consumed on first use; after bootstrap, disk keys are required.
TOKEN=""

# Source 1: ~/.config/engram/api_key
if [[ -z "$TOKEN" && -f "$HOME/.config/engram/api_key" ]]; then
    TOKEN=$(tr -d '[:space:]' < "$HOME/.config/engram/api_key" 2>/dev/null || true)
fi

# Source 2: ~/projects/engram-go/.env (ENGRAM_API_KEY=...)
if [[ -z "$TOKEN" && -f "$HOME/projects/engram-go/.env" ]]; then
    TOKEN=$(grep -m1 '^ENGRAM_API_KEY=' "$HOME/projects/engram-go/.env" 2>/dev/null \
        | cut -d= -f2- | tr -d '[:space:]' || true)
fi

# Source 3 (TOFU unauthenticated bootstrap) removed: ran on every SessionStart,
# making an unauthenticated request to /setup-token regardless of token status.
# The TOFU grant is consumed on first use — repeated calls are a silent no-op at
# best and a SSRF/token-hijack risk if the endpoint is ever misconfigured. (FM-96)
# If bootstrapping is needed: run `make init` in ~/projects/engram-go manually.

[[ -z "$TOKEN" ]] && { echo "⚠️  engram-flush-fallback: no token — skipping flush" >&2; exit 0; }

# Parse, snapshot, flush entries — lock held minimally (#398)
# Phase 1: snapshot under lock. Phase 2: HTTP flush (no lock). Phase 3: re-append failures under lock.
FLUSHED=$(TOKEN="$TOKEN" python3 - "$FALLBACK" "$BASE" <<'PYEOF'
import json, re, sys, urllib.request, urllib.error, os, tempfile, fcntl
from datetime import datetime, timezone, timedelta

fallback_path, base_url = sys.argv[1], sys.argv[2]
# Read TOKEN from env var then scrub it — passing secrets as argv exposes them
# in /proc/<pid>/cmdline (world-readable on Linux). (FM-97)
token = os.environ.pop('TOKEN', '')
lock_path = fallback_path + ".lock"
entry_re = re.compile(r'^## \[\d{4}-\d{2}-\d{2}\] .+', re.MULTILINE)

template = '''

<!-- Add entries below when Engram is down. Format:
## [YYYY-MM-DD] <title>
**Project:** <project>
**Type:** <decision|error|pattern|context>
**Tags:** [tag1, tag2]

<content>
-->
'''

def parse_chunks(content):
    """Return (header, chunks) from fallback.md content."""
    pending_start = content.find("## Pending Entries")
    if pending_start == -1:
        return None, []
    header = content[:pending_start + len("## Pending Entries")]
    entries_block = content[pending_start + len("## Pending Entries"):]
    entry_positions = [(m.start(), m.end()) for m in entry_re.finditer(entries_block)]
    chunks = []
    for i, (start, _) in enumerate(entry_positions):
        end = entry_positions[i + 1][0] if i + 1 < len(entry_positions) else len(entries_block)
        chunk = entries_block[start:end].strip()
        if chunk:
            chunks.append(chunk)
    return header, chunks

def write_atomic(path, content):
    dir_ = os.path.dirname(path) or "."
    fd, tmp = tempfile.mkstemp(dir=dir_, prefix='.fallback_tmp')
    try:
        with os.fdopen(fd, 'w') as f:
            f.write(content)
        os.replace(tmp, path)
    except Exception:
        os.unlink(tmp)
        raise

# --- Phase 1: snapshot under lock, clear the file ---
with open(lock_path, "w") as lock_fd:
    fcntl.flock(lock_fd, fcntl.LOCK_EX)

    try:
        with open(fallback_path) as f:
            content = f.read()
    except FileNotFoundError:
        sys.exit(0)

    header, chunks = parse_chunks(content)
    if header is None or not chunks:
        sys.exit(0)

    # Clear fallback.md while holding lock so no new writer sees stale entries
    write_atomic(fallback_path, header + template)
    # Lock released when with block exits

# --- Phase 2: HTTP flush (no lock held) ---
flushed = 0
failed_chunks = []

for chunk in chunks:
    lines = chunk.splitlines()
    if not lines:
        continue

    title_match = re.match(r'^## \[(\d{4}-\d{2}-\d{2})\] (.+)$', lines[0])
    if not title_match:
        failed_chunks.append(chunk)
        continue

    date_str, title = title_match.group(1), title_match.group(2)

    # Drop entries older than 7 days (#401)
    entry_date = datetime.strptime(date_str, '%Y-%m-%d').replace(tzinfo=timezone.utc)
    if datetime.now(timezone.utc) - entry_date > timedelta(days=7):
        sys.stderr.write(f'[engram-flush] Dropping stale entry from {date_str} (>7 days old)\n')
        flushed += 1  # count as flushed so it is not re-appended
        continue

    meta = {}
    body_lines = []
    in_body = False
    for line in lines[1:]:
        if not in_body:
            m = re.match(r'^\*\*(\w+)(?:\s+\w+)?:\*\*\s+(.+)$', line)
            if m:
                meta[m.group(1).lower()] = m.group(2).strip()
            elif line.strip() == '':
                in_body = True
        else:
            body_lines.append(line)

    project = meta.get('project', 'global')
    mem_type = meta.get('type', 'context')
    tags_raw = meta.get('tags', '')
    importance_raw = meta.get('importance', '1')

    tags = [t.strip().strip('[]') for t in re.split(r'[,\s]+', tags_raw) if t.strip().strip('[]')]
    tags.append('flushed-from-fallback')

    try:
        importance = int(importance_raw)
    except ValueError:
        importance = 1

    body = '\n'.join(body_lines).strip()
    if not body:
        failed_chunks.append(chunk)
        continue

    payload = json.dumps({
        'content': f'[{date_str}] {title}\n\n{body}',
        'project': project,
        'memory_type': mem_type,
        'tags': tags,
        'importance': importance,
    }).encode()

    MAX_RETRIES = 3
    retry_match = re.search(r'<!-- retry:(\d+) -->', chunk)
    retry_count = int(retry_match.group(1)) if retry_match else 0

    def _requeue(c, rc):
        # Drop after MAX_RETRIES total attempts (#397)
        # retry_count is attempts-so-far; rc+1 would be the next attempt number
        if rc + 1 >= MAX_RETRIES:
            sys.stderr.write(f'[engram-flush] Dropping entry "{title}" after {MAX_RETRIES} retries\n')
            return  # not re-appended — silently removed from fallback.md
        clean = re.sub(r'\n<!-- retry:\d+ -->', '', c)
        failed_chunks.append(clean + f'\n<!-- retry:{rc + 1} -->')

    req = urllib.request.Request(
        f'{base_url}/quick-store',
        data=payload,
        headers={'Authorization': f'Bearer {token}', 'Content-Type': 'application/json'},
        method='POST',
    )
    try:
        with urllib.request.urlopen(req, timeout=5) as resp:
            if resp.status < 300:
                flushed += 1
            else:
                # Unexpected 2xx+ non-success — requeue as transient
                _requeue(chunk, retry_count)
    except urllib.error.HTTPError as e:
        if 400 <= e.code < 500:
            # Permanent client error — log and drop, do not retry (#397)
            sys.stderr.write(f'[engram-flush] Dropping entry "{title}" — permanent {e.code}\n')
            flushed += 1
        else:
            # 5xx transient — requeue with incremented counter
            _requeue(chunk, retry_count)
    except Exception as e:
        # Network error (timeout, reset, etc.) — requeue, may create duplicate
        sys.stderr.write(f'[engram-flush] Network error for "{title}": {e}\n')
        _requeue(chunk, retry_count)

# --- Phase 3: re-append failures under lock ---
if failed_chunks:
    with open(lock_path, "w") as lock_fd:
        fcntl.flock(lock_fd, fcntl.LOCK_EX)

        try:
            with open(fallback_path) as f:
                current = f.read()
        except FileNotFoundError:
            current = header + template

        new_content = current.rstrip() + '\n\n' + '\n\n'.join(failed_chunks) + '\n'
        write_atomic(fallback_path, new_content)
        # Lock released when with block exits

print(flushed)
PYEOF
)

if [[ -n "$FLUSHED" && "$FLUSHED" -gt 0 ]]; then
    printf '{"sessionMessage":"Engram: flushed %s pending entries from fallback.md"}\n' "$FLUSHED"
    # Reset state counters on successful flush (#404)
    _now=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    update_state "last_flush_at" "\"${_now}\"" 2>/dev/null || true
    update_state "sessions_since_last_flush" "0" 2>/dev/null || true
    update_state "fallback_entry_count" "0" 2>/dev/null || true
fi
# nothing-to-flush case: silent — no output needed
