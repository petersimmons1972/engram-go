---
name: feedback-no-cloudflare-dns-databases
description: NEVER add database hostnames to Cloudflare DNS — use local DNS (Ubiquiti appliance) only
type: feedback
Category: feedback
---

Database DNS records go on the Ubiquiti appliance (local DNS), NEVER Cloudflare.

**Why:** Cloudflare DNS is public-facing. Database hostnames in public DNS expose internal infrastructure topology to the internet. This is a foundational security control — non-negotiable.

**How to apply:**
- PostgreSQL, Redis, Qdrant, any database → Ubiquiti local A record only
- Web services, APIs with public access → Cloudflare DNS is fine
- When a service needs DNS: ask "is this internal-only?" → yes = Ubiquiti, no = Cloudflare
- kubectl port-forward is a temporary workaround, not a permanent solution
- The research-postgres DB needs a permanent A record on the Ubiquiti appliance
