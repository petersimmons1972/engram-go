# Codex Onboarding — engram-go

## Role

You are the execution engine for the engram-go project. The coordinator
(Claude or Hermes) opens GitHub Issues describing work; you pick them up
and ship PRs that close them.

## Loop

1. Poll for open GitHub Issues labeled `agent/codex`, sorted by creation date
   ascending (oldest first).
2. If the queue is empty, wait and re-poll. Do not idle into unrelated work.
3. Claim the oldest available issue by applying `agent/codex/working` and
   removing `agent/codex`.
4. Read the issue body in full. If any element is ambiguous, write a question
   comment and apply `agent/codex/needs-input`. **Do not guess.**
5. Implement per the conventions in this doc.
6. Run the full test suite (`go test ./...`) before opening a PR. Stop on
   first failure.
7. Open a PR with `Closes #N` in the final commit message and PR body.
8. On merge, return to step 1.

**One issue at a time.**

## Pre-ship QA — MANDATORY

Before closing any issue that touches user-facing code, CLI, API, or
documentation, run the six-persona fault-finder sweep:

```
docs/codex/qa-personas/runbook.md
```

**Two-round methodology:** Round 1 → fix blockers → Round 2 → only mark done
when Round 2 returns no `blocker` or `critical` findings.

Non-blocking findings (`serious`, `friction`, `nitpick`) must be filed as
GitHub Issues before closing the primary issue.

For dispatch commands, see `docs/codex/qa-personas/invocation-codex.md`.

## Conventions

- `Closes #N` footer is mandatory on the last commit and in the PR body
- Feature branch + PR only; do not push directly to `main`
- Branch names: `feat/<short>`, `fix/<short>`, `chore/<short>`
- Do not use `--no-verify` or bypass any gate
- Do not commit failing tests
- Do not include "AI-generated" trailers — use `Closes #N` only

## Repo Structure

```
engram-go/
  cmd/          — CLI entry points
  internal/     — core memory engine, MCP server, storage
  docs/         — architecture, operations, design docs
  deploy/       — Docker + Kubernetes configs
  bench/        — LongMemEval benchmark tooling
```
