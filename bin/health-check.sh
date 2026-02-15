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
status=$(curl -s -o /dev/null -w "%{http_code}" https://homepage.petersimmons.com)
if [ "$status" = "200" ]; then
  echo "✅ https://homepage.petersimmons.com"
else
  echo "❌ https://homepage.petersimmons.com ($status)"
fi

echo -e "\n=== Proxmox Storage ==="
# Check if we can SSH to Proxmox and query storage
if ssh -o ConnectTimeout=5 psimmons@192.168.0.100 "command -v pvesm >/dev/null 2>&1" 2>/dev/null; then
  ssh psimmons@192.168.0.100 "pvesm status" 2>/dev/null | grep zp3 || echo "⚠️  Could not query zp3 storage"
else
  echo "⚠️  Cannot access Proxmox storage commands (may need to run as root or configure sudo)"
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
pending_challenges=$(kubectl get challenge -A --no-headers 2>/dev/null | grep -c "pending" || echo "0")
if [ "$pending_challenges" -gt 0 ]; then
  echo "⚠️  $pending_challenges pending ACME challenge(s) - check DNS propagation"
  kubectl get challenge -A --no-headers 2>/dev/null | grep "pending"
fi
