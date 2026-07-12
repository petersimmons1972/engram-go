# Layer C B2 Supersession Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist safe, type-aware, confidence-gated supersession chains with assertion-time validity, an audit-only mode, and an adversarial corruption probe.

**Architecture:** Deduplication produces an ordered mutation plan. Status changes chain by `ObservedAt` with stable input-order ties; events remain independent. The backend persists each supersession atomically by inserting the linked replacement before retiring its predecessor in one transaction and one `UPDATE`; audit mode converts planned supersessions to unlinked inserts.

**Tech Stack:** Go, pgx/PostgreSQL, slog, testify, fake worker backend.

---

### Task 1: Deduplication semantics

**Files:**
- Modify: `internal/atom/dedup.go`
- Test: `internal/atom/dedup_test.go`

- [ ] Add failing tests for type-aware keys, confidence boundary/denial, assertion-time retirement, event coexistence, and ordered status chains.
- [ ] Run focused tests and confirm failures express missing B2 behavior.
- [ ] Implement the minimal ordered, type-aware, confidence-gated mutation planner.
- [ ] Run focused tests green and refactor without changing behavior.

### Task 2: Atomic worker persistence and audit mode

**Files:**
- Modify: `internal/atom/worker.go`
- Modify: `internal/db/postgres_atom.go`
- Modify: `cmd/engram/main.go`
- Test: `internal/atom/worker_test.go`
- Test: `internal/db/postgres_atom_test.go`

- [ ] Add failing worker/backend tests for insert-then-retire atomic persistence, loud failures, and dry-run plain inserts.
- [ ] Run focused tests and confirm expected failures.
- [ ] Extend the existing retirement pathway so PostgreSQL inserts the replacement then performs the sole retirement `UPDATE` in one transaction.
- [ ] Wire `ENGRAM_ATOM_SUPERSESSION_DRY_RUN`, default false, into worker configuration and structured audit logs.
- [ ] Run focused tests green under the race detector.

### Task 3: Corruption probe and documentation

**Files:**
- Test: `internal/atom/worker_test.go`
- Modify: `docs/configuration/atoms.md` (create if no atom configuration page exists)

- [ ] Add `TestB2CorruptionProbe*` seeded corpus coverage for true/false chains, mixed types, recurring events, idempotence, referential/retirement invariants, and dry-run row identity.
- [ ] Run the probe red before completing any missing behavior, then green.
- [ ] Document assertion-time validity, confidence threshold, type partitioning, atomic ordering, and first-production-run audit mode.

### Task 4: Verification and adversarial review

- [ ] Run `gofmt` and repository lint commands.
- [ ] Run `go test -race -count=1 ./internal/atom ./internal/db ./cmd/engram`.
- [ ] Run `go test -race -count=1 ./...` and the full linter.
- [ ] Obtain independent adversarial review and resolve all blocker/important findings.
- [ ] Re-run all affected and full verification commands after review fixes.
- [ ] Commit the implementation and probe artifact with the required Layer C/schema authorization details for dispatcher handoff.
