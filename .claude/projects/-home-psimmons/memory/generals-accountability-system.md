---
name: Generals Accountability & Attribution System
description: Malus tracking, operation attribution, observer program, and design team accountability for the Generals multi-agent system — launched 2026-03-22
type: project
Category: project
---

Accountability system launched 2026-03-22, integrated into Clearwatch immediately.

**Core concepts:**
- **Malus**: Separate from XP (XP only goes up). Tracks bug accountability. 14-day half-life decay.
- **Malpractice**: Strategic (bad counsel) and operational (refused assigned role) — NO decay, permanent record.
- **Attribution**: Every operation records team roles (coordinator, implementer, designer, reviewer, observer) in service records.
- **Observers**: Absolute malus immunity. Earn saves bonuses. "Scouts, not soldiers."
- **Root cause drives allocation**: Implementation error, design flaw, coordination failure, bad upstream data, missed in review — each shifts the malus split differently.

**Founding precedents:**
- CISO (2026-03-20): Strategic malpractice → retired
- Eisenhower (2026-03-22): Operational malpractice + insubordination → 160 malus points (WARNING), career review

**Key files:**
- Ledger: `~/projects/generals/accountability/malus-ledger.yaml`
- Attribution: `~/projects/generals/accountability/attribution-index.yaml`
- Saves: `~/projects/generals/accountability/saves-log.yaml`
- Rules: `~/projects/generals/PROGRESSION-SYSTEM.md` Sections 10-12
- Report tool: `~/projects/generals/bin/malus-report.py` (--summary, --trends, --brief)
- Attribution checker: `~/projects/generals/bin/attribution-check.py`
- Visual docs: `~/projects/generals/docs/accountability-system.html`
- Clearwatch integration: `~/projects/clearwatch/docs/accountability-integration.html`

**Why:** Two incidents (CISO strategic malpractice, Eisenhower operational malpractice) showed the system had no backward-looking accountability — no way to trace bugs to responsible generals or distinguish honest mistakes from malpractice.

**How to apply:** Every Clearwatch operation must include Attribution in its service record. Bugs get malus entries with root cause analysis. Session start shows malus status via `--brief` flag.
