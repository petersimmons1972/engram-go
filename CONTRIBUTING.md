# Contributing to engram-go

This is a local-first memory system. It stores everything in a PostgreSQL database on your machine. No data leaves your network unless you explicitly ship it somewhere. There is no hosted service, no cloud sync endpoint, no telemetry. The complexity lives in the server — multi-signal retrieval, graph traversal, background workers — so that the user's workflow stays simple: store a thought, recall it later.

That philosophy shapes every design decision. Keep it in mind when you propose changes.

---

## Development Setup

### Go + PostgreSQL (MCP Server)

Start the stack:

```bash
docker compose up -d
```

This starts PostgreSQL. The server reads `DATABASE_URL` from the environment — defaults are in `docker-compose.yml`.

Run the test suite:

```bash
go test ./... -count=1 -race
```

The `-race` flag is non-negotiable. Most concurrency bugs in retrieval pipelines are invisible without it.

Run integration tests (requires a running Postgres):

```bash
TEST_DATABASE_URL=postgres://engram:PASSWORD@localhost:5432/engram go test ./internal/... -run Integration
```

Integration tests hit a real database. Unit tests do not. Keep them clearly separated — a test that relies on database state but is not named `*Integration*` will fail silently in CI when the test database is not present.

### Rust (Re-embedder)

The `reembed-rs/` directory contains the high-throughput re-embedding worker in Rust. It shares the same PostgreSQL backend but runs in its own container to isolate re-embedding concurrency from MCP request handling.

**Prerequisites:**
- Rust 1.75+ (`rustc --version`)
- Cargo (installed with Rust)

**Build:**

```bash
cd reembed-rs
cargo build --release
```

**Test:**

```bash
cd reembed-rs
cargo test --release
```

**Run locally:**

```bash
cd reembed-rs
cargo run --release -- \
  --database-url postgres://engram:PASSWORD@localhost:5432/engram \
  --litellm-url http://localhost:4000 \
  --engram-embed-model mxbai-embed-large
```

See `reembed-rs/README.md` for detailed Rust-specific documentation.

---

## Test-First Policy

Write the failing test before the first line of implementation.

This is not a religious commitment to process. It is how you verify that the test actually tests what you think it tests. If you write the implementation first and then add a test, you have a test that passes — but you have no evidence it would catch a regression. A test written against a failing implementation gives you that evidence.

The pattern: write the test, watch it fail with the right error message, write the minimum implementation to make it pass, refactor. If the test fails with the wrong error message, the test is wrong.

---

## New MCP Tool Requirements

Every new tool handler needs at minimum three tests:

1. **Happy path** — normal inputs, expected output
2. **Zero/empty input** — empty string, empty list, zero values; the handler should return a meaningful error, not panic
3. **One boundary condition** — limit at its maximum, a string at the character cap, a project name with special characters

These three tests catch the failure modes that appear in production within the first week. A handler with only a happy path test is untested for the cases that actually break.

See `internal/mcp/tools_test.go` for examples of what adequate coverage looks like in this codebase.

---

## Coverage Gate

CI enforces 60% minimum statement coverage on every PR. New files with lower coverage will fail the build.

Check locally before pushing:

```bash
go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out
```

60% is a floor, not a target. The safety tools rewrite (`safety.go`) demonstrates what adequate coverage looks like — read it if you are unsure what to aim for. The coverage gate exists because PR #162 shipped 20 untested functions into production. It stayed there for two weeks before anyone noticed.

---

## AI-Generated PR Policy

PRs submitted by AI agents, code generation tools, or LLM-assisted development must carry the `ai-generated` label and require three rounds of adversarial review before merge:

1. **Correctness Review** — boundary conditions, logic bugs, error handling, off-by-one errors, nil pointer dereferences, panic conditions
2. **Coverage Review** — at minimum 70% function coverage on new files, complete test for every exported API
3. **Structural Review** — fresh-eyes perspective, architecture alignment, naming clarity, no unnecessary complexity

All three reviews must return zero `severity/blocker` findings. `severity/nice-to-have` findings are tracked as issues but do not block merge.

**Why?** PR #162 (April 2026) was AI-generated. It passed syntax checks. It had:
- Four logic bugs (two off-by-one errors, one nil dereference, one panic on edge case)
- 20 of 24 functions with no tests
- Three functions shadowing standard library methods without clear intent

The adversarial review process caught all of them. The clean TDD reimplementation in commit `aaf56c6` is the documented reference for what adequate AI-submitted work looks like.

**Tools at contributor's discretion:** Use GitHub Copilot, Claude Code, LLM pair programming, or any other AI tooling. The requirement is review depth, not prohibition on AI use.

**For authors:** If your code is AI-assisted, label the PR `ai-generated` upfront. It ensures the right review depth and is faster than explaining it later.

---

## Commit Style

Subject line: short, present tense, imperative mood. Under 72 characters. No period.

```
Add BM25 fallback weight when Ollama is unreachable
Fix recency decay coefficient for memories older than 30 days
Refactor chunk splitter to respect sentence boundaries
```

Body: explain why, not what. The diff shows what changed. The commit message should explain the reasoning that is not visible in the diff. Reference issues with `Closes #N` or `See #N`.

```
Fix recency decay coefficient for memories older than 30 days

The original coefficient (0.1) meant a 30-day-old memory scored at
0.05 of its original weight. In practice this caused relevant but
older architectural decisions to fall below the result threshold
entirely. Changed to 0.01, which gives 0.74 at 30 days — still a
meaningful decay without dropping important memories off the results.

Closes #218
```

---

## What We Are Not Building

Engram is local-first. It will stay that way.

We are not building cloud sync. We are not building a hosted SaaS endpoint. We are not building a multi-tenant memory service. If your proposal requires your memories to leave your machine and land on a server you do not control, it belongs in a fork, not in this repository.

This is not a restriction imposed for technical reasons. It is a conviction about what the tool should be. AI assistants accumulate sensitive context — architectural decisions, security constraints, business logic, personal preferences. That context belongs on your machine, in your PostgreSQL instance, under your control. The moment it transits through someone else's infrastructure it is exposed to breach, subpoena, and policy changes you have no say in.

The GPL v3 license requires you to share modifications if you distribute derived work. If you build the cloud sync version, share it. But build it in a fork.

Everything in this repository is written for the case where the user wants to own their own data, run their own stack, and not depend on any service staying up.

---

## Questions

Open an issue. Label it `question`. We will answer it there so the answer is searchable for the next person who has the same question.
