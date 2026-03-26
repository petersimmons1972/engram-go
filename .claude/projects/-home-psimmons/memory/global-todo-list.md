---
name: Global To-Do List
description: Periodic tasks and checks to revisit across all projects
type: project
---

# Global To-Do List

Items to check periodically when requested. Add with date discovered and context.

## Clearwatch

- **Check linting rules** (discovered 2026-03-26)
  - Review report linting rules for quality gates
  - Context: General quality maintenance for Clearwatch reports
  - Status: Pending user request to review

## Infrastructure

- Monitor cert-manager DNS-01 challenges (known issue with local CNAME records)
- Verify Cloudflare negative DNS cache purges (1800s TTL)
- Check K8s node readiness and PVC status

## Other

(None currently)

---

**How to use:** User periodically asks "give me the global to-do list" and this file is retrieved and summarized.
