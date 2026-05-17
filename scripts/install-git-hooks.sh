#!/usr/bin/env bash
# install-git-hooks.sh — wire scripts/check-secrets.sh as a git pre-commit hook.
# Idempotent. Safe to re-run.
set -euo pipefail

REPO_DIR="$(git rev-parse --show-toplevel)"
COMMON_DIR="$(git rev-parse --git-common-dir)"
HOOKS_DIR="${COMMON_DIR}/hooks"
mkdir -p "$HOOKS_DIR"

HOOK="${HOOKS_DIR}/pre-commit"
GUARD="${REPO_DIR}/scripts/check-secrets.sh"

if [ ! -x "$GUARD" ]; then
    echo "ERROR: ${GUARD} not found or not executable" >&2
    exit 1
fi

# If a pre-commit hook already exists and is not our managed one, refuse to clobber it.
if [ -e "$HOOK" ] && ! grep -q "MANAGED-BY: engram-go check-secrets" "$HOOK" 2>/dev/null; then
    echo "WARNING: ${HOOK} already exists and is not managed by this installer." >&2
    echo "         Move it aside or merge manually." >&2
    exit 1
fi

cat > "$HOOK" <<HOOK_BODY
#!/usr/bin/env bash
# MANAGED-BY: engram-go check-secrets installer
# Installed by: scripts/install-git-hooks.sh
# Re-run that script to refresh.
exec "${GUARD}" "\$@"
HOOK_BODY
chmod +x "$HOOK"

echo "✓ Installed pre-commit hook at ${HOOK}"
echo "  → invokes ${GUARD}"
