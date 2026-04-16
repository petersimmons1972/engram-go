---
name: Chainguard image policy
description: Chainguard images cannot be pinned on free tier (latest only); this is accepted policy, not a security issue
type: feedback
originSessionId: 00d4e7f7-5725-4b06-8ed8-5f165a30a49c
---
Chainguard images (`cgr.dev/chainguard/*`) cannot be pinned to specific versions on the free/open-source tier — only `:latest` is available. Paid tiers support immutable digest tags.

**Why:** This is an accepted constraint. Chainguard `:latest` is still the preferred choice over standard Docker Hub images because Chainguard maintains zero/near-zero CVEs, a hardened minimal build, and a significantly more trustworthy supply chain. Distroless/static images eliminate shell-based attack surfaces.

**How to apply:** Do NOT file issues or flag Chainguard `:latest` usage as a security problem or build-reproducibility risk. It IS the security solution for our budget. Pin what you can (Go binary version via `go.mod`, postgres image version on Docker Hub, etc.) but Chainguard image tags themselves stay at `:latest`.

Additionally: `cgr.dev/chainguard/static` runs non-root by default — do not file USER directive issues for Chainguard-based images.

Affected projects: engram-go (`cgr.dev/chainguard/go`, `cgr.dev/chainguard/static`, `cgr.dev/chainguard/postgres`), clearwatch (migration in progress via #3143), homelab-config.
