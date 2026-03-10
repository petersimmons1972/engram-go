# Learning Index

**Last Updated**: 2026-03-10T16:46:08Z
**Session**: 20260310-124608

---

## Recent Activity (Last 7 Days)

- 2026-03-10: feat: add 3 TDD-validated skills for operational efficiency
- 2026-03-05: docs: update skills validation week 1 data (2026-03-05)
- 2026-03-05: docs: update ACTIVE-PRIORITIES.md and MEMORY.md - complete chart restoration
- 2026-03-05: docs: update ACTIVE-PRIORITIES - mark chart spec violations as COMPLETE
- 2026-03-05: docs: document 3 critical bug fixes (pricing data loss, index crash, hardcoded values)
- 2026-03-05: docs: document bug fixes completion - 36 test failures fixed, all tests passing (3334/3334)
- 2026-03-05: docs: update ACTIVE-PRIORITIES - Clearwatch chart cleanup complete (3 failing charts removed)
- 2026-03-05: docs: refocus on Clearwatch as independent project - remove security-intelligence-business scope
- 2026-03-05: docs: update ACTIVE-PRIORITIES - Phase 1 chart removal complete (5 charts removed, 907 lines)
- 2026-03-05: docs: update ACTIVE-PRIORITIES - pricing verified, chart audit complete (34 charts analyzed)

**Sessions**: - No recent session files found
**Uncommitted**: ⚠️  23 modified, 0 staged

---

## Infrastructure Health

**Cluster**: ✅ All 9 nodes ready | **Services**: ⚠️  9 OK, 0 failed, 1 warnings
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

## Topic Files

- Homelab quick reference (triage, fixes, warnings, anti-patterns) → memory/homelab-quick-reference.md
- cert-manager patterns → memory/homelab-cert-manager.md
- K8s deployment patterns → memory/homelab-k8s-patterns.md
- Incident summaries → memory/homelab-incidents.md
- URL validation patterns → memory/url-validation-patterns.md
- Chart regression analysis → memory/chart-regression-2026-02-06.md
- HTML processing patterns → memory/html-processing-patterns.md
- Projects catalog → [PROJECTS-CATALOG.md](/home/psimmons/PROJECTS-CATALOG.md)

---

**Auto-updated at session start by `~/bin/generate-session-context.py`**
