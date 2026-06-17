#!/usr/bin/env bash
# Sync armies XP from local profiles (~/.armies/profiles/) into ~/AGENTS.md
# Local profiles are the source of truth. GitHub (camp-david) is a backup-only target.
# Runs on SessionStart — no network calls.

set -euo pipefail

PROFILES_DIR="$HOME/.armies/profiles"
AGENTS_FILE="$HOME/AGENTS.md"

[[ -f "$AGENTS_FILE" ]] || exit 0
[[ -d "$PROFILES_DIR" ]] || exit 0

UPDATED=$(python3 - "$AGENTS_FILE" "$PROFILES_DIR" <<'PYEOF'
import sys, re, tempfile, os
from pathlib import Path

agents_path = Path(sys.argv[1])
profiles_dir = Path(sys.argv[2])

# Short display name (as used in AGENTS.md table) → profile filename stem
NAME_MAP = {
    "Rickover":   "rickover-coordinator",
    "Eisenhower": "eisenhower",
    "Spruance":   "spruance",
    "Montgomery": "montgomery",
    "Bradley":    "omar-bradley",
    "Nimitz":     "nimitz",
    "Layton":     "edwin-layton",
    "Smith":      "bedell-smith",
    "Zhukov":     "zhukov",
    "Halsey":     "halsey",
    "King":       "king",
    "Ramsay":     "gordon-ramsay",
    "Hopper":     "grace-hopper",
}

# Load XP from local profile frontmatter
xp_by_name = {}
for display, stem in NAME_MAP.items():
    profile = profiles_dir / f"{stem}.md"
    if not profile.exists():
        continue
    for line in profile.read_text().splitlines():
        if line.startswith("xp:"):
            try:
                xp_by_name[display] = int(line.split(":", 1)[1].strip())
            except ValueError:
                pass
            break

if not xp_by_name:
    print(0)
    sys.exit(0)

# Single-pass update of AGENTS.md — read once, write once
text = agents_path.read_text()
updated = 0

for display, xp in xp_by_name.items():
    xp_fmt = f"{xp:,}"
    # Match: | DisplayName<whitespace><anything>|<anything>| <old_xp> |<rest>
    # Column 4 (1-indexed from pipes) is the XP column in the AGENTS.md table
    pattern = rf"(^\| {re.escape(display)}\s[^|]*\|[^|]*\|)\s*[\d,]+\s*(\|)"
    replacement = rf"\g<1> {xp_fmt} \g<2>"
    new_text, n = re.subn(pattern, replacement, text, flags=re.MULTILINE)
    if n and new_text != text:
        text = new_text
        updated += 1

# Atomic write — prevents CC's 30s SessionStart timeout from leaving a
# half-truncated AGENTS.md if it sends SIGKILL mid-write. (FM-93)
dir_ = str(agents_path.parent)
fd, tmp = tempfile.mkstemp(dir=dir_, prefix='.agents_tmp')
try:
    with os.fdopen(fd, 'w') as f:
        f.write(text)
    os.replace(tmp, str(agents_path))
except Exception:
    try:
        os.unlink(tmp)
    except Exception:
        pass
    raise

print(updated)
PYEOF
)

if [[ "$UPDATED" -gt 0 ]]; then
    echo "{\"systemMessage\": \"📊 Synced ${UPDATED} XP values from local profiles\"}"
else
    echo "{\"suppressOutput\": true}"
fi
