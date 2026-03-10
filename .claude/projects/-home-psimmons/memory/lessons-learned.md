---
Category: permanent
---
# Lessons Learned (Detailed)

## Homelab (ordered by usage count)

- (4x) Backup before modifying critical files - design phase IS implementation for docs
- (2x) MCP config lives in ~/.claude.json - use `claude mcp add`, never create mcp_servers.json
- (2x) Validate what the customer sees, not intermediate formats
- (1x) RWO PVCs need Recreate deployment strategy, not RollingUpdate
- (1x) Chainguard images need fsGroup for PVC write access (non-root UID 65532)
- (1x) cert-manager pods use 10.42.x.x overlay network, not 192.168.x.x - breaks Cloudflare IP filtering
- (1x) WordPress behind reverse proxy needs WP_HOME/WP_SITEURL/FORCE_SSL_ADMIN in wp-config.php
- (1x) CronJob not Deployment for periodic tasks - liveness probes kill sleep loops (exit 137, low memory = not OOM)
- (1x) Homepage issues are almost always network policy label mismatch
- (1x) Unifi DNS CNAME/A records: founder wants to teach proper method — ask for DNS CNAME lesson on next session. Kubelets resolve DNS independently of CoreDNS (use node /etc/resolv.conf), so CoreDNS NodeHosts alone doesn't help for image pulls. Unifi records should propagate through Pi-hole upstream chain automatically if configured correctly.
- (1x) Local DNS CNAME records break cert-manager DNS-01 challenges - use dnsPolicy: None + Cloudflare DNS

## Clearwatch Patterns

- (1x) **Gate 27 multi-layer bug chain** — 7 bugs total: Threshold mismatch (#2385) → char-budget edge case (#2388) → regex misses class-attr paragraphs (#2389) → boundary misses period-before-opening-tag (#2390) → split algorithm wrong split point (#2392) → boundary misses period-before-CLOSING-tag (split after closing tag, not before it — #2393) → single-pass algorithm can't produce 2 split points for 951-char paragraphs (#2394). Fix order matters: must fix ALL or Gate 27 still fails. Root cause was #2389+#2390 together — splitter never fired because regex didn't match any paragraphs. Bugs #2392-2394 were revealed only after #2389+#2390 were fixed (splitter now fires but splits wrong).
- (1x) **Python module caching in batch processes** — File fixes applied to .py files do NOT take effect in already-running Python processes. Must start fresh process after any code change. Never apply fixes mid-run and expect them to take effect.
- (1x) **Stage 3 timeout budget starvation** — 600s total budget shared across all sections. If last section is uncached, it gets starved (7s remaining → timeout). Fix: increase to 1200s. Symptom: "593s elapsed of 600s available, only 7s remaining".
- (1x) **HTML-aware sentence boundary detection** — Standard regex sentence splitting fails on HTML prose. Must track inside_tag state AND handle ALL period-before-tag transitions: (a) period before opening tag → split BEFORE tag (safe, new para opens its own tags), (b) period before CLOSING tag → split AFTER the closing tag (not rejected — keeping balanced HTML by consuming the full closing tag), (c) period before capital letter with no space → AI prose generation artifact, detect as boundary, (d) period in href/attribute → inside_tag=True, correctly ignored.
- (1x) **Single-pass split algorithm is insufficient** — When a paragraph needs 2+ split points (e.g., 951 chars with only 2 detected boundaries where boundary[0]=280 and boundary[1]=280+671), a single-pass algorithm that picks ONE split point leaves the second half still violating. Use a MULTI-PASS loop that iterates over remaining segments until every segment fits within MAX_CHARS.

- (1x) **EDR labor cost data structurally unavailable** — 3 weeks + ~$58 Claude compute confirmed: no independent research quantifies EDR admin labor for SMB 500-2,500 endpoints. All available data is vendor-commissioned Forrester TEI. CrowdStrike/SentinelOne intentionally omit absolute FTE baselines (% reductions only). The "50-70% of TCO is labor" axiom used across Clearwatch has ZERO external citations. 14 dollar-range estimates in tco-methodology.md are uncited. research_prompt.md falsely attributes "1.0-1.5 FTE/1K endpoints" to Forrester. See `domain-knowledge/reference/labor-cost-verification-memo.md`.
- (1x) **Chart width constant hardcoded for 12+ years** caused 30-40% right margin waste. User feedback repeated 150+ times before recognition. Root cause: SVG viewBox 900px didn't match modern web containers (1200px+). Lesson: When users repeat feedback >50 times, it's a system-wide constant/architecture issue, not individual bugs. Fix: increase CHART_DEFAULT_WIDTH to 1200px, update all 40+ chart methods.
- (1x) **CISO 3-Gate Chart Evaluation** - Technical validation (viewBox, fonts) is necessary but NOT sufficient. Charts must also pass: (1) Business value not insulting, (2) Shows vendor differentiation, (3) Provable with research. Applied portfolio-wide: 20/36 methods retired. See `docs/CISO-CHART-EVALUATION-FRAMEWORK.md`.
- (1x) **Chart fitness matrix was lying** - Marked KEEP with "0 violations" while data was identical for both vendors, pseudoscience, or fabricated. Automated validation catches rendering issues, NOT content quality.
- (1x) **Think portfolio, not single report** - Same bad charts appear in ALL 17 vendor pair reports. Chart retirements must apply portfolio-wide, not per-report.
- (1x) **Gates passing ≠ report ready — MUST visually inspect HTML AND PDF** — Automated gates catch structural issues (viewBox, chart count, word count) but miss formatting defects visible to any human reader: stray markdown `>` characters, missing spaces around bold tags (`.**Bold.**Next`), run-together sentences. After every report generation:
  1. Run Playwright chart screenshot extractor (`pipeline/validators/chart_screenshot_extractor.py`) to get per-chart screenshots
  2. Visually inspect the HTML report
  3. Read every page of the PDF using the Read tool (it renders PDF pages visually)
  4. Flag ALL defects found before declaring any status
  Gate pass is necessary but NOT sufficient. This was explicitly in the 2026-03-07 chart quality overhaul plan (Task 13, Step 5) and I skipped it. The plan said "any new defect = STOP, file bug, fix, re-run" — not "declare shipped."
- (1x) **Two systemic Stage 3 LLM output artifacts** — (a) Stray `>` from markdown blockquotes leak into HTML as literal text; (b) Missing spaces around `**bold**` at sentence boundaries produce `word.**Bold sentence.**Next`. Both must be caught by Stage 4 post-processing since LLM output can't be made 100% consistent. Added cleanup to stage_4.py.
- (1x) **Verdict-first report structure** — 3 separate reviewers (Ramsay ×2, CISO ×1) flagged that the report buries the verdict below technical deep-dives. For a $495 decision-insurance product, the exec summary must OPEN with a direct verdict ("CrowdStrike wins for X profile because Y"), not context-setting prose. Decision Framework section must appear immediately after exec summary, before technical analysis. Fixed in REPORT-GENERATION-PROMPT.md (#2497, #2500, #2501).
- (1x) **Audience-calibrate complexity** — CISO reviewer flagged integration scoring (11/15, 13/15) as too complex for 1-3 person IT teams. Numerical frameworks require consultant interpretation. Use binary format instead: "Works / Doesn't / Add-on Required". Always calibrate analytical frameworks to the target audience's capacity (#2498).
- (1x) **Close stale version feedback promptly** — 8 issues from v185 sat open while v189 was already generated. Old-version reviewer feedback should be closed as superseded when a new version ships. Re-open only if the same issue persists in the latest version.

## Behavioral Rules

- (1x) **ALWAYS file issues for ANY failure, regardless of origin** — When a smoke test or pipeline run surfaces failures, file GitHub issues for EVERY one. Never dismiss issues as "pre-existing" or "not caused by our changes." The issue tracker is how we learn and track. Filing after fixing is also required — the record matters for pattern detection. Dismissing issues silently violates the learning loop.

## General Patterns

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
