# Learning Index

**Last Updated**: 2026-02-23T21:49:17Z
**Session**: 20260223-164917

---

## 🔥 Recent Activity (Last 7 Days)

- 2026-02-23: docs: Self-learning system redesign - eliminate 10 silent failures via diagnostics
- 2026-02-21: feat: migrate TrueNAS scripts to JSON-RPC 2.0 WebSocket API
- 2026-02-21: fix: health-check.sh - add TruNAS section, fix 3 bugs
- 2026-02-18: chore: restore 3 behavioral rules to home CLAUDE.md
- 2026-02-18: chore: optimize CLAUDE.md for token efficiency (~54% reduction)
- 2026-02-17: docs: update MEMORY.md - clearwatch legal framework session summary
- 2026-02-17: chore: remove misplaced REPORT-GENERATION-PROMPT.md from Pictures/claude
- 2026-02-17: docs: update MEMORY.md - clearwatch project, 3 new technical lessons
- 2026-02-17: docs: homelab lessons from 2026-02-17 cluster maintenance

**Recent Sessions**:
- SESSION-CONTEXT-OPTIMIZATION-COMPLETE.md
- SESSION-2026-02-14-GORDON-CISO-VERIFICATION-FINDINGS.md
- SESSION-2026-02-14-14-VARIANT-DEPLOYMENT.md
- SESSION-2026-02-13-PLAYWRIGHT-QA-INFRASTRUCTURE.md

**Uncommitted Changes**:
⚠️  6 modified, 0 staged

---

## ⚡ Infrastructure Health

**Cluster Status**: ✅ All 9 nodes ready
**Critical Services**: ⚠️  9 OK, 0 failed, 1 warnings
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

**Clearwatch Self-Learning System** (2026-02-23):
- (1x) Lessons captured but injected as abstract advice without examples → "virtual cul de sac" (present but ineffective)
- (1x) Silent failures hide broken learning system until token budget wasted (400+ docs with same error)
- (1x) Must detect recurrence BEFORE generation starts (pre-flight health check prevents token waste)
- (1x) Three provable diagnostics beat prompt engineering: pattern matching (B), effectiveness delta (C), error classification (D)
- (1x) Lessons need state machine: PROBATION → TESTING (prove helpful) → ACTIVE (prove non-regression) to be trusted
- (1x) Same root cause manifests differently across documents → pattern matching fails, need semantic error classification
- (1x) Lessons ordered by dependency but order discarded at injection → topological sort unused, breaking dependent lessons

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
