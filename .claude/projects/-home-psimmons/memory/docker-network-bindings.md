---
name: Docker Port Bindings and Network Access
description: Common gotcha where localhost-only bindings prevent network connectivity
type: feedback
originSessionId: d46e5a7d-eb1a-4dfa-9c85-ebd5568cac87
---
## The Issue

Docker port bindings with `127.0.0.1` are localhost-only and unreachable from the network, including from other containers, even on the same host.

```yaml
# ❌ WRONG — unreachable from 192.168.0.98, leviathan.petersimmons.com, or other machines
ports:
  - "127.0.0.1:11434:11434"

# ✅ CORRECT — accessible from network
ports:
  - "0.0.0.0:11434:11434"

# ✅ ALSO CORRECT — accessible from specific interface
ports:
  - "192.168.0.98:11434:11434"
```

## Why This Matters

**Why:** This binding was present in engram's docker-compose.yml and broke network connectivity to Ollama. Appears to be a security-first default that wasn't updated when the service needed external access.

**How to apply:**
- When a containerized service needs to be reached from outside the container (including from other containers, other machines, or DNS names), use `0.0.0.0` binding
- When debugging "connection refused" errors, **always check the port binding first** — `docker ps` shows the binding clearly
- For internal-only services (like databases that should only be accessed by other containers in the same compose file), localhost binding is correct
