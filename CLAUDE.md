# Claude Assistant Instructions

## Critical Rules

**NEVER:**
- Commit secrets to git (API keys, passwords, tokens, .env files)
- Replace Traefik with Nginx
- Assign static IPs in DHCP range (192.168.0.2-.98)
- Restart services before checking logs
- Edit live manifests (edit source, apply source)
- Perform destructive ops without verified backup

**ALWAYS:**
- `git diff --staged` before every commit (check for secrets)
- Run `~/bin/health-check.sh` before troubleshooting
- Check logs before restarting services
- Verify services functional before claiming done
- Use skills for procedural work
- Nothing ships without a verification plan
- **VERIFY END-TO-END OUTPUT, NOT JUST CODE** (see `docs/MULTI-STAGE-VERIFICATION.md`)
- If you can't verify it works, it's not done
- **Use SearXNG as primary for web searches** (fallback to WebSearch if SearXNG fails)
- **BEFORE shutting down teams**: Write service records (accomplishments + negatives), commit to GH, THEN shutdown
- **USE PROPER GENERAL PERSONALITIES**: NEVER spawn agents with generic names (like "generator" or "worker"). ALWAYS use actual generals from `~/projects/generals/profiles/` matched to mission requirements. See `~/projects/generals/COMMAND-ROSTER.md` for available generals and their specializations.
- **GitHub = single source of truth**: AI may use worktrees, Docker, or K8s for performance/isolation, but all deliverables MUST be committed to GitHub (bad for human multi-machine workflows). Feature branches live on GitHub.
- **AI isolation tools**: Use worktrees, Docker, K8s namespaces, or any isolation mechanism for experimental work. Keep human environment clean. GitHub = deliverables.

---

## Quick Reference

- Proxmox: pve.petersimmons.com:100
- DNS: 192.168.0.231, .232
- K8s Nodes: 192.168.0.131-139 (local + tailscale access)
- Nextcloud: 192.168.0.200
- Registry: registry.petersimmons.com
- **SearXNG: searxng.petersimmons.com** (API: `/search?q={query}&format=json`)
- Health: `~/bin/health-check.sh`
- Logs: [LOG-COMMANDS-REFERENCE.md](/home/psimmons/LOG-COMMANDS-REFERENCE.md)

**K8s Cluster for AI Experiments:**
- Local network: 192.168.0.131-139
- Tailscale network: Available via VPN
- Use namespaces for isolation (e.g., `kubectl create ns ai-experiment-<task>`)
- Clean up experimental namespaces when done

---

## Web Search

**PRIMARY:** SearXNG (`https://searxng.petersimmons.com/search?q={query}&format=json`)
**FALLBACK:** WebSearch tool (if SearXNG fails)

Quick Reference: `/search?q={query}&format=json&language=en`
K8s: `kubectl scale deployment searxng -n default --replicas=N`

---

## DNS Management

**Use APIs - never ask user to do it manually**

| System | Use For | Credentials |
|--------|---------|-------------|
| Unifi API | Internal DNS (fastest) | `~/.claude/.unifi-credentials` |
| Cloudflare | Public domains | `~/projects/kubernetes/cert-manager/.env` |
| Pi-hole | Monitoring only | `~/.claude/pihole-api-credentials.md` |

**Workflow:** Internal-only → Unifi; Public → Cloudflare; Both → Add to both

Full examples: `docs/DNS-API-REFERENCE.md`

---

## Triage

```
Service Down?
├─ Multiple? → Traefik, K8s, Proxmox (use homelab:trace-dependencies)
├─ Single K8s? → homelab:troubleshoot-common-issues
├─ Homepage? → homelab:troubleshoot-homepage
├─ Nextcloud? → homelab:troubleshoot-nextcloud
├─ Hardware? → homelab:troubleshoot-hardware
└─ Then: superpowers:systematic-debugging
```

---

## Team Management

**BEFORE shutting down teams:** Complete workflow in `docs/TEAM-MANAGEMENT-WORKFLOW.md`
- Document service records (accomplishments + failures + observations)
- Commit to GitHub with descriptive message
- Verify push succeeded
- THEN shutdown team directories

**Generals integration:** See `docs/GENERALS-INTEGRATION.md` for XP, ribbons, competence tracking

**Model selection:** Field Marshal assigns models case-by-case (Haiku/Sonnet/Opus)

---

## Skills

Use skills for procedural work. CLAUDE.md is a map, not encyclopedia.

- Service down → homelab:troubleshoot-common-issues
- Homepage issues → homelab:troubleshoot-homepage
- Nextcloud issues → homelab:troubleshoot-nextcloud
- Hardware (mouse, GPU) → homelab:troubleshoot-hardware
- Trace dependencies → homelab:trace-dependencies
- Check anti-patterns → homelab:check-anti-patterns
- Predict failures → homelab:predict-failure
- Log incident → homelab:log-incident
- Monthly review → homelab:monthly-review
- Evolve runbook → homelab:evolve-runbook
- Deep debugging → superpowers:systematic-debugging
- Before claiming done → superpowers:verification-before-completion
- Before implementing → superpowers:brainstorming

Zero auto-invocation: All skills require explicit context or request.

---

## Indexes

- [KNOWLEDGE-INDEX.md](/home/psimmons/KNOWLEDGE-INDEX.md) - Lessons, troubleshooting, sessions
- [PROJECTS-CATALOG.md](/home/psimmons/PROJECTS-CATALOG.md) - 45+ projects
- [RUNBOOKS-INDEX.md](/home/psimmons/RUNBOOKS-INDEX.md) - Emergency procedures
- [FAILURE-MODES-CATALOG.md](/home/psimmons/FAILURE-MODES-CATALOG.md) - Known failures

---

## Decisions

- 100% confidence → Just do it
- 80-99% → Do it, explain why
- 50-80% → Propose, wait approval
- <50% → Ask questions first

Pre-approved: Read logs, kubectl get/describe, health-check, diagnostics
Always ask: Delete resources, modify production, data loss risk

---

## Quality

- Verify correct HTTP status AND expected content
- No error messages, all features work
- Use `superpowers:verification-before-completion`

---

---

## Principles

- Security mindset, no fabrication
- Learn from failures, document what doesn't work
- Verify live state before trusting docs
- Never present broken as complete

## Collaboration Style

- Challenge categorization: push back if categories seem wrong or overlapping
- Flag logical errors respectfully but clearly
- Intellectual honesty over harmony

## Visual Output Preference

- Prefer inline SVG charts/infographics over text-heavy tables for data comparisons
- Style: Dark-themed cards (navy #0F172A, gold #D4A574, cream #f8fafc)
- Namespace SVG IDs with prefixes when embedding multiple SVGs
- Wrap SVGs in `<div style="margin: 2rem 0; page-break-inside: avoid;">`
- Keep data tables as collapsed fallback below SVGs using `<details>` tags

---

## Learning System

Lessons live in `~/.claude/projects/-home-psimmons/memory/`:
- MEMORY.md = always-loaded index with key lessons and usage counters
- Topic files = detailed knowledge by domain, read on-demand
- When learning: detail → topic file, one-liner → MEMORY.md, behavioral rule → CLAUDE.md
- Structured data: `~/.homelab/knowledge/` (consumed by skills)
- Monthly review: `homelab:monthly-review` to analyze trends
