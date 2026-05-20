#!/usr/bin/env bash
# Installs instinct hooks into ~/.claude/hooks/ and registers in ~/.claude/settings.json
# Idempotent: safe to run multiple times.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
HOOKS_SRC="$REPO_DIR/hooks"
HOOKS_DEST="$HOME/.claude/hooks"
SETTINGS="$HOME/.claude/settings.json"

cat <<'DISCLOSURE'

DATA COLLECTION NOTICE
----------------------
The instinct hooks observe Claude Code tool activity. On every Edit, Write, Bash,
Task, Agent, or MCP write call, the hook captures:

  - Tool name and a hash of the tool input (no raw input is stored)
  - The first 200 characters of the tool response, with secrets redacted
  - Session ID and a hashed project identifier

This data is written to ~/.local/state/instinct/buffer.jsonl (mode 0600) and
periodically sent to Anthropic's API by the instinct binary.

To opt out at any time without uninstalling, set INSTINCT_ENABLED=0 in your
environment (e.g., export INSTINCT_ENABLED=0 in ~/.bashrc).

Press ENTER to accept and continue, or Ctrl-C to abort.
DISCLOSURE
# #681: explicit TTY check BEFORE the prompt. The previous `read -r 2>/dev/null`
# pattern was porous — `echo "" | bash hooks/install.sh` would silently treat
# an empty line as consent. A stdin-redirected installer cannot give informed
# consent and must refuse.
if [ ! -t 0 ]; then
    echo "ERROR: installer requires an interactive terminal for consent." >&2
    echo "       stdin is not a TTY (was something piped in?). Re-run from a TTY," >&2
    echo "       or set INSTINCT_ENABLED=0 to skip data collection entirely." >&2
    exit 1
fi
if ! read -r; then
    # Defensive: if we have a TTY but read still fails (EOF), refuse.
    echo "ERROR: read failed — refusing to proceed without explicit consent." >&2
    exit 1
fi

echo "Installing instinct hooks..."

# 1. Copy hook scripts
mkdir -p "$HOOKS_DEST" "$HOOKS_DEST/lib"
cp "$HOOKS_SRC/pre-tool-use.sh"  "$HOOKS_DEST/instinct-pre-tool-use.sh"
cp "$HOOKS_SRC/post-tool-use.sh" "$HOOKS_DEST/instinct-post-tool-use.sh"
chmod +x "$HOOKS_DEST/instinct-pre-tool-use.sh" "$HOOKS_DEST/instinct-post-tool-use.sh"
echo "  Copied hooks to $HOOKS_DEST"

# 1b. Install Phase 1 timing-v2 library (#396) and instrumented engram hooks.
# These are measurement-only; they source timing-v2.sh and emit one TSV row per
# invocation to ~/.claude/hook-timings-v2.tsv. Safe no-op if Engram is absent.
if [ -f "$HOOKS_SRC/lib/timing-v2.sh" ]; then
    cp "$HOOKS_SRC/lib/timing-v2.sh" "$HOOKS_DEST/lib/timing-v2.sh"
    echo "  Installed timing-v2.sh (Phase 1 measurement, #396)"
fi
if [ -d "$HOOKS_SRC/engram" ]; then
    for f in "$HOOKS_SRC/engram"/*.sh; do
        [ -f "$f" ] || continue
        base=$(basename "$f")
        cp "$f" "$HOOKS_DEST/$base"
        chmod +x "$HOOKS_DEST/$base"
    done
    echo "  Installed engram hooks with timing-v2 instrumentation"
fi

# 2. Patch settings.json (idempotent via Python)
python3 - <<PYEOF
import json, os, shutil, datetime
from pathlib import Path

settings_path = Path(os.environ['HOME']) / '.claude' / 'settings.json'
backup = settings_path.with_suffix(f".{datetime.datetime.now().strftime('%Y%m%dT%H%M%S')}.bak")

data = json.loads(settings_path.read_text()) if settings_path.exists() else {}
data.setdefault('hooks', {})

INSTINCT_PRE  = '~/.claude/hooks/instinct-pre-tool-use.sh'
INSTINCT_POST = '~/.claude/hooks/instinct-post-tool-use.sh'

def hook_entry(cmd):
    return {'type': 'command', 'command': cmd}

def has_hook(hook_list, cmd):
    return any(h.get('command') == cmd for entry in hook_list for h in entry.get('hooks', []))

# Register PostToolUse (no matcher = fires for all tools)
post_hooks = data['hooks'].setdefault('PostToolUse', [])
if not has_hook(post_hooks, INSTINCT_POST):
    post_hooks.append({
        'hooks': [hook_entry(INSTINCT_POST)]
    })
    print(f"  Registered PostToolUse hook")
else:
    print(f"  PostToolUse hook already registered (skipped)")

# Register PreToolUse (no matcher = fires for all tools)
pre_hooks = data['hooks'].setdefault('PreToolUse', [])
if not has_hook(pre_hooks, INSTINCT_PRE):
    pre_hooks.append({
        'hooks': [hook_entry(INSTINCT_PRE)]
    })
    print(f"  Registered PreToolUse hook")
else:
    print(f"  PreToolUse hook already registered (skipped)")

# Atomic write via temp+rename
tmp = settings_path.with_suffix('.tmp')
tmp.write_text(json.dumps(data, indent=2))
shutil.copy2(settings_path, backup) if settings_path.exists() else None
tmp.rename(settings_path)
print(f"  Backed up settings to {backup.name}")
print(f"  Wrote {settings_path}")
PYEOF

# 3. Build instinct binary
echo "Building instinct binary..."
go build -o "$HOME/bin/instinct" "$REPO_DIR/cmd/instinct"
echo "  Binary installed at $HOME/bin/instinct"

echo "Done."
echo "  instinct binary: $HOME/bin/instinct"
echo "  To disable without uninstalling: export INSTINCT_ENABLED=0"
