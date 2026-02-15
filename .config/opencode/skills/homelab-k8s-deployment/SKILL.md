---
name: homelab-k8s-deployment
description: Use when deploying new services to K8s cluster - covers image building, registry push, manifests, Traefik routing, and health verification
---

# K8s Service Deployment

Complete workflow for deploying services to the homelab K8s cluster.

## When to Use

- Deploying new applications to K8s
- Creating deployments for containerized services
- Setting up Traefik ingress routing
- Any time you're pushing an image and creating K8s resources

## Quick Reference

| Phase | Key Commands | Verification |
|-------|-------------|--------------|
| Build | `docker build -t registry.petersimmons.com/app:tag .` | Image listed in `docker images` |
| Push | `docker push registry.petersimmons.com/app:tag` | Pull works from different node |
| Deploy | `kubectl apply -f manifests/` | Pods Running, service endpoints exist |
| Route | Create IngressRoute for Traefik | curl returns expected content |

## Complete Workflow

### Phase 1: Container Image

**1. Build with registry prefix:**
```bash
docker build -t registry.petersimmons.com/<app-name>:<tag> .
```

**2. Push to local registry:**
```bash
docker push registry.petersimmons.com/<app-name>:<tag>
```

**3. Verify pull from different node:**
```bash
ssh psimmons@192.168.0.132 "docker pull registry.petersimmons.com/<app-name>:<tag>"
```

### Phase 2: K8s Manifests

Create manifests in order:

**Namespace** (if new service area):
```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: <app-name>
```

**Deployment:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: <app-name>
  namespace: <namespace>
spec:
  replicas: 2
  selector:
    matchLabels:
      app: <app-name>
  template:
    metadata:
      labels:
        app: <app-name>
    spec:
      containers:
      - name: <app-name>
        image: registry.petersimmons.com/<app-name>:<tag>
        ports:
        - containerPort: 3000  # Adjust to your app
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /
            port: 3000
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /
            port: 3000
          initialDelaySeconds: 5
          periodSeconds: 5
```

**Service:**
```yaml
apiVersion: v1
kind: Service
metadata:
  name: <app-name>
  namespace: <namespace>
spec:
  selector:
    app: <app-name>
  ports:
  - protocol: TCP
    port: 80
    targetPort: 3000  # Container port
```

**IngressRoute** (Traefik):
```yaml
apiVersion: traefik.containo.us/v1alpha1
kind: IngressRoute
metadata:
  name: <app-name>
  namespace: <namespace>
spec:
  entryPoints:
    - websecure
  routes:
    - match: Host(`<app-name>.petersimmons.com`)
      kind: Rule
      services:
        - name: <app-name>
          port: 80
  tls:
    certResolver: letsencrypt-prod
```

### Phase 3: Apply and Verify

**1. Apply manifests:**
```bash
kubectl apply -f manifests/namespace.yaml
kubectl apply -f manifests/deployment.yaml
kubectl apply -f manifests/service.yaml
kubectl apply -f manifests/ingressroute.yaml
```

**2. Wait for pods:**
```bash
kubectl wait --for=condition=ready pod -l app=<app-name> -n <namespace> --timeout=120s
```

**3. Check rollout status:**
```bash
kubectl rollout status deployment/<app-name> -n <namespace>
```

**4. Verify service endpoints:**
```bash
kubectl get endpoints <app-name> -n <namespace>
```

**5. Test internal access:**
```bash
kubectl run -it --rm debug --image=curlimages/curl --restart=Never -- \
  curl http://<app-name>.<namespace>.svc.cluster.local
```

**6. Verify external access:**
```bash
curl -I https://<app-name>.petersimmons.com
```

## Chainguard Image Considerations

If using Chainguard hardened images:

**Port Changes:**
- Non-root images use port 8080 instead of 80
- Update `containerPort` and `targetPort` accordingly

**Security Context:**
```yaml
securityContext:
  fsGroup: 65532  # For PVC write access
  runAsUser: 65532
  runAsNonRoot: true
```

## Traefik vs Standard Ingress

**Homelab uses Traefik IngressRoute CRDs**, not standard Kubernetes Ingress.

Standard Ingress will NOT route traffic. Use IngressRoute as shown above.

**To update existing route:**
```bash
kubectl patch ingressroute <route-name> -n <namespace> --type='json' \
  -p='[{"op": "replace", "path": "/spec/routes/0/services/0/name", "value":"<new-service>"}]'
```

## Common Issues

**ImagePullBackOff:**
- Check image exists: `curl https://registry.petersimmons.com/v2/<app>/tags/list`
- Verify registry config on nodes: `ssh psimmons@192.168.0.131 "cat /etc/rancher/k3s/registries.yaml"`

**CrashLoopBackOff:**
- Check logs: `kubectl logs -n <namespace> -l app=<app-name> --tail=50`
- Verify liveness probe path/port correct
- Check resource limits aren't too low

**Service not accessible externally:**
- Verify IngressRoute (not Ingress) created
- Check certificate: `kubectl get certificate -n <namespace>`
- Test internal first, then external

**Pods not ready:**
- Check readiness probe path/port
- Verify application actually listening on configured port
- Check for startup delays - adjust `initialDelaySeconds`

## DNS Configuration

For external access, add DNS record:

**Internal DNS (Unifi):**
```bash
# Credentials in ~/.claude/.unifi-credentials
# Add A record pointing to K8s ingress IP (192.168.0.180)
```

**Public DNS (Cloudflare):**
```bash
# Credentials in ~/projects/kubernetes/cert-manager/.env
# Add A record or CNAME
```

See `docs/DNS-API-REFERENCE.md` for full API examples.

## Verification Checklist

Before marking deployment complete:

- [ ] Pods Running (2/2 ready)
- [ ] Service has endpoints
- [ ] IngressRoute exists (not standard Ingress)
- [ ] Certificate issued (Ready: True)
- [ ] Internal curl returns expected content
- [ ] External curl returns HTTP 200 + expected content
- [ ] DNS resolves correctly
- [ ] Health checks passing
- [ ] Logs show no errors

## Related Skills

- **homelab-production-readiness**: Run PRR checklist before deploying
- **homelab-troubleshoot-common-issues**: If deployment fails
- **superpowers-verification-before-completion**: Before claiming done
