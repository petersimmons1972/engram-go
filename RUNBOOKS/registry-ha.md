# Runbook: Registry HA Apply (Issue #70)

Applies the HA hardening changes to the docker-registry deployment in the
container-registry namespace. No downtime expected: RollingUpdate strategy
with maxUnavailable=0 keeps the old pod live until the new one is ready.

**Files changed:**
- `container-registry/deployment.yaml` — RollingUpdate, nodeAffinity, 4Gi memory
- `container-registry/pdb.yaml` — PodDisruptionBudget minAvailable: 1

---

## Pre-flight

```bash
# Confirm current state
kubectl get deployment docker-registry -n container-registry
kubectl get pdb -n container-registry
kubectl top pod -n container-registry

# Note current memory usage (expect ~1.2 Gi / ~60% of old 2Gi limit)
kubectl describe pod -n container-registry -l app=docker-registry | grep -A4 Limits
```

---

## Apply

```bash
# 1. Apply PDB first (non-disruptive, takes effect immediately)
kubectl apply -f container-registry/pdb.yaml

# 2. Apply deployment changes
kubectl apply -f container-registry/deployment.yaml

# 3. Watch rollout — expect one new pod to come up before old one terminates
kubectl rollout status deployment/docker-registry -n container-registry --timeout=120s
```

---

## Verify

```bash
# Confirm strategy is now RollingUpdate
kubectl get deployment docker-registry -n container-registry -o jsonpath='{.spec.strategy}' | jq .

# Confirm PDB is present and healthy
kubectl get pdb docker-registry -n container-registry

# Confirm memory limit is 4Gi
kubectl get deployment docker-registry -n container-registry   -o jsonpath='{.spec.template.spec.containers[0].resources}' | jq .

# Confirm node affinity is set
kubectl get deployment docker-registry -n container-registry   -o jsonpath='{.spec.template.spec.affinity}' | jq .

# Confirm pod landed on worker131 (preferred node)
kubectl get pod -n container-registry -l app=docker-registry -o wide

# Smoke test: registry API responds
kubectl run -it --rm smoke --image=curlimages/curl --restart=Never -n container-registry   -- curl -s http://docker-registry:5000/v2/ | jq .
```

---

## Rollback

If the rollout hangs or the pod fails readiness:

```bash
kubectl rollout undo deployment/docker-registry -n container-registry
kubectl rollout status deployment/docker-registry -n container-registry
```

The PDB does not affect rollback. Remove it only if you need to force-drain
and there is no healthy replica:

```bash
kubectl delete pdb docker-registry -n container-registry
```

---

## Notes

- The PVC (registry-data-pvc-nfs) is ReadWriteMany over NFS, so the surge pod
  during RollingUpdate can safely mount the same volume without conflict.
- nodeAffinity is preferred, not required. If worker131 is unavailable, the
  scheduler will place the pod on another node without operator intervention.
- Memory limit raised from 2Gi to 4Gi because observed idle usage (~1.2 Gi)
  left only ~40% headroom. Push operations spike higher. OOMKill risk was
  real on any bulk push or GC cycle.
