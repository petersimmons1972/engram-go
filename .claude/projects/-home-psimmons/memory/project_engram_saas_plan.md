---
name: engram-go SaaS expansion — P1 Infrastructure design record
description: Full architectural decisions from 2026-04-28 planning session. Covers language strategy, DB topology, embedding model, security, scaling, and critical risks.
type: project
originSessionId: 2026-04-28
---

Full plan file: `~/.claude/plans/last-session-projects-engram-go-we-declarative-cosmos.md`

**Why:** Peter wants to productize engram-go as a multi-tenant SaaS platform (first target: yourai.com on AWS), starting from homelab validation on TruNAS.

---

## Locked Decisions

| Decision | Detail |
|----------|--------|
| **Language** | Go core stays (35k lines, 225 files, battle-tested). All new standalone services in Rust. 12–18mo migration horizon. |
| **DB topology** | Always external to app cluster. Homelab: TruNAS port 5433 (5432 taken by postgres-consolidated). AWS: Aurora Serverless v2 pending POC; ECS+EBS fallback. |
| **DB image** | Chainguard + pgvector, pg17. Self-built for now; Chainguard enterprise contract post-revenue. |
| **Embedding dims** | **1024 dims fixed.** qwen3-embedding:8b on Leviathan (7900 XT 20GB, 4.7GB Q4). Voyage-3 for SaaS prod. Cohere embed-v4 for AWS Bedrock. All providers output 1024 — no schema migration on swap. |
| **SSL/TLS** | sslmode=disable locally (DNS chaos). sslmode=verify-full at TruNAS cutover + AWS. Binary transition. |
| **DB roles** | 3 Infisical-managed roles: engram_app (RLS enforced, runtime), engram_migrate (RLS bypass, one-shot Job), engram_monitor (SELECT pg_stat only). |
| **PgBouncer** | Transaction mode. SET LOCAL works correctly with transaction mode. K8s Deployment (homelab), ECS sidecar (AWS). |
| **Audit log** | Embedded (not external pipeline). Postgres record atomic with operation. S3 Object Lock WORM async post-commit. |
| **API key billing** | BYOK now; billing_model enum for platform-absorbed/reseller later — no schema migration. |
| **Documentation** | Two-voice mandatory: Veteran (technical) + Business (plain language). |

---

## TruNAS Current State (confirmed 2026-04-28)

- Port 5432: postgres-consolidated running `postgres:18.3-trixie`, NO pgvector (ADR-5)
- Port 5433: reserved for engram dedicated container
- Port 5432 reachable from this machine confirmed. K8s cluster pod reachability not yet verified.
- 252GB RAM, load ~1.24 (idle), 60TB RAIDZ2

---

## Project Breakdown

| Project | Scope | Gate |
|---------|-------|------|
| **P1: Infrastructure** | TruNAS container, AWS Aurora POC, embedding migration | Current focus |
| **P2: Multi-tenancy** | owners/api_keys, RLS, BYOK billing schema | After P1 |
| **P3: Audit Layer** | Embedded audit, S3 WORM pipeline | After P2 |
| **P4: yourai.com** | API surface, key management | After P2 |
| **P5: K8s SaaS deployment** | engram namespace, Traefik, cert-manager | After P1+P2 |
| **P6: HIPAA** | Compliance certification | After P3 |

---

## Critical Risks (Gregory Richardson questions)

1. **RLS SET LOCAL + RDS Proxy** — may silently leak cross-tenant data. Must POC before any tenant on Aurora.
2. **GDPR right-to-erasure vs S3 WORM** — direct legal conflict. Need lawyer opinion before first EU customer.
3. **Embedding model versioning** — no model tracking in current schema. Add embedding_model_id before Phase 2.
4. **768→1024 dim migration** — must run BEFORE Phase 2. Reembed locally first, then pg_dump/restore to TruNAS.
5. **RLS NULL behavior** — what does policy return when app.current_owner_id is not set? Test explicitly.
6. **Tenant isolation not in CI** — must be continuous regression test, not a one-time POC.
7. **No pricing model** — cost stages documented but no revenue model. Validate unit economics before Stage 1.
8. **Incident response plan** — missing entirely. Required before first customer.
9. **Data residency** — no EU region planned. Blocks EU enterprise customers.

---

## Aurora POC Required Tests

- [ ] SET LOCAL + RDS Proxy isolation (CRITICAL)
- [ ] pgvector version ≥ 0.5.0 (HNSW)
- [ ] HNSW build time vs homelab baseline
- [ ] Vector query p50/p95 benchmark
- [ ] work_mem tuning effectiveness
- [ ] 72hr soak at Stage 1 simulated load

---

## Scale Stages (AWS)

| Stage | Customers | Monthly DB cost | Move-up trigger |
|-------|-----------|-----------------|-----------------|
| 0 | 0 (dev) | $0–15 | First paying customer |
| 1 | 1–50 | $75–200 | DB bill >$200 OR p95 >200ms |
| 2 | 50–500 | $400–1,200 | DB bill >$1,000 OR conn wait >10ms |
| 3 | 500+ | Model at Stage 2 top | Reserved Instance analysis |

---

## Security Investment Layers

| Layer | Trigger | Tools |
|-------|---------|-------|
| 1 | Day 0 | Chainguard, Infisical, RLS, embedded audit, S3 WORM, Trivy |
| 2 | First customer | Infisical Cloud, Falco runtime |
| 3 | First regulated data | DSPM (Sentra/Cyera), MDR (Expel/Arctic Wolf), DLP (Nightfall at ingestion) |
| 4 | $100K ARR | XDR (SentinelOne), annual pen test |
| 5 | $1M ARR | SOC 2 Type II, full MSSP |

Infisical path: self-hosted (homelab) → Infisical Cloud (Stage 1) → AWS Secrets Manager (Stage 2+ for tenant BYOK at scale, has BAA for HIPAA).

---

## Deferred: Embedding Model Versioning (DDT-1)

Full design in plan file. Key points:
- EmbeddingProvider trait (Rust) — model_name, dimensions, embed(), max_batch_size()
- Model registry table (migration ~023): embedding_models(id, provider, model_name, dimensions, is_current)
- Lazy re-embed: background worker, graceful degradation during migration
- Dimension change requires table rewrite — plan as maintenance window
- Dedicated design session needed before Phase 2 ships

**How to apply:** Next session on engram-go P1 — recall this, resolve P1 remaining open items, then execute Phase 1 with worktree.
