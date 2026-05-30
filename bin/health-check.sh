#!/bin/bash
# Homelab health check — all probes run in parallel, total wall time ≤ 3s

TMPDIR_HC=$(mktemp -d)
cleanup() { rm -rf "$TMPDIR_HC"; }
trap cleanup EXIT

# ── Kubernetes cluster ───────────────────────────────────────────────────────
check_k8s() {
  local out="$TMPDIR_HC/k8s"
  {
    echo "=== Kubernetes Cluster ==="
    node_issues=$(timeout 5 kubectl get nodes --no-headers 2>/dev/null | grep -v " Ready " | wc -l)
    if [ "$node_issues" -gt 0 ]; then
      echo "⚠️  $node_issues node(s) not ready"
      timeout 5 kubectl get nodes --no-headers 2>/dev/null | grep -v " Ready "
    else
      echo "✅ All nodes ready"
    fi

    echo ""
    echo "=== Critical Pods ==="
    pod_issues=$(timeout 5 kubectl get pods -A 2>/dev/null | grep -E "(Error|CrashLoop|Pending)")
    if [ -n "$pod_issues" ]; then
      echo "$pod_issues"
      echo "⚠️  Pod issues detected"
    else
      echo "✅ All pods healthy"
    fi

    echo ""
    echo "=== Internal DNS Routing (CoreDNS → Unifi) ==="
    if timeout 5 kubectl get configmap coredns-custom -n kube-system &>/dev/null; then
      echo "✅ coredns-custom ConfigMap present (*.petersimmons.com routes to Unifi)"
    else
      echo "❌ coredns-custom ConfigMap MISSING — pods cannot resolve *.petersimmons.com"
      echo "   Fix: kubectl apply the petersimmons.server block forwarding to 192.168.0.1"
    fi

    echo ""
    echo "=== Archived Deployments (DB-less CrashLoop Check) ==="
    timeout 5 kubectl get deployment -A --no-headers 2>/dev/null | while read ns name ready up avail age; do
      db_host=$(timeout 3 kubectl get deployment "$name" -n "$ns" -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="DB_HOST")].value}' 2>/dev/null)
      if [ -n "$db_host" ]; then
        sts_name=$(echo "$db_host" | cut -d. -f1)
        sts_replicas=$(timeout 3 kubectl get statefulset "$sts_name" -n "$ns" -o jsonpath='{.spec.replicas}' 2>/dev/null)
        deploy_replicas=$(timeout 3 kubectl get deployment "$name" -n "$ns" -o jsonpath='{.spec.replicas}' 2>/dev/null)
        if [ "$sts_replicas" = "0" ] && [ "$deploy_replicas" != "0" ]; then
          echo "⚠️  $ns/$name: DB StatefulSet '$sts_name' has 0 replicas but Deployment has $deploy_replicas — CrashLoop risk"
        fi
      fi
    done
    echo "✅ Archived deployment check complete"

    echo ""
    echo "=== TLS Certificates ==="
    stuck_certs=$(timeout 5 kubectl get certificate -A -o json 2>/dev/null | jq -r '.items[] | select(.status.conditions[0].status=="False" and (.status.conditions[0].lastTransitionTime | fromdateiso8601) < (now - 604800)) | [.metadata.namespace, .metadata.name] | @tsv' 2>/dev/null)
    if [ -z "$stuck_certs" ]; then
      echo "✅ All certificates renewing normally"
    else
      echo "⚠️  Certificates stuck for >7 days:"
      echo "$stuck_certs" | while read ns name; do
        echo "   $ns/$name - May need manual intervention"
      done
    fi
    pending_challenges=$(timeout 5 kubectl get challenge -A --no-headers 2>/dev/null | grep -c "pending" 2>/dev/null)
    if [ "${pending_challenges:-0}" -gt 0 ]; then
      echo "⚠️  $pending_challenges pending ACME challenge(s) - check DNS propagation"
      timeout 5 kubectl get challenge -A --no-headers 2>/dev/null | grep "pending"
    fi
  } > "$out" 2>&1
}

# ── Public services (HTTPS) ──────────────────────────────────────────────────
check_public_services() {
  local out="$TMPDIR_HC/public_services"
  {
    echo "=== Public Services ==="
    status=$(xh --print=h --timeout 3 HEAD https://nextcloud.petersimmons.com 2>/dev/null | awk 'NR==1{print $2}')
    if [[ "$status" =~ ^(200|302)$ ]]; then
      echo "✅ https://nextcloud.petersimmons.com ($status)"
    else
      echo "❌ https://nextcloud.petersimmons.com (${status:-timeout})"
    fi

    status=$(xh --print=h --timeout 3 HEAD https://homepage.petersimmons.com 2>/dev/null | awk 'NR==1{print $2}')
    if [ "$status" = "200" ]; then
      echo "✅ https://homepage.petersimmons.com"
    else
      echo "❌ https://homepage.petersimmons.com (${status:-timeout})"
    fi
  } > "$out" 2>&1
}

# ── TruNAS storage ───────────────────────────────────────────────────────────
check_trunas() {
  local out="$TMPDIR_HC/trunas"
  {
    echo "=== TruNAS Storage ==="
    pool=$(timeout 4 truenas-rpc pool.query 2>/dev/null | \
      jq -r '.[] | "\(.name): \(.status) | used: \((.allocated/.size*100)|round)% of \((.size/1099511627776*10|round)/10)TB"' 2>/dev/null)
    alerts=$(timeout 4 truenas-rpc alert.list 2>/dev/null | \
      jq -r '[.[] | select(.dismissed==false and .level!="INFO")] | length' 2>/dev/null)
    if [ -n "$pool" ]; then
      while IFS= read -r line; do
        if echo "$line" | grep -q "ONLINE"; then
          echo "✅ $line"
        else
          echo "❌ $line"
        fi
      done <<< "$pool"
      [ "${alerts:-0}" -gt 0 ] && echo "⚠️  $alerts active TruNAS alert(s)" || echo "✅ No active TruNAS alerts"
    else
      echo "⚠️  Could not reach TruNAS API"
    fi
  } > "$out" 2>&1
}

# ── Proxmox storage (SSH) ────────────────────────────────────────────────────
check_proxmox() {
  local out="$TMPDIR_HC/proxmox"
  {
    echo "=== Proxmox Storage ==="
    zp3_info=$(timeout 6 ssh -o ConnectTimeout=3 -o BatchMode=yes \
      -o StrictHostKeyChecking=no psimmons@192.168.0.100 \
      "zpool list zp3 -H -o name,cap,health" 2>/dev/null)
    if [ -n "$zp3_info" ]; then
      name=$(echo "$zp3_info" | awk '{print $1}')
      cap=$(echo "$zp3_info" | awk '{print $2}' | tr -d '%')
      health=$(echo "$zp3_info" | awk '{print $3}')
      if [ "${cap:-0}" -gt 85 ] 2>/dev/null; then
        echo "⚠️  $name: ${cap}% used (ABOVE 85% threshold) | $health"
      else
        echo "✅ $name: ${cap}% used | $health"
      fi
    else
      echo "⚠️  Could not SSH to Proxmox to check zp3 storage"
    fi
  } > "$out" 2>&1
}

# ── DNS servers ──────────────────────────────────────────────────────────────
# Pi-holes (192.168.0.231/.232) retired — probes removed 2026-05-30 (aifleet#181)
check_dns() {
  local out="$TMPDIR_HC/dns"
  {
    echo "=== DNS Servers ==="
    echo "⏸️  Pi-hole 231/232 — retired (aifleet#181)"
  } > "$out" 2>&1
}

# ── AI fleet — direct endpoint probes (no Docker health status) ─────────────
# Rule: every check must verify the actual API response, not Docker's cached status.
# leviathan.petersimmons.com has Cloudflare AAAA records blocking non-443 ports;
# use localhost for local containers to avoid IPv6 Cloudflare routing.
check_ai_fleet() {
  local out="$TMPDIR_HC/ai_fleet"
  {
    echo "=== AI Fleet ==="

    # embed: leviathan 7900XT — DECOMMISSIONED (issue #25: gfx1100 PyTorch VRAM OOM)
    # Re-enable this check when issue #25 is resolved and model is re-added to fleet.
    # echo "⏸️  embed  | leviathan 7900XT :8004     decommissioned (issue #25)"

    # embed: precision W6800 — internal hostname routes on LAN
    resp=$(curl -sf --max-time 8 -X POST http://precision.petersimmons.com:8005/v1/embeddings \
      -H 'Content-Type: application/json' \
      -d '{"model":"BAAI/bge-m3","input":["probe"]}' 2>/dev/null)
    dim=$(echo "$resp" | python3 -c "import json,sys; print(len(json.load(sys.stdin)['data'][0]['embedding']))" 2>/dev/null)
    if [ "$dim" = "1024" ]; then
      echo "✅ embed  | precision W6800 :8005      dim=$dim"
    else
      echo "❌ embed  | precision W6800 :8005      no response or hung (dim=${dim:-?})"
    fi

    # embed: olla proxy — must route and return a real embedding
    resp=$(curl -sf --max-time 8 -X POST https://olla.petersimmons.com/olla/openai/v1/embeddings \
      -H 'Content-Type: application/json' \
      -d '{"model":"BAAI/bge-m3","input":["probe"]}' 2>/dev/null)
    dim=$(echo "$resp" | python3 -c "import json,sys; print(len(json.load(sys.stdin)['data'][0]['embedding']))" 2>/dev/null)
    if [ "$dim" = "1024" ]; then
      echo "✅ embed  | olla proxy (routed)        dim=$dim"
    else
      echo "❌ embed  | olla proxy (routed)        routing failed or all endpoints down"
    fi

    # inference: oblivion vLLM — verify a model is listed; detect stuck startup
    # If not serving after VLLM_START_TIMEOUT_MIN minutes, flag as STUCK not "loading"
    VLLM_START_TIMEOUT_MIN=15
    resp=$(curl -sf --max-time 8 http://oblivion.petersimmons.com:8000/v1/models 2>/dev/null)
    model=$(echo "$resp" | python3 -c "import json,sys; print(json.load(sys.stdin)['data'][0]['id'])" 2>/dev/null)
    if [ -n "$model" ]; then
      echo "✅ infer  | oblivion vLLM :8000        model=$model"
    else
      # Get container start time from controller registry to detect hung startup
      container_start=$(kubectl port-forward -n ai-fleet deploy/ai-fleet-controller 18080:8080 >/dev/null 2>&1 &         PF2=$! ; sleep 2 ;         curl -sf --max-time 5 http://localhost:18080/registry 2>/dev/null           | python3 -c "
import json,sys,datetime
data=json.load(sys.stdin)
hosts=data if isinstance(data,list) else data.get('hosts',data.get('registry',[data]))
for h in (hosts if isinstance(hosts,list) else [hosts]):
    if 'oblivion' in str(h.get('host','')):
        ls=h.get('lastSeen','')
        print(ls[:19] if ls else '')
" 2>/dev/null ; kill $PF2 2>/dev/null ; wait $PF2 2>/dev/null)
      if [ -n "$container_start" ] && [ "$container_start" != "0" ]; then
        start_epoch=$(date -d "$container_start" +%s 2>/dev/null || echo 0)
        now_epoch=$(date +%s)
        age_min=$(( (now_epoch - start_epoch) / 60 ))
        if [ "$age_min" -lt 0 ] || [ "$start_epoch" -eq 0 ]; then
          echo "❌ infer  | oblivion vLLM :8000        not serving (container not started)"
        elif [ "$age_min" -gt "$VLLM_START_TIMEOUT_MIN" ]; then
          echo "❌ infer  | oblivion vLLM :8000        STUCK — not serving after ${age_min}min (wrong image or bad flags)"
        else
          echo "⏳ infer  | oblivion vLLM :8000        loading (${age_min}min elapsed, timeout=${VLLM_START_TIMEOUT_MIN}min)"
        fi
      else
        echo "❌ infer  | oblivion vLLM :8000        not serving (no container)"
      fi
    fi

    # MCP: engram-go local
    resp=$(curl -sf --max-time 5 http://localhost:8788/health 2>/dev/null)
    if echo "$resp" | python3 -c "import json,sys; assert json.load(sys.stdin).get('status')=='ok'" 2>/dev/null; then
      echo "✅ mcp    | engram-go :8788            ok"
    else
      echo "❌ mcp    | engram-go :8788            $(echo "$resp" | head -c 60)"
    fi

    # proxy: olla internal health
    resp=$(curl -sf --max-time 5 https://olla.petersimmons.com/internal/health 2>/dev/null)
    if echo "$resp" | python3 -c "import json,sys; s=json.load(sys.stdin).get('status',''); assert s in ('ok','healthy')" 2>/dev/null; then
      echo "✅ proxy  | olla /internal/health      ok"
    else
      echo "❌ proxy  | olla /internal/health      $(echo "$resp" | head -c 60)"
    fi

    # GPU: 7900XT VRAM — informational, not pass/fail (zombie detection)
    kfd_pids=$(docker run --rm --device /dev/kfd --device /dev/dri/renderD128 \
      rocm/pytorch:latest rocm-smi --showpids 2>/dev/null | grep -c '^[0-9]' 2>/dev/null || echo 0)
    vram_pct=$(docker run --rm --device /dev/kfd --device /dev/dri/renderD128 \
      rocm/pytorch:latest rocm-smi 2>/dev/null \
      | awk '/[0-9]+Mhz/{for(i=1;i<=NF;i++){if($i~/^[0-9]+%$/){gsub(/%/,"",$i);print $i;exit}}}' 2>/dev/null || echo "?")
    echo "ℹ️  gpu   | 7900XT KFD procs=$kfd_pids  VRAM≈${vram_pct}%"

    # reembed backlog
    backlog=$(docker exec engram-postgres psql -U engram engram -t -A \
      -c "SELECT count(*) FROM chunks WHERE embedding IS NULL AND project LIKE 'lme-c3d9f1-%'" \
      2>/dev/null | tr -d ' ')
    [ -n "$backlog" ] && echo "ℹ️  reemb | lme-c3d9f1 null embeddings  $backlog"

  } > "$out" 2>&1
}

# ── Launch all probes in parallel ────────────────────────────────────────────
check_k8s &
check_public_services &
check_trunas &
check_proxmox &
check_dns &
check_ai_fleet &
wait

# ── Print results in deterministic order ────────────────────────────────────
for section in k8s public_services trunas proxmox dns ai_fleet; do
  f="$TMPDIR_HC/${section}"
  [ -f "$f" ] && cat "$f" && echo ""
done
