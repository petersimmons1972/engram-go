# Kubernetes Deployment Patterns (Homelab)

## RWO PVCs Need Recreate Strategy

RWO (ReadWriteOnce) PVCs cannot attach to multiple pods simultaneously. RollingUpdate tries to start the new pod before the old one terminates, causing Multi-Attach errors.

Fix: `spec.strategy.type: Recreate` for any Deployment with RWO PVCs.
Alternative: Use RWX (ReadWriteMany) volumes if storage class supports it.

Rule: When creating a Deployment with PVC, check access mode first. RWO = Recreate. RWX = RollingUpdate is safe.

Source: 2026-01-24 WordPress deployment on Longhorn.

## Chainguard Images + PVC Permissions

Chainguard images run as non-root (UID 65532 typically). Longhorn PVCs are created with root ownership.

Fix: Add `securityContext.fsGroup: <GID>` to pod spec. Check image docs for expected UID/GID.

Additional patterns:
- Use TCP socket probes instead of exec probes (no shell in distroless images)
- Chainguard nginx needs writable dirs via emptyDir: `/var/lib/nginx/tmp`, `/run`, `/var/cache/nginx`, `/tmp`

Source: 2026-01-23 MariaDB deployment, 2026-01-28 nginx deployment.

## CronJob, Not Deployment, for Periodic Tasks

A Deployment with `while true; do work; sleep 43200; done` pattern conflicts with liveness probes. The probe kills the pod during the sleep, causing 100+ restarts/day instead of 2 runs/day.

Fix: Use CronJob with `schedule: '0 */12 * * *'` instead of Deployment with sleep loop.

Diagnostic clue: Exit code 137 (SIGKILL) with low memory usage (<10% of limit) = liveness probe killing pod, not OOM.

Source: 2026-01-25 gmail-tracker restart loop (850 restarts in 8 days).

## WordPress Behind Reverse Proxy

WordPress serves HTTP asset URLs when behind TLS-terminating proxy, causing mixed content / unstyled pages.

Fix: Add to wp-config.php BEFORE "That's all, stop editing":
```php
define('WP_HOME', 'https://yourdomain.com');
define('WP_SITEURL', 'https://yourdomain.com');
define('FORCE_SSL_ADMIN', true);

if (isset($_SERVER['HTTP_X_FORWARDED_PROTO']) && $_SERVER['HTTP_X_FORWARDED_PROTO'] === 'https') {
    $_SERVER['HTTPS'] = 'on';
}
```

Applies to: WordPress behind Traefik, nginx, or any TLS-terminating proxy.

Source: 2026-01-24 WordPress CSS not loading.

## Chainguard WordPress Architecture

Chainguard `wordpress:latest-dev` is PHP-FPM only, not a full stack.

Required: Multi-container pod:
- Init container: Download WordPress core, create wp-config.php
- nginx container: `cgr.dev/chainguard/nginx` - serve static files, proxy PHP to port 9000
- php-fpm container: `cgr.dev/chainguard/wordpress:latest-dev` - process PHP
- Shared emptyDir for WordPress core files
- PVC for wp-content (uploads, plugins, themes)
- Both nginx and php-fpm run as UID 65532

Source: 2026-01-23 WordPress deployment.

## Cloudflare Tunnel on K8s

- Decode base64 tunnel token to get credentials JSON (a=AccountTag, t=TunnelID, s=TunnelSecret)
- Use `hostAliases` in pod spec to resolve origin hostname inside cloudflared pod
- Dashboard origin override requires the hostname to resolve from within the pod

Source: 2026-01-28 resume website deployment.

## Network Policy Labels

Homepage's most common failure: network policy label mismatch. Policy uses `app=homepage` but deployment uses `app.kubernetes.io/name=homepage`.

Diagnostic: `kubectl describe networkpolicy -n default | grep Selector`
Fix: Align labels between network policy and deployment.

Source: Multiple homepage incidents (5+ occurrences, 85% of homepage issues).
