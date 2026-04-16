---
name: lessons-learned
description: Non-obvious lessons from past work — only items NOT already in CLAUDE.md or derivable from code. Details in Engram; this file is the quick index.
type: feedback
Category: reference
originSessionId: e809ff6a-dd7c-46bc-9888-929ad54f775c
---
# Lessons Learned (compressed index)

Full detail in Engram: `memory_recall("<topic>", project="<project>")`. This file keeps only the one-line kernel.

## Homelab
- RWO PVCs need `Recreate` strategy, not RollingUpdate
- Chainguard images need `fsGroup` for PVC write (non-root UID 65532)
- cert-manager pods use 10.42.x.x overlay — breaks Cloudflare IP filtering
- CronJob for periodic tasks, not Deployment (liveness probes kill sleep loops → exit 137)
- Homepage failures are almost always network policy label mismatches
- Local DNS CNAMEs break cert-manager DNS-01 — use `dnsPolicy: None` + Cloudflare DNS
- `kubectl apply` on ConfigMaps >262144B fails (annotation limit) — use `--server-side=true --force-conflicts`
- MCP config lives in `~/.claude.json` — use `claude mcp add`, never hand-edit `mcp_servers.json`
- Backup before modifying critical files — design phase IS implementation for docs

## Clearwatch — Pipeline
- LLM prompt rules need deterministic backstops — rules without programmatic gates come back every 10+ versions
- Structured prefixes poison tokenization dedup — strip prefix + normalize entity names before hashing
- "Architecturally complete but operationally dead" — when disabling a mechanism, grep everything that depends on its output
- Multi-layer pipeline bugs fix in dependency order — downstream bugs only reveal after upstream fixes
- Most pipeline failures are stochastic — re-run once before modifying code; only fix deterministic failures
- Structural data gaps don't resolve on re-run — identify structural vs stochastic first, re-runs waste compute
- Contention retry must select a different resource (PID-mod selection collides under N+1 callers)
- Contradictory rules across files cause LLM to generate malformed output — grep overlapping concepts
- Blocking gates should not block on recoverable structural warnings — warn and continue, reserve hard blocks for fatal
- CLAUDE.md rules are invisible to Stage 3 LLM — mirror founder rules in `domain-knowledge/`
- Regen anti-pattern: regen→feedback→fix→regen never closes — categorize must-fix-before-regen vs regen-only FIRST
- Stage 6 gates are pure Python on saved artifacts — replay in ~60s via `bin/validate-existing.py`
- Dossier is the fabrication vector — Stage 3 trusts dossier unconditionally; forbidden terms or invented facts in manifest entries reproduce verbatim in prose regardless of domain-knowledge rules. Fix = dossier surgery, not feedback constraints.
- QUALITY-GATE-FAILED HTML_NOT_FOUND = Stage 6 hard block (gate deleted HTML before Stage 7 ran). DELIVERY_CHECKLIST_FAILED = passed Stage 6 + grading (A-), blocked by post-grade delivery checklist. These are distinct failure modes with different fixes.
- Gate 36 (non-flagship tier) is triggered by dossier manifest entries naming forbidden product tiers — check dossier before investigating Stage 3 prompt compliance

## Clearwatch — Chart Quality
- CISO 3-Gate: business value, vendor differentiation, provable with research. Portfolio-wide, not single-report
- Audience-calibrate complexity — 1-3 person IT teams need binary charts, not 15-point scoring
- Incomparable tiers break bar charts — remove outlier, footnote it
- EDR labor cost data is structurally unavailable — no external citations quantify SMB admin labor

## Go / pgx Patterns
- pgx fixed-position scanner + extra SELECT col = silent scan mismatch (never `Values()` then `rowToMemory()` on same row)
- mcp-go ≥ v0.45: use `req.GetArguments()`, not `req.Params.Arguments` (typed `any`)
- Timing-safe bearer auth: `crypto/subtle.ConstantTimeCompare` — plain string compare leaks timing
- Bounded `httpServer.Shutdown` via `context.WithTimeout` — otherwise blocks forever
- Walk error policy: separate nil check from IsDir (`if err != nil || d.IsDir()` silently eats walk errors)
- Python hash compatibility requires exact normalization: TrimSpace → ToLower → `\s+`→single space → sha256 → hex[:32]
- RE2 (Go `regexp`) has no backreferences or lookahead — split `<h([23])>` into two regexes; use sentinel chars for lookahead workarounds
- `regexp.MustCompile` inside loops = panic risk + performance regression — compile at package level always
- `rune(s[i])` is a byte cast, not a rune decode — use `utf8.DecodeRuneInString(s[i:])` for UTF-8 safety

## Agent Operations
- Always verify issue count matches reality — `gh issue list` with label filters undercounts, use unfiltered `--json number --jq 'length'`
- Zero-XP commanders perform well on READ-ONLY audits — 9-wide parallel with zone isolation gives <3% duplication
- Parallel agents can touch different lines of same function — flag shared functions, run full suite after
- Zone briefs need file existence verification — pre-flight `ls` every referenced file
- Third-party agent skills are attack surface — check auto-update, recurring compute, identity linkage, content ingestion
- Fresh subagent per task prevents context pollution

## Security Posture
- "Minimal change / accept risk" security is how breaches happen (Home Depot 2014 pattern) — NetworkPolicies, RBAC, supply chain are non-negotiable foundations

## Analysis Methodology
- Use competitor code as a lens, not a blueprint — the vocabulary is the value, not the patterns
- Prescriptive vs descriptive profiling are fundamentally different — don't merge them

## General Patterns
- BeautifulSoup destroys SVG xmlns — extract SVG first, process HTML, restore after (pattern: `stage_5.py`)
- Python 3.10 f-strings cannot contain backslashes — extract to variable. Fixed in 3.12
- HTMLParser misreads `<TAB>` inside `<pre>` — escape as `&lt;TAB&gt;`
- `urllib.request.urlopen()` default UA gets 403 from Cloudflare — use `Mozilla/5.0` UA header
- Every plan needs an explicit validation checklist, never "verify it works"
- TDD with failing test first prevents spec drift

## Writers Phase 1 (2026-04-14)
- argparse at module scope causes `SystemExit(2)` on import — always guard `parse_args()` behind `if __name__ == "__main__"`
- When replacing hardcoded maps with registry/dynamic lookups, verify what keys real data files use — not what the old code expected (hyphenated full-names vs bare keys caused silent data loss)
- YAML multi-writer reviewer attribution: line numbers in multi-entry files mislead — grep for the writer's key to confirm which entry the reviewer actually meant
- Updating one step in a multi-step pipeline: scan all downstream steps for references to the same data — labels like "full content" become misleading after upstream changes

## Signing Discipline
- Sign the code manifest, not just docs — Ed25519, one key per project, one verify script
- v1 → audit → v2 is the publication discipline — v1 immutable, v2 primary, both accessible
- Workflow case studies CAN describe method without publishing prompts (trade secret = content, not structure)
