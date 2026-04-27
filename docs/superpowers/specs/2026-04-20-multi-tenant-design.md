# engram-go Multi-Tenant Design Spec

**Date:** 2026-04-20  
**Status:** Approved  
**Author:** Peter Simmons (design session with Claude)

---

## Context

engram-go is currently a single-tenant memory server: one bearer token, one logical user, project-based isolation only. The goal is to evolve it into the backend infrastructure for a private AI SaaS product (analogous to yourai.com, privatellm.com) supporting potentially thousands of users, each with a fully isolated memory space and independent LLM sessions.

This requires three parallel shifts:
1. **Multi-tenancy** — every user's memories are logically isolated via PostgreSQL Row-Level Security
2. **Docker → Kubernetes** — stateless, horizontally scalable deployment
3. **Dual API surface** — REST API for web frontends alongside the existing MCP/SSE interface for AI agents

---

## Architecture: Control Plane + Data Plane Split

Two services, each with a single responsibility:

```
Internet
   │
   ▼
K8s Ingress (nginx)
   │
   ├─── /v1/*  ──────────────► engram-gateway  (REST API + tenant resolution)
   │                                │
   │                                │ X-Tenant-ID injected
   │                                ▼
   └─── /sse, /message ──────► engram-go        (MCP data plane)
        (bearer token)              │
                                    ▼
                              PostgreSQL (RLS)
                              Ollama Pool / Pluggable Embedder
```

### engram-gateway
- New Go binary at `cmd/engram-gateway/`
- Owns: REST API, session parsing, tenant resolution, `X-Tenant-ID` injection
- Does NOT own: memory storage, search, embeddings, MCP protocol

### engram-go
- Existing binary — minimal changes
- Adds: `tenant_id` schema column, RLS enforcement, `TenantResolver` interface, pluggable embedder
- Trusts `X-Tenant-ID` header from internal network only (`ENGRAM_TRUST_PROXY_HEADERS=true`)

---

## Section 1: Database Schema

### Migration 014 — Add `tenant_id`

New `tenants` table:
```sql
CREATE TABLE tenants (
    id           TEXT PRIMARY KEY,
    status       TEXT NOT NULL DEFAULT 'active',  -- active | suspended | deleted
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    suspended_at TIMESTAMPTZ,
    config       JSONB NOT NULL DEFAULT '{}'       -- per-tenant limits, embedder override
);
```

Add `tenant_id TEXT NOT NULL REFERENCES tenants(id)` to every table:
- `memories`, `chunks`, `relationships`, `episodes`, `retrieval_events`,
  `memory_versions`, `documents`, `project_meta`, `canonical_entities`

All composite indexes gain `tenant_id` as the leading column:
```sql
-- Example replacement
DROP INDEX idx_memories_active;
CREATE INDEX idx_memories_active
    ON memories(tenant_id, project, updated_at DESC)
    WHERE valid_to IS NULL;
```

Backfill: a `migration` tenant is created; all existing rows set to `tenant_id = 'migration'`.

### Migration 015 — Row-Level Security

```sql
ALTER TABLE memories ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON memories
    USING (tenant_id = current_setting('app.tenant_id'));
-- Same policy applied to all 9 tables
```

engram-go sets `SET LOCAL app.tenant_id = $1` at the start of every transaction. No explicit `WHERE tenant_id = ?` clauses needed anywhere — RLS enforces isolation automatically.

---

## Section 2: TenantResolver Interface

Location: `internal/tenant/resolver.go`

```go
type Tenant struct {
    ID          string
    Status      string
    Config      map[string]any
}

type TenantResolver interface {
    Resolve(ctx context.Context, tenantID string) (*Tenant, error)
    Provision(ctx context.Context, tenantID string) (*Tenant, error)
}
```

**Default implementation (Phase A — auto-provision):**
- `Resolve`: look up by ID; return `ErrNotFound` if missing or suspended
- `Provision`: `INSERT INTO tenants ... ON CONFLICT DO NOTHING`, return the tenant

This interface is the only code that changes when moving to explicit admin provisioning (B) or external registry (C). The `/admin/tenants/:id/suspend` and `/admin/tenants/:id` DELETE endpoints are stubbed in engram-gateway now and wired to a `NoopAdminHandler` — ready to activate when Phase A is deprecated.

---

## Section 3: engram-gateway Service

### Responsibilities
1. Parse incoming session (cookie, JWT, or API key — format set by `SESSION_HEADER` env var)
2. Extract `tenant_id` from session
3. Call `TenantResolver.Provision()` on first use; reject suspended/deleted tenants with `403`
4. Proxy request to engram-go with `X-Tenant-ID` injected
5. Translate REST payloads to engram-go MCP tool call format

### REST API

```
POST   /v1/memories              → memory_store
GET    /v1/memories              → memory_list
GET    /v1/memories/search       → memory_recall
GET    /v1/memories/:id          → memory_fetch
PATCH  /v1/memories/:id          → memory_correct
DELETE /v1/memories/:id          → memory_forget
POST   /v1/memories/:id/connect  → memory_connect
GET    /v1/projects              → memory_projects
GET    /v1/status                → memory_status

POST   /admin/tenants/:id/suspend   (stubbed — Phase B/C)
DELETE /admin/tenants/:id           (stubbed — Phase B/C)
```

### Configuration
```
ENGRAM_UPSTREAM_URL     # engram-go internal K8s service URL
ENGRAM_GATEWAY_PORT     # default 8789
SESSION_HEADER          # header name carrying user identity (e.g. X-User-ID)
```

---

## Section 4: Pluggable Embedder

Location: `internal/embed/`

```go
type Embedder interface {
    Embed(ctx context.Context, text string) ([]float32, error)
    Dimensions() int
    Model() string
}
```

Implementations:
- `ollama.go` — existing logic wrapped (default)
- `openai.go` — new, `text-embedding-3-small` / `text-embedding-3-large`

Factory reads from environment:
```
ENGRAM_EMBEDDER=ollama         # ollama | openai
ENGRAM_EMBEDDER_MODEL=nomic-embed-text
ENGRAM_EMBEDDER_URL=http://ollama:11434
ENGRAM_EMBEDDER_API_KEY=""     # required for openai only
```

On startup, engram-go calls `Embedder.Dimensions()` and validates against the DB schema's vector column dimension. Mismatch → fatal error (prevents silent corruption).

Per-tenant embedder override: `tenants.config` JSONB can carry `{"embedder":"openai","model":"text-embedding-3-large"}` for premium tiers.

---

## Section 5: Kubernetes Manifests

```
deploy/k8s/
├── namespace.yaml
├── postgres/
│   ├── statefulset.yaml      # stable identity, PVC
│   ├── service.yaml          # ClusterIP only
│   └── secret.yaml           # Infisical-injected
├── ollama/
│   ├── deployment.yaml       # GPU node affinity
│   ├── service.yaml
│   └── pvc.yaml
├── engram-go/
│   ├── deployment.yaml       # stateless, 2+ replicas
│   ├── service.yaml          # ClusterIP, no external exposure
│   ├── hpa.yaml
│   └── configmap.yaml
├── engram-gateway/
│   ├── deployment.yaml       # stateless, 2+ replicas
│   ├── service.yaml
│   ├── hpa.yaml
│   └── configmap.yaml
└── ingress.yaml              # nginx routing table
```

| Concern | Decision |
|---------|----------|
| PostgreSQL | StatefulSet — stable network identity for PVC |
| engram-go | Deployment, no external ingress rule, `ENGRAM_TRUST_PROXY_HEADERS=true` |
| engram-gateway | Deployment, externally exposed |
| Secrets | Infisical operator injects at pod startup |
| HPA | engram-go and engram-gateway scale independently |
| Docker Compose | Preserved for local dev |
| Network policy | engram-go only accepts traffic from engram-gateway + ingress (MCP path) |

**Ingress routing:**
```
/v1/*     → engram-gateway:8789
/sse      → engram-go:8788
/message  → engram-go:8788
/health   → engram-go:8788
```

---

## Section 6: Migration Strategy

### Docker → Kubernetes (zero data loss)

**Phase 1 — Schema (live, no downtime)**
- Run migrations 014 + 015 against the running Docker database
- Backfill existing rows to `tenant_id = 'migration'`
- engram-go continues running; RLS defaults to `migration` tenant

**Phase 2 — Parallel run**
- Import existing pgdata PVC into K8s StatefulSet
- Deploy engram-go + engram-gateway to K8s pointing at same DB
- Run Docker and K8s in parallel; verify K8s health checks pass

**Phase 3 — Cut over**
- Switch DNS / ingress to K8s
- Decommission Docker Compose stack
- Docker Compose remains as local dev tooling

---

## Section 7: Testing & Verification

| Layer | Test |
|-------|------|
| RLS | Set `app.tenant_id` to tenant A; assert tenant B rows are invisible |
| TenantResolver | Auto-provision creates row; idempotent second call returns existing |
| engram-gateway | Integration: REST → proxy → engram-go round trip with injected header |
| Embedder | Unit: mock interface; integration: real Ollama call with dimension check |
| K8s | `kubectl rollout status` + health endpoint smoke test post-deploy |
| Cross-tenant bleed | **CI-required:** tenant A cannot read tenant B memories via any tool or endpoint |

The cross-tenant bleed test is non-negotiable and must pass in CI before any production traffic is served.

---

## Out of Scope (this spec)

- Redis/Valkey cache layer (future — stateless-first for now)
- Billing / quota enforcement
- Explicit admin provisioning API (Phase B/C — stubs only)
- External IdP / OIDC integration
- Per-tenant Ollama pod allocation
