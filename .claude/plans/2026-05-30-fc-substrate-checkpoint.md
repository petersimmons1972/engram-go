# FC Self-Healing Substrate + Claude↔Codex — Session Checkpoint (2026-05-30)

## Mission
FC (Flight Controller) self-healing substrate for the homelab AI fleet + Claude↔Codex (C3) coordination. Claude coordinates/reviews/merges; Codex implements via GitHub Issues queue.

## SHIPPED this session
- **FC Phase 1 (#314) MERGED** to aifleet `main` (squash `5900402`); #314 closed. FailureClass/FailureEvent capture + watcher classifier (10-step precedence, disk-before-GPU-init #304), controller log-only.
- **Poller starvation fix** — `codex-poll.sh` iterates skip-and-continue instead of exit-on-first-no-checkout. Commit `2317edf`, PUSHED on homelab-config `fix/olla-embed-priority`. NEXT SESSION: verify Codex actually works the queue.
- **Protocol readiness gates** filed: homelab-config#80 (blocks Protocols 13–17). Adoption HELD until 3 gates green: (1) loop executes, (2) one feature E2E [PARTIAL: #337 merged], (3) truth-reporting reliable.
- **olla drift diagnosed**: live cluster runs OLD STATIC olla config (deprecated homelab-config/ai-fleet-controller fork); authoritative = aifleet `controller/deploy/olla.yaml` FC-discovery mode. Real drift = un-migrated cutover.

## OPEN draft PRs — MERGE IN ORDER (all off pre-#337 main; rebase each onto current main + prior; they conflict on handlers.go/types.go)
1. PR #338 (aifleet#306) — FC discovery nil-panic + endpoint-type. Branch `agent/claude/issue-306-fc-nilpanic`.
2. PR #339 (aifleet#325) — ModelSpec.Capabilities. Branch `agent/claude/issue-325-modelspec-capabilities`. Merge after #338 (both touch registry()/ModelSpec).
3. PR #340 (aifleet#315) — NetworkPolicy reconciler, recommend-only, WIRED IN. Branch `agent/claude/issue-315-netpol-reconciler`. Merge last.
Run `go test -race ./controller/internal/...` after each rebase.

## QUARANTINE
- Branch `agent/claude/fc-phase2-wip` (`79d109d`) — unauthorized FC Phase 2 work (kill-switch/policy-mode). Review for Phase 2; do NOT merge into Phase 1. (`cc0eb09` #334-auto-close dropped intentionally.)

## NEXT ACTIONS (priority)
1. Verify poller fix works (Codex picks up queued issues; olla/agent-gateway were starving).
2. Merge PR train #338 → #339 → #340 (rebase + test between).
3. Dispatch olla#36 (embedder endpoints missing capabilities) — was blocked on #325; unblocks once #339 merges.
4. olla FC-migration cutover (tasks #4/#5): after #306/#325/#315 + olla#36, cut live olla static→FC-discovery, verify routes, soak. EXPENSIVE/infra — founder go required.
5. Review fc-phase2-wip for FC Phase 2.
6. Retry Engram store once trunas DB out of recovery (was SQLSTATE 57P03 / recovery mode at checkpoint).

## PENDING FOUNDER DECISIONS
olla FC cutover go/no-go · homelab-config #65 (Engram token rotation), #69 (registry PVC 50Gi — verify if done), #78 (UniFi key rotation) · GitHub Pro (branch protection) · agent-gateway RSS (needs poller-host checkout to unstarve) · harness-port process-layer (not cleared) · clearwatch #4711 GOLD regen (expensive) · instinct olla-keys #20/#21/#22.

## ENVIRONMENT / HAZARDS
- `disableAllHooks: true` is in ~/.claude/settings.json but set mid-session — needs SESSION RESTART to go live (else ~8s hook tax/dispatch).
- trunas Postgres was in RECOVERY mode at checkpoint — Engram + engram-go DB affected; monitor.
- aifleet repo: 30 worktrees + 6 stashes (PRESERVE stash@{0} queue-health WIP). Cleanup advisable; aifleet-wt-306/315/325 removable after PRs merge.
- home repo ~59 uncommitted/deleted noise files (memory/feedback_*.md) — longstanding, ignore.
- INCIDENT: 2 commits appeared off-script on the #314 main-checkout branch (cc0eb09, 79d109d) — handled (dropped/quarantined); root cause undetermined; watch for agents committing to a main checkout instead of their worktree.
- Security: never reboot leviathan (FDE locked-screen), no Proxmox/TruNAS reboot w/o approval, oblivion=ARM images, bge-m3/1024-dim ONLY embedder, subagents commit / coordinator pushes.

## KEY FACTS
- olla paths: /internal/status (health), /olla/openai/v1/... (OpenAI proxy). Bare /v1/embeddings 404s by design.
- Embedders: mi50 precision:8007 (primary pri 200), w6800 precision:8005 (secondary pri 100). Both on precision host = single-host SPOF (accepted, #305). leviathan:8004 never existed.
- engram-go DB = external Postgres trunas.petersimmons.com:5434, table public.chunks vector(1024). Embedding backlog = ZERO (verified).

## HOW TO RESUME
1. Restart the session first (makes disableAllHooks live).
2. Confirm trunas DB healthy; retry Engram store.
3. Verify poller working (Codex queue draining).
4. Rebase + merge PR train #338→#339→#340.
5. Dispatch olla#36.
6. Plan olla FC cutover with founder.
