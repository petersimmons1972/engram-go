---
name: Locked Shields 2026 — Project Overview
description: Canonical state of the Locked Shields 2026 project — URLs, signed artifacts, data sources, and critical rules
type: project
Category: project
originSessionId: f13be8e1-9cae-4933-afaa-71fe701071a8
---
# Locked Shields 2026 — Canonical Project State

NATO CCDCOE live-fire exercise. Peter on US Blue Team. Repo: `~/projects/locked-shields/`. Public artifacts: **https://www.petersimmons.com/locked-shields/**.

## Published signed documents

| URL slug | SHA prefix |
|---|---|
| `red-team-prediction.html` | `b22f3ce9…bd335` |
| `blue-team-counterprediction.html` (v1 archive) | `581dca0f…1b245` |
| `blue-team-counterprediction-v2.html` (primary) | `4915c7a6…c47f` |
| `methodology.html` | `d7af20e4…a8880` |
| `case-study-multi-agent-workflow.html` | `a80dc834…5221` |

**Ed25519 fingerprint:** `83b7538d2d5baef8ab72e15358e19d6c1c4232233c4395eb0191c784906db5ac`
**Signing key:** `~/.config/locked-shields/private_key.pem` (chmod 600, local-only — NOT in Infisical)

## Dataset

`research-db.petersimmons.com:30432/locked_shields` (NodePort) — 8,728 docs, 18,972 edges, 39MB.
Credentials: Infisical `/locked-shields/postgres/` (project `f49c5b01-4bd1-4883-afbd-51c1fef53a2f`, env `prod`).

## Critical rules (immutable)
1. Signed documents are immutable — new versions get new filenames
2. Agent prompts are trade secrets — never publish Murrow brief, role definitions, methodology prompts
3. Only predictions MUST be signed; methodology/verify pages optional
4. Pre-commit hook at `.git/hooks/pre-commit` blocks credentials

## Workflow (army team up)
Layton (intel) → Rommel (visual) → zero-context reviewer → Murrow (writer) → Ramsay (QA 18-item pre-flight). Quality bar set 2026-04-07.

## Bluesteel (defensive kit, built 2026-04-08)
`tools/bluesteel/` — PowerShell fire-against-machines deployment kit, 17 files. Eight modules (Audit, HardenCredentials, HardenLateral, HardenAD, DeploySysmon, DeployAuditPolicy, Hunt, Rollback). Default Audit (read-only); DCs require `-AllowDC`; destructive actions opt-in twice. Sysmon.exe NOT bundled — download separately. Details in `tools/bluesteel/README.md`.

## Open follow-ups
- Post-exercise re-grading (CONFIRMED / REFUTED / INCONCLUSIVE)
- Case study comms piece about the workflow

## Verify signature
```bash
cd ~/projects/locked-shields
curl -sA "Mozilla/5.0" https://www.petersimmons.com/locked-shields/red-team-prediction.html > /tmp/p.html
curl -sA "Mozilla/5.0" https://www.petersimmons.com/locked-shields/red-team-prediction.html.sig > /tmp/p.html.sig
python3 tools/signing/verify_document.py /tmp/p.html --pubkey tools/signing/public_key.pem
```
