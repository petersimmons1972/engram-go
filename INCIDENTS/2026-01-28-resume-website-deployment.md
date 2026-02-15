# Incident Report: resume-website Deployment

**Date:** 2026-01-28
**Service:** resume-website (new deployment)
**Duration:** ~45 minutes (design to live)
**Status:** Resolved — site live at https://resume.petersimmons.com

---

## Summary

New static website deployment using Chainguard Nginx + Cloudflare tunnel. Multiple issues encountered during first deployment, all resolved.

---

## Issues Encountered & Resolutions

### 1. Chainguard Nginx: readOnlyRootFilesystem requires multiple writable dirs

**Symptom:** CrashLoopBackOff
**Error:** `mkdir() "/var/lib/nginx/tmp/client_body" failed (30: Read-only file system)`
**Then:** `open() "/run/nginx.pid" failed (30: Read-only file system)`

**Root Cause:** Chainguard Nginx (distroless) needs these directories writable:
- `/var/cache/nginx`
- `/var/lib/nginx/tmp` (client_body, proxy, fastcgi, uwsgi, scgi)
- `/run` (nginx.pid)
- `/tmp`

**Fix:** Add emptyDir volumes for each writable path.

**Lesson:** When using `readOnlyRootFilesystem: true` with Chainguard images, check the application's startup logs iteratively. Nginx specifically needs 4 writable dirs, not just `/tmp`.

---

### 2. Traefik IngressRoute: `kind: rule` is case-sensitive

**Symptom:** `spec.routes[0].kind: Unsupported value: "rule": supported values: "Rule"`

**Root Cause:** Traefik CRD requires `kind: Rule` (capital R), not `kind: rule`.

**Lesson:** Always use `kind: Rule` in Traefik IngressRoute specs.

---

### 3. Cloudflare tunnel: Dashboard origin overrides local config.yaml

**Symptom:** cloudflared logs show `originService=https://resume.petersimmons.com` instead of `http://192.168.0.180` even though local config.yaml specifies the LB IP.

**Root Cause:** When a tunnel is created/configured via the Cloudflare dashboard, the remote configuration takes precedence over the local `config.yaml` ingress rules. The "Updated to new configuration" log message confirms Cloudflare is pushing config remotely.

**Fix:** Used `hostAliases` in the pod spec to resolve the dashboard's origin hostname to the Traefik LB IP:
```yaml
hostAliases:
- ip: "192.168.0.180"
  hostnames:
  - "resume.petersimmons.com"
```

**Alternative (preferred long-term):** Update the origin URL in the Cloudflare dashboard tunnel configuration to `http://192.168.0.180`.

**Lesson:** Cloudflare dashboard tunnel config ALWAYS overrides local config.yaml. Either:
- Set the origin correctly in the dashboard, OR
- Use hostAliases to make the dashboard's origin resolvable inside K8s

---

### 4. Cloudflare tunnel: Distroless image has no shell

**Symptom:** `exec: "/bin/sh": stat /bin/sh: no such file or directory`

**Root Cause:** `cloudflare/cloudflared:latest` is distroless — no shell available for `command` wrappers.

**Fix:** Use `args` (not `command`) with `--credentials-file` pointing to a Secret-mounted JSON file. Cannot use shell env var expansion.

**Lesson:** For distroless images, never use `command: ["/bin/sh", "-c", ...]`. Use `args` directly and mount secrets as files.

---

### 5. Cloudflare tunnel token → credentials file conversion

**Symptom:** Token-based `--token` flag doesn't work with `--config` (cloudflared requires explicit tunnel ID).

**Root Cause:** The token (`eyJhI...`) is a base64 JSON containing `{a: AccountTag, t: TunnelID, s: TunnelSecret}`. Cloudflared needs either:
- `--token` without `--config`, OR
- `--credentials-file` with a JSON file containing `{AccountTag, TunnelID, TunnelSecret}`

**Fix:** Decoded the token and created a credentials JSON file:
```python
# Token decodes to: {"a":"...", "t":"...", "s":"..."}
# Credentials file: {"AccountTag":"...", "TunnelID":"...", "TunnelSecret":"..."}
```

**Lesson:** When converting Cloudflare dashboard tokens to K8s credentials, decode the base64 token and map: `a→AccountTag`, `t→TunnelID`, `s→TunnelSecret`.

---

### 6. Stale tunnel credentials

**Symptom:** `control stream encountered a failure while serving` — tunnel retries indefinitely.

**Root Cause:** Old credential files in `~/.cloudflared/` referenced tunnels that were deleted or expired in Cloudflare.

**Lesson:** Always verify tunnel health via `cloudflared tunnel list` or the Cloudflare dashboard before reusing old credentials. Stale tunnels fail silently with retry loops.

---

## What Worked First Try

- Namespace creation
- ConfigMap with embedded HTML
- Service definition
- Traefik IngressRoute HTTPS routing (after case fix)
- HTTP→HTTPS redirect middleware
- cert-manager wildcard TLS (no new cert needed)
- Cloudflare DNS API (CNAME record creation)

## Patterns for Future `websites` Namespace Deployments

1. **Chainguard Nginx template** — use the deployment.yaml from resume-website as a baseline (includes all 4 writable emptyDir mounts)
2. **Cloudflare tunnel** — always use credentials-file approach with hostAliases; don't rely on local config.yaml override
3. **Traefik IngressRoute** — always `kind: Rule` (capital R)
4. **TLS** — wildcard cert `petersimmons-com-wildcard-tls` covers all `*.petersimmons.com` subdomains

---

## Files Created/Modified

- `projects/resume-website/` — full K8s deployment repo
- `docs/plans/2026-01-28-resume-website-design.md` — architecture design
- This incident report
