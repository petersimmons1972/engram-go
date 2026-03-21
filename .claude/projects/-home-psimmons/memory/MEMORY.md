# Learning Index

**Last Updated**: 2026-03-21T00:49:40Z
**Session**: 20260320-204940

---

## Recent Activity (Last 7 Days)

- 2026-03-20: Implemented reviewer complaint dedup fix, Gates 33-35, wired EffectivenessTracker
- 2026-03-15: docs: add pre-flight protocol, test-after-edit rule, and /status skill

**Sessions**: - No recent session files found
**Uncommitted**: ⚠️  10 modified, 0 staged

---

## Infrastructure Health

**Cluster**: ✅ All 9 nodes ready | **Services**: ✅ All critical services healthy (10 checked)
**Warnings**: None detected

Health check: `~/bin/health-check.sh` | Recent failures: - timestamp: 2025-12-20T14:32:00Z | service: homepage
- timestamp: 2025-12-20T14:34:00Z
- timestamp: 2025-12-20T14:36:00Z

---

## Key Lessons

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

**Clearwatch**:
- (1x) LLM prompt rules need deterministic gate backstops - prompts are suggestions, code gates are guarantees
- (1x) Structured prefixes poison tokenization dedup - strip prefix, normalize entities before comparing
- (1x) "Architecturally complete but operationally dead" - verify feedback loops close end-to-end
- (1x) Multi-layer pipeline bugs must fix in dependency order - downstream bugs hide behind upstream

**General**:
- (2x) BeautifulSoup destroys SVG xmlns attributes - extract SVG first, process HTML, restore after
- (1x) Every plan needs explicit validation checklist, not just "verify it works"
- (1x) Cloudflare negative DNS cache lasts 1800s - CDN purge won't help, wait or use different record name
- (1x) TDD with failing test first prevents spec drift
- (1x) Fresh subagent per task prevents context pollution
- (1x) URL validation: mimic browser headers to avoid 403 from legitimate sites

---

## Topic Files

- Full lessons with context → memory/lessons-learned.md
- Active priorities and action items → memory/ACTIVE-PRIORITIES.md
- Clearwatch learning system architecture → memory/clearwatch-github-issues-work.md
- Clearwatch marketing content locations → memory/clearwatch-marketing-content.md
- Operation Triple Crown intel → memory/operation-triple-crown.md
- Homelab quick reference (triage, fixes, IPs, anti-patterns) → memory/homelab-quick-reference.md
- Homelab cert-manager patterns → memory/homelab-cert-manager.md
- Homelab K8s deployment patterns → memory/homelab-k8s-patterns.md
- URL validation patterns → memory/url-validation-patterns.md
- Chart regression case study (3 compounding bugs) → memory/chart-regression-2026-02-06.md
- HTML/SVG processing pattern → memory/html-processing-patterns.md
- LinkedIn API hard lessons → memory/linkedin-api-lessons.md
- Cloudflare Workers fetch capabilities → memory/cloudflare-workers-web-fetch.md
- Autoresearch reference (Karpathy patterns) → memory/autoresearch-reference.md
- Projects catalog → [PROJECTS-CATALOG.md](/home/psimmons/PROJECTS-CATALOG.md)

---

**Auto-updated at session start by `~/bin/generate-session-context.py`**
