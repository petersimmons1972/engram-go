# Runbook: Troubleshoot Certificate Renewal Failures

## Quick Diagnosis (5 minutes)

```bash
# 1. Check certificate status
kubectl get certificate -A

# 2. If any show Ready=False, get details
kubectl describe certificate <name> -n <namespace>

# 3. Check for pending challenges
kubectl get challenge -A

# 4. If challenge exists, verify DNS propagation
nslookup _acme-challenge.<domain> 1.1.1.1
```

---

## Symptom-Based Troubleshooting

### Symptom: Certificate shows Ready=False, has been for >7 days

**Time to fix**: 15 minutes
**Severity**: High (will expire within ~90 days)

```bash
# Step 1: Get full certificate status
kubectl describe certificate <name> -n <namespace>

# Look for Reason and Message fields
```

**Possible Causes**:

#### 1. Issuer Misconfiguration
**Reason**: "IncorrectIssuer"

```bash
# Problem: Certificate references wrong issuer
kubectl get certificate <name> -n <namespace> -o yaml | grep issuerRef

# Solution: Verify issuer exists and is ready
kubectl get clusterissuer letsencrypt-prod -o yaml | grep "Ready:"

# If issuer name is wrong (e.g., "letsencrypt-production" vs "letsencrypt-prod"):
# Update the source Ingress resource
kubectl get ingress <name> -n <namespace> -o yaml | grep cluster-issuer
# Update the annotation: cert-manager.io/cluster-issuer: letsencrypt-prod
```

#### 2. DNS Challenge Propagation Failure
**Reason**: "Waiting for DNS-01 challenge propagation"

```bash
# Step 1: Check if challenge TXT record exists in DNS
nslookup _acme-challenge.<domain> 1.1.1.1

# If record doesn't exist:
# This means Cloudflare API call didn't create the record

# Step 2: Check Cloudflare API token
kubectl get secret -n cert-manager cloudflare-api-token-secret -o yaml

# Step 3: Check cert-manager pod IP
kubectl get pod -n cert-manager -l app=cert-manager -o wide | grep "cert-manager-"
# Note: Pod IP will be 10.42.x.x (Kubernetes overlay network)

# Step 4: Check if IP filtering on token blocks pod
# Log into Cloudflare dashboard:
# - API Tokens > "Kubernetes - cert-manager"
# - Check "Client IP Address Filtering"
# - If 192.168.x.x only, add 10.42.0.0/16 OR remove filtering

# Step 5: Force renewal after fixing IP filtering
kubectl delete certificaterequest -n <namespace> <cert>-*
# This triggers cert-manager to create a new challenge
```

#### 3. Cloudflare Zone Issues
**Symptom**: Challenge shows "Presented: true" but DNS never updates

```bash
# Check Cloudflare nameservers are correct
dig <domain> NS

# Check zone is active
# Log into Cloudflare dashboard and verify:
# - Zone is active (not pending, restricted, etc.)
# - Nameservers are configured correctly
# - API token has permissions on this zone
```

#### 4. Cert-Manager Pod Network Issue
**Symptom**: DNS propagation fails, but Cloudflare API says success

```bash
# This indicates the API request reached Cloudflare but was rejected
# Usually: IP filtering on Cloudflare token blocks pod IP

# Check pod IP range
kubectl get pod -n cert-manager -o wide | grep cert-manager | awk '{print $6}'
# Result will be 10.42.x.x (Kubernetes overlay)

# Check token IP filtering in Cloudflare
# If restricted to 192.168.x.x, update to include 10.42.0.0/16

# OR simplify: Remove IP filtering entirely
# The zone + permission scoping is sufficient security
```

---

## Full Diagnostic Procedure

```bash
#!/bin/bash
# Save as ~/bin/diagnose-cert.sh

if [ -z "$1" ]; then
  echo "Usage: diagnose-cert.sh <cert-name> [namespace]"
  exit 1
fi

CERT=$1
NS=${2:-default}

echo "=== Certificate Status ==="
kubectl get certificate $CERT -n $NS
kubectl describe certificate $CERT -n $NS | tail -20

echo -e "\n=== Related Challenges ==="
kubectl get challenge -n $NS | grep $CERT

echo -e "\n=== Challenge Details ==="
for challenge in $(kubectl get challenge -n $NS -o name | grep $CERT); do
  echo "Challenge: $challenge"
  kubectl describe $challenge -n $NS | grep -A 10 "Status:"
done

echo -e "\n=== DNS Propagation Check ==="
DOMAIN=$(kubectl get certificate $CERT -n $NS -o yaml | grep "dnsNames:" -A 3 | grep "-" | head -1 | xargs)
if [ -n "$DOMAIN" ]; then
  echo "Domain: $DOMAIN"
  echo "Checking _acme-challenge.$DOMAIN..."
  nslookup "_acme-challenge.$DOMAIN" 1.1.1.1 || echo "Record not found (expected if challenge pending)"
fi

echo -e "\n=== cert-manager Logs (recent ACME/DNS errors) ==="
kubectl logs -n cert-manager -l app=cert-manager --tail=100 | \
  grep -i "acme\|dns\|cloudflare\|challenge\|propagation\|error" | tail -20

echo -e "\n=== Issuer Status ==="
ISSUER=$(kubectl get certificate $CERT -n $NS -o yaml | grep "issuerRef:" -A 3 | grep "name:" | awk '{print $2}')
echo "Issuer: $ISSUER"
kubectl describe clusterissuer $ISSUER | tail -15
```

---

## Common Fixes

### Fix 1: Update Cloudflare Token IP Filtering
```bash
# In Cloudflare Dashboard:
1. API Tokens > "Kubernetes - cert-manager"
2. Scroll to "Client IP Address Filtering"
3. Either:
   a) Remove all IP restrictions, OR
   b) Add 10.42.0.0/16 to the whitelist
4. Save

# Verify in K8s:
kubectl delete pod -n cert-manager -l app=cert-manager
# This restarts cert-manager and triggers renewal
```

### Fix 2: Correct Issuer Reference
```bash
# Find the Ingress that owns the certificate
kubectl get ingress -A -o yaml | grep -l "cert-manager.io/cluster-issuer"

# Update the annotation
kubectl patch ingress <ingress-name> -n <namespace> \
  -p '{"metadata":{"annotations":{"cert-manager.io/cluster-issuer":"letsencrypt-prod"}}}'

# Or edit the source and apply
kubectl apply -f ingress.yaml
```

### Fix 3: Force Certificate Renewal
```bash
# Delete related CertificateRequests to trigger new attempt
kubectl delete certificaterequest -n <namespace> -l cert-manager.io/certificate-name=<cert-name>

# cert-manager will automatically create a new CertificateRequest
# Monitor progress:
kubectl get certificate <cert-name> -n <namespace> --watch
```

---

## Verification Checklist

After applying a fix:

- [ ] `kubectl get certificate -A` shows target cert with Ready=True
- [ ] `kubectl get challenge -A` shows no pending challenges for this domain
- [ ] `nslookup _acme-challenge.<domain> 1.1.1.1` finds no TXT record (expected after challenge completes)
- [ ] Certificate expiry date is correct: `kubectl get certificate <name> -n <namespace> -o yaml | grep notAfter`
- [ ] health-check.sh shows no certificate warnings: `~/bin/health-check.sh | grep -i cert`
- [ ] Test service HTTPS works: `curl -I https://<domain>`

---

## Prevention

Add to regular health checks:
```bash
# Weekly: Verify all certs renewing normally
kubectl get certificate -A | grep -v Ready

# Monthly: Check cert-manager logs for ACME/DNS errors
kubectl logs -n cert-manager -l app=cert-manager --since=720h | grep -i error | head -20

# Monthly: Review Cloudflare API audit logs for token issues
# Cloudflare Dashboard > Audit Logs > Filter by "Kubernetes - cert-manager" token
```

---

## When to Escalate

If the above steps don't resolve the issue:

1. Check Cloudflare status page for API issues
2. Review Cloudflare API audit logs for detailed rejection reasons
3. Test Cloudflare API manually with token to verify it works
4. Check if Let's Encrypt ACME servers are having issues: `dig +short acme-v02.api.letsencrypt.org`

---

## Related Documentation

- [LESSONS-LEARNED-CERT-MANAGER.md](/home/psimmons/LESSONS-LEARNED-CERT-MANAGER.md)
- [CLAUDE.md - Certificate Management Section](/home/psimmons/CLAUDE.md)
- Warning Pattern: cloudflare-dns-challenge-propagation-failure
- Incident: cert-manager-cloudflare-dns-challenge-failure
