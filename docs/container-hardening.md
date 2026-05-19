# Container Image Hardening — Non-Chainguard Procedure

Companion to [`container-images.md`](container-images.md). Chainguard remains the default; this doc covers the **ROCm exception path** and any other case where the Chainguard base is not viable.

The premise: when we can't use Chainguard, we substitute *real* hardening — not vibes. Every item below is a concrete checklist entry, ordered by build time so it doubles as a Dockerfile review checklist.

---

## Why this exists

Chainguard's Wolfi distribution does not package ROCm runtime libraries (`libamdhip64.so`, `librocblas.so`, `libhipblas.so`, etc.). AMD ships them only via their Ubuntu/RHEL images. Until that changes, ROCm-bound containers (Infinity, vLLM-rocm, llama.cpp, future training images) must use AMD's Ubuntu base. This document is how we make that acceptable.

First instance documented here: `registry.petersimmons.com/ai-fleet/llama-cpp:rocm-v0.1.2` (built 2026-05-19, [aifleet#TBD](https://github.com/petersimmons1972/aifleet)).

---

## Build-time checklist

### Base image
- [ ] Pin base by **digest**, not tag. Tags are mutable; digests are content-addressed.
  ```dockerfile
  FROM rocm/dev-ubuntu-22.04@sha256:79aa43981a4296ca45f6827a0d0bcdc8239824c8e97643a7a86c1ce2c9dddc8b
  ```
- [ ] Match the ROCm version to your application's runtime requirements. llama.cpp HEAD requires FP8 type symbols (`__hip_fp8_e4m3`) added in ROCm 6.3+; building on ROCm 6.2.4 fails with `unknown type name` errors. Refresh the base when the app's needs change, not on an arbitrary cadence.
- [ ] Refuse `:latest` for both base AND your own image. Use explicit version tags everywhere.

### Multi-stage build
- [ ] Always multi-stage. Build artifacts (cmake, build-essential, source code) must not appear in the runtime image even if the same physical layer ends up there via the base.
  ```dockerfile
  FROM rocm/dev-ubuntu-22.04@sha256:... AS builder
  # build everything
  FROM rocm/dev-ubuntu-22.04@sha256:...
  # COPY --from=builder only what's needed
  ```
- [ ] Pin application version (git tag, commit SHA) via `ARG`. Record the resolved SHA in the image: `RUN git rev-parse HEAD > /app/.app-sha`.
- [ ] `ARG` variables in the builder stage are **not visible** in the runtime stage. To use them in runtime `LABEL` substitution, re-declare:
  ```dockerfile
  FROM ... AS builder
  ARG APP_REF=v1.2.3
  # ...
  FROM ...
  ARG APP_REF=v1.2.3   # <-- re-declare for LABEL access
  LABEL com.example.app-ref="${APP_REF}"
  ```

### Runtime layer hygiene
- [ ] Install ONLY runtime needs. No `-dev` packages unless the application explicitly requires headers at runtime (rare).
- [ ] Clean apt after install: `&& apt-get clean && rm -rf /var/lib/apt/lists/* /var/cache/apt/* /tmp/* /var/tmp/*`. Same RUN as the install — separate RUNs leave the cache in an earlier layer.
- [ ] **Dynamic library dependencies are not optional.** Run `ldd` against your binary in the runtime stage and verify zero `not found` entries. Common gaps:
  - Application's own `.so` files (llama.cpp builds `libllama-common.so.0`, `libggml-*.so.0`, `libmtmd.so.0`). Copy them and set `LD_LIBRARY_PATH`:
    ```dockerfile
    COPY --from=builder /src/build/bin/*.so* /usr/local/lib/app/
    ENV LD_LIBRARY_PATH=/usr/local/lib/app
    ```
  - ROCm BLAS runtime libs (`libhipblas`, `librocblas`, `libhipblaslt`). `rocm-dev` metapackage does NOT include these; install explicitly:
    ```dockerfile
    RUN apt-get install -y --no-install-recommends hipblas rocblas
    ```
- [ ] If the base image is from a `dev-*` family (like `rocm/dev-ubuntu-22.04`), strip what you can post-install:
  ```dockerfile
  RUN apt-get remove --purge -y \
          gnupg gnupg-l10n gpgv \
          sudo linux-libc-dev \
       && apt-get autoremove --purge -y
  ```
  These packages have HIGH CVE counts on every scan and zero runtime function for an HTTP service.

### Nonroot user
- [ ] Create user with UID **65532** (Chainguard convention; matches `fsGroup: 65532` in our K8s pod specs).
- [ ] Application group additions for hardware access:
  ```dockerfile
  RUN groupadd --system --gid 65532 app \
   && useradd  --system --uid 65532 --gid 65532 --shell /sbin/nologin --home-dir /app app \
   && usermod -aG video,render app    # video/render = /dev/kfd + /dev/dri/renderD* access for ROCm
   && mkdir -p /app && chown app:app /app
  ```
- [ ] `USER 65532:65532` BEFORE `ENTRYPOINT`.

### PID 1 + signals
- [ ] Install `tini`. Use as PID 1 via `ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/your-binary"]`. Without tini, your container won't reap zombies or forward SIGTERM cleanly during pod termination.

### Healthcheck
- [ ] Always include a `HEALTHCHECK`. Match what the watcher / orchestrator expects:
  ```dockerfile
  HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
      CMD curl -sf http://localhost:PORT/health || exit 1
  ```
- [ ] `--start-period` covers model-load time (model files can take 30–90s to mmap from network storage).

### OCI labels
- [ ] Minimum label set for traceability:
  ```dockerfile
  ARG BUILD_DATE
  ARG VCS_REF
  LABEL org.opencontainers.image.title="org/app" \
        org.opencontainers.image.description="..." \
        org.opencontainers.image.source="https://github.com/org/repo" \
        org.opencontainers.image.vendor="petersimmons.com" \
        org.opencontainers.image.licenses="MIT" \
        org.opencontainers.image.created="${BUILD_DATE}" \
        org.opencontainers.image.revision="${VCS_REF}"
  ```
- [ ] Custom label documenting what hardening was applied (lets reviewers spot regressions):
  ```dockerfile
  LABEL com.petersimmons.hardening="nonroot-65532,tini-pid1,no-build-tools,no-source,healthcheck,digest-pinned-base"
  ```

---

## Pre-push gate

### CVE scan
- [ ] Run trivy (or grype) **before pushing**:
  ```bash
  trivy image --severity CRITICAL,HIGH --no-progress --quiet \
      --format table registry.example/org/app:tag
  ```
- [ ] **Block on**: any CRITICAL.
- [ ] **Accept with documentation**: HIGH findings in base-image packages that have no runtime exposure (e.g., `linux-libc-dev`, build-time `gnupg`). File a GH issue tracking removal in next minor version.
- [ ] **Recheck after slim**: removing dev-image bloat (gnupg, sudo, linux-libc-dev) typically drops 30+ HIGH findings.

### Image-size discipline
- [ ] Compare against the previous version. ROCm bases land at 2–10 GB; this is unavoidable but movement matters.
- [ ] If a layer >2 GB, expect registry-ingress timeouts. Workaround: `kubectl port-forward svc/registry 5000:5000`, tag/push to `localhost:5000/...`, image lands in same backing storage. The proper fix is upstream — see [homelab-config#52](https://github.com/petersimmons1972/homelab-config/issues/52) for the Traefik timeout config.
- [ ] rocBLAS ships pre-compiled GEMM kernels for every supported GPU arch in `/opt/rocm-*/lib/rocblas/library/`. Each arch is ~1 GB. For known-fleet deployments, strip unused archs:
  ```dockerfile
  RUN find /opt/rocm-*/lib/rocblas/library -name '*gfx906*' -delete \
                                          -o -name '*gfx940*' -delete \
                                          -o -name '*gfx941*' -delete \
                                          -o -name '*gfx942*' -delete
  ```

### Tag discipline
- [ ] Never reuse a tag for different content. Bump version on every rebuild that ships.
- [ ] Push BEFORE updating downstream CRDs / manifests to point at the new tag. Half a CRD apply pointing at a non-existent tag = preventable crashloop. (Bitter-experience lesson — make sure the tag is `curl`-able on the registry before the CRD edit.)
- [ ] Tag with both semver AND commit SHA for forensic traceability:
  ```bash
  docker tag org/app:v1.2.3 org/app:v1.2.3-$(git rev-parse --short HEAD)
  docker push org/app:v1.2.3
  docker push org/app:v1.2.3-$(git rev-parse --short HEAD)
  ```

---

## Runtime-side (K8s pod spec)

Lifted from `container-images.md` for completeness — these are pod-spec requirements, not Dockerfile:

```yaml
spec:
  securityContext:
    fsGroup: 65532                    # MANDATORY for nonroot 65532 + volumes
  containers:
  - name: app
    securityContext:
      runAsUser: 65532
      runAsGroup: 65532
      runAsNonRoot: true
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true    # add /tmp emptyDir if app needs scratch
      capabilities:
        drop: [ALL]
      seccompProfile:
        type: RuntimeDefault
```

For ai-fleet GPU containers specifically, the watcher injects `/dev/kfd` + `/dev/dri/renderD*` device binds at container creation; pod spec is handled by the watcher, not authored by hand.

---

## Verification before declaring done

A new image is not "done" until:
1. `trivy image` produces 0 CRITICAL and a documented disposition for any HIGH.
2. The image is published to the registry at the tagged version, and `curl https://registry.example/v2/org/app/manifests/tag` returns the manifest.
3. A pod or container has actually started on the image, completed its health probe, and served a real request end-to-end.
4. The dynamic linker reports zero `not found` entries under the container's working `LD_LIBRARY_PATH` (`docker run --rm --entrypoint sh image -c 'ldd /path/to/binary | grep "not found"'` returns empty).

Steps 3 and 4 catch the failure modes that ldd-on-the-host or unit-tests-on-the-host cannot: missing runtime libs from a dev-base mismatch (the v0.1 → v0.1.2 iteration that took three rebuilds to land llama-cpp ROCm).

---

## Known exceptions log

When you cannot apply something on this checklist, document it here with a date and reason:

| Date | Image | Exception | Reason |
|------|-------|-----------|--------|
| 2026-05-19 | `ai-fleet/llama-cpp:rocm-v0.1.2` | Not Chainguard base | No ROCm packages in Wolfi apk repos; AMD ships only Ubuntu/RHEL |
| 2026-05-19 | `ai-fleet/llama-cpp:rocm-v0.1.2` | 41 HIGH CVEs from dev-image base | Deferred slim to v0.2 ([aifleet#44](https://github.com/petersimmons1972/aifleet/issues/44)); v0.1.2 attack surface bounded by nonroot+HTTP-API-only |
| 2026-05-19 | `ai-fleet/llama-cpp:rocm-v0.1.2` | 10 GB image size | rocBLAS GEMM kernels for all gfx archs; v0.2 will strip unused per same issue |

---

## Related

- [`container-images.md`](container-images.md) — Chainguard default standard (still the entry point)
- [aifleet#44](https://github.com/petersimmons1972/aifleet/issues/44) — llama-cpp rocm-v0.2 slim
- [homelab-config#52](https://github.com/petersimmons1972/homelab-config/issues/52) — Traefik 504 on large registry pushes
- `~/projects/ai-fleet-watcher/images/llama-cpp-rocm/Dockerfile` — first case study Dockerfile
