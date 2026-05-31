# Codex Prompt Memory Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce Codex prompt and memory overhead while preserving Engram persistent memory and local safety rules.

**Architecture:** Borrow Claude's compact structure: short top-level mandates, trigger/action tables, and references to detailed backing files instead of repeated historical narrative. Keep `MEMORY.md`, `raw_memories.md`, rollout summaries, and memory skills as searchable backing stores; optimize the always-visible surfaces and stale permission rules.

**Tech Stack:** Markdown prompt files, TOML Codex config, Codex prefix-rule config, Bash helper script.

---

### Task 1: Preserve Engram Behavior Before Secret Cleanup

**Files:**
- Create: `/home/psimmons/bin/load-engram-env`
- Modify: `/home/psimmons/.codex/config.toml`

- [ ] **Step 1: Add the missing Engram environment loader**

Create `/home/psimmons/bin/load-engram-env` with a `load_engram_env` function that returns immediately when `ENGRAM_API_KEY` is already set, otherwise reads `/home/psimmons/.config/engram/api_key`, and finally falls back to Claude's MCP header in `/home/psimmons/.claude/mcp_servers.json`.

- [ ] **Step 2: Remove hardcoded key from Codex config**

Delete `[mcp_servers.codex-tools.env] ENGRAM_API_KEY = "..."` from `/home/psimmons/.codex/config.toml`. Keep `[mcp_servers.engram] bearer_token_env_var = "ENGRAM_API_KEY"`.

- [ ] **Step 3: Verify**

Run:
```bash
bash -n /home/psimmons/bin/load-engram-env
python3 -c "import pathlib,tomllib; tomllib.loads(pathlib.Path('/home/psimmons/.codex/config.toml').read_text()); print('valid TOML')"
rg -n '80681617|ENGRAM_API_KEY = "' /home/psimmons/.codex/config.toml /home/psimmons/bin/load-engram-env
```

Expected: Bash syntax passes, TOML prints `valid TOML`, and `rg` finds no literal key assignment.

### Task 2: Compact Prompt and Rule Surfaces

**Files:**
- Modify: `/home/psimmons/AGENTS.md`
- Modify: `/home/psimmons/.codex/rules/default.rules`

- [ ] **Step 1: Rewrite AGENTS.md in Claude-inspired trigger/action style**

Keep the same safety semantics: wildcard delete prompts, Docker/database data protection, preservation-first rebuilds, and extra Engram protection.

- [ ] **Step 2: Prune stale allow-rules**

Keep reusable low-risk rules: read-only GitHub, build/test commands, Docker inspection/build/restart commands, Codex MCP add, and local search. Remove stale exact one-off rules, token-bearing curl helpers, historical LongMemEval debug commands, and broad push shortcuts.

- [ ] **Step 3: Verify**

Run:
```bash
rg -n 'rm -rf /tmp|Authorization: Bearer|ENGRAM_API_KEY|push|drain|uncordon|longmemeval|w6800-test' /home/psimmons/.codex/rules/default.rules
wc -l -c /home/psimmons/AGENTS.md /home/psimmons/.codex/rules/default.rules
```

Expected: no stale one-off or token-bearing allow-rules remain; files are smaller while safety policy remains explicit.

### Task 3: Rewrite Always-Visible Memory Front Door

**Files:**
- Modify: `/home/psimmons/.codex/memories/memory_summary.md`

- [ ] **Step 1: Compact `memory_summary.md`**

Use Claude's pattern: profile, hard operating rules, Engram memory rules, high-value lookup anchors, and a concise project index. Keep the instruction to search `MEMORY.md` first for exact anchors.

- [ ] **Step 2: Verify**

Run:
```bash
wc -l -c /home/psimmons/.codex/memories/memory_summary.md
rg -n 'Engram|MEMORY.md|No ingest|score-only|FactVault|Clearwatch|Proxmox|SearXNG|devops_secrets' /home/psimmons/.codex/memories/memory_summary.md
```

Expected: the summary remains operationally complete and materially smaller than the previous 12.8 KB.

### Task 4: Final Review

**Files:**
- Review all modified files.

- [ ] **Step 1: Diff review**

Run:
```bash
git -C /home/psimmons diff -- AGENTS.md .codex/config.toml .codex/rules/default.rules .codex/memories/memory_summary.md bin/load-engram-env docs/superpowers/plans/2026-05-26-codex-prompt-memory-optimization.md
```

- [ ] **Step 2: Final checks**

Run:
```bash
bash -n /home/psimmons/bin/load-engram-env
python3 -c "import pathlib,tomllib; tomllib.loads(pathlib.Path('/home/psimmons/.codex/config.toml').read_text()); print('valid TOML')"
wc -l -c /home/psimmons/AGENTS.md /home/psimmons/.codex/rules/default.rules /home/psimmons/.codex/memories/memory_summary.md
```

Expected: valid syntax/config and reduced prompt weight.
