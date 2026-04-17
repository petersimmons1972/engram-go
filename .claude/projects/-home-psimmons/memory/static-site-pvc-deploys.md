---
name: Deploying static HTML to shared-html PVC (websites/personal-site)
description: How to drop a file into www.petersimmons.com when the nginx pod is distroless Chainguard and kubectl cp fails
type: reference
category: homelab-k8s
originSessionId: a15c0b00-0071-4065-ad0f-eae37b6c33fe
---
**Target:** `www.petersimmons.com/<path>` — served by `personal-site` nginx in `websites` namespace, reading from `shared-html` NFS RWX PVC.

**Why kubectl cp fails:** Chainguard nginx is distroless — no `tar`, `sh`, `mkdir`, `ls`. `kubectl cp` uses tar under the hood and errors with `exec: "tar": executable file not found`. `kubectl exec` also useless for setup.

**Why simple PVC-mount pod fails:** `/usr/share/nginx/html` is `drwxr-xr-x root:root`. Files inside are `1000:1000`. So pods running as uid 1000 (or the nginx 65532) cannot `mkdir` new subdirs at root.

**Working pattern — one-shot Job, root + ConfigMap + chown:**

```bash
# 1. Stage file in ConfigMap (82 KB fits under 1 MB limit)
kubectl create configmap <name>-tmp -n websites \
  --from-file=index.html=/path/to/local.html \
  --dry-run=client -o yaml | kubectl apply -f -

# 2. Launch Job: root pod mounts PVC + CM, cp, chown to 1000:1000
cat <<'EOF' | kubectl apply -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: copy-<name>
  namespace: websites
spec:
  ttlSecondsAfterFinished: 60
  template:
    spec:
      restartPolicy: Never
      securityContext: {runAsUser: 0, runAsGroup: 0}
      containers:
      - name: copy
        image: cgr.dev/chainguard/busybox:latest
        securityContext: {runAsUser: 0, runAsNonRoot: false}
        command: ["sh","-c"]
        args:
        - |
          set -e
          mkdir -p /html/<subpath>
          cp /src/index.html /html/<subpath>/index.html
          chown -R 1000:1000 /html/<subpath>
          chmod 755 /html/<subpath>
          chmod 644 /html/<subpath>/index.html
        volumeMounts:
        - {name: html, mountPath: /html}
        - {name: src,  mountPath: /src}
      volumes:
      - {name: html, persistentVolumeClaim: {claimName: shared-html}}
      - {name: src,  configMap: {name: <name>-tmp}}
EOF
kubectl wait --for=condition=complete job/copy-<name> -n websites --timeout=60s
kubectl delete configmap <name>-tmp -n websites
```

**Verify:** `xh GET https://www.petersimmons.com/<path>/ -h` — expect 200. `xh` without explicit method sends HEAD → 405 against this nginx.

**CSP constraints on this site:** `script-src 'none'` (no JS allowed). `style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com`. Static pages must be JS-free; Google Fonts via `<link>` OK. Responses also carry `x-robots-tag: noindex, nofollow` — intentional for private reports.

**Where the nginx config lives:** ConfigMap `personal-site-html` key `nginx.conf` (websites ns). `try_files $uri $uri/ /index.html` fallback — subpath dirs must contain an `index.html` or request fails through to the root SPA.

**Known blocked paths:** `/locked-shields/` returns 403 (embargo until 2026-05-01 per nginx config comment).
