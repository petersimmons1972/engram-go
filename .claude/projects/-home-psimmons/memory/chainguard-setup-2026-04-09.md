---
name: Chainguard Internal Tooling Setup
description: Organization registry, auth flow, and pending supply chain audit
type: context
Category: homelab
originSessionId: faae1adf-ea01-4a5d-8eee-4c371b4db43c
---
## Setup Complete
- **Org**: a5010deea12711099daf408bed808cc078da367c
- **Registry**: cgr.dev/petersimmons.com  
- **APK Repo**: apk.cgr.dev/petersimmons.com
- **Plan**: Free (sufficient for current scale)
- **Token**: Stored in Infisical (expires 2026-08-29)

## Architecture
Internal Python + Go services behind Traefik + Nginx proxies. K8s deployments with ImagePullSecrets for authenticated pulls. Templates provided and working.

## Current State
Some images migrated. **ISSUE**: Currently pulling anonymously, not with authenticated token.

## Pending Work
**Task #1**: Audit Chainguard integration — verify token auth enabled end-to-end (CI/CD, K8s, vulnerability scanning). Deferred due to token budget constraints.

**Why**: Ensure supply chain is secured before scaling usage. Anonymous pulls work but don't leverage Chainguard's verification/attestation benefits.
