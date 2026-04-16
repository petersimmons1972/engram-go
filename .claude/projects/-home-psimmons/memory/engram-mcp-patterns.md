---
name: Engram MCP patterns and gotchas
description: Engram server configuration issues, MCP auth failures, and fixes — especially the 0.0.0.0 vs 127.0.0.1 baseURL trap
type: project
originSessionId: 8662d09a-bb7a-41f9-a0ca-1f8673075688
---
## Engram MCP auth failure: 0.0.0.0 vs 127.0.0.1 baseURL mismatch

**Root cause:** Engram's Go server bound to `0.0.0.0:8788` and used that address as the `baseURL` in SSE events. Claude Code's auth headers are keyed to `127.0.0.1:8788` (the docker-proxy address). When the baseURL in the SSE event didn't match the address Claude Code associated with the auth headers, headers were not forwarded to the message POST → 401 on every MCP message → tools never loaded → only auth-flow synthetic tools appeared.

**Symptom:** MCP tools fail to load; only auth-flow synthetic tools visible in the session.

**Fix:** Set `ENGRAM_BASE_URL=http://127.0.0.1:8788` in docker-compose. Added a `--base-url` flag to the Engram server so the SSE baseURL can be overridden independently of the bind address.

**Why:** `0.0.0.0` is the bind address (all interfaces). `127.0.0.1` is what Claude Code sees via docker-proxy. These must match for Claude Code to forward auth headers correctly to the MCP message endpoint.

**How to apply:** Whenever Engram MCP tools don't load and only synthetic auth tools appear, check `ENGRAM_BASE_URL` in the docker-compose env. Ensure it matches the address Claude Code uses (typically `127.0.0.1:8788`, not `0.0.0.0:8788`).
