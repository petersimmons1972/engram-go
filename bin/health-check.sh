#!/bin/bash
# 30-second health check for entire homelab

echo "=== Kubernetes Cluster ==="
node_issues=$(kubectl get nodes --no-headers | grep -v " Ready " | wc -l)
if [ "$node_issues" -gt 0 ]; then
  echo "⚠️  $node_issues node(s) not ready"
  kubectl get nodes --no-headers | grep -v " Ready "
else
  echo "✅ All nodes ready"
fi

echo -e "\n=== Critical Pods ==="
kubectl get pods -A | grep -E "(Error|CrashLoop|Pending)" && echo "⚠️  Pod issues detected" || echo "✅ All pods healthy"

echo -e "\n=== Public Services ==="
# Nextcloud: 200 or 302 (redirect to login) are OK
status=$(curl -s -o /dev/null -w "%{http_code}" https://nextcloud.petersimmons.com)
if [[ "$status" =~ ^(200|302)$ ]]; then
  echo "✅ https://nextcloud.petersimmons.com ($status)"
else
  echo "❌ https://nextcloud.petersimmons.com ($status)"
fi

# Homepage: HTTPS only (no HTTP redirect configured)
status=$(curl -s --max-time 10 -o /dev/null -w "%{http_code}" https://homepage.petersimmons.com)
if [ "$status" = "200" ]; then
  echo "✅ https://homepage.petersimmons.com"
else
  echo "❌ https://homepage.petersimmons.com ($status)"
fi

echo -e "\n=== TruNAS Storage ==="
pool=$(truenas-rpc pool.query 2>/dev/null | \
  jq -r '.[] | "\(.name): \(.status) | used: \((.allocated/.size*100)|round)% of \((.size/1099511627776*10|round)/10)TB"' 2>/dev/null)
alerts=$(truenas-rpc alert.list 2>/dev/null | \
  jq -r '[.[] | select(.dismissed==false and .level!="INFO")] | length' 2>/dev/null)
if [ -n "$pool" ]; then
  while IFS= read -r line; do
    if echo "$line" | grep -q "ONLINE"; then
      echo "✅ $line"
    else
      echo "❌ $line"
    fi
  done <<< "$pool"
  [ "$alerts" -gt 0 ] 2>/dev/null && echo "⚠️  $alerts active TruNAS alert(s)" || echo "✅ No active TruNAS alerts"
else
  echo "⚠️  Could not reach TruNAS API"
fi

echo -e "\n=== Proxmox Storage ==="
# Use zpool list (works as non-root) instead of pvesm status (requires root)
zp3_info=$(ssh -o ConnectTimeout=5 psimmons@192.168.0.100 "zpool list zp3 -H -o name,cap,health" 2>/dev/null)
if [ -n "$zp3_info" ]; then
  name=$(echo "$zp3_info" | awk '{print $1}')
  cap=$(echo "$zp3_info" | awk '{print $2}' | tr -d '%')
  health=$(echo "$zp3_info" | awk '{print $3}')
  if [ "$cap" -gt 85 ] 2>/dev/null; then
    echo "⚠️  $name: ${cap}% used (ABOVE 85% threshold) | $health"
  else
    echo "✅ $name: ${cap}% used | $health"
  fi
else
  echo "⚠️  Could not SSH to Proxmox to check zp3 storage"
fi

echo -e "\n=== DNS Servers ==="
dig @192.168.0.231 google.com +short > /dev/null && echo "✅ Primary Pi-hole (231)" || echo "❌ Primary Pi-hole (231)"
dig @192.168.0.232 google.com +short > /dev/null && echo "✅ Secondary Pi-hole (232)" || echo "❌ Secondary Pi-hole (232)"

echo -e "\n=== TLS Certificates ==="
# Check for certificates not ready for >7 days (will expire before renewal completes)
stuck_certs=$(kubectl get certificate -A -o json | jq -r '.items[] | select(.status.conditions[0].status=="False" and (.status.conditions[0].lastTransitionTime | fromdateiso8601) < (now - 604800)) | [.metadata.namespace, .metadata.name] | @tsv' 2>/dev/null)
if [ -z "$stuck_certs" ]; then
  echo "✅ All certificates renewing normally"
else
  echo "⚠️  Certificates stuck for >7 days:"
  echo "$stuck_certs" | while read ns name; do
    echo "   $ns/$name - May need manual intervention"
  done
fi

# Check for active ACME challenges that aren't propagating
pending_challenges=$(kubectl get challenge -A --no-headers 2>/dev/null | grep -c "pending" 2>/dev/null)
if [ "$pending_challenges" -gt 0 ]; then
  echo "⚠️  $pending_challenges pending ACME challenge(s) - check DNS propagation"
  kubectl get challenge -A --no-headers 2>/dev/null | grep "pending"
fi
