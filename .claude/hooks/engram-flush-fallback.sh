#!/usr/bin/env bash
# SessionStart hook: flush pending fallback.md entries to Engram via /quick-store.
# Runs after engram-token-refresh.sh so the server is confirmed up and token is fresh.
set -euo pipefail

FALLBACK="$HOME/.claude/projects/-home-psimmons/memory/fallback.md"
PORT=8788
BASE="http://127.0.0.1:${PORT}"

[[ -f "$FALLBACK" ]] || exit 0

# Quick exit if no pending entries
if ! grep -q "^\*\*Project:\*\*" "$FALLBACK" 2>/dev/null; then
    exit 0
fi

# Server must be up (engram-token-refresh.sh already ensured this, but double-check)
if ! curl -sf --max-time 2 "${BASE}/health" > /dev/null 2>&1; then
    echo "⚠️  engram-flush-fallback: Engram not reachable — leaving fallback.md intact"
    exit 0
fi

TOKEN=$(curl -sf --max-time 3 "${BASE}/setup-token" 2>/dev/null \
    | python3 -c "import json,sys; print(json.load(sys.stdin).get('token',''))" 2>/dev/null || true)
[[ -z "$TOKEN" ]] && { echo "⚠️  engram-flush-fallback: no token — skipping flush"; exit 0; }

# Parse, snapshot, flush entries — lock held minimally (#398)
# Phase 1: snapshot under lock. Phase 2: HTTP flush (no lock). Phase 3: re-append failures under lock.
FLUSHED=$(python3 - "$FALLBACK" "$BASE" "$TOKEN" <<'PYEOF'
import json, re, sys, urllib.request, urllib.error, os, tempfile, fcntl
from datetime import datetime, timezone, timedelta

fallback_path, base_url, token = sys.argv[1], sys.argv[2], sys.argv[3]
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
                failed_chunks.append(chunk)
    except Exception as e:
        sys.stderr.write(f'flush error for "{title}": {e}\n')
        failed_chunks.append(chunk)

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
    echo "✅ engram-flush-fallback: flushed ${FLUSHED} pending entries to Engram"
else
    echo "ℹ️  engram-flush-fallback: nothing to flush"
fi
