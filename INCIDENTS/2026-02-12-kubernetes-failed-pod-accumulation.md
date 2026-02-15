# Incident Report: Failed Pod Accumulation

**Date:** 2026-02-12
**Service:** Kubernetes
**Tier:** 3 (Moderate - Monitoring Noise)
**MTTR:** 5 minutes
**Status:** Resolved

---

## Summary

4 Failed pods accumulated in the cluster over 38 hours, creating monitoring noise and obscuring real failures. All pods were completed job pods (linkerd-heartbeat and trivy vulnerability scans) that Kubernetes did not automatically garbage collect.

---

## Timeline

| Time | Event |
|------|-------|
| 2026-02-10 19:46 | 3x linkerd-heartbeat jobs failed due to DNS resolution errors |
| 2026-02-12 06:06 | 1x trivy scan completed with partial container failures |
| 2026-02-12 05:00 | New warning pattern detected 4 Failed pods (threshold >3) |
| 2026-02-12 05:15 | Investigated pod details - identified completed job pods |
| 2026-02-12 05:25 | Deleted all 4 Failed pods, cleanup verified successful |

---

## Symptoms

- 4 pods showing `status.phase=Failed` in cluster
- Health check warnings about pod issues
- 3x `linkerd-heartbeat-29512546-*` pods in Error state
- 1x `scan-vulnerabilityreport-7bccd4b646-dg2sc` showing Completed but phase=Failed

---

## Root Cause

**Category:** Operational Hygiene

Kubernetes Job pods remain in Failed/Completed state by default for debugging purposes. No automatic cleanup mechanism configured.

**Technical Details:**
- Linkerd heartbeat jobs failed due to DNS resolution error: `lookup versioncheck.linkerd.io on 10.43.0.10:53: server misbehaving`
- Trivy scan pod had 3/4 init containers fail (exitCode 1) but main scan completed successfully
- Pods accumulate over time without manual or automated cleanup

---

## Resolution

### Commands Used

```bash
# Identified Failed pods
kubectl get pods -A --field-selector=status.phase=Failed -o wide

# Investigated root cause
kubectl logs linkerd-heartbeat-29512546-lbrhm -n linkerd

# Cleaned up linkerd pods
kubectl delete pod linkerd-heartbeat-29512546-lbrhm linkerd-heartbeat-29512546-nvplq linkerd-heartbeat-29512546-vf27s -n linkerd

# Cleaned up trivy pod
kubectl delete pod scan-vulnerabilityreport-7bccd4b646-dg2sc -n trivy-system
```

### Success Rate
- All cleanup commands: 100% success
- Diagnostic commands: 100% success

---

## Prevention Measures

**Implemented:**
- ✅ Added `failed-pod-accumulation` warning pattern (threshold: >3 pods)
- ✅ Documented cleanup procedure in FAILURE-MODES-CATALOG.md

**Recommended (Not Yet Implemented):**
- Configure `ttlSecondsAfterFinished` on Job resources
- Add automated cleanup script for Failed pods >48h old
- Investigate linkerd DNS issues (versioncheck.linkerd.io resolution)

---

## Learning

### Warning Signs
- `failed_pod_count > 3` indicates cleanup gap
- Pods in Failed state >48h suggest garbage collection issues

### Pattern Tags
- kubernetes
- job-pods
- garbage-collection
- linkerd
- trivy
- operational-hygiene

### Automation Candidate
**Priority:** Medium

**Rationale:** Cleanup could be automated safely for Failed pods >48h old with completed jobs. Consider scheduled CronJob or operator pattern.

---

## Related Documentation

- Knowledge file: `K8S-POD-GARBAGE-COLLECTION-LESSONS.md`
- Catalog entry: `FAILURE-MODES-CATALOG.md` (Tier 4)
- Warning pattern: `warning-patterns.yaml` (failed-pod-accumulation)

---

**Session ID:** 2026-02-12-warning-pattern-gaps
