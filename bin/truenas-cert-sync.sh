#!/bin/bash
# Syncs the wildcard *.petersimmons.com cert from K8s (cert-manager) → TruNAS
# Run via cron or manually. Safe to run repeatedly.
# Source: kubectl secret petersimmons-tls (default namespace)
# Destination: TruNAS SCALE HTTPS certificate
# Uses JSON-RPC 2.0 WebSocket API (truenas-rpc helper)

set -euo pipefail

K8S_SECRET="petersimmons-tls"
K8S_NAMESPACE="default"
CERT_NAME="wildcard-petersimmons-k8s-$(date +%Y%m%d)"

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"; }

# Get current cert expiry from K8s
CERT_DATA=$(kubectl get secret "$K8S_SECRET" -n "$K8S_NAMESPACE" \
  -o jsonpath='{.data.tls\.crt}' | base64 -d)
EXPIRY=$(echo "$CERT_DATA" | openssl x509 -noout -enddate | cut -d= -f2)
EXPIRY_EPOCH=$(date -d "$EXPIRY" +%s)
NOW_EPOCH=$(date +%s)
DAYS_REMAINING=$(( (EXPIRY_EPOCH - NOW_EPOCH) / 86400 ))

log "K8s cert expires: $EXPIRY ($DAYS_REMAINING days remaining)"

# Get current TruNAS cert expiry via WebSocket API
CURRENT_CERT=$(truenas-rpc system.general.config | jq -r '.ui_certificate.until // "unknown"')
log "TruNAS current cert expires: $CURRENT_CERT"

TRUENAS_EXPIRY_EPOCH=$(date -d "$CURRENT_CERT" +%s 2>/dev/null || echo 0)
TRUENAS_DAYS=$(( (TRUENAS_EXPIRY_EPOCH - NOW_EPOCH) / 86400 ))

if [ "$TRUENAS_DAYS" -gt 30 ] 2>/dev/null; then
  log "TruNAS cert has $TRUENAS_DAYS days remaining — no update needed"
  exit 0
fi

log "TruNAS cert expires in $TRUENAS_DAYS days — updating..."

# Extract cert + key from K8s
CERT=$(kubectl get secret "$K8S_SECRET" -n "$K8S_NAMESPACE" \
  -o jsonpath='{.data.tls\.crt}' | base64 -d)
KEY=$(kubectl get secret "$K8S_SECRET" -n "$K8S_NAMESPACE" \
  -o jsonpath='{.data.tls\.key}' | base64 -d)

# Build params JSON safely (handles multiline PEM strings)
CERT_PARAMS=$(jq -n \
  --arg name "$CERT_NAME" \
  --arg cert "$CERT" \
  --arg key "$KEY" \
  '[{name: $name, create_type: "CERTIFICATE_CREATE_IMPORTED", certificate: $cert, privatekey: $key}]')

# Import new cert into TruNAS (async job — --wait-job polls until done)
log "Importing cert as '$CERT_NAME'..."
CERT_RESULT=$(truenas-rpc certificate.create "$CERT_PARAMS" --wait-job)
CERT_ID=$(echo "$CERT_RESULT" | jq -r '.id')
log "Cert imported with ID $CERT_ID — setting as active..."

# Set as active UI certificate
truenas-rpc system.general.update "[{\"ui_certificate\": $CERT_ID}]" > /dev/null

log "Done — TruNAS now using '$CERT_NAME' (expires in $DAYS_REMAINING days)"
