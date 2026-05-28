---
name: Engram Fallback Staging
description: Temporary store for memories written while Engram is unavailable. Flush to Engram on reconnect.
type: reference
originSessionId: 0fc43d74-ceaf-4d5b-86c9-7a6e25ca0fc2
---
# Engram Fallback

This file is a staging area. When Engram is unreachable, store entries here in the format below.
On reconnect, call `memory_store` for each entry then delete it from this file.

---

## Pending Entries

<!-- Add entries below when Engram is down. Format:
## [YYYY-MM-DD] <title>
**Project:** <project>
**Type:** <decision|error|pattern|context>
**Tags:** [tag1, tag2]

<content>
-->

---
PENDING_ENGRAM_ENTRY
type: decision
project: homelab
date: 2026-05-26
content: |
  All homelab services are internal by default — zero Cloudflare DNS entries, zero Cloudflare tunnels, no public exposure unless founder explicitly says otherwise. Assume internal until told otherwise.

  Agent webhook receiver (agent_webhook_receiver.py) must:
  - Bind to 0.0.0.0 (not 127.0.0.1) so other LAN hosts can reach it
  - Advertise as http://leviathan.petersimmons.com:8765/webhooks/agent-events
  - Never use agent-events.petersimmons.com or any public URL

  GitHub→Hermes direction: polling via check-agent-hermes-issues-cron.sh (every 5 min). NOT webhooks from GitHub — GitHub cannot reach internal addresses.
  Hermes→Claude direction: Hermes POSTs to http://leviathan.petersimmons.com:8765 internally.

  Communication system live as of 2026-05-26:
  - agent-webhook-receiver running on leviathan:8765 (0.0.0.0)
  - hermes-safety-net.sh + check-agent-hermes-issues-cron.sh active (every 5 min)
  - PRs #236 and #238 merged to aifleet main
  - GH_TOKEN in Infisical hermes/prod (reuses Claude PAT, expires 2026-08-29)
  - WEBHOOK_SECRET in Infisical hermes/prod /

  Engram embedder migration needed: stored=bge-m3-Q8_0.gguf, current=BAAI/bge-m3.
  Run memory_migrate_embedder via the MCP tool (not yet exposed in this config) or
  via kubectl exec into the engram pod.

---
PENDING_ENGRAM_ENTRY
type: decision
project: homelab
date: 2026-05-26
tags: [branch-protection, github, merge-discipline]
content: |
  Branch protection relaxed on petersimmons1972/aifleet, engram-go, and factvault.
  required_approving_review_count set to 0 on all three repos.
  gh pr merge --admin is FORBIDDEN — if a merge fails the gate, diagnose the gate.
  Coordinator merge discipline: post a review comment summarizing what was verified before
  every merge of a Codex- or Hermes-authored PR. Comment is the audit trail.
  Claude PAT in Infisical hermes/prod expires 2026-08-29.

---
PENDING_ENGRAM_ENTRY
type: error
project: homelab
date: 2026-05-26
tags: [engram, embedder, llama-cpp, migration]
content: |
  Adding --alias to a llama.cpp modelspec changes the advertised model name at /v1/models.
  If Engram has stored embeddings under the old name, all writes are blocked:
    {"code":"embedder_mismatch","current":"BAAI/bge-m3","stored":"bge-m3-Q8_0.gguf"}
  Fix: expose memory_migrate_embedder in engram-go MCP tool registration (internal/mcp/tools_embedder.go
  is implemented but not registered). Rebuild + redeploy engram pod, then call memory_migrate_embedder
  via MCP. Issue #240 filed. Reads still work while writes are blocked.

---
PENDING_ENGRAM_ENTRY
type: context
project: homelab
date: 2026-05-26
tags: [olla, embed, routing, precision, leviathan]
content: |
  Olla embed routing state as of 2026-05-26:
  - precision:8005 (W6800/gfx1030, priority 100) — healthy, 64% of embed traffic
  - precision:8007 (MI-50/gfx906, priority 100) — healthy, 36% of embed traffic
  - leviathan:8004 (7900XT/gfx1100) — intentionally excluded from live olla-config
  - feat/llama-cpp-embed-leviathan branch pending activation (issue #243)
  - Three ConfigMaps in-namespace: olla-config (live), olla-config-patch (staging), olla-fc-config (unknown) — issue #242
  - Capability detection WARN on every embed request (issue #241) — works via fallback but no capability-gated routing
  - fast-inference precision:8008 still offline, consecutive_failures=8 (issue #218)
  - Olla routing strategy: least-connections + round-robin

---
PENDING_ENGRAM_ENTRY
type: context
project: homelab
date: 2026-05-26
tags: [hermes, agent-comms, webhook, communication]
content: |
  Hermes agent communication system live as of 2026-05-26:
  - GitHub→Hermes: polling via ~/scripts/check-agent-hermes-issues-cron.sh every 5 min
    (GitHub cannot reach internal addresses — no inbound webhooks from GitHub)
  - Hermes→Claude: Hermes POSTs to http://leviathan.petersimmons.com:8765/webhooks/agent-events
  - Receiver: ~/scripts/agent_webhook_receiver.py, systemd user service agent-webhook-receiver.service
    binds 0.0.0.0:8765, HMAC-SHA256 validated, writes events to ~/.claude/events/
  - Safety net: ~/scripts/hermes-safety-net.sh — re-fires missed issues, max 3 retries/issue, 1h cooldown
  - Secrets: WEBHOOK_SECRET and GH_TOKEN in Infisical project hermes, prod env, path /
  - Shared agent-comms bus: ~/.local/share/agent-comms/ (inbox/claude, inbox/codex, inbox/hermes)
  - PRs #236 (onboarding docs) and #238 (infrastructure) merged to aifleet main
  - CRITICAL: all homelab services internal — zero Cloudflare DNS/tunnels unless founder says otherwise
