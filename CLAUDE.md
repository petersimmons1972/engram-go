# Claude Assistant Instructions

## Critical Rules

**NEVER:**
- Commit secrets (API keys, passwords, tokens, .env files)
- Replace Traefik with Nginx
- Assign static IPs in DHCP range (192.168.0.2-.98)
- Restart services before checking logs
- Edit live manifests (edit source, apply source)
- Perform destructive ops without verified backup

**ALWAYS:**
- `git diff --staged` before every commit
- `~/bin/health-check.sh` before troubleshooting
- Check logs before restarting services
- Verify end-to-end output, not just code (see `docs/MULTI-STAGE-VERIFICATION.md`)
- Use skills for procedural work
- Write service records + commit to GH before shutting down teams (`docs/TEAM-MANAGEMENT-WORKFLOW.md`)
- Use actual generals from `~/projects/generals/profiles/` — never generic agent names (see `~/projects/generals/COMMAND-ROSTER.md`)
- Model selection: Field Marshal assigns models case-by-case (Haiku/Sonnet/Opus)
- GitHub = single source of truth; deliverables must be committed

---

## Quick Reference

| Service | Address |
|---------|---------|
| Proxmox | pve.petersimmons.com:100 |
| K8s Nodes | 192.168.0.131-139 |
| DNS | 192.168.0.231, .232 |
| Nextcloud | 192.168.0.200 |
| Registry | registry.petersimmons.com |
| SearXNG | searxng.petersimmons.com (`/search?q={query}&format=json`) |
| Health check | `~/bin/health-check.sh` |
| Log commands | `LOG-COMMANDS-REFERENCE.md` |

**K8s AI experiments:** Use namespaces (`kubectl create ns ai-experiment-<task>`), clean up when done.

---

## Web Search
**PRIMARY:** SearXNG `https://searxng.petersimmons.com/search?q={query}&format=json`
**FALLBACK:** WebSearch tool

---

## DNS Management
Use APIs — never ask user to do it manually.
- Internal DNS → Unifi API (`~/.claude/.unifi-credentials`)
- Public domains → Cloudflare (`~/projects/kubernetes/cert-manager/.env`)
- Monitoring only → Pi-hole (`~/.claude/pihole-api-credentials.md`)

Full examples: `docs/DNS-API-REFERENCE.md`

---

## Triage
```
Service Down?
├─ Multiple? → homelab:trace-dependencies
├─ Single K8s? → homelab:troubleshoot-common-issues
├─ Homepage? → homelab:troubleshoot-homepage
├─ Nextcloud? → homelab:troubleshoot-nextcloud
├─ Hardware? → homelab:troubleshoot-hardware
└─ Deep debug → superpowers:systematic-debugging
```

---

## Skills
- Service down → homelab:troubleshoot-common-issues
- Homepage → homelab:troubleshoot-homepage
- Nextcloud → homelab:troubleshoot-nextcloud
- Hardware → homelab:troubleshoot-hardware
- Trace dependencies → homelab:trace-dependencies
- Anti-patterns → homelab:check-anti-patterns
- Predict failures → homelab:predict-failure
- Log incident → homelab:log-incident
- Monthly review → homelab:monthly-review
- Evolve runbook → homelab:evolve-runbook
- Deep debug → superpowers:systematic-debugging
- Before claiming done → superpowers:verification-before-completion
- Before implementing → superpowers:brainstorming

Zero auto-invocation: all skills require explicit context or request.

---

## Indexes
- `KNOWLEDGE-INDEX.md` — Lessons, troubleshooting, sessions
- `PROJECTS-CATALOG.md` — 45+ projects
- `RUNBOOKS-INDEX.md` — Emergency procedures
- `FAILURE-MODES-CATALOG.md` — Known failures

---

## Decisions
- 100% → Just do it | 80-99% → Do + explain | 50-80% → Propose first | <50% → Ask
- Pre-approved: logs, kubectl get/describe, health-check, diagnostics
- Always ask: delete resources, modify production, data loss risk

---

## Principles
- Security mindset, no fabrication, verify live state before trusting docs
- Challenge wrong categorization; intellectual honesty over harmony
- Never present broken as complete

## Visual Output
- SVG charts over tables; dark cards (navy #0F172A, gold #D4A574, cream #f8fafc)
- Namespace SVG IDs; wrap in `<div style="margin: 2rem 0; page-break-inside: avoid;">`
- Data tables as `<details>` fallback

## Learning System
- Detail → topic file | one-liner → MEMORY.md | behavioral rule → CLAUDE.md
- Files: `~/.claude/projects/-home-psimmons/memory/`
- Monthly review: `homelab:monthly-review`
