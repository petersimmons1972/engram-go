# Advisory Protocol — Full Detail

Reference doc for `~/CLAUDE.md`. Loaded on demand when spawning `opus-advisor` or when ADV.1–ADV.5 triggers fire.

---

## Opus Escalation Triggers (ADV.1–ADV.5)

Spawn `opus-advisor` before any of:

- **ADV.1 — Architecture fork:** 2+ approaches with meaningfully different long-term consequences
- **ADV.2 — Infrastructure change:** K8s manifests, DNS, cert-manager, Cloudflare, storage
- **ADV.3 — Large refactor:** restructuring a module/class/boundary >100 lines
- **ADV.4 — Stuck on reasoning:** same root cause failed twice AND the failure is logic, not capacity
- **ADV.5 — Irreversible + ambiguous:** can't easily undo and the right answer isn't clear

## Opus Briefing Format

1. **Decision** — one sentence
2. **Options** — A, B, (C) with one-sentence tradeoffs
3. **Lean** — current preference and source of uncertainty
4. **Context** — file paths, constraints

Wait for RECOMMENDATION before proceeding.
