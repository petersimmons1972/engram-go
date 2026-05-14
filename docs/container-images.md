# Container Image Standard — NON-NEGOTIABLE

Reference doc for `~/CLAUDE.md`. Apply whenever writing a Dockerfile or K8s pod spec.

---

**Default to Chainguard base images for every Dockerfile in homelab / clearwatch / substack / engram repos.** Burden of justification is on the author for any other base. Free tier exposes `:latest` and `:latest-dev` only; no digest pinning unless paid.

## Python-with-tools pattern

For Python apps needing git/ssh/etc at runtime:

- **Stage 1 (build):** `cgr.dev/chainguard/python:latest-dev` → pip install into `/app/venv`
- **Stage 2 (runtime):** `cgr.dev/chainguard/wolfi-base:latest` → `apk add python-3.12 git openssh-client tini ca-certificates-bundle`, then `COPY --from=build /app/venv`
- Nonroot UID **65532**. Tini at **/sbin/tini**.

## K8s pod spec requirements

**`fsGroup: 65532` is MANDATORY** (Chainguard nonroot user) or volume mounts crashloop on first write.

Container-level security context:
```yaml
allowPrivilegeEscalation: false
capabilities:
  drop: [ALL]
seccompProfile:
  type: RuntimeDefault
```
