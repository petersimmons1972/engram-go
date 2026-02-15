# Incident Report: Certificate Issuer Name Mismatch

**Date:** 2026-02-12
**Service:** cert-manager
**Tier:** 2 (Major - Multiple Certificates Affected)
**MTTR:** 17 minutes
**Status:** Resolved

---

## Summary

Three certificates were stuck in "Issuing" state with no progress due to ClusterIssuer name mismatch. Ingresses referenced `letsencrypt-production` which doesn't exist; cluster only has `letsencrypt-prod`. This prevented cert-manager from creating certificate orders for 30 days in one case.

---

## Timeline

| Time | Event |
|------|-------|
| 2026-01-13 | Registry certificate created, stuck with no order |
| 2026-02-08 | Clearwatch certificate created, challenge stuck in unknown state |
| 2026-02-12 05:30 | New warning patterns detected certificate issues |
| 2026-02-12 05:35 | Root cause identified: ClusterIssuer name mismatch |
| 2026-02-12 05:42 | Updated registry ingress, order created in 5 seconds |
| 2026-02-12 05:43 | Updated homepage ingress (discovered secret collision) |
| 2026-02-12 05:45 | Deleted stuck clearwatch challenge |
| 2026-02-12 05:47 | Registry certificate issued successfully |

---

## Symptoms

- 3 certificates showing `Ready=False` with message "Issuing certificate"
- `container-registry/registry-petersimmons-tls` stuck for 30 days with no order created
- `default/petersimmons-tls` showing "IncorrectIssuer" message
- Clearwatch certificate challenge in "unknown" state
- Health check warnings: "Certificates stuck for >7 days"
- No error logs in cert-manager (silent failure)

---

## Root Cause

**Category:** Configuration

**Technical Details:**

Ingress annotations referenced ClusterIssuer `letsencrypt-production` which doesn't exist in the cluster. Only `letsencrypt-prod` is configured.

```yaml
# Wrong (ingress annotation):
cert-manager.io/cluster-issuer: letsencrypt-production

# Correct (actual ClusterIssuer):
metadata:
  name: letsencrypt-prod
```

This caused:
1. **Registry cert (30 days)**: No order/challenge created - cert-manager couldn't find issuer
2. **Homepage cert**: Certificate marked as "IncorrectIssuer", renewal would fail
3. **Clearwatch cert**: Challenge stuck in unknown state due to order issues

**Silent Failure:** Cert-manager produced no error logs, only detectable by checking certificate status directly.

---

## Resolution

### Pattern Analysis

First identified that all working certificates use `letsencrypt-prod`:

```bash
$ kubectl get certificate -A | grep Ready=True
clearwatch/clearwatch-tls: issuer=letsencrypt-prod
default/petersimmons-wildcard: issuer=letsencrypt-prod
linkerd-viz/linkerd-tls: issuer=letsencrypt-prod
```

### Commands Used

```bash
# Identified missing issuer
kubectl get clusterissuer  # Only letsencrypt-prod exists

# Fixed registry certificate
kubectl annotate ingress docker-registry-ingress \
  -n container-registry \
  cert-manager.io/cluster-issuer=letsencrypt-prod \
  --overwrite

# Verified order created
kubectl get order -n container-registry
# registry-petersimmons-tls-1-235197378   valid   5s

# Fixed homepage certificate
kubectl annotate ingress homepage \
  -n default \
  cert-manager.io/cluster-issuer=letsencrypt-prod \
  --overwrite

# Cleaned up stuck clearwatch challenge
kubectl delete challenge clearwatchresearch-wildcard-1-4265635565-1056644059 \
  -n clearwatch
```

### Success Metrics

- Registry certificate: Order created in **5 seconds** (was stuck 30 days)
- Certificate issued successfully: `Ready=True`
- Secret created: `registry-petersimmons-tls`
- All commands: 100% success rate

---

## Prevention Measures

**Implemented:**
- ✅ Added `cert-stuck-no-events` warning pattern (>24h with no events)
- ✅ Added `cert-challenge-unknown-state` warning pattern
- ✅ Documented ClusterIssuer naming requirement
- ✅ Updated FAILURE-MODES-CATALOG.md with certificate failure modes

**Recommended:**
- Validate ClusterIssuer exists before creating ingress
- Add monitoring for certificates stuck >24h with no events
- Document standard ClusterIssuer names in cluster

---

## Certificates Affected

1. **container-registry/registry-petersimmons-tls**
   - Stuck: 30 days
   - Impact: New registry deployments might have cert issues
   - Resolution: Fixed in 17 minutes

2. **default/petersimmons-tls**
   - Message: "IncorrectIssuer"
   - Impact: Renewal would fail (cert valid until April 2026)
   - Additional issue: Secret name collision with petersimmons-wildcard
   - Resolution: Issuer reference fixed

3. **clearwatch/clearwatchresearch-wildcard**
   - Challenge stuck in "unknown" state for 4 days
   - Order pending, 1 of 3 challenges not completing
   - Resolution: Challenge deleted, certificate later removed (orphaned)

---

## Learning

### Warning Signs Identified

1. **Certificate stuck issuing with no events >24h**
   - Silent cert-manager failure
   - No automatic alerts

2. **Challenge in unknown state >10 minutes**
   - DNS or ACME issue
   - Requires investigation

### Pattern Tags
- cert-manager
- certificates
- clusterissuer
- ingress-annotations
- configuration
- silent-failure

### Automation Candidate

**Priority:** High

**Rationale:**
- Silent failure mode (no error logs)
- Detectable: certificate stuck >24h with no events
- High impact: services without valid certificates
- 30 day undetected failure in one case

**Proposed Automation:**
- Daily check for certificates with `Ready=False` >24h
- Alert if no events in certificate for >24h
- Validate ClusterIssuer exists at ingress creation time

---

## Related Documentation

- Warning patterns: `warning-patterns.yaml`
  - `cert-stuck-no-events`
  - `cert-challenge-unknown-state`
- Catalog entry: `FAILURE-MODES-CATALOG.md` (Tier 4)
- ClusterIssuer: `letsencrypt-prod` (not letsencrypt-production!)

---

## Key Lesson

**Silent failures are the hardest to detect.** Cert-manager couldn't find the issuer but produced no error logs. Only detectable by directly checking certificate status. This incident was caught by new automated warning patterns after 30 days.

**Pattern for future:** Any resource referencing another resource by name should validate the reference exists.

---

**Session ID:** 2026-02-12-warning-pattern-gaps
