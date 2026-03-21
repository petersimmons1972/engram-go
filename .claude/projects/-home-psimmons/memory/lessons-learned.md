---
name: lessons-learned
description: Non-obvious lessons from past work — only items NOT already in CLAUDE.md or derivable from code
type: feedback
---
# Lessons Learned

## Homelab

- (4x) Backup before modifying critical files — design phase IS implementation for docs
- (2x) MCP config lives in ~/.claude.json — use `claude mcp add`, never create mcp_servers.json
- (2x) Validate what the customer sees, not intermediate formats
- (1x) RWO PVCs need Recreate strategy, not RollingUpdate
- (1x) Chainguard images need fsGroup for PVC write access (non-root UID 65532)
- (1x) cert-manager pods use 10.42.x.x overlay — breaks Cloudflare IP filtering
- (1x) WordPress behind proxy needs WP_HOME/WP_SITEURL/FORCE_SSL_ADMIN
- (1x) CronJob not Deployment for periodic tasks — liveness probes kill sleep loops (exit 137)
- (1x) Homepage issues are almost always network policy label mismatch
- (1x) Local DNS CNAME records break cert-manager DNS-01 — use dnsPolicy: None + Cloudflare DNS

## Clearwatch — Chart Quality

- (1x) **CISO 3-Gate Chart Evaluation** — Charts must: (1) show business value, (2) show vendor differentiation, (3) be provable with research. Applied portfolio-wide: 20/36 methods retired.
- (1x) **Think portfolio, not single report** — Same bad charts appear in ALL vendor pair reports. Retirements must apply portfolio-wide.
- (1x) **Audience-calibrate complexity** — Integration scoring (11/15) too complex for 1-3 person IT teams. Use binary: "Works / Doesn't / Add-on Required."
- (1x) **Incomparable tiers make bar charts unreadable** — When one bar is 3.7x others, remove the outlier, relegate to footnote.
- (1x) **EDR labor cost data structurally unavailable** — No independent research quantifies EDR admin labor for SMB 500-2,500 endpoints. "50-70% of TCO is labor" axiom has ZERO external citations.

## Clearwatch — Pipeline

- (1x) **LLM prompt rules need deterministic backstops** — CONTENT-RULES.md rules sometimes ignored under token pressure. Patterns with programmatic gates stopped recurring; patterns without gates came back across 10+ versions. **Fix: Gates 33-35 added 2026-03-20.**
- (1x) **Structured prefixes poison tokenization dedup** — Dedup on formatted strings wastes overlap budget on prefix tokens. Strip structural prefix and normalize entity names first. **Fix: extract_complaint() added 2026-03-20.**
- (1x) **"Architecturally complete but operationally dead"** — EffectivenessTracker was fully coded but never called. 1,666 lessons at runs_applied=0. Verify feedback loops close end-to-end, not just that each component exists.
- (1x) **Multi-layer pipeline bugs must fix in dependency order** — Gate 27: 7 bugs in chain. Downstream bugs only reveal after upstream fixes.
- (1x) **Research goes in K8s PostgreSQL, not just git** — Use `ResearchStore` or direct DB insert. Git files are a materialized view.

## General Patterns

- (2x) BeautifulSoup destroys SVG xmlns attributes — extract SVG first, process HTML, restore after. Pattern in code: `stage_5.py`.
- (1x) Every plan needs explicit validation checklist, not "verify it works"
- (1x) Cloudflare negative DNS cache lasts 1800s — wait or use different record name
- (1x) Cloudflare zone records don't resolve → check TLD NS delegation first
- (1x) TDD with failing test first prevents spec drift
- (1x) Fresh subagent per task prevents context pollution
- (1x) URL validation: mimic browser headers to avoid 403 — see memory/url-validation-patterns.md

## Agent Operations

- (1x) **Zero-XP commanders perform well on READ-ONLY audits** — 9 zero-XP generals delivered 112 issues. Code audits are ideal first-deployment tasks.
- (1x) **9-wide parallel audit with zone isolation: <3% duplication** — READ-ONLY + different audit angles minimizes duplication without coordinator.
- (1x) **Parallel agents can safely touch different lines of same function** — But flag shared functions and run full test suite after.
- (1x) **Zone briefs need file existence verification** — Pre-flight should `ls` every referenced file before deploying agents.
- (1x) **Third-party agent skills are attack surface** — Check for: auto-update mechanisms, recurring compute demands, identity linkage, untrusted content ingestion.
