# bench-roster.md — DEPRECATED

This file is no longer maintained.

- **Live roster (human-readable):** [ROSTER.md](./ROSTER.md) — auto-generated
- **Canonical machine roster:** [roster.jsonl](./roster.jsonl) — one general per line
- **Curated guidance:** [roster-guidance.md](./roster-guidance.md) — hand-edited prose appended to ROSTER.md by the generator
- **Source of truth:** `profiles/*.md` frontmatter
- **Activation:** edit a profile's `status:` field, then run `bin/sync-to-claude-agents.sh` (which runs `bin/generate-roster.py` first)
