# Learning Index

**Last Updated**: 2026-02-17T05:08:20Z
**Session**: 20260217-000820

---

## 🔥 Recent Activity (Last 7 Days)

- 2026-02-15: docs: master prompt v2.9 - GATE2 validator technical requirements
- 2026-02-15: docs: session summary and learning index update
- 2026-02-15: docs: master prompt v2.8 - EDR/XDR focus, exclude consumer/NGAV-only
- 2026-02-15: docs: master prompt v2.7 - exclude homelab tier pricing
- 2026-02-15: docs: master prompt v2.6 - personalize baseball games narrative
- 2026-02-15: docs: master prompt v2.5 - niche vendor citation exceptions
- 2026-02-15: docs: master prompt v2.4 - citation count minimum + visual badge implementation
- 2026-02-15: docs: master prompt v2.3 - endnote legend + mandatory legal sections
- 2026-02-15: docs: Free sample download infrastructure design
- 2026-02-14: fix: Route Enterprise traffic to service with buy button

**Recent Sessions**:
- SESSION-CONTEXT-OPTIMIZATION-COMPLETE.md
- SESSION-2026-02-14-GORDON-CISO-VERIFICATION-FINDINGS.md
- SESSION-2026-02-14-14-VARIANT-DEPLOYMENT.md
- SESSION-2026-02-13-PLAYWRIGHT-QA-INFRASTRUCTURE.md

**Uncommitted Changes**:
⚠️  7 modified, 0 staged

---

## ⚡ Infrastructure Health

**Cluster Status**: ✅ All 9 nodes ready
**Critical Services**: ⚠️  7 OK, 0 failed, 1 warnings
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

**Homelab** (ordered by usage count):
- (4x) Backup before modifying critical files - design phase IS implementation for docs
- (2x) MCP config lives in ~/.claude.json - use `claude mcp add`, never create mcp_servers.json
- (2x) Validate what the customer sees, not intermediate formats
- (1x) RWO PVCs need Recreate deployment strategy, not RollingUpdate
- (1x) Chainguard images need fsGroup for PVC write access (non-root UID 65532)
- (1x) cert-manager pods use 10.42.x.x overlay network, not 192.168.x.x - breaks Cloudflare IP filtering
- (1x) WordPress behind reverse proxy needs WP_HOME/WP_SITEURL/FORCE_SSL_ADMIN in wp-config.php
- (1x) CronJob not Deployment for periodic tasks - liveness probes kill sleep loops (exit 137, low memory = not OOM)
- (1x) Homepage issues are almost always network policy label mismatch
- (1x) Local DNS CNAME records break cert-manager DNS-01 challenges - use dnsPolicy: None + Cloudflare DNS

**General**:
- (2x) BeautifulSoup destroys SVG xmlns attributes (all parsers) - extract SVG first, process HTML, restore SVG after
- (1x) Every plan needs explicit validation checklist, not just "verify it works"
- (1x) Cloudflare negative DNS cache lasts 1800s - CDN purge won't help, wait or use different record name
- (1x) When Cloudflare zone records don't resolve, check TLD NS delegation before debugging the zone
- (1x) Duplicate Python method definitions: last definition wins, silently overwrites earlier ones
- (1x) When generated content disappears, trace through ALL post-processing steps - intermediate success ≠ final success
- (1x) Regex HTML manipulation is fragile - corrupts tags, creates malformed HTML - use BeautifulSoup with SVG extraction instead
- (1x) TDD with failing test first prevents spec drift - confirms you're testing the right thing before implementation
- (1x) Two-stage review (spec compliance first, code quality second) catches both functional and implementation issues
- (1x) Fresh subagent per task prevents context pollution - clean slate for each independent unit of work
- (1x) URL validation: mimic Windows 11 + Chrome (current stable) user-agent to avoid 403 from legitimate sites - update monthly

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
