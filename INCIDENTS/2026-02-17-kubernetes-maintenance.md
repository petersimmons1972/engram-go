# Kubernetes Cluster Maintenance — 2026-02-17

**Session type**: Proactive health check + cleanup  
**Duration**: ~60 minutes  
**Services affected**: cluster-wide, clearwatch, job-search, monitoring

---

## Summary

Routine health check revealed four distinct issues consuming cluster resources silently. No outages — all services remained accessible via stable old pods — but significant resource waste and etcd bloat were corrected.

---

## Issue 1: ReplicaSet Bloat (278 empty RSes)

**Root cause**: Default `revisionHistoryLimit=10` with high-frequency deployments in clearwatch namespace.

**Impact**: 278 empty (0/0/0) ReplicaSets across the cluster. clearwatch services had 10-12 dead RSes each. Slow kubectl operations, etcd memory pressure, obscured cluster state.

**Fix**:
```bash
# Delete all empty RSes
kubectl get replicasets -A --no-headers | awk '$3=="0" && $4=="0" && $5=="0" {print $1, $2}' \
  | while read ns rs; do kubectl delete rs -n "$ns" "$rs" --ignore-not-found; done

# Prevent recurrence: patch all 69 deployments
kubectl get deployments -A --no-headers | awk '{print $1, $2}' \
  | while read ns d; do kubectl patch deployment "$d" -n "$ns" --type=merge -p '{"spec":{"revisionHistoryLimit":3}}'; done
```

**Prevention**: Set `revisionHistoryLimit: 3` in all deployment templates at creation time.

---

## Issue 2: Stuck Rolling Updates — CrashLoopBackOff (clearwatch)

**Affected**: `uss-constitution` (369 restarts), `uss-enterprise` (547 restarts), `uss-tang` (369 restarts)

**Root cause**: New deployment versions failed to start (health check/crash on startup). Rolling update kept old pods alive but new pods consumed CPU cycling through crash/restart loops for days.

**Detection signal**: Two pods for same deployment from *different* ReplicaSet hashes.

**Fix**:
```bash
kubectl rollout undo -n clearwatch deployment/uss-constitution
kubectl rollout undo -n clearwatch deployment/uss-enterprise
kubectl rollout undo -n clearwatch deployment/uss-tang
```

**Result**: Crashing pods terminated immediately, stable old-version pods retained.

**TODO**: Investigate why new versions crash. Add `progressDeadlineSeconds` to deployments so Kubernetes auto-fails stuck rollouts.

---

## Issue 3: Missing Registry Images — ImagePullBackOff (job-search, monitoring)

**Affected**:
- `job-search/fastapi` — `registry.petersimmons.com/job-search-api:latest` (missing, 6 days)
- `job-search/gmail-tracker` — `registry.petersimmons.com/gmail-job-tracker:latest` (missing, 6 days)
- `monitoring/proxmox-monitor` — `192.168.0.131:30500/proxmox-monitor:latest` (missing)

**Root cause**: Images referenced in deployments were never built+pushed to the registry (or were deleted). Pods held node resource reservations for days while unable to start.

**Fix**: Scaled deployments to 0 to stop resource waste. Permanent fix requires rebuilding and pushing images.

**Prevention**: 
- Validate image exists in registry before `kubectl apply`
- CI/CD pipeline must build+push before deploying
- Health check should flag `ImagePullBackOff` pods older than 1 hour

---

## Issue 4: No Autoscaling on searxng (20 replicas at 0% CPU)

**Root cause**: Fixed replica count of 20 with no HPA. At idle, running at 0% CPU / 24% memory = 18 unnecessary pods.

**Fix**: Created HPA:
```yaml
minReplicas: 2
maxReplicas: 20
metrics:
  - CPU target: 70%
  - Memory target: 80%
behavior:
  scaleDown: stabilizationWindowSeconds: 300, max 2 pods/min
  scaleUp: stabilizationWindowSeconds: 30, max 4 pods/min
```

**Result**: Will trim from 20 → ~2 replicas within ~10 minutes of low traffic.

---

## Lessons Learned

1. **Set `revisionHistoryLimit: 3` in every new deployment manifest** — never rely on the default of 10
2. **Two pods from different RSes for same deployment = stuck rollout** — check restart counts immediately
3. **`kubectl rollout undo` is the fastest fix for stuck rollouts** — don't delete pods, don't scale — just undo
4. **ImagePullBackOff >1 hour = image doesn't exist** — verify with registry API before spending time debugging
5. **Stateless multi-replica deployments should always have an HPA** — fixed counts waste resources at idle
6. **`revisionHistoryLimit` should be in deployment templates at creation time** — retrofitting is tedious

---

## Files Updated

- `~/.homelab/knowledge/failure-history.yaml` — 3 new incidents
- `~/.homelab/knowledge/warning-patterns.yaml` — 3 new patterns
- `~/.homelab/knowledge/fix-effectiveness.yaml` — 6 new commands
- `~/FAILURE-MODES-CATALOG.md` — 4 new Tier 2/3 entries
- `~/KNOWLEDGE-INDEX.md` — updated
