---
name: Go is the default language for new tooling
description: User preference — new services, CLIs, shippers, and cluster tooling default to Go, not Python
type: feedback
originSessionId: 3f40ebc5-5dc9-4d04-b5ab-efdbad955dfa
---
Default to Go for any new service, CLI, shipper, daemon, or cluster workload unless there's a specific reason not to.

**Why:** Active Clearwatch migration is toward Go. Static binaries produce clean K8s artifacts (scratch/distroless containers, ~15MB, no base-image CVE churn, no pip/venv). Faster startup matters for CronJobs. Unifies the toolchain across the stack.

**How to apply:**
- Proposing a new service, shipper, CLI, daemon → Go first.
- Only reach for Python if: (a) the task is heavily ML/data-science, (b) a Python-only library is load-bearing, or (c) it's a one-off script <50 lines where startup cost dominates.
- Bash still fine for genuinely shell-shaped work (hooks, glue, 20-line scripts with flock/pipes).
- Rust only if performance or memory safety is load-bearing beyond what Go provides.
- When presenting language choice in brainstorming/design, lead with Go and justify alternatives, not the reverse.
