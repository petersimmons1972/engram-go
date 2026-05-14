#!/usr/bin/env bash
# apply.sh — Deploy precision-host systemd units to precision.petersimmons.com
#
# Usage:
#   ./infra/precision-host/apply.sh [--no-confirm] [--dry-run]
#
# Environment:
#   PRECISION_HOST — target hostname (default: precision.petersimmons.com)
#
# What it does:
#   1. Diffs local unit files against remote
#   2. Asks for confirmation if any diff is non-empty (skipped with --no-confirm or --dry-run)
#   3. Copies unit files via scp
#   4. Runs systemctl daemon-reload and restarts services
#   5. Verifies services are active
#   6. Exits non-zero on any failure
#
# GPU mapping on precision.petersimmons.com:
#   card0 / renderD128  — AMD Radeon PRO W6800  (gfx1030, 32 GB)  ROCR_VISIBLE_DEVICES=0
#   card1 / renderD129  — AMD Radeon VII / MI50 (gfx906,  16 GB)  ROCR_VISIBLE_DEVICES=0 (in container)
#
# Rollback: if a service fails to start, apply.sh restores the previous remote
# file from /tmp/engram-apply-backup-<timestamp>/ on the remote host.

set -euo pipefail

PRECISION_HOST="${PRECISION_HOST:-precision.petersimmons.com}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DRY_RUN=false
NO_CONFIRM=false
BACKUP_STAMP="$(date +%Y%m%d-%H%M%S)"
REMOTE_BACKUP_DIR="/tmp/engram-apply-backup-${BACKUP_STAMP}"

# Parse flags
for arg in "$@"; do
  case "$arg" in
    --dry-run)   DRY_RUN=true ;;
    --no-confirm) NO_CONFIRM=true ;;
    *) echo "Unknown argument: $arg" >&2; exit 1 ;;
  esac
done

# Colors (degrade gracefully if no tput)
RED=$(tput setaf 1 2>/dev/null || true)
GRN=$(tput setaf 2 2>/dev/null || true)
YLW=$(tput setaf 3 2>/dev/null || true)
BLD=$(tput bold 2>/dev/null || true)
RST=$(tput sgr0 2>/dev/null || true)

info()  { echo "${BLD}[apply]${RST} $*"; }
ok()    { echo "${GRN}[OK]${RST} $*"; }
warn()  { echo "${YLW}[WARN]${RST} $*"; }
fail()  { echo "${RED}[FAIL]${RST} $*" >&2; }

# Unit file map: local-path → remote-path
declare -A UNIT_MAP
# ollama-w6800.service is a drop-in for ollama.service (legacy filename retained)
UNIT_MAP["${SCRIPT_DIR}/ollama-w6800.service"]="/etc/systemd/system/ollama.service.d/mi50.conf"
UNIT_MAP["${SCRIPT_DIR}/ollama-mi50.service"]="/etc/systemd/system/ollama-mi50.service"

# Services to restart after applying (in order)
SERVICES=("ollama" "ollama-mi50")

# ── 1. Diff remote vs local ───────────────────────────────────────────────────
info "Diffing remote ${PRECISION_HOST} against local files..."

ANY_DIFF=false
for LOCAL in "${!UNIT_MAP[@]}"; do
  REMOTE="${UNIT_MAP[$LOCAL]}"
  echo ""
  info "  Local:  ${LOCAL}"
  info "  Remote: ${PRECISION_HOST}:${REMOTE}"

  # Fetch remote file; if missing, treat as empty
  REMOTE_CONTENT=$(ssh "${PRECISION_HOST}" "sudo cat '${REMOTE}' 2>/dev/null" || true)
  LOCAL_CONTENT=$(cat "${LOCAL}")

  if [ "${REMOTE_CONTENT}" = "${LOCAL_CONTENT}" ]; then
    ok "  No diff — remote matches local"
  else
    ANY_DIFF=true
    echo "${YLW}  --- remote${RST}"
    diff <(echo "${REMOTE_CONTENT}") <(echo "${LOCAL_CONTENT}") || true
  fi
done

echo ""

# ── 2. Dry-run / confirmation ─────────────────────────────────────────────────
if $DRY_RUN; then
  info "Dry run — no changes applied."
  exit 0
fi

if ! $ANY_DIFF; then
  info "Remote already matches local for all unit files. No apply needed."
  exit 0
fi

if ! $NO_CONFIRM; then
  read -r -p "${BLD}Apply changes to ${PRECISION_HOST}? [y/N]${RST} " CONFIRM
  case "${CONFIRM}" in
    y|Y|yes|YES) ;;
    *) info "Aborted."; exit 0 ;;
  esac
fi

# ── 3. Backup remote files before overwriting ─────────────────────────────────
info "Creating remote backup at ${REMOTE_BACKUP_DIR}..."
ssh "${PRECISION_HOST}" "mkdir -p '${REMOTE_BACKUP_DIR}'"
for LOCAL in "${!UNIT_MAP[@]}"; do
  REMOTE="${UNIT_MAP[$LOCAL]}"
  BACKUP_NAME="${REMOTE//\//_}"
  ssh "${PRECISION_HOST}" "sudo cp '${REMOTE}' '${REMOTE_BACKUP_DIR}/${BACKUP_NAME}' 2>/dev/null || true"
done
ok "Backup created at ${PRECISION_HOST}:${REMOTE_BACKUP_DIR}"

# Rollback function — restores backed-up files if a service fails to start
rollback() {
  fail "Rolling back changes on ${PRECISION_HOST}..."
  for LOCAL in "${!UNIT_MAP[@]}"; do
    REMOTE="${UNIT_MAP[$LOCAL]}"
    BACKUP_NAME="${REMOTE//\//_}"
    ssh "${PRECISION_HOST}" "sudo cp '${REMOTE_BACKUP_DIR}/${BACKUP_NAME}' '${REMOTE}' 2>/dev/null || true"
  done
  ssh "${PRECISION_HOST}" "sudo systemctl daemon-reload" || true
  fail "Rollback complete. Previous units restored from ${REMOTE_BACKUP_DIR}"
}

# ── 4. Copy unit files ────────────────────────────────────────────────────────
info "Deploying unit files to ${PRECISION_HOST}..."
for LOCAL in "${!UNIT_MAP[@]}"; do
  REMOTE="${UNIT_MAP[$LOCAL]}"
  REMOTE_DIR="$(dirname "${REMOTE}")"
  info "  ${LOCAL##*/} → ${REMOTE}"
  # Ensure drop-in directory exists
  ssh "${PRECISION_HOST}" "sudo mkdir -p '${REMOTE_DIR}'"
  scp "${LOCAL}" "${PRECISION_HOST}:/tmp/engram-apply-unit-tmp"
  ssh "${PRECISION_HOST}" "sudo mv /tmp/engram-apply-unit-tmp '${REMOTE}' && sudo chmod 644 '${REMOTE}'"
done
ok "Unit files deployed."

# ── 5. Reload and restart ─────────────────────────────────────────────────────
info "Running daemon-reload..."
ssh "${PRECISION_HOST}" "sudo systemctl daemon-reload"
ok "daemon-reload complete."

for SVC in "${SERVICES[@]}"; do
  info "Restarting ${SVC}..."
  # Attempt restart; capture exit code
  if ! ssh "${PRECISION_HOST}" "sudo systemctl restart '${SVC}'" 2>&1; then
    warn "systemctl restart ${SVC} returned non-zero (service may be disabled — continuing)"
  fi
done

# ── 6. Verify services ────────────────────────────────────────────────────────
echo ""
info "Verifying service states..."
FAILURES=0
for SVC in "${SERVICES[@]}"; do
  STATE=$(ssh "${PRECISION_HOST}" "systemctl is-active '${SVC}'" 2>/dev/null || echo "unknown")
  if [ "${STATE}" = "active" ]; then
    ok "  ${SVC}: ${STATE}"
  elif [ "${STATE}" = "inactive" ]; then
    warn "  ${SVC}: ${STATE} (service exists but is not enabled — may be intentional)"
  else
    fail "  ${SVC}: ${STATE}"
    FAILURES=$((FAILURES + 1))
  fi
done

if [ "${FAILURES}" -gt 0 ]; then
  fail "${FAILURES} service(s) failed to start. Rolling back..."
  rollback
  exit 1
fi

# ── 7. Port check ─────────────────────────────────────────────────────────────
echo ""
info "Checking listening ports..."
PORTS=$(ssh "${PRECISION_HOST}" "ss -tlnp 2>/dev/null | grep -E '1143[4-9]' || true")
echo "${PORTS}"

ok "Apply complete. Backup retained at ${PRECISION_HOST}:${REMOTE_BACKUP_DIR}"
