# Lessons Learned: cert-manager & Cloudflare DNS-01 Integration

**Date**: 2026-01-25
**Duration**: 4+ months (Nov 2025 - Jan 2026)
**Impact**: All services using `petersimmons-com-wildcard-tls` certificate
**Severity**: High (cert expiry = HTTPS outage)

---

## Executive Summary

A certificate renewal failure went undetected for 4 months because:
1. Initial DNS issues (CNAME misconfiguration) were partially fixed
2. But a deeper issue remained: Cloudflare API token IP filtering was blocking cert-manager
3. The failure mode was silent: Challenge API calls succeeded, but DNS records weren't created
4. Health checks didn't validate certificate renewal status

**Root Cause**: cert-manager pod network IP (10.42.x.x) was outside Cloudflare token's IP whitelist (192.168.x.x)

---

## What Happened

### Timeline

| Date | Event |
|------|-------|
| Nov 22, 2025 | Original certificate issued, renewal scheduled for Jan 21 |
| ~Nov 27, 2025 | CNAME misconfiguration broke cluster DNS |
| Jan 21, 2026 | Renewal time reached - renewal fails (DNS issue) |
| Jan 24, 2026 | Fixed cert-manager pod with static DNS servers (1.1.1.1, 1.0.0.1) |
| Jan 25, 2026 | Fixed Ingress issuer annotation; renewal still blocked |
| Jan 25 14:14 | Challenge created but DNS records never appear |
| Jan 25 14:30 | Identified root cause: K8s pod IP (10.42.2.247) outside whitelist |

### Why It Wasn't Caught Earlier

1. **No certificate health checks**: health-check.sh only validated pod/node status, not TLS certificate renewal
2. **Certificate didn't expire yet**: Feb 20, 2026 expiry meant no immediate service impact
3. **Silent failure mode**: Challenge showed "Presented: true" (API reachable) but DNS record missing (API blocked by IP filter)
4. **No cert-manager logs**: IP filtering happens at Cloudflare API gateway, not in application logs
5. **DNS issue masked the real problem**: Initial DNS fix (static nameservers) seemed to resolve it, but only partially

---

## Technical Deep Dive

### The Silent Failure Mode

```
EXPECTED:
cert-manager → Cloudflare API (from 10.42.2.247) → Create TXT record → DNS propagates

ACTUAL:
cert-manager → Cloudflare API Gateway (IP check) → REJECTED (IP not in whitelist) → No TXT record created
              ↓
              Silent rejection at API gateway level
              ↓
              cert-manager receives "success" response (connection reached)
              ↓
              Challenge marked as "Presented: true"
              ↓
              ACME validation fails (no TXT record in DNS)
              ↓
              Certificate renewal fails
```

### Why This Is Insidious

1. **No error messages**: The API call reaches the gateway (HTTP connection succeeds), but request is rejected before processing
2. **Incomplete observability**: Challenge status shows "Presented: true" which looks successful
3. **DNS validates fine**: Domain resolves correctly, proving basic DNS works
4. **Other services unaffected**: Certificates issued before the IP filter was added still work
5. **Default failure detection misses it**: Standard monitoring checks if domain resolves, but not if ACME TXT records can be created

### Network Architecture Issue

```
homelab external network:    192.168.0.0/24 (nodes, VMs, physical machines)
Kubernetes overlay network:  10.42.0.0/16   (pods, services)

These are SEPARATE networks.
When you configure IP filtering for "homelab security", you're thinking of 192.168.x.x
But pod traffic originates from 10.42.x.x
This mismatch is not obvious and easy to miss.
```

---

## Root Causes (Layers)

### Layer 1: Immediate (Why the Challenge Failed)
- Cloudflare token IP filter: `192.168.x.x` (homelab external IPs only)
- cert-manager pod IP: `10.42.2.247` (Kubernetes overlay network)
- Mismatch resulted in silent rejection by Cloudflare API gateway

### Layer 2: Why It Went Undetected (4+ months)
- No health checks for certificate renewal status
- No monitoring of ACME challenge propagation
- No validation that TLS certificates are actually renewing

### Layer 3: Why IP Filtering Was Unnecessary
- Token already had zone-level scoping (only petersimmons.com)
- Token already had permission-level scoping (only DNS edit)
- IP filtering added no meaningful security, only complexity

---

## What We Learned

### 1. Kubernetes Network Topology Must Be Considered for API Tokens
**Lesson**: When configuring API tokens for services running in Kubernetes:
- Pods use overlay network (typically 10.x.x.x), NOT host network (192.168.x.x)
- IP filtering must account for pod network ranges
- Better approach: Use zone/permission scoping, skip IP filtering
- If IP filtering is required: Document that K8s network range must be included

### 2. Defense in Depth Can Create False Security
**Lesson**: IP filtering on top of zone + permission scoping provided:
- False sense of extra security
- Actual security: zone (only this domain) + permissions (only DNS)
- Added cost: broke legitimate use case (cert renewal)
- Result: worse security (undetected renewal failure for 4+ months)

### 3. Silent Failure Modes Are Dangerous
**Lesson**: Challenge showing "Presented: true" is NOT the same as "DNS record exists"
- HTTP request reaching API != request being accepted by IP filter
- Need to validate end-to-end: Challenge → DNS record → Actual DNS query
- Challenge status is misleading without DNS propagation check

### 4. Network Heterogeneity Requires Explicit Thinking
**Lesson**: When infrastructure spans multiple networks (homelab VMs + Kubernetes cluster):
- Don't assume a single IP range
- Explicitly document which services use which networks
- When configuring policies by IP, specify which network/component each range applies to
- Example: "API tokens must work from 10.42.0.0/16 (K8s), not just 192.168.0.0/24"

### 5. Health Checks Must Validate End-to-End Behavior
**Lesson**: Just checking "pod is running" or "API is reachable" is insufficient
- Check actual outcomes: Is DNS record created? Is cert renewing? Is renewal succeeding?
- Challenge "Presented: true" looks healthy but might indicate failure
- Need DNS propagation validation: `nslookup _acme-challenge.domain 1.1.1.1`

---

## How to Prevent This

### Immediate Fixes (Done)
1. ✅ Fixed Cloudflare token IP filtering or removed it
2. ✅ Updated health-check.sh to validate certificate renewal status
3. ✅ Added warning patterns for stuck certificates and failed DNS propagation
4. ✅ Documented the failure in failure-history.yaml with full context

### Ongoing Prevention
1. **Regular certificate audits**: `kubectl get certificate -A` should show all Ready=True
2. **Monitor challenge propagation**: Check for pending challenges that never complete
3. **Validate ACME end-to-end**:
   - Challenge created ✓
   - TXT record in DNS ✓
   - ACME validation succeeds ✓
   - Certificate issued ✓
4. **Health check automation**: `health-check.sh` now validates stuck certs and pending challenges
5. **Warning pattern monitoring**: Detect "Presented=true but no DNS record" pattern automatically

### Architecture Improvements
1. **Document network ranges explicitly**:
   - Homelab external: 192.168.0.0/24
   - Kubernetes overlay: 10.42.0.0/16
   - Add to CLAUDE.md Infrastructure section

2. **Simplify API token security**:
   - Rely on zone + permission scoping, not IP filtering
   - IP filtering only if you have multi-tenant scenarios that require it
   - If used, explicitly test from pod network range

3. **Add certificate validation to deployment checklist**:
   - Verify Ingress issuer annotation points to correct ClusterIssuer
   - Verify ClusterIssuer exists and is Ready
   - Verify DNS provider token has correct permissions
   - Verify DNS provider token IP filter includes pod network if applicable

---

## Key Takeaways

| Issue | Prevention |
|-------|-----------|
| cert-manager pod IP blocked by token IP filter | Remove IP filtering or add 10.42.0.0/16 to whitelist |
| Silent DNS propagation failure | Add `nslookup _acme-challenge.domain` check to health script |
| Stuck certificate undetected for 4 months | Monitor certificate Ready status continuously |
| Challenge shows success but DNS fails | Validate Challenge → DNS record → Query separately |
| Multiple layers of DNS/ACME confusion | Simplify: static nameservers (1.1.1.1) for cert-manager |
| Issuer misconfiguration (letsencrypt-production vs letsencrypt-prod) | Document correct issuer names in comments, validate in health checks |

---

## Files Updated

- ✅ `/home/psimmons/.homelab/knowledge/failure-history.yaml` - Full incident details with cert-manager pod IP
- ✅ `/home/psimmons/.homelab/knowledge/warning-patterns.yaml` - Added pattern for stuck certificates and DNS propagation failures
- ✅ `/home/psimmons/bin/health-check.sh` - Added certificate renewal status checks
- ✅ `/home/psimmons/CLAUDE.md` - Added Certificate Management section
- ✅ `/home/psimmons/LESSONS-LEARNED-CERT-MANAGER.md` - This document

---

## Related References

- **Incident**: cert-manager-dns-cname-renewal-failure, cert-manager-cloudflare-dns-challenge-failure
- **Warning Pattern**: cloudflare-dns-challenge-propagation-failure
- **Cloudflare Token**: "Kubernetes - cert-manager" with DNS=Edit permissions
- **Certificate**: petersimmons-com-wildcard-tls (expires Feb 20, 2026)

---

**Version**: 1.0
**Last Updated**: 2026-01-25
