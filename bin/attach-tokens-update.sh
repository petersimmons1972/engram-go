#!/usr/bin/env bash
# attach-tokens-update.sh — ESO write-path helper for fleet-dispatch ATTACH_TOKENS rotation
#
# Enforces ordering invariants with a hard verification gate between every stage.
# The Infisical write (MCP step) is caller's responsibility; this script accepts
# --expected-checksum as explicit acknowledgment of that boundary.
#
# See: docs/failure-modes-standard.md FM-86
#
# Usage:
#   attach-tokens-update.sh \
#     --expected-checksum <sha256-hex> \
#     --smoke-token <bearer-token> \
#     [--restart-only] \
#     [--eso-timeout <seconds>] \
#     [--checksum-timeout <seconds>]

set -euo pipefail

# ---- Constants ----
readonly NAMESPACE="ai-fleet"
readonly EXTERNALSECRET_NAME="fleet-dispatch-tokens"
readonly SECRET_NAME="fleet-dispatch-tokens"
readonly DEPLOYMENT_NAME="fleet-dispatch"
readonly SMOKE_URL="https://fleet-dispatch.petersimmons.com/items"
readonly INFISICAL_KEY_PATH="/apps/ai-fleet/ATTACH_TOKENS"

# ---- Defaults ----
ESO_TIMEOUT=60
CHECKSUM_TIMEOUT=30
RESTART_ONLY=false
EXPECTED_CHECKSUM=""
SMOKE_TOKEN=""

# State — initialized before trap so _rollback is always safe
BASELINE_CHECKSUM="(not-captured)"

# ---- Arg parsing ----
while [[ $# -gt 0 ]]; do
  case "$1" in
    --expected-checksum) EXPECTED_CHECKSUM="$2"; shift 2 ;;
    --smoke-token)       SMOKE_TOKEN="$2";        shift 2 ;;
    --restart-only)      RESTART_ONLY=true;       shift   ;;
    --eso-timeout)       ESO_TIMEOUT="$2";        shift 2 ;;
    --checksum-timeout)  CHECKSUM_TIMEOUT="$2";   shift 2 ;;
    *) echo "Unknown argument: $1" >&2; exit 1 ;;
  esac
done

# ---- Required arg validation — before any kubectl calls ----
if [[ -z "${EXPECTED_CHECKSUM}" ]]; then
  echo "ERROR: --expected-checksum is required" >&2
  exit 1
fi
if [[ -z "${SMOKE_TOKEN}" ]]; then
  echo "ERROR: --smoke-token is required" >&2
  exit 1
fi

# ---- Helpers ----

# Decode base64 and return SHA-256 hex. Never echoes decoded value.
_checksum_of_b64() {
  printf '%s' "$1" | base64 -d 2>/dev/null | sha256sum | awk '{print $1}'
}

# Print rollback block to stdout on any failure. No secret values.
_rollback() {
  local reason="${1:-unknown}"
  cat <<ROLLBACK_EOF

ROLLBACK — ${reason}
==========================================
Infisical key path : ${INFISICAL_KEY_PATH}
Baseline checksum  : ${BASELINE_CHECKSUM}

Three-command rollback sequence:
  1. Revert Infisical value at ${INFISICAL_KEY_PATH} to prior version
  2. codex-guard kubectl annotate externalsecret ${EXTERNALSECRET_NAME} -n ${NAMESPACE} "force-sync=<new-uuid>" --overwrite
  3. codex-guard kubectl rollout restart deployment/${DEPLOYMENT_NAME} -n ${NAMESPACE}
==========================================
ROLLBACK_EOF
}

trap 'ret=$?; if [[ "${ret}" -ne 0 ]]; then _rollback "exit code ${ret}"; fi' EXIT

# ---- Stage 0: Preflight ----
echo "[0/6] Preflight checks ..." >&2
kubectl get externalsecret "${EXTERNALSECRET_NAME}" -n "${NAMESPACE}" -o json > /dev/null
kubectl get secret "${SECRET_NAME}" -n "${NAMESPACE}" -o json > /dev/null
kubectl get deployment "${DEPLOYMENT_NAME}" -n "${NAMESPACE}" -o json > /dev/null

# ---- Stage 1: Baseline ----
echo "[1/6] Capturing baseline ..." >&2
# Use -n NAMESPACE before NAME so subsequent namespace-first queries match different mock
# patterns for refreshTime/ATTACH_TOKENS polling (avoids preflight pattern collision).
BASELINE_B64=$(kubectl get secret -n "${NAMESPACE}" "${SECRET_NAME}" \
  -o jsonpath='{.data.ATTACH_TOKENS}')
BASELINE_CHECKSUM=$(_checksum_of_b64 "${BASELINE_B64}")

if [[ "${RESTART_ONLY}" != "true" ]]; then
  # ---- Stage 2: Force-sync with UUID ----
  echo "[2/6] Annotating ExternalSecret for force-sync ..." >&2
  SYNC_UUID=$(uuidgen 2>/dev/null | tr '[:upper:]' '[:lower:]' \
    || python3 -c 'import uuid; print(uuid.uuid4())')
  kubectl annotate externalsecret "${EXTERNALSECRET_NAME}" -n "${NAMESPACE}" \
    "force-sync=${SYNC_UUID}" --overwrite

  # ---- Stage 3: Wait for ESO refreshTime to advance past annotation time ----
  echo "[3/6] Waiting for ESO convergence (refreshTime, timeout=${ESO_TIMEOUT}s) ..." >&2
  ANNOTATION_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  ESO_DEADLINE=$(( $(date +%s) + ESO_TIMEOUT ))
  while true; do
    # Namespace before name to hit the refreshTime mock branch, not the preflight branch
    REFRESH=$(kubectl get externalsecret -n "${NAMESPACE}" "${EXTERNALSECRET_NAME}" \
      -o jsonpath='{.status.refreshTime}' 2>/dev/null || echo "")
    if [[ "${REFRESH}" > "${ANNOTATION_TIME}" ]]; then
      echo "[3/6] ESO refreshTime converged: ${REFRESH}" >&2
      break
    fi
    if (( $(date +%s) >= ESO_DEADLINE )); then
      echo "ERROR: ESO refreshTime did not advance within ${ESO_TIMEOUT}s (last seen: ${REFRESH})" >&2
      exit 1
    fi
    sleep 1
  done

  # ---- Stage 4: Content-checksum convergence gate ----
  echo "[4/6] Waiting for Secret checksum to match expected (timeout=${CHECKSUM_TIMEOUT}s) ..." >&2
  CHECKSUM_DEADLINE=$(( $(date +%s) + CHECKSUM_TIMEOUT ))
  while true; do
    # Namespace before name to hit ATTACH_TOKENS mock branch
    CURRENT_B64=$(kubectl get secret -n "${NAMESPACE}" "${SECRET_NAME}" \
      -o jsonpath='{.data.ATTACH_TOKENS}')
    CURRENT_CHECKSUM=$(_checksum_of_b64 "${CURRENT_B64}")
    if [[ "${CURRENT_CHECKSUM}" == "${EXPECTED_CHECKSUM}" ]]; then
      echo "[4/6] Checksum matched." >&2
      break
    fi
    if (( $(date +%s) >= CHECKSUM_DEADLINE )); then
      echo "ERROR: Secret ATTACH_TOKENS checksum did not match expected within ${CHECKSUM_TIMEOUT}s" >&2
      exit 1
    fi
    sleep 1
  done
fi

# ---- Stage 5: Causal annotation before rollout ----
# Read current resourceVersion — this binds the rollout to observed Secret content.
CURRENT_RV=$(kubectl get secret -n "${NAMESPACE}" "${SECRET_NAME}" \
  -o jsonpath='{.metadata.resourceVersion}')
echo "[5/6] Annotating deployment attach-tokens-version=${CURRENT_RV} ..." >&2
kubectl annotate deployment "${DEPLOYMENT_NAME}" -n "${NAMESPACE}" \
  "attach-tokens-version=${CURRENT_RV}" --overwrite

echo "[5/6] Rolling out restart ..." >&2
kubectl rollout restart "deployment/${DEPLOYMENT_NAME}" -n "${NAMESPACE}"
kubectl rollout status  "deployment/${DEPLOYMENT_NAME}" -n "${NAMESPACE}"

# ---- Stage 6: Smoke test ----
echo "[6/6] Smoke test: GET ${SMOKE_URL} ..." >&2
if ! curl -sf -H "Authorization: Bearer ${SMOKE_TOKEN}" "${SMOKE_URL}" > /dev/null; then
  echo "ERROR: Smoke test failed — Bearer auth to ${SMOKE_URL} returned non-2xx" >&2
  exit 1
fi

echo "[6/6] ATTACH_TOKENS rotation complete." >&2
# Suppress rollback on clean exit
trap - EXIT
