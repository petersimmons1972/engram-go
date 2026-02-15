# Deploy Next.js Multi-Variant Applications to Kubernetes

**Purpose:** Deploy multiple Next.js app variants (same codebase, different styling/content) with shared infrastructure pattern

**When to use:** Marketing sites, A/B testing variants, customer-specific deployments

---

## Prerequisites

- [ ] Docker registry accessible (registry.petersimmons.com)
- [ ] Kubernetes cluster with Traefik ingress
- [ ] Domain DNS configured (*.clearwatchresearch.com)
- [ ] Source apps in monorepo structure (`apps/{variant}/`)

---

## Critical Configuration Rules

### 1. imagePullPolicy: Always for :latest tags

**ALWAYS use `imagePullPolicy: Always` when using `:latest` tags**

```yaml
spec:
  template:
    spec:
      containers:
      - name: nextjs
        image: registry.petersimmons.com/app-variant:latest
        imagePullPolicy: Always  # CRITICAL - prevents stale cache
```

**Why:** Kubernetes nodes cache images. Without `Always`, pods pull cached images even when registry has newer builds.

**Failure mode:** Rebuilt app with new features → deployed → pods use old cached image → features don't appear.

### 2. Port Consistency

**Ensure all port references match the actual app port (usually 3000 for Next.js)**

```yaml
spec:
  template:
    spec:
      containers:
      - ports:
        - containerPort: 3000  # Must match Next.js listen port
        livenessProbe:
          httpGet:
            port: 3000           # Must match containerPort
        readinessProbe:
          httpGet:
            port: 3000           # Must match containerPort
---
apiVersion: v1
kind: Service
spec:
  ports:
  - port: 80
    targetPort: 3000             # Must match containerPort
```

**Failure mode:** Probes check port 3004, app listens on 3000 → probes fail → pods CrashLoopBackOff → no traffic routed.

### 3. Traefik IngressRoute API Version

**Use `traefik.io/v1alpha1` NOT `traefik.containo.us/v1alpha1`**

```yaml
apiVersion: traefik.io/v1alpha1  # CORRECT
kind: IngressRoute
```

**Verify available API version:**
```bash
kubectl api-resources | grep ingressroute
```

**Failure mode:** Wrong API version → `kubectl apply` fails → "no matches for kind IngressRoute" error.

### 4. Check for Conflicting Routes

**Before creating new IngressRoute, check for old routes with same hostname**

```bash
kubectl get ingressroute -n clearwatch | grep {variant}
```

**Failure mode:** Two IngressRoutes with same Host() → Traefik routes to old service → site serves stale content or 404.

---

## Deployment Template

### File: variant-deployment-template.yaml

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app-VARIANT
  namespace: default
  labels:
    app: app-VARIANT
    version: VARIANT
spec:
  replicas: 1
  selector:
    matchLabels:
      app: app-VARIANT
  template:
    metadata:
      labels:
        app: app-VARIANT
        version: VARIANT
    spec:
      containers:
      - name: nextjs
        image: registry.petersimmons.com/app-VARIANT:latest
        imagePullPolicy: Always  # CRITICAL
        ports:
        - containerPort: 3000
          name: http
        env:
        - name: NODE_ENV
          value: production
        resources:
          requests:
            cpu: 100m
            memory: 256Mi
          limits:
            cpu: 500m
            memory: 512Mi
        livenessProbe:
          httpGet:
            path: /
            port: 3000
          initialDelaySeconds: 30
          periodSeconds: 10
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /
            port: 3000
          initialDelaySeconds: 5
          periodSeconds: 5
          failureThreshold: 3
---
apiVersion: v1
kind: Service
metadata:
  name: app-VARIANT-svc
  namespace: default
spec:
  selector:
    app: app-VARIANT
  ports:
  - port: 80
    targetPort: 3000
    protocol: TCP
    name: http
  type: ClusterIP
---
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata:
  name: app-VARIANT
  namespace: default
spec:
  entryPoints:
    - websecure
  routes:
  - match: Host(`VARIANT.example.com`)
    kind: Rule
    services:
    - name: app-VARIANT-svc
      port: 80
  tls:
    certResolver: cloudflare
```

---

## Deployment Script

### File: deploy-variant.sh

```bash
#!/bin/bash
set -e

VARIANT=$1
REGISTRY="registry.petersimmons.com"
APP_NAME="myapp"
NAMESPACE="default"
DOMAIN="example.com"

if [ -z "$VARIANT" ]; then
  echo "Usage: $0 <variant-name>"
  exit 1
fi

echo ">>> Deploying $VARIANT variant..."

# 1. Build Next.js app
echo "[1/7] Building Next.js..."
cd ~/projects/myapp/apps/$VARIANT
npm run build

# 2. Build Docker image with --no-cache
echo "[2/7] Building Docker image..."
docker build --no-cache -t $REGISTRY/$APP_NAME-$VARIANT:latest .

# 3. Push to registry
echo "[3/7] Pushing to registry..."
docker push $REGISTRY/$APP_NAME-$VARIANT:latest

# 4. Check for conflicting routes
echo "[4/7] Checking for conflicting routes..."
OLD_ROUTES=$(kubectl get ingressroute -n $NAMESPACE | grep "$VARIANT" | grep -v "$APP_NAME-$VARIANT" || true)
if [ -n "$OLD_ROUTES" ]; then
  echo "  ⚠ WARNING: Found existing routes for $VARIANT"
  echo "$OLD_ROUTES"
  read -p "  Delete old routes? (y/n) " -n 1 -r
  echo
  if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "$OLD_ROUTES" | awk '{print $1}' | xargs -I {} kubectl delete ingressroute {} -n $NAMESPACE
  fi
fi

# 5. Generate and apply manifests
echo "[5/7] Applying Kubernetes manifests..."
sed "s/VARIANT/$VARIANT/g; s/example.com/$DOMAIN/g; s/app-/$APP_NAME-/g; s/default/$NAMESPACE/g" \
  variant-deployment-template.yaml | kubectl apply -f -

# 6. Wait for rollout
echo "[6/7] Waiting for rollout..."
kubectl rollout status deployment/$APP_NAME-$VARIANT -n $NAMESPACE --timeout=120s

# 7. Verify deployment
echo "[7/7] Verifying..."
ENDPOINT="https://$VARIANT.$DOMAIN"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$ENDPOINT")
if [ "$HTTP_CODE" = "200" ]; then
  echo "✓ $VARIANT deployed successfully - $ENDPOINT"
else
  echo "✗ Deployment verification failed - HTTP $HTTP_CODE"
  exit 1
fi
```

---

## Verification Checklist

After deployment, verify:

- [ ] **Pod Running:** `kubectl get pods -n {namespace} -l app={variant}`
- [ ] **Service Endpoints:** `kubectl describe svc {variant}-svc -n {namespace} | grep Endpoints`
- [ ] **HTTP 200:** `curl -I https://{variant}.{domain}`
- [ ] **Content Check:** `curl -s https://{variant}.{domain} | grep {expected-content}`
- [ ] **Button/Feature Present:** Check specific features deployed
- [ ] **E2E Test:** Run Playwright/Cypress tests if available
- [ ] **Logs Clean:** `kubectl logs -n {namespace} -l app={variant} --tail=50`

---

## Common Issues & Fixes

### Issue: "Deployment shows old content after rebuild"

**Symptoms:**
- Rebuilt app with new features
- Pushed new image to registry
- Restarted deployment
- Site still shows old content

**Root Cause:** imagePullPolicy: IfNotPresent + node image cache

**Fix:**
```bash
# 1. Update deployment to use imagePullPolicy: Always
kubectl get deployment {name} -n {namespace} -o yaml | \
  sed 's/imagePullPolicy: IfNotPresent/imagePullPolicy: Always/' | \
  kubectl apply -f -

# 2. Delete pods to force fresh pull
kubectl delete pods -n {namespace} -l app={name}

# 3. Verify new image is pulled
kubectl get pod -n {namespace} -l app={name} -o jsonpath='{.items[0].status.containerStatuses[0].imageID}'
```

### Issue: "Pods stuck in CrashLoopBackOff"

**Symptoms:**
- Pods restart repeatedly
- Logs show "Ready" but probes fail

**Root Cause:** Probe ports don't match container port

**Fix:**
```bash
# Check configured ports
kubectl get deployment {name} -n {namespace} -o yaml | grep -A 3 "livenessProbe:\|readinessProbe:\|containerPort:"

# Verify app is actually listening
kubectl exec -n {namespace} deployment/{name} -- netstat -ln | grep LISTEN

# Update probes to correct port
kubectl get deployment {name} -n {namespace} -o yaml | \
  sed 's/port: 3004/port: 3000/g' | \
  kubectl apply -f -
```

### Issue: "Site returns 502 Bad Gateway"

**Symptoms:**
- IngressRoute created
- Deployment running
- Site returns 502

**Possible Causes:**

1. **Service has no endpoints:**
   ```bash
   kubectl describe svc {name}-svc -n {namespace} | grep Endpoints
   # If empty, pods aren't matching service selector
   ```

2. **Service targetPort wrong:**
   ```bash
   kubectl get svc {name}-svc -n {namespace} -o yaml | grep targetPort
   # Must match containerPort (usually 3000)
   ```

3. **Readiness probe failing:**
   ```bash
   kubectl describe pod -n {namespace} -l app={name} | grep -A 5 "Readiness"
   ```

### Issue: "kubectl apply fails - no matches for kind IngressRoute"

**Root Cause:** Wrong Traefik API version

**Fix:**
```bash
# Check available API version
kubectl api-resources | grep ingressroute

# Update YAML to use correct version
# Change: traefik.containo.us/v1alpha1
# To:     traefik.io/v1alpha1
```

---

## Multi-Variant Bulk Deployment

When deploying many variants at once:

```bash
#!/bin/bash
VARIANTS="variant1 variant2 variant3 variant4"

for variant in $VARIANTS; do
  echo "=== Deploying $variant ==="
  ./deploy-variant.sh $variant || {
    echo "✗ $variant failed - continuing..."
    continue
  }
  echo "✓ $variant complete"
  echo ""
done

echo "=== Verification ==="
for variant in $VARIANTS; do
  echo -n "$variant: "
  curl -s -o /dev/null -w "HTTP %{http_code}" "https://${variant}.example.com"
  echo ""
done
```

---

## Related Documents

- **Failure Modes:** See FAILURE-MODES-CATALOG.md → "K8s ImagePull Caching"
- **Session Record:** SESSION-2026-02-14-14-VARIANT-DEPLOYMENT.md
- **Skills:** Use `homelab:troubleshoot-common-issues` for diagnosis
