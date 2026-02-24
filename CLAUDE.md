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
- **Generals multi-agent system**: See `~/AGENTS.md` for roster, spawn templates, and operational reference
  - GitHub source of truth: https://github.com/petersimmons1972/generals
  - Service records + commit to GH before shutting down teams (see AGENTS.md checklist)
- GitHub = single source of truth; deliverables must be committed

---

## Quick Reference

| Service | Address |
|---------|---------|
| Proxmox | pve.petersimmons.com:100 |
| K8s Nodes | 192.168.0.131-139 |
| DNS | 192.168.0.231, .232 |
| Nextcloud | 192.168.0.200 |
| TruNAS | trunas.petersimmons.com (API: `~/.claude/.truenas-credentials`) |
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
- `AGENTS.md` — Generals roster, spawn templates, service record checklist

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

## Self-Learning & Quality Automation (NON-NEGOTIABLE)

**NEVER ask permission for:**
- Bug fixes (any severity: CRITICAL, HIGH, MEDIUM, LOW)
- Feedback integration and quality improvements
- Prompt cycle enhancements
- Self-improvement and system refinement

These are **background automation** that must happen autonomously. When you find bugs:
1. Fix them immediately
2. Write tests to validate the fix
3. Commit the fix
4. Report what was fixed (after the fact)

**ONLY escalate to user if:**
- You detect **repeated pattern cycles** — same bugs/patterns appearing multiple times without improvement
- You're wasting tokens on circular self-learning loops
- Example: "I've run 5 self-learning cycles and they keep finding the same issue in different files. This suggests a systemic/architectural problem that needs human diagnosis."

**DO ask permission for:**
- Running reports or generating deliverables (external impact)
- Resource-intensive operations that require deliberate scheduling
- Actions with external visibility or side effects

This is non-negotiable. Self-improvement happens in the background, silently and continuously.

## Visual Output
- SVG charts over tables; dark cards (navy #0F172A, gold #D4A574, cream #f8fafc)
- Namespace SVG IDs; wrap in `<div style="margin: 2rem 0; page-break-inside: avoid;">`
- Data tables as `<details>` fallback

## Learning System
- Detail → topic file | one-liner → MEMORY.md | behavioral rule → CLAUDE.md
- Files: `~/.claude/projects/-home-psimmons/memory/`
- Monthly review: `homelab:monthly-review`
