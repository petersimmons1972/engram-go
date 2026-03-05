# Learning Index

**Last Updated**: 2026-03-05T12:32:54Z
**Session**: 20260305-073254

---

## 🔥 Recent Activity (Last 7 Days)

- 2026-03-04: docs: add Clearwatch narrative chart selection 3-phase plan to memory
- 2026-03-02: chore: update MEMORY.md and .gitignore
- 2026-03-02: docs: add human-readable table requirement to CLAUDE.md
- 2026-03-02: docs: restore zero-XP roster with deployment preference policy
- 2026-03-02: docs: trim zero-XP roster to pointer, save 29 lines of context
- 2026-02-28: fix: remove DOW 30 from SentinelOne bullet in resume DOCX - conversions were McAfee
- 2026-02-28: docs: rewrite resume with ECO methodology (impact-first bullets)
- 2026-02-28: docs: rename ATS resume to remove ATS suffix
- 2026-02-28: docs: rename source resume to include 2025 year
- 2026-02-28: docs: rename resume DOCXs to remove spaces

**Recent Sessions**:
- SESSION-CONTEXT-OPTIMIZATION-COMPLETE.md
- SESSION-2026-02-14-GORDON-CISO-VERIFICATION-FINDINGS.md
- SESSION-2026-02-14-14-VARIANT-DEPLOYMENT.md
- SESSION-2026-02-13-PLAYWRIGHT-QA-INFRASTRUCTURE.md

**Uncommitted Changes**:
⚠️  5 modified, 0 staged

---

## ⚡ Infrastructure Health

**Cluster Status**: ✅ All 9 nodes ready
**Critical Services**: ⚠️  8 OK, 0 failed, 2 warnings
**Active Warnings**: None detected

Quick health check: `~/bin/health-check.sh`

---

## 🎯 Top Known-Good Fixes (By Success Rate)

| Command | Success | MTTR | Service | Use When |
|---------|---------|------|---------|----------|
| `~/bin/mouse.sh` | 100% | 30s | Hardware | Mouse unresponsive after idle |
| `kubectl apply -f networkpolicy.yaml` | 100% | 15s | Homepage | Network policy mismatch |
| `kubectl rollout restart deployment/homepage` | 85% | 2m | Homepage | Pod issues after config change |

**Full list**: See `~/.homelab/knowledge/fix-effectiveness.yaml`

---

## ⚠️ Active Warning Patterns

**Storage Pressure** (Tier 1 - Critical):
- Check: `ssh psimmons@192.168.0.100 'pvesm status' | grep zp3`
- Warning: >85% usage
- Impact: VM freeze within 48h (95% confidence)
- Action: Run cleanup immediately

**Homepage Restarts** (Tier 2 - Major):
- Check: `kubectl get pods -l app.kubernetes.io/name=homepage -o jsonpath='{.items[*].status.containerStatuses[*].restartCount}'`
- Warning: >3 restarts/hour
- Impact: CrashLoopBackOff imminent
- Action: Check network policy labels

**Certificate Renewal** (Tier 2 - Major):
- Check: `kubectl get certificate -A`
- Warning: Renewal stuck >7 days
- Impact: Service outage before expiry
- Action: Check cert-manager logs, verify Cloudflare API token

---

## 📖 Key Lessons

**See detailed lessons**: `memory/lessons-learned.md`

**Quick summary**: 31 lessons across homelab infrastructure, deployment patterns, and general programming (7 homelab most-used, 24 general patterns)

---

## 🚫 Known Anti-Patterns

1. **Don't restart services before checking logs** - Treats symptom, not cause
2. **Don't use :latest tags in production** - Silent breaking changes
3. **Don't delete pods as first troubleshooting step** - Masks root cause
4. **Don't trust documentation over live state** - Docs represent point in time
5. **Don't assign static IPs in DHCP range** (192.168.0.2-.98) - IP conflicts

Full list: `~/.homelab/config/anti-patterns.yaml`

---

## 🗂️ Active Projects

**Infrastructure** (6 projects):
- k3s-ha-cluster-rebuild - 9-node K8s cluster (Production)
- nextcloud-deployment - Cloud storage at 192.168.0.200 (Production)
- container-registry - registry.petersimmons.com (Production)
- proxmox-performance-measurement - Grafana dashboards (Monitoring)
- linkwarden - Bookmark manager (Needs architecture fix)
- homepage - Dashboard at homepage.petersimmons.com (Production, problematic)

**Development** (3 projects):
- security-intelligence-business - Business website + LinkedIn automation
- job-search-system - Application tracking (Development)
- gmail-tracker - Email campaign tracking (Development)

**See Full Catalog**: [PROJECTS-CATALOG.md](/home/psimmons/PROJECTS-CATALOG.md)

---

## 📊 Recent Failure Summary (Last 30 Days)

- timestamp: 2025-12-20T14:32:00Z | service: homepage
- timestamp: 2025-12-20T14:34:00Z
- timestamp: 2025-12-20T14:36:00Z

**Full history**: `~/.homelab/knowledge/failure-history.yaml`

---

## 📚 Topic Files

**Homelab**:
- cert-manager patterns → memory/homelab-cert-manager.md
- K8s deployment patterns → memory/homelab-k8s-patterns.md
- Incident summaries → memory/homelab-incidents.md

**Research & Validation**:
- URL validation patterns → memory/url-validation-patterns.md
- Chart regression analysis → memory/chart-regression-2026-02-06.md
- HTML processing patterns → memory/html-processing-patterns.md

---

## 🔧 Quick Reference

**Triage Decision Tree**:
```
Service Down?
├─ Multiple? → Traefik, K8s, Proxmox (use homelab:trace-dependencies)
├─ Single K8s? → homelab:troubleshoot-common-issues
├─ Homepage? → homelab:troubleshoot-homepage
├─ Nextcloud? → homelab:troubleshoot-nextcloud
├─ Hardware? → homelab:troubleshoot-hardware
└─ Then: superpowers:systematic-debugging
```

**Critical Commands**:
- Health: `~/bin/health-check.sh`
- Logs: [LOG-COMMANDS-REFERENCE.md](/home/psimmons/LOG-COMMANDS-REFERENCE.md)
- K8s Nodes: 192.168.0.131-139
- Proxmox: pve.petersimmons.com:100
- DNS: 192.168.0.231, .232

---

## 🧠 How This System Works

When a lesson is learned:
1. Write detail to the relevant topic file in memory/
2. Add or increment counter in "Key Lessons" above
3. If it's a behavioral NEVER/ALWAYS rule, add to CLAUDE.md Critical Rules

When an incident occurs:
1. Use `homelab:log-incident` to capture to failure-history.yaml
2. Use `homelab:log-fix-result` to track command effectiveness
3. Monthly: Use `homelab:monthly-review` to analyze trends

When stuck >20 minutes:
1. Use `homelab:check-anti-patterns` to identify wrong approaches
2. Use `homelab:troubleshoot-common-issues` for known solutions

---

**This file is auto-updated at session start by `~/.claude/session-start-hook.sh`**
