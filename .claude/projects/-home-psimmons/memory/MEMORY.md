# Learning Index

**Last Updated**: 2026-03-26T18:34:46Z
**Session**: 20260326-143446

---

## Recent Activity (Last 7 Days)

- 2026-03-26: docs: add global to-do list and database incident memory
- 2026-03-25: docs: Phase 2E — spawn eligibility table in AGENTS.md §7
- 2026-03-25: fix: add memory: project to gordon-ramsay agent frontmatter
- 2026-03-25: docs: add Emergency Override row to task-to-team lookup
- 2026-03-25: chore: update memory files and analyzing-companies skill
- 2026-03-25: docs: Groves leads content production pipeline
- 2026-03-25: docs(phase-2b): coordinator tool restriction doctrine in AGENTS.md §3
- 2026-03-25: docs: add global art direction rule to behavioral rules section
- 2026-03-25: docs: add Section 9 Designated Art Direction Team to AGENTS.md with full rule integration
- 2026-03-25: docs: test first — write failing test before implementation

**Sessions**: - No recent session files found
**Uncommitted**: ⚠️  1 modified, 0 staged

---

## Infrastructure Health

**Cluster**: ✅ All 9 nodes ready | **Services**: ⚠️  9 OK, 0 failed, 1 warnings
**Warnings**: None detected

Health check: `~/bin/health-check.sh` | Recent failures: - timestamp: 2025-12-20T14:32:00Z | service: homepage
- timestamp: 2025-12-20T14:34:00Z
- timestamp: 2025-12-20T14:36:00Z

**Generals Accountability**: Malus: Eisenhower 160.0 (WARNING) | 1 commander(s) tracked

**J-2 Intelligence**: J-2 INTEL [2026-03-26T16:00]: Pods In Error State (WARNING, 5 patrol(s))

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

- DNS: use Cloudflare API for *.petersimmons.com, not Pi-hole/SSH → memory/feedback-dns-use-cloudflare.md
- Homelab quick reference (triage, fixes, warnings, anti-patterns) → memory/homelab-quick-reference.md
- cert-manager patterns → memory/homelab-cert-manager.md
- K8s deployment patterns → memory/homelab-k8s-patterns.md
- Incident summaries → memory/homelab-incidents.md
- URL validation patterns → memory/url-validation-patterns.md
- Chart regression analysis → memory/chart-regression-2026-02-06.md
- HTML processing patterns → memory/html-processing-patterns.md
- Projects catalog → [PROJECTS-CATALOG.md](/home/psimmons/PROJECTS-CATALOG.md)
- Generals Accountability System → memory/generals-accountability-system.md

---

**Auto-updated at session start by `~/bin/generate-session-context.py`**
