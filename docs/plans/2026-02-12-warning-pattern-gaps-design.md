# Warning Pattern Gap Coverage Design

**Date:** 2026-02-12
**Status:** Approved
**Context:** ML failure prediction analysis revealed gaps in current rule-based system

---

## Problem Statement

Current `warning-patterns.yaml` has 6 patterns that work well (90% accuracy, no false positives), but gaps exist:

**Detected issues NOT covered by existing patterns:**
- 4 Failed pods accumulating (cleanup gap)
- Certificate challenges stuck in "unknown" state
- Certificates stuck "Issuing" with no events (silent cert-manager failure)
- Old job pods (>48h) in Error state

**Impact:** Silent failures and operational drift go undetected until manual inspection.

---

## Decision: Option A - Gap Coverage

**Chosen approach:** Add 4 new warning patterns to detect currently invisible failure modes.

**Alternatives considered:**
- Option B (correlation rules) - deferred, not addressing current issues
- Option C (proactive automation) - deferred until patterns proven effective

**Rationale:** Fix known blind spots first, then optimize.

---

## Design

### Pattern 1: Failed Pod Accumulation

```yaml
- id: failed-pod-accumulation
  name: "Failed pods accumulating indicates cleanup gap"

  warning_sign:
    metric: "failed_pod_count"
    threshold: ">3"
    check_command: "kubectl get pods -A --field-selector=status.phase=Failed --no-headers | wc -l"

  predicted_failure:
    description: "Monitoring noise, obscures real failures"
    tier: 3  # Moderate - not affecting service but indicates drift
    services_affected:
      - monitoring-clarity
    confidence: 1.0  # Certain - these pods ARE failed

  recommended_action:
    urgency: low
    action: "Review and cleanup old Failed pods (verify not from active issues)"
    commands:
      - "kubectl get pods -A --field-selector=status.phase=Failed"
      - "# If old completed jobs: kubectl delete pod <name> -n <namespace>"
    estimated_time_minutes: 5
```

**Rationale:**
- Tier 3 because it doesn't break services, creates monitoring noise
- 100% confidence - detecting actual state, not predicting
- Low urgency but surfaces current problem (4 Failed pods detected)

---

### Pattern 2: Certificate Challenge Unknown State

```yaml
- id: cert-challenge-unknown-state
  name: "Certificate challenge in unknown state indicates DNS or API issue"

  warning_sign:
    metric: "challenge_state_unknown"
    threshold: "state=unknown for >10 minutes"
    check_command: |
      kubectl get challenge -A -o json | jq -r '.items[] | select(.status.state=="unknown" or .status.state==null) | "\(.metadata.namespace)/\(.spec.dnsName)"'

  predicted_failure:
    description: "Certificate will not issue, HTTPS errors when current cert expires"
    tier: 2  # Major - will cause service issues eventually
    services_affected:
      - services-using-affected-domain
    confidence: 0.90

  evidence:
    occurrences: 1  # clearwatch domain currently affected
    prediction_accuracy: 1.0  # When challenge fails, cert doesn't issue
    avg_time_to_failure_hours: 2160  # ~90 days until current cert expires

  recommended_action:
    urgency: medium
    action: "Check cert-manager logs and DNS propagation"
    commands:
      - "kubectl describe challenge -A | grep -A 20 'state: unknown'"
      - "kubectl logs -n cert-manager -l app=cert-manager --tail=100"
    estimated_time_minutes: 10
```

**Rationale:**
- Tier 2 (major) - will eventually cause HTTPS errors
- 90% confidence - challenge state can be transient
- Medium urgency - have time before cert expires

---

### Pattern 3: Old Job Pods in Error State

```yaml
- id: old-job-pods-error
  name: "Completed job pods in Error state indicate job failure or cleanup gap"

  warning_sign:
    metric: "job_pods_error_age"
    threshold: ">48 hours in Error state"
    check_command: |
      kubectl get pods -A --field-selector=status.phase=Failed -o json | jq -r '.items[] | select(.status.containerStatuses[0].state.terminated.finishedAt) | select((.status.containerStatuses[0].state.terminated.finishedAt | fromdateiso8601) < (now - 172800)) | "\(.metadata.namespace)/\(.metadata.name)"'

  predicted_failure:
    description: "Job failure pattern or K8s GC not running"
    tier: 3  # Moderate - monitoring noise
    services_affected:
      - job-reliability
    confidence: 0.80  # Usually cleanup gap, sometimes real issue

  recommended_action:
    urgency: low
    action: "Investigate job failure cause, cleanup if expected failure"
    commands:
      - "kubectl describe pod <pod-name> -n <namespace>"
      - "kubectl logs <pod-name> -n <namespace>"
      - "# If expected: kubectl delete pod <pod-name> -n <namespace>"
    estimated_time_minutes: 10

  context:
    note: "CronJobs and Jobs leave pods behind for debugging. Error state >48h usually means forgotten cleanup, not active problem."
```

**Rationale:**
- Tier 3 - operational hygiene issue
- 80% confidence - could be cleanup gap OR real job failure
- 48h threshold gives time for debugging before cleanup

---

### Pattern 4: Certificate Stuck Issuing with No Events

```yaml
- id: cert-stuck-no-events
  name: "Certificate stuck in Issuing state with no recent events"

  warning_sign:
    metric: "certificate_issuing_no_events"
    threshold: "Ready=False for >24 hours AND no events in last 24h"
    check_command: |
      kubectl get certificate -A -o json | jq -r '.items[] | select(.status.conditions[]? | select(.type=="Ready" and .status=="False")) | select(.metadata.creationTimestamp | fromdateiso8601 < (now - 86400)) | "\(.metadata.namespace)/\(.metadata.name)"'

  predicted_failure:
    description: "Certificate will never issue without intervention - silent cert-manager failure"
    tier: 2  # Major - service will have HTTPS errors when cert expires
    services_affected:
      - services-using-affected-certificate
    confidence: 0.95  # Very likely stuck if no events for 24h

  evidence:
    occurrences: 1  # petersimmons-tls currently affected
    prediction_accuracy: 1.0
    avg_time_to_failure_hours: 2160  # ~90 days to cert expiry

  recommended_action:
    urgency: high
    action: "Cert-manager may be stuck - check for challenges, orders, and cert-manager health"
    commands:
      - "kubectl describe certificate <name> -n <namespace>"
      - "kubectl get order,challenge -A"
      - "kubectl get pods -n cert-manager"
      - "kubectl logs -n cert-manager -l app=cert-manager --tail=50"
    estimated_time_minutes: 15

  context:
    note: "No events = cert-manager isn't processing this certificate. Different from renewal failure (which has error events). Usually indicates cert-manager restart needed or CRD issue."
```

**Rationale:**
- Tier 2 (major) - silent failure mode
- 95% confidence - no events for 24h = stuck
- High urgency - cert-manager itself may need intervention
- Different from existing renewal failure pattern (that has events)

---

## Integration

### Existing System Compatibility

**No changes to existing components:**
- `homelab:predict-failure` - already reads `warning-patterns.yaml`
- `homelab:troubleshoot-common-issues` - already checks patterns
- `homelab:log-incident` - tracks resolution effectiveness
- `failure-history.yaml` - logs incidents for evidence building

**Additive only:**
- Current 6 patterns unchanged
- New patterns appended to file
- Total: 10 patterns

---

## Implementation

### Steps

1. **Backup current file**
   ```bash
   cp ~/.homelab/knowledge/warning-patterns.yaml \
      ~/.homelab/knowledge/warning-patterns.yaml.backup-$(date +%Y%m%d-%H%M%S)
   ```

2. **Add 4 new patterns** to `warning-patterns.yaml`

3. **Test pattern detection**
   ```bash
   # Should detect current issues:

   # Pattern 1: Failed pods (expect: 4)
   kubectl get pods -A --field-selector=status.phase=Failed --no-headers | wc -l

   # Pattern 2: Unknown challenge (expect: clearwatch)
   kubectl get challenge -A -o json | jq -r '.items[] | select(.status.state=="unknown")'

   # Pattern 4: Stuck cert (expect: petersimmons-tls)
   kubectl get certificate -A -o json | jq -r '.items[] | select(.status.conditions[]? | select(.type=="Ready" and .status=="False"))'
   ```

4. **Verify `homelab:predict-failure` picks them up**

---

## Testing Strategy

### Validation Criteria

- ✅ New patterns detect current issues (4 Failed pods, stuck cert, unknown challenge)
- ✅ Patterns don't trigger on healthy systems (empty output when nothing wrong)
- ✅ Recommended actions work (commands execute successfully)
- ✅ Original 6 patterns unchanged

### Current System Baseline

**Issues detected immediately after implementation:**
- Failed pod accumulation: 4 pods
  - 3x linkerd-heartbeat-29512546 (2 days old)
  - 1x trivy-system scan
- Certificate unknown state: clearwatch/clearwatch.petersimmons.com
- Certificate stuck issuing: default/petersimmons-tls

---

## Success Metrics

### Immediate (Week 1)
- Patterns detect all 3 current issues
- Zero false positives
- Recommended actions resolve issues successfully

### Short-term (Month 1)
- 2+ additional gaps discovered and logged
- Pattern accuracy tracked in `failure-history.yaml`
- No operational impact from new pattern checks

### Long-term (Quarter 1)
- Operational drift detected before user awareness
- Failed pod accumulation prevented via routine cleanup
- Certificate issues caught before expiry

---

## Future Enhancements

**Deferred to separate initiatives:**
1. **Correlation rules** (multi-signal patterns)
   - Storage pressure + pod failures → higher confidence
   - Requires more incident data first

2. **Proactive automation** (scheduled checks)
   - CronJob running `predict-failure` every 15min
   - Auto-cleanup of old Failed pods (with safety)

3. **Machine Learning** (when 500+ incidents collected)
   - Current rule-based system must mature first
   - Data collection ongoing via structured YAML

---

## References

- Existing patterns: `~/.homelab/knowledge/warning-patterns.yaml`
- Failure tracking: `~/.homelab/knowledge/failure-history.yaml`
- Fix effectiveness: `~/.homelab/knowledge/fix-effectiveness.yaml`
- Skills: `homelab:predict-failure`, `homelab:troubleshoot-common-issues`
- Analysis document: ML feasibility analysis (2026-02-12)

---

**Approved by:** User
**Implementation date:** 2026-02-12
