> **IMPORTANT**: Before spawning ANY agent, check the full roster above — not just the inline quick-reference in ~/AGENTS.md.
> A zero-XP commander whose specialization fits the task should be PREFERRED over a high-XP commander to build bench depth.

## Default Fault-Finder Personas (always-active QA team)

> **Purpose**: A standing six-persona team for pre-ship QA. Each is a read-only adversarial reviewer playing a specific user role. Deployed together via **Pattern 6: Persona Fault-Finder Sweep** in `spawn-patterns.md`.
> **Source**: Substack §2 — "Subagents Are a Necessity." Adopted 2026-04-24.
> **Cost discipline**: model `sonnet`, all six in parallel — well under the Opus 3-concurrent guardrail.

| Name | Lens | When to Use |
|------|------|-------------|
| Skeptical Staff Engineer | side effects, hidden deps, trust assumptions | Any code change with non-trivial scope; sanity-check before merge |
| Security Reviewer | secret leaks, permission boundaries, RCE surface | Any change touching auth, config, input handling, or shell execution |
| New Maintainer | docs clarity, onboarding path, source-required gaps | Repo restructures, README rewrites, onboarding-doc updates |
| Heavy CLI User | command consistency, composability, idempotency | CLI design changes, new subcommands, flag-rename PRs |
| Operator / SRE | observability, failure recovery, alerting | Production-impacting code; new services; cron/scheduled jobs |
| Docs-First Newcomer | README accuracy, example correctness | Documentation updates; example code; tutorial changes |

**Always include all six** for a full sweep. They are designed to operate as a team; running a subset defeats the multi-axis opposition design. For lighter checks, prefer a single-persona spawn or use existing validators (Ramsay, Rickover, Spruance, zero-context-reviewer).
