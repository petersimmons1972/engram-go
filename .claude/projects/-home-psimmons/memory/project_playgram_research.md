---
name: project-playgram-research
description: "playgram.ai project — Clearwatch research on Playgram vs AgentGateway, integration thesis and investor analysis"
metadata: 
  node_type: memory
  type: project
  originSessionId: 61abe33a-c811-4282-b7de-9323185f1b1b
---

Research project at `/home/psimmons/projects/playgram.ai/` comparing Playgram (team AI workspace) and AgentGateway (agency control plane).

**Why:** Clearwatch Research comparative analysis for AI infrastructure vendors. Thesis expanded from basic comparison to integration analysis + investor recommendation.

**Current state (2026-05-12):**
- `playgram-vs-agentgateway.md` — Markdown comparison (magazine-style, 7-commit reformat)
- `playgram-vs-agentgateway.html` — Economist/FT Weekend aesthetic HTML (EB Garamond + Barlow Condensed, cream palette)
- `playgram-vs-agentgateway-clearwatch.html` — Full Clearwatch brand HTML (navy + gold, Kufam + Inter, integration thesis + investor analysis)
- `playgram_vs_agentgateway.json` — Clearwatch dossier format JSON with full competitive landscape, integration analysis, investor gates

**Key finding:** No single vendor combines all six capabilities: team memory + multi-LLM + agent deployment + per-agent identity + audit chain + client billing. 12–18 month window before model providers close this gap.

**Visual-tester blocker:** GitHub issue #4784 — DNS resolution failure on worker nodes prevents `registry.petersimmons.com/clearwatch/visual-tester:latest` from pulling. Confirmed AgentGateway trust pages are genuinely 404 via Playwright fallback.

**How to apply:** When continuing this research, start by reading the JSON dossier for current state. New findings should update the JSON first, then regenerate the HTML.
