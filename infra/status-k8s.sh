#!/usr/bin/env bash
# infra/status-k8s.sh — read-only Kubernetes health check for the engram namespace.

set -uo pipefail

NAMESPACE="${ENGRAM_K8S_NAMESPACE:-engram}"
LOCAL_PORT="${ENGRAM_PORT:-8788}"
PROBE_TIMEOUT="${PROBE_TIMEOUT:-5}"

COL1=28
COL2=8

print_header() {
  echo ""
  printf "%-${COL1}s  %-${COL2}s  %s\n" "LAYER" "STATUS" "DETAIL"
  printf '%0.s=' {1..80}
  echo ""
}

print_row() {
  printf "%-${COL1}s  %-${COL2}s  %s\n" "$1" "$2" "$3"
}

require_kubectl() {
  if ! command -v kubectl >/dev/null 2>&1; then
    echo "kubectl not found" >&2
    exit 1
  fi
}

probe_deploy() {
  local name="$1"
  local ready available desired images detail
  ready=$(kubectl -n "$NAMESPACE" get deploy "$name" -o jsonpath='{.status.readyReplicas}' 2>/dev/null || true)
  available=$(kubectl -n "$NAMESPACE" get deploy "$name" -o jsonpath='{.status.availableReplicas}' 2>/dev/null || true)
  desired=$(kubectl -n "$NAMESPACE" get deploy "$name" -o jsonpath='{.spec.replicas}' 2>/dev/null || true)
  images=$(kubectl -n "$NAMESPACE" get deploy "$name" -o jsonpath='{range .spec.template.spec.containers[*]}{.name}{"="}{.image}{" "}{end}' 2>/dev/null || true)
  ready="${ready:-0}"
  available="${available:-0}"
  desired="${desired:-0}"
  images="${images:-unknown}"
  detail="ready=${ready}/${desired}, available=${available}/${desired}, image=${images% }"
  if [ "$ready" = "$desired" ] && [ "$available" = "$desired" ] && [ "$desired" != "0" ]; then
    print_row "$name" "OK" "$detail"
  else
    print_row "$name" "WARN" "$detail"
  fi
}

probe_pod_restarts() {
  local selector="$1"
  local label="$2"
  local rows status
  rows=$(kubectl -n "$NAMESPACE" get pods -l "$selector" -o jsonpath='{range .items[*]}{.metadata.name}{"|"}{range .status.containerStatuses[*]}{.name}{":"}{.restartCount}{":prev="}{.lastState.terminated.reason}{":exit="}{.lastState.terminated.exitCode}{" "}{end}{"\n"}{end}' 2>/dev/null || true)
  if [ -z "$rows" ]; then
    print_row "$label pods" "WARN" "no pods found for selector ${selector}"
    return
  fi
  status="OK"
  if echo "$rows" | grep -Eq ':[1-9][0-9]*:prev='; then
    status="WARN"
  fi
  rows=$(echo "$rows" | sed -e 's/prev=:exit=/prev=none:exit=none/g' -e 's/:exit= /:exit=none /g')
  print_row "$label pods" "$status" "$rows"
}

probe_local_http() {
  local path="$1"
  local code body
  body=$(mktemp)
  code=$(curl -sS --max-time "$PROBE_TIMEOUT" -o "$body" -w "%{http_code}" "http://127.0.0.1:${LOCAL_PORT}${path}" 2>/dev/null || echo "000")
  if [ "$code" = "200" ]; then
    print_row "localhost${path}" "OK" "$(tr '\n' ' ' < "$body" | cut -c1-140)"
  else
    print_row "localhost${path}" "WARN" "HTTP ${code}; if no port-forward is active, run: kubectl -n ${NAMESPACE} port-forward svc/engram-go ${LOCAL_PORT}:8788"
  fi
  rm -f "$body"
}

probe_recent_warnings() {
  local warnings
  warnings=$(kubectl -n "$NAMESPACE" get events --field-selector type=Warning --sort-by=.lastTimestamp 2>/dev/null | tail -10 || true)
  if [ -z "$warnings" ]; then
    print_row "recent warnings" "OK" "none"
  else
    print_row "recent warnings" "WARN" "$warnings"
  fi
}

probe_metrics() {
  local body code pending acquired max status detail ratio
  if [ -z "${ENGRAM_API_KEY:-}" ]; then
    print_row "metrics auth" "WARN" "skipped; set ENGRAM_API_KEY to probe authenticated /metrics"
    return
  fi

  body=$(mktemp)
  code=$(curl -sS --max-time "$PROBE_TIMEOUT" -o "$body" -w "%{http_code}" \
    -H "Authorization: Bearer ${ENGRAM_API_KEY}" \
    "http://127.0.0.1:${LOCAL_PORT}/metrics" 2>/dev/null || echo "000")
  if [ "$code" != "200" ]; then
    print_row "metrics auth" "WARN" "HTTP ${code}; verify ENGRAM_API_KEY and any active port-forward"
    rm -f "$body"
    return
  fi

  pending=$(awk '$1 == "engram_chunks_pending_reembed" {print $2; found=1; exit} END {if (!found) print "missing"}' "$body")
  status="OK"
  detail="pending=${pending}"
  if [ "$pending" = "missing" ]; then
    status="WARN"
  elif [ "$pending" -gt 0 ] 2>/dev/null; then
    status="WARN"
  fi
  print_row "metrics backlog" "$status" "$detail"

  acquired=$(awk '$1 == "engram_db_pool_acquired_conns" {print $2; found=1; exit} END {if (!found) print "missing"}' "$body")
  max=$(awk '$1 == "engram_db_pool_max_conns" {print $2; found=1; exit} END {if (!found) print "missing"}' "$body")
  status="OK"
  detail="acquired=${acquired}, max=${max}"
  if [ "$acquired" = "missing" ] || [ "$max" = "missing" ] || [ "$max" = "0" ]; then
    status="WARN"
  else
    ratio=$(awk -v acquired="$acquired" -v max="$max" 'BEGIN { if (max > 0) printf "%.2f", acquired / max; else print "nan" }')
    detail="${detail}, saturation=${ratio}"
    if awk -v ratio="$ratio" 'BEGIN { exit !(ratio >= 0.80) }'; then
      status="WARN"
    fi
  fi
  print_row "metrics db pool" "$status" "$detail"
  rm -f "$body"
}

require_kubectl
print_header
print_row "context" "INFO" "$(kubectl config current-context 2>/dev/null || echo unknown)"
probe_deploy "engram-go"
probe_deploy "engram-reembed"
probe_pod_restarts "app=engram-go" "engram-go"
probe_pod_restarts "app=engram-reembed" "engram-reembed"
probe_local_http "/health"
probe_local_http "/ready"
probe_metrics
probe_recent_warnings
echo ""
echo "Metrics require Bearer auth: curl -H \"Authorization: Bearer \$ENGRAM_API_KEY\" http://127.0.0.1:${LOCAL_PORT}/metrics"
