---
name: Session handoff 2026-03-26
description: Major infrastructure session — platinum coin fix, Engram SQLite→PostgreSQL migration, issue triage and fixes
type: project
---

## Completed

### Platinum Coin
- Fixed white portrait disc covering capybara on reverse face (disc starts at z=relief instead of z=0, gray field fills reverse within portrait_r)
- Added "HOLLY SPRINGS" to reverse text ("FINE PLATINUM • HOLLY SPRINGS")
- Updated arc text spacing (char_angle 7→9, size 3.5→4.5)
- 3MF rebuilt, print was running — awaiting results
- Code review found 3 critical SVG issues: viewBox missing from split SVGs, capybara scale wrong (0.4 vs ~1.6), no visual verification

### Engram Infrastructure
- **Switched from SQLite to Docker Compose PostgreSQL** — `docker compose up -d` in ~/projects/engram
- All 68 memories confirmed in PostgreSQL across 10 projects
- Ports bound to 127.0.0.1 (fix #77)
- Ollama API key rotated, hardcoded secret removed from ~/.claude.json
- MCP config switched from stdio to SSE at localhost:8788 (**takes effect next session**)
- Engram connects directly to Ollama (not through Open-WebUI proxy)
- SQLite databases archived to ~/.engram-archive/
- SQLite backup cron removed (PostgreSQL handles this natively)
- Researched Hindsight patterns: RRF scoring, semantic dedup, confidence scoring (plan at ~/.claude/plans/valiant-hatching-hare.md)

### Issues Resolved (19 closed this session)
- #42 httpx in requirements.txt (already fixed)
- #47 Configurable Ollama model via ENGRAM_OLLAMA_MODEL
- #54 SSRF port allowlist
- #55 URL validation bypass (closed as not actionable)
- #56 Ollama auto-detect error logging
- #65 Compound index (project, memory_type)
- #69 Better embedding mismatch error message
- #76-78 Backup script, port exposure, API key (all resolved)
- #79-82 Drift detection, dual configs, WAL, rotation counter

### Other
- Installed Google Workspace CLI (gws 0.22.0) — auth not configured
- Updated CLAUDE.md: Engram fallback behavior rule
- Updated feedback rule: Engram indexes files, never replaces them

## Next Steps

### Ready to Fix (Sonnet)
- **#48** Asymmetric superseded memory detection — fix relationship direction logic
- **#61** PostgreSQL connection validation on init
- **#64** Rate limiting on memory_store endpoint
- **#66** Cascade deletes → FTS rebuild
- **#68** Jaccard dedup threshold too aggressive

### Needs Opus Planning
- **#45** Race condition in LRU engine cache eviction
- **#70** Engine cache bottleneck with single global lock
- **#73** No migration path between embedding providers
- **#74** Weak schema migration and versioning story
- **#67** No memory corruption detection or checksums

### Platinum Coin
- Fix SVG viewBox in split_capybara.py
- Verify capybara scale factor
- Visual verification before reprinting

### Deferred
- gws auth setup
- Branch `fix/quick-issue-batch` needs merge to main
