---
name: Chainguard container rule
description: Always use Chainguard base images in Docker containers where possible
type: feedback
Category: feedback
---

Always prefer Chainguard base images (`cgr.dev/chainguard/...`) for Docker containers.

**Why:** User rule — minimal attack surface, CVE reduction, continuously rebuilt against current Wolfi packages.

**How to apply:** When writing any Dockerfile, use `cgr.dev/chainguard/python:latest-dev` (or relevant language image) instead of upstream images. If Chainguard isn't viable (e.g., Playwright's `--with-deps` is apt-only), document the blocker in the Dockerfile with comments, use the best available Chainguard approach, and file a GitHub issue to track migration.
