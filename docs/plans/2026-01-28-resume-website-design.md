# Resume Website — Infrastructure Design

**Date:** 2026-01-28
**Domain:** resume.petersimmons.com
**Repo:** petersimmons1972/resume-website (private)
**Namespace:** websites (new)

---

## Architecture & Traffic Flow

```
User Browser
    │
    ▼
Cloudflare Edge (resume.petersimmons.com)
    │  ← "Always Use HTTPS" enforced here
    ▼
Cloudflare Tunnel (cloudflared daemon in homelab)
    │
    ▼
Traefik LoadBalancer (192.168.0.180)
    │  ← Ingress routes Host: resume.petersimmons.com → pod
    │  ← Wildcard TLS cert terminates here
    ▼
Nginx Pod (websites namespace)
    │  ← Serves index.html from ConfigMap volume
    ▼
resume.petersimmons.com (HTTPS, TLS-terminated)
```

**Key decisions:**
- TLS termination at Traefik using existing `petersimmons-com-wildcard-tls` secret — no new cert-manager Certificate needed
- HTTP→HTTPS redirect at two layers: Cloudflare "Always Use HTTPS" (external) + Traefik RedirectScheme middleware (internal)
- Single Chainguard Nginx container, fully stateless, no persistent storage
- HTML content in ConfigMap — updates via `kubectl apply`, no image rebuild

---

## K8s Manifests

### Namespace
```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: websites
```

### ConfigMap
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: resume-html
  namespace: websites
data:
  index.html: |
    <contents of peter-simmons-resume-presentation.html>
  nginx.conf: |
    server {
        listen 8080;
        server_name resume.petersimmons.com;
        root /usr/share/nginx/html;
        index index.html;
        location / {
            try_files $uri $uri/ /index.html;
        }
    }
```

### Deployment
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: resume-site
  namespace: websites
spec:
  replicas: 1
  selector:
    matchLabels:
      app: resume-site
  template:
    metadata:
      labels:
        app: resume-site
    spec:
      containers:
      - name: nginx
        image: cgr.dev/chainguard/nginx:latest
        ports:
        - containerPort: 8080
        resources:
          requests: { cpu: 50m, memory: 64Mi }
          limits:  { cpu: 200m, memory: 128Mi }
        securityContext:
          runAsNonRoot: true
          readOnlyRootFilesystem: true
          allowPrivilegeEscalation: false
          capabilities:
            drop: ["ALL"]
        volumeMounts:
        - name: html
          mountPath: /usr/share/nginx/html/index.html
          subPath: index.html
        - name: nginx-conf
          mountPath: /etc/nginx/conf.d/default.conf
          subPath: nginx.conf
        - name: cache
          mountPath: /var/cache/nginx
        - name: tmp
          mountPath: /tmp
      volumes:
      - name: html
        configMap: { name: resume-html, items: [{key: index.html, path: index.html}] }
      - name: nginx-conf
        configMap: { name: resume-html, items: [{key: nginx.conf, path: nginx.conf}] }
      - name: cache
        emptyDir: {}
      - name: tmp
        emptyDir: {}
```

### Service
```yaml
apiVersion: v1
kind: Service
metadata:
  name: resume-site
  namespace: websites
spec:
  selector:
    app: resume-site
  ports:
  - port: 8080
    targetPort: 8080
```

---

## Traefik Ingress + HTTP Redirect

### Middleware
```yaml
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: https-redirect
  namespace: websites
spec:
  redirectScheme:
    scheme: https
    permanent: true
```

### HTTP IngressRoute (port 80 → redirect)
```yaml
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata:
  name: resume-site-http
  namespace: websites
spec:
  entryPoints:
  - web
  routes:
  - match: Host(`resume.petersimmons.com`)
    kind: rule
    middlewares:
    - name: https-redirect
      namespace: websites
    services:
    - name: resume-site
      port: 8080
```

### HTTPS IngressRoute (port 443 → serve)
```yaml
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata:
  name: resume-site-https
  namespace: websites
spec:
  entryPoints:
  - websecure
  routes:
  - match: Host(`resume.petersimmons.com`)
    kind: rule
    services:
    - name: resume-site
      port: 8080
  tls:
    secretName: petersimmons-com-wildcard-tls
```

---

## Cloudflare Tunnel Routing

Add ingress rule to existing tunnel config (determine which tunnel during implementation):

```yaml
ingress:
- hostname: resume.petersimmons.com
  service: http://192.168.0.180
# ... existing rules ...
- service: bastion
```

Bind DNS record:
```bash
cloudflared tunnel route dns <tunnel-name> resume.petersimmons.com
```

---

## Verification

```bash
# TLS + content
curl -Iv https://resume.petersimmons.com
# Expect: HTTP/2 200, TLS cert *.petersimmons.com, resume HTML

# HTTP redirect
curl -Iv http://resume.petersimmons.com
# Expect: 301 → https://resume.petersimmons.com
```

---

## GitHub Issues

| # | Title |
|---|-------|
| 1 | Deploy static site to K8s (websites namespace) |
| 2 | Configure Traefik ingress + HTTP redirect |
| 3 | Wire Cloudflare tunnel routing |
| 4 | End-to-end verification |
| 5 | Post-launch: capture lessons learned |

---

## Learning Capture

After deployment, log to homelab knowledge base via `homelab:log-incident` skill. Track:
- What worked first try vs. required iteration
- Chainguard Nginx quirks (port, filesystem, user)
- Cloudflare tunnel config discovery process
- Any cert/TLS surprises
