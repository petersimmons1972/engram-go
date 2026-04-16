---
name: Debugging Multi-Component Systems
description: Evidence gathering at component boundaries finds root cause faster than assumptions
type: feedback
originSessionId: d46e5a7d-eb1a-4dfa-9c85-ebd5568cac87
---
## The Pattern

When debugging network/connectivity issues across components (container → network → service):

1. **Don't assume the service is running** — verify with `docker ps`
2. **Don't assume the binding is correct** — `docker ps` shows port bindings; `127.0.0.1` is the gotcha
3. **Check each layer before proposing fixes:**
   - Layer 1: Container running?
   - Layer 2: Port binding correct?
   - Layer 3: Service listening?
   - Layer 4: Network reachable?

## Why This Matters

**Why:** "open-webui can't reach Ollama" could be DNS, network, firewall, service down, wrong port, or binding. Systematic evidence gathering at each layer immediately reveals which one.

**How to apply:** When debugging connectivity, run diagnostics at each component boundary **before** proposing any fix. In the Ollama case, `docker ps` alone showed `127.0.0.1:11434->11434` which directly identified the root cause — no need to check DNS, firewall, or service health.
