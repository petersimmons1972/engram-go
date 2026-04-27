# Engram-Go Multi-Tenant Platform — Implementation Candidate Prompt

## Context
This is a ready-to-execute implementation brief for the engram-go multi-tenant transition. The full design was developed in a brainstorming session on 2026-04-20 and the spec is committed at:
`docs/superpowers/specs/2026-04-20-multi-tenant-design.md`

## What This Transforms
engram-go from: single-tenant Docker Compose memory server (one bearer token, project-based isolation)
engram-go into: multi-tenant Kubernetes SaaS backend (thousands of users, RLS isolation, dual REST+MCP API)

## Target Use Case
Backend infrastructure for private AI SaaS products (analogous to yourai.com, privatellm.com, agentgateway.com) — persistent per-user memory that compounds over time, never crossing tenant boundaries.

## Architecture Decision: Control Plane + Data Plane Split

Two services:
1. **engram-gateway** (new) — REST API, tenant resolution, X-Tenant-ID injection
2. **engram-go** (enhanced) — MCP data plane, RLS enforcement, pluggable embedder

## Key Design Decisions Made

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Tenant isolation | PostgreSQL RLS (logical, row-level) | Scales to hundreds of thousands; standard SaaS pattern |
| Auth model | Proxy-injected X-Tenant-ID header | engram-go stays auth-agnostic; K8s ingress owns identity |
| Tenant provisioning | Auto-provision on first request (Phase A) | Fast to ship; TenantResolver interface allows future migration to explicit admin API (B) or external registry (C) |
| Embedding layer | Pluggable Embedder interface | Ollama default; OpenAI swappable; per-tenant override via tenants.config JSONB |
| Horizontal scaling | Stateless-first | Redis/Valkey cache deferred as future optimization |
| API surface | REST + MCP/SSE dual interface | REST for web frontends; MCP preserved for AI agent clients |
| Deployment | Docker → Kubernetes | Docker Compose preserved for local dev |

## Implementation Scope

### 1. Database (migrations 014 + 015)
- New `tenants` table (id, status, created_at, suspended_at, config JSONB)
- Add `tenant_id TEXT NOT NULL REFERENCES tenants(id)` to all 9 tables
- Rebuild all indexes with tenant_id as leading column
- RLS policies: `SET LOCAL app.tenant_id = $1` per transaction

### 2. TenantResolver Interface (`internal/tenant/resolver.go`)
```go
type TenantResolver interface {
    Resolve(ctx context.Context, tenantID string) (*Tenant, error)
    Provision(ctx context.Context, tenantID string) (*Tenant, error)
}
```
Default: auto-provision on first request. Interface design allows Phase A → B → C swap without touching data layer.

### 3. Pluggable Embedder (`internal/embed/`)
```go
type Embedder interface {
    Embed(ctx context.Context, text string) ([]float32, error)
    Dimensions() int
    Model() string
}
```
Implementations: `ollama.go` (existing wrapped), `openai.go` (new).
Startup dimension guard: fatal error if embedder dimensions ≠ DB vector column.

### 4. engram-gateway (`cmd/engram-gateway/`)
REST endpoints proxying to engram-go MCP tools:
- POST /v1/memories, GET /v1/memories, GET /v1/memories/search
- GET/PATCH/DELETE /v1/memories/:id
- POST /v1/memories/:id/connect
- GET /v1/projects, GET /v1/status
- POST /admin/tenants/:id/suspend (stubbed), DELETE /admin/tenants/:id (stubbed)

Config: ENGRAM_UPSTREAM_URL, ENGRAM_GATEWAY_PORT=8789, SESSION_HEADER

### 5. Kubernetes Manifests (`deploy/k8s/`)
- namespace, postgres (StatefulSet), ollama (Deployment+PVC), engram-go (Deployment+HPA), engram-gateway (Deployment+HPA), ingress
- Ingress routing: /v1/* → gateway:8789, /sse+/message → engram-go:8788
- Network policy: engram-go only accepts traffic from gateway + ingress

### 6. Migration Path (Docker → K8s, zero data loss)
1. Run migrations 014+015 against live Docker DB; backfill tenant_id='migration'
2. Import pgdata PVC into K8s StatefulSet; run both in parallel
3. Switch DNS to K8s; decommission Docker Compose

## Non-Negotiable Test
Cross-tenant bleed test must be in CI before production: tenant A cannot read tenant B memories via any tool or endpoint.

## Out of Scope (this phase)
- Redis/Valkey cache layer
- Billing / quota enforcement
- Explicit admin provisioning API (stubs only)
- External IdP / OIDC integration
- Per-tenant Ollama pod allocation

## Files to Read Before Starting
- `internal/types/types.go` — core data models
- `internal/db/migrations/` — all 13 existing migrations
- `internal/mcp/tools.go` — 32 MCP tool handlers
- `internal/embed/` — current Ollama client
- `docker-compose.yml` — current deployment
- `docs/superpowers/specs/2026-04-20-multi-tenant-design.md` — full spec
