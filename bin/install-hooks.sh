#!/usr/bin/env bash
set -euo pipefail
REPO_ROOT="$(git rev-parse --show-toplevel)"
HOOK="$REPO_ROOT/.git/hooks/pre-commit"
cat > "$HOOK" <<'HOOKEOF'
#!/usr/bin/env bash
exec "$(git rev-parse --show-toplevel)/bin/checkin-lint.sh"
HOOKEOF
chmod +x "$HOOK"
echo "✓ pre-commit hook installed → $HOOK"
