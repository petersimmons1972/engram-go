---
Category: reference
---
# Kubernetes Deployment Patterns (Homelab)

- **RWO PVCs** → `strategy.type: Recreate` (RollingUpdate causes Multi-Attach errors)
- **Chainguard images** → `fsGroup: 65532` for PVC write access; TCP socket probes (no shell); nginx needs emptyDir for `/var/lib/nginx/tmp`, `/run`, `/tmp`
- **Periodic tasks** → CronJob, NOT Deployment+sleep. Exit 137 with low memory = liveness probe kill, not OOM
- **WordPress behind proxy** → `WP_HOME`, `WP_SITEURL`, `FORCE_SSL_ADMIN` in wp-config.php + `$_SERVER['HTTPS']='on'` from `HTTP_X_FORWARDED_PROTO`
- **Chainguard WordPress** → PHP-FPM only, needs multi-container pod (nginx + php-fpm + init), shared emptyDir, both run as UID 65532
- **Cloudflare Tunnel** → Decode base64 token for credentials JSON; use `hostAliases` for origin resolution inside pod
- **Homepage failures** → Almost always network policy label mismatch (`app=homepage` vs `app.kubernetes.io/name=homepage`). Check: `kubectl describe networkpolicy -n default | grep Selector`

## Security Hardening (2026-03-04)

- **Traefik BasicAuth**: Secret `kube-system/admin-basicauth`, Middleware `kube-system/admin-auth`. Applied to prometheus, alertmanager, longhorn, linkerd-viz, docker-registry. Creds in Infisical `/admin/basicauth`
- **etcd encryption**: `/etc/rancher/k3s/encryption.yaml` (AES-CBC), audit log at `/var/log/k3s-audit.log` (30d retention). Rolling restart of 3 control plane nodes
- **Infisical ExternalSecret**: Use `creationPolicy: Orphan` when replacing existing secrets. Never generate htpasswd with shell-special chars. Traefik needs BCrypt (`$2b$`) or APR-MD5
- **Reflector**: Won't overwrite wrong-type secrets (Opaque vs tls). Delete old secret first, manually seed with correct type + annotation
