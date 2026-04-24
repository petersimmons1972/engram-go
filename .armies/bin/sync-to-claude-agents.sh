#!/usr/bin/env bash
# sync-to-claude-agents.sh — Copy active armies profiles into ~/.claude/agents/
# Only profiles with `status: active` are synced. bench/retired stay in armies only.

set -euo pipefail

ARMIES_PROFILES="${HOME}/.armies/profiles"
CLAUDE_AGENTS="${HOME}/.claude/agents"

if [[ ! -d "$ARMIES_PROFILES" ]]; then
  echo "ERROR: armies profiles dir not found: $ARMIES_PROFILES" >&2
  exit 1
fi

mkdir -p "$CLAUDE_AGENTS"

COPIED=0
SKIPPED=0
PRUNED=0

for src in "$ARMIES_PROFILES"/*.md; do
  name=$(basename "$src")

  # Preserve founder.md — only exists in .claude/agents, not in armies
  if [[ "$name" == "founder.md" ]]; then
    SKIPPED=$((SKIPPED + 1))
    continue
  fi

  # Only sync profiles with status: active
  if ! grep -q "^status: active" "$src"; then
    SKIPPED=$((SKIPPED + 1))
    # Remove stale copies that are no longer active
    if [[ -f "$CLAUDE_AGENTS/$name" ]]; then
      rm "$CLAUDE_AGENTS/$name"
      PRUNED=$((PRUNED + 1))
    fi
    continue
  fi

  cp "$src" "$CLAUDE_AGENTS/$name"
  COPIED=$((COPIED + 1))
done

echo "Synced $COPIED active profiles from armies → ~/.claude/agents/ (skipped $SKIPPED, pruned $PRUNED stale)"
