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
check_dns() {
  local out="$TMPDIR_HC/dns"
  {
    echo "=== DNS Servers ==="
    timeout 2 dig @192.168.0.231 google.com +short > /dev/null 2>&1 \
      && echo "✅ Primary Pi-hole (231)" || echo "❌ Primary Pi-hole (231)"
    timeout 2 dig @192.168.0.232 google.com +short > /dev/null 2>&1 \
      && echo "✅ Secondary Pi-hole (232)" || echo "❌ Secondary Pi-hole (232)"
  } > "$out" 2>&1
}

# ── Launch all probes in parallel ────────────────────────────────────────────
check_k8s &
check_public_services &
check_trunas &
check_proxmox &
check_dns &
wait

# ── Print results in deterministic order ────────────────────────────────────
for section in k8s public_services trunas proxmox dns; do
  f="$TMPDIR_HC/${section}"
  [ -f "$f" ] && cat "$f" && echo ""
done
