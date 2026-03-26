---
name: feedback-dns-use-cloudflare
description: DNS records for petersimmons.com are managed via Cloudflare API — do not ask for passwords or SSH sudo
type: feedback
---

Use the Cloudflare API (CF_TOKEN in ~/.claude/.cloudflare-credentials) to add DNS records for petersimmons.com. Do NOT ask for Pi-hole sudo passwords or Ubiquiti credentials.

**Why:** Internal services use CNAMEs to traefik.petersimmons.com in Cloudflare DNS. Pi-hole's app_sudo blocks config writes via API, and SSH sudo requires a password that isn't stored. Cloudflare is the authoritative DNS and is always accessible.

**How to apply:** Any time a DNS record needs adding for *.petersimmons.com — reach for Cloudflare first. Zone ID: `460653bc26ff94fdc0910a13defa4afb`. Internal services: CNAME → traefik.petersimmons.com, proxied: false. Public services: check existing pattern (some use cfargotunnel.com targets).
