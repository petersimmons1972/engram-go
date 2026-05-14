#!/usr/bin/env bash
# infra/status.sh — engram stack health check
#
# Usage:
#   make status              # full check including precision SSH probes
#   make status NO_REMOTE=1  # skip precision SSH probes
#   bash infra/status.sh [--no-remote]
#
# Probes every layer:
#   postgres      — container up + connection count
#   engram-go     — /health HTTP reachable + container state
#   embed router  — olla container healthy + model count from /v1/models
#   reembed       — per-GPU worker containers + NULL chunk backlog (chunks table)
#   precision     — ollama service states + port listeners (SSH, 8s timeout)
#   config drift  — ENGRAM_EMBED_MODEL in .env matches olla model list

set -uo pipefail

# ── Config ────────────────────────────────────────────────────────────────────
PRECISION_HOST="${PRECISION_HOST:-precision.petersimmons.com}"
ENGRAM_PORT="${ENGRAM_PORT:-8788}"
OLLA_PORT="${OLLA_PORT:-40114}"
PROBE_TIMEOUT=5      # network probe timeout (seconds)
SSH_TIMEOUT=8        # SSH probe timeout (seconds)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Locate .env: prefer cwd (where make runs), fall back to repo root
if [ -f ".env" ]; then
  ENV_FILE=".env"
elif [ -f "${SCRIPT_DIR}/../.env" ]; then
  ENV_FILE="${SCRIPT_DIR}/../.env"
else
  ENV_FILE=".env"  # will silently fail to load — probes still run
fi

# Parse flags
NO_REMOTE=false
for arg in "$@"; do
  case "$arg" in
    --no-remote) NO_REMOTE=true ;;
  esac
done

# ── Colors ────────────────────────────────────────────────────────────────────
if command -v tput &>/dev/null && [ -t 1 ]; then
  GRN=$(tput setaf 2)
  YLW=$(tput setaf 3)
  RED=$(tput setaf 1)
  BLD=$(tput bold)
  RST=$(tput sgr0)
else
  GRN=""; YLW=""; RED=""; BLD=""; RST=""
fi

OK="${GRN}OK${RST}"
WARN="${YLW}WARN${RST}"
FAIL="${RED}FAIL${RST}"

# ── Table helpers ─────────────────────────────────────────────────────────────
COL1=26  # layer name width
COL2=6   # status width

print_header() {
  echo ""
  printf "${BLD}%-${COL1}s  %-${COL2}s  %s${RST}\n" "LAYER" "STATUS" "DETAIL"
  printf '%0.s=' {1..72}
  echo ""
}

print_row() {
  local layer="$1" status="$2" detail="$3"
  printf "%-${COL1}s  %-${COL2}s  %s\n" "$layer" "$status" "$detail"
}

# ── Load .env ─────────────────────────────────────────────────────────────────
ENV_EMBED_MODEL=""
ENV_ROUTER_URL=""
ENV_API_KEY=""
if [ -f "$ENV_FILE" ]; then
  ENV_EMBED_MODEL=$(grep -E '^ENGRAM_EMBED_MODEL=' "$ENV_FILE" 2>/dev/null | cut -d= -f2- | tr -d '"' | head -1 || true)
  ENV_ROUTER_URL=$(grep -E '^(ENGRAM_EMBED_URL|LITELLM_URL)=' "$ENV_FILE" 2>/dev/null | head -1 | cut -d= -f2- | tr -d '"' || true)
  ENV_API_KEY=$(grep -E '^ENGRAM_API_KEY=' "$ENV_FILE" 2>/dev/null | cut -d= -f2- | tr -d '"' | head -1 || true)
fi

# ── Probe: postgres ───────────────────────────────────────────────────────────
probe_postgres() {
  local ctr="engram-postgres"
  if ! docker inspect "$ctr" &>/dev/null 2>&1; then
    print_row "postgres" "${FAIL}" "container not found"
    return
  fi
  local state
  state=$(docker inspect "$ctr" --format '{{.State.Status}}' 2>/dev/null)
  if [ "$state" != "running" ]; then
    print_row "postgres" "${FAIL}" "container state: $state"
    return
  fi
  local uptime conns
  uptime=$(docker inspect "$ctr" --format '{{.State.StartedAt}}' 2>/dev/null | \
    python3 -c "
import sys, datetime, re
s = sys.stdin.read().strip()
s = re.sub(r'\.\d+Z$', '+00:00', s)
try:
    started = datetime.datetime.fromisoformat(s)
    now = datetime.datetime.now(datetime.timezone.utc)
    delta = now - started
    d = delta.days; h = delta.seconds // 3600; m = (delta.seconds % 3600) // 60
    print(f'{d}d {h}h {m}m' if d > 0 else f'{h}h {m}m')
except:
    print('?')
" 2>/dev/null || echo "?")
  conns=$(docker exec "$ctr" sh -c "psql -U engram -d engram -At -c 'SELECT count(*) FROM pg_stat_activity WHERE datname='\''engram'\'';'" 2>/dev/null | tr -d '[:space:]' || echo "?")
  print_row "postgres" "${OK}" "up ${uptime}, ${conns} connections"
}

# ── Probe: engram-go ──────────────────────────────────────────────────────────
probe_engram() {
  local ctr="engram-go-app"
  if ! docker inspect "$ctr" &>/dev/null 2>&1; then
    print_row "engram-go" "${FAIL}" "container not found"
    return
  fi
  local state
  state=$(docker inspect "$ctr" --format '{{.State.Status}}' 2>/dev/null)
  if [ "$state" != "running" ]; then
    print_row "engram-go" "${FAIL}" "container state: $state"
    return
  fi
  # Health check via HTTP
  local http_code
  if [ -n "$ENV_API_KEY" ]; then
    http_code=$(curl -s -o /dev/null -w "%{http_code}" \
      -H "Authorization: Bearer ${ENV_API_KEY}" \
      --max-time "$PROBE_TIMEOUT" \
      "http://localhost:${ENGRAM_PORT}/health" 2>/dev/null || echo "000")
  else
    http_code=$(curl -s -o /dev/null -w "%{http_code}" \
      --max-time "$PROBE_TIMEOUT" \
      "http://localhost:${ENGRAM_PORT}/health" 2>/dev/null || echo "000")
  fi
  # Uptime from container
  local uptime
  uptime=$(docker inspect "$ctr" --format '{{.State.StartedAt}}' 2>/dev/null | \
    python3 -c "
import sys, datetime, re
s = sys.stdin.read().strip()
s = re.sub(r'\.\d+Z$', '+00:00', s)
try:
    started = datetime.datetime.fromisoformat(s)
    now = datetime.datetime.now(datetime.timezone.utc)
    delta = now - started
    d = delta.days; h = delta.seconds // 3600; m = (delta.seconds % 3600) // 60
    print(f'{d}d {h}h {m}m' if d > 0 else f'{h}h {m}m')
except:
    print('?')
" 2>/dev/null || echo "?")
  if [ "$http_code" = "200" ]; then
    print_row "engram-go" "${OK}" "up ${uptime}, port ${ENGRAM_PORT} reachable (HTTP ${http_code})"
  else
    print_row "engram-go" "${WARN}" "up ${uptime}, /health returned HTTP ${http_code}"
  fi
}

# ── Probe: olla embed router ──────────────────────────────────────────────────
probe_olla() {
  local ctr="olla"
  if ! docker inspect "$ctr" &>/dev/null 2>&1; then
    print_row "embed router (olla)" "${FAIL}" "container not found"
    return
  fi
  local state health
  state=$(docker inspect "$ctr" --format '{{.State.Status}}' 2>/dev/null)
  health=$(docker inspect "$ctr" --format '{{.State.Health.Status}}' 2>/dev/null || echo "none")
  if [ "$state" != "running" ]; then
    print_row "embed router (olla)" "${FAIL}" "container state: $state"
    return
  fi
  # Count models via OpenAI-compatible /v1/models
  local model_count
  model_count=$(curl -s --max-time "$PROBE_TIMEOUT" \
    "http://localhost:${OLLA_PORT}/olla/openai/v1/models" 2>/dev/null | \
    python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    models = d.get('data', [])
    print(f'{len(models)} models available')
except:
    print('model list unavailable')
" 2>/dev/null || echo "API unreachable")
  if [ "$health" = "healthy" ]; then
    print_row "embed router (olla)" "${OK}" "${model_count} (container: ${health})"
  else
    print_row "embed router (olla)" "${WARN}" "${model_count} (container: ${health})"
  fi
}

# ── Probe: reembed worker ─────────────────────────────────────────────────────
# Cached NULL count — computed once, shared across all workers
_NULL_CHUNKS_CACHED=""
_null_chunks() {
  if [ -n "$_NULL_CHUNKS_CACHED" ]; then
    echo "$_NULL_CHUNKS_CACHED"
    return
  fi
  local result
  result=$(docker exec engram-postgres sh -c \
    "psql -U engram -d engram -At -c 'SELECT count(*) FROM chunks WHERE embedding IS NULL;'" \
    2>/dev/null | tr -d '[:space:]' || echo "?")
  _NULL_CHUNKS_CACHED="$result"
  echo "$result"
}

probe_reembed() {
  local gpu="$1" ctr="$2"
  if ! docker inspect "$ctr" &>/dev/null 2>&1; then
    print_row "reembed-${gpu}" "${FAIL}" "container not found"
    return
  fi
  local state
  state=$(docker inspect "$ctr" --format '{{.State.Status}}' 2>/dev/null)
  if [ "$state" != "running" ]; then
    print_row "reembed-${gpu}" "${FAIL}" "container state: $state"
    return
  fi
  local null_chunks
  null_chunks=$(_null_chunks)
  print_row "reembed-${gpu}" "${OK}" "active, ${null_chunks} NULL chunks pending"
}

# ── Probe: precision host ollama services ─────────────────────────────────────
probe_precision_ollama() {
  if $NO_REMOTE; then
    print_row "precision: w6800 ollama" "${WARN}" "skipped (--no-remote)"
    print_row "precision: mi50 ollama" "${WARN}" "skipped (--no-remote)"
    return
  fi
  local result
  result=$(ssh -o ConnectTimeout="${SSH_TIMEOUT}" -o BatchMode=yes \
    "$PRECISION_HOST" \
    'printf "w6800:%s\n" "$(systemctl is-active ollama 2>/dev/null)"; printf "mi50:%s\n" "$(systemctl is-active ollama-mi50 2>/dev/null)"; ss -tlnp 2>/dev/null | grep -oE "1143[4-9]" | sort -u | tr "\n" " "' \
    2>/dev/null || echo "SSH_FAIL")

  if [ "$result" = "SSH_FAIL" ] || [ -z "$result" ]; then
    print_row "precision: w6800 ollama" "${FAIL}" "SSH unreachable (${PRECISION_HOST})"
    print_row "precision: mi50 ollama" "${FAIL}" "SSH unreachable (${PRECISION_HOST})"
    return
  fi

  local w6800_state mi50_state ports
  w6800_state=$(echo "$result" | grep '^w6800:' | cut -d: -f2 | tr -d '[:space:]')
  mi50_state=$(echo "$result" | grep '^mi50:' | cut -d: -f2 | tr -d '[:space:]')
  ports=$(echo "$result" | tail -1 | tr -s ' ' | sed 's/^[0-9]/port &/' || echo "?")

  # W6800 via native ollama.service (inactive = stopped; ollama may be managed differently)
  if [ "$w6800_state" = "active" ]; then
    print_row "precision: w6800 ollama" "${OK}" "active (port 11435)"
  elif [ "$w6800_state" = "inactive" ]; then
    print_row "precision: w6800 ollama" "${WARN}" "inactive — systemd service stopped (manual mgmt or W6800 unused)"
  else
    print_row "precision: w6800 ollama" "${FAIL}" "state: ${w6800_state:-unknown}"
  fi

  # MI50 via Docker systemd unit (ollama-mi50.service)
  if [ "$mi50_state" = "active" ]; then
    print_row "precision: mi50 ollama" "${OK}" "active (port 11436) — listening ports: ${ports}"
  elif [ "$mi50_state" = "inactive" ]; then
    print_row "precision: mi50 ollama" "${WARN}" "inactive"
  else
    print_row "precision: mi50 ollama" "${FAIL}" "state: ${mi50_state:-unknown}"
  fi
}

# ── Probe: config drift ───────────────────────────────────────────────────────
probe_config_drift() {
  local drift_ok=true detail=""

  # Check ENGRAM_EMBED_MODEL is set
  if [ -z "$ENV_EMBED_MODEL" ]; then
    drift_ok=false
    detail="ENGRAM_EMBED_MODEL unset in .env"
  else
    # Verify model appears in olla's model list
    local olla_models
    olla_models=$(curl -s --max-time "$PROBE_TIMEOUT" \
      "http://localhost:${OLLA_PORT}/olla/openai/v1/models" 2>/dev/null | \
      python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    ids = [m.get('id','') for m in d.get('data',[])]
    print('\n'.join(ids))
except:
    pass
" 2>/dev/null || true)

    if echo "$olla_models" | grep -qF "$ENV_EMBED_MODEL"; then
      detail="ENGRAM_EMBED_MODEL=${ENV_EMBED_MODEL} found in olla routing"
    else
      drift_ok=false
      detail="ENGRAM_EMBED_MODEL=${ENV_EMBED_MODEL} NOT found in olla model list"
    fi
  fi

  # Check router URL
  if [ -n "$ENV_ROUTER_URL" ]; then
    if echo "$ENV_ROUTER_URL" | grep -qE "olla|40114"; then
      detail="${detail}; router=olla"
    else
      detail="${detail}; router URL may not be olla: ${ENV_ROUTER_URL}"
    fi
  fi

  if $drift_ok; then
    print_row "config drift" "${OK}" "$detail"
  else
    print_row "config drift" "${WARN}" "$detail"
  fi
}

# ── Main ──────────────────────────────────────────────────────────────────────
print_header

probe_postgres
probe_engram
probe_olla
probe_reembed "7900xt"  "engram-reembed-7900xt"
probe_reembed "w6800"   "engram-reembed-w6800"
probe_reembed "oblivion" "engram-reembed-oblivion"
probe_precision_ollama
probe_config_drift

echo ""
if $NO_REMOTE; then
  echo "(precision SSH probes skipped — run 'make status' for full check)"
fi
