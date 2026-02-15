# Chainguard Security Intelligence System

**Date:** 2026-01-22
**Status:** Design Complete - Ready for Implementation
**Replaces:** container-security-research, container-security-upgrade

---

## Executive Summary

This project creates a comprehensive security intelligence system for the homelab K8s cluster, centered on Chainguard image migration but extending to full security observability. The system provides:

- **Attack Surface Scorecard** - Gamified CVE tracking with composite scoring
- **Blast Radius Mapping** - Visualization of compromise impact per container
- **Supply Chain Intelligence** - SBOM generation and dependency depth tracking
- **Self-Learning Integration** - Compatibility matrix and migration outcome tracking

**Primary Goal:** CVE reduction through systematic Chainguard migration with full observability.

**First Milestone:** Baseline scan + Grafana dashboard before any migrations.

---

## Current State

### Cluster Inventory

**56 unique container images** across the cluster:

| Category | Count | Examples | Approach |
|----------|-------|----------|----------|
| **Drop-in Chainguard** | 7 | postgres, redis, nginx, python | Direct replacement |
| **Multi-stage Rebuild** | 5 | Homepage, N8N, Open-WebUI, Linkwarden, SearXNG | Custom Dockerfiles |
| **Vendor-managed** | 24 | Linkerd, Longhorn, Cert-Manager, Prometheus | Skip - vendor controls |
| **K3s System** | 12 | Rancher images, CoreDNS, Traefik | Skip - K3s managed |
| **Custom Registry** | 4 | job-search-api, gmail-tracker, proxmox-monitor | Rebuild from Chainguard |
| **Sidecars/Exporters** | 4 | redis_exporter, postgres_exporter | Evaluate case-by-case |

### Existing Chainguard Usage

Already running: `cgr.dev/chainguard/postgres:latest` in n8n namespace.

### Existing Security Projects (to archive)

- `/home/psimmons/projects/container-security-research` - Phase 2 incomplete
- `/home/psimmons/projects/container-security-upgrade` - Never started
- `/home/psimmons/projects/security-upgrade-plan.md` - Partial implementation

---

## Architecture

### Security Observability Pipeline

```
┌─────────────────────────────────────────────────────────────────────┐
│                         SECURITY PIPELINE                            │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────────────┐  │
│  │ Trivy        │───▶│ Metrics      │───▶│ Prometheus           │  │
│  │ Operator     │    │ Exporter     │    │ (existing)           │  │
│  │ (scans)      │    │              │    │                      │  │
│  └──────────────┘    └──────────────┘    └──────────┬───────────┘  │
│         │                                            │              │
│         ▼                                            ▼              │
│  ┌──────────────┐                         ┌──────────────────────┐  │
│  │ VulnReports  │                         │ Grafana Dashboard    │  │
│  │ (CRDs)       │                         │ "Security Posture"   │  │
│  └──────────────┘                         └──────────────────────┘  │
│         │                                            │              │
│         ▼                                            ▼              │
│  ┌──────────────┐                         ┌──────────────────────┐  │
│  │ SBOM         │                         │ AlertManager         │  │
│  │ Generator    │                         │ (CVE alerts)         │  │
│  │ (Syft)       │                         │                      │  │
│  └──────────────┘                         └──────────────────────┘  │
│         │                                                           │
│         ▼                                                           │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │ ~/.homelab/knowledge/security-metrics.yaml                   │   │
│  │ (self-learning system integration)                           │   │
│  └─────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

### Security Dashboard Mockup

```
┌─────────────────────────────────────────────────────────────────┐
│                    HOMELAB SECURITY DASHBOARD                   │
├─────────────────────────────────────────────────────────────────┤
│  Cluster Score: 847 → 234  ▼73%     Last Scan: 2 hours ago     │
├──────────────────┬──────────────────┬───────────────────────────┤
│ ATTACK SURFACE   │ BLAST RADIUS     │ SUPPLY CHAIN              │
│ ────────────────│ ────────────────│ ─────────────────────────│
│ CVEs: 1,247→89   │ High Risk: 3     │ Avg Depth: 7→3 layers     │
│ Packages: 4,891  │ Medium: 12       │ Trusted Sources: 78%      │
│ Shells: 23→4     │ Low: 41          │ SBOM Coverage: 100%       │
├──────────────────┴──────────────────┴───────────────────────────┤
│ CHAINGUARD MIGRATION PROGRESS                                    │
│ ████████░░ 78%                                                   │
│ ├─ Drop-in:      ██████████ 100% (4/4)                          │
│ ├─ Multi-stage:  ████░░░░░░ 40%  (2/5)                          │
│ └─ Custom:       ██████████ 100% (2/2)                          │
└─────────────────────────────────────────────────────────────────┘
```

---

## Security Outcome Pillars

### 1. Attack Surface Scorecard

**Composite Score Formula:**
```
Score = (Critical CVEs × 10) + (High × 5) + (Medium × 2) + (Low × 1)
      + (total_packages / 10)
      + (has_shell × 50)
```

**Metrics Tracked:**
- CVE count by severity (Critical/High/Medium/Low)
- Total installed packages per image
- Shell availability (distroless = no shell)
- Network capabilities
- Filesystem writability

**Gamification:**
- Weekly score tracking
- Namespace leaderboard
- Regression alerts (score increases)

### 2. Blast Radius Mapping

For each container, document what an attacker can reach if compromised:

| Container | Network Access | Secrets | Volumes | Service Account |
|-----------|---------------|---------|---------|-----------------|
| postgres-n8n | n8n, linkwarden | DB creds | workflow-data | default |
| redis-owui | open-webui | session-keys | cache-vol | default |

**Visualization:** Graph showing container relationships and compromise paths.

**Prioritization:** Migrate highest blast-radius containers first.

### 3. Supply Chain Intelligence

- **SBOM Generation:** Syft for every production image
- **Dependency Depth:** Track layers from app to base OS
- **Registry Trust:** Classify registries (Chainguard=trusted, Docker Hub=verify)
- **Allowlist:** Approved images for production use

---

## Implementation Phases

### Phase 1: Baseline & Visibility (First Milestone)

**Objective:** See current security posture before changing anything.

**Tasks:**
1. Deploy Trivy Operator to cluster
2. Configure scanning for all namespaces
3. Build Grafana dashboard with:
   - Cluster security score
   - CVE breakdown by severity
   - Per-image vulnerability counts
   - Per-namespace heat map
4. Set up AlertManager rules:
   - New Critical CVE introduced
   - Security score regression >10%
5. Export baseline to `~/.homelab/knowledge/security-baseline-2026-01.yaml`

**Deliverables:**
- Trivy Operator running in `security-system` namespace
- Grafana dashboard "Security Posture"
- Baseline metrics file
- Alert rules configured

### Phase 2: Compatibility Testing

**Objective:** Build compatibility matrix before production migrations.

**Tasks:**
1. Create test namespace `chainguard-testing`
2. Deploy Chainguard versions of:
   - postgres (test with n8n, linkwarden schemas)
   - redis (test with open-webui sessions)
   - nginx (test as reverse proxy)
   - python (test with custom apps)
3. Document compatibility in matrix
4. Identify apps needing multi-stage rebuilds
5. Create Dockerfiles for rebuild candidates

**Compatibility Matrix Structure:**
```yaml
chainguard/postgres:
  tested_with:
    - app: n8n
      status: working
      notes: "No changes needed"
    - app: linkwarden
      status: working
      notes: "Needs POSTGRES_USER env var"
  migration_effort: low
  cve_reduction: "142 → 0"
```

### Phase 3: Migration Waves

**Wave 1: Drop-in Replacements**
- postgres:15 → cgr.dev/chainguard/postgres (job-search, linkwarden)
- postgres:16-alpine → cgr.dev/chainguard/postgres (monitoring)
- redis:7-alpine → cgr.dev/chainguard/redis (open-webui)
- nginx → cgr.dev/chainguard/nginx (test-proxy, linkerd-test)

**Wave 2: Python Apps**
- python:3.11-alpine → rebuild with chainguard/python base
- python:3.12-slim → rebuild with chainguard/python base
- Affected: streamlit, custom scripts

**Wave 3: Node Apps (if feasible)**
- Homepage - evaluate chainguard/node base
- N8N - evaluate chainguard/node base

**Wave 4: Complex Apps**
- Open-WebUI - multi-stage with chainguard/python
- Linkwarden - multi-stage with chainguard/node
- SearXNG - multi-stage with chainguard/python

### Phase 4: Blast Radius & Supply Chain

**Tasks:**
1. Generate SBOMs for all production images (Syft)
2. Map network policies to blast radius visualization
3. Add dependency depth metrics to dashboard
4. Create "known good" image allowlist
5. Implement admission controller for allowlist enforcement (optional)

### Phase 5: Self-Learning Integration

**Tasks:**
1. Weekly metric export to homelab knowledge base
2. Migration outcome tracking (success/failure/rollback)
3. Compatibility matrix auto-update from real outcomes
4. Predictive warnings based on failure patterns

---

## Recovery Architecture (MANDATORY)

**No migration proceeds without ALL safety mechanisms in place.**

### Pre-Migration Checklist

```
┌─────────────────────────────────────────────────────────────────┐
│              MANDATORY PRE-MIGRATION CHECKLIST                  │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  □ DATABASE BACKUP (if applicable)                              │
│    ├─ Longhorn snapshot created                                 │
│    ├─ pg_dumpall/redis-dump exported                           │
│    ├─ Restore procedure tested in last 30 days                  │
│    └─ Backup retention: 7 days minimum                          │
│                                                                  │
│  □ CANARY DEPLOYMENT                                            │
│    ├─ Chainguard image tested in isolation                      │
│    ├─ Traffic split configured (10% canary)                     │
│    ├─ Canary metrics baseline captured                          │
│    └─ Rollback manifest ready                                   │
│                                                                  │
│  □ AUTO-ROLLBACK                                                │
│    ├─ Health endpoint identified                                │
│    ├─ Rollback timer configured (5 min default)                 │
│    ├─ Previous image tag recorded in annotation                 │
│    └─ Alert configured for rollback events                      │
│                                                                  │
│  □ VALIDATION GATES                                             │
│    ├─ Pod ready check                                           │
│    ├─ Health endpoint responding                                │
│    ├─ Data integrity query defined                              │
│    └─ Downstream services healthy                               │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Recovery Mechanisms

**1. Database Backups**
- Longhorn snapshot before any stateful migration
- pg_dumpall/redis-dump for human-readable backup
- Test restore procedure monthly
- 7-day retention on old PVCs post-migration

**2. Canary Deployments**
- Deploy Chainguard version as separate deployment
- Use Linkerd traffic splitting (10% to canary)
- Monitor canary for 1 hour minimum
- Only proceed if canary healthy

**3. Auto-Rollback Timer**
- 5-minute default window after migration
- Health check runs at end of window
- Automatic rollback if health fails
- Logs rollback to self-learning system

**4. Validation Gates**
| Gate | Check | Fail Action |
|------|-------|-------------|
| Pod Ready | `kubectl wait --for=condition=ready` | Auto-rollback |
| Health Endpoint | HTTP 200 on /health | Auto-rollback |
| Data Integrity | App-specific query | Alert + manual review |
| Performance | Response time < 2x baseline | Alert + manual review |
| Dependencies | Downstream services healthy | Halt migration |

### Rollback Annotations

Every migrated deployment carries rollback metadata:
```yaml
metadata:
  annotations:
    security.homelab/previous-image: "postgres:16-alpine"
    security.homelab/migration-date: "2026-01-22T10:30:00Z"
    security.homelab/rollback-tested: "true"
```

---

## Self-Learning Data Structures

### Compatibility Matrix

Location: `~/.homelab/knowledge/chainguard-compatibility.yaml`

```yaml
images:
  chainguard/postgres:
    tested_with:
      - app: n8n
        namespace: n8n
        status: working
        date_tested: 2026-01-25
        notes: "No changes needed"
      - app: linkwarden
        namespace: linkwarden
        status: working
        date_tested: 2026-01-25
        notes: "Needs POSTGRES_USER env var"
    known_issues:
      - issue: "Missing pg_dump in distroless"
        workaround: "Use :latest-dev for backup jobs"
    migration_effort: low

  chainguard/redis:
    tested_with:
      - app: open-webui
        namespace: open-webui
        status: failed
        date_tested: 2026-01-26
        notes: "Health check requires redis-cli"
    known_issues:
      - issue: "No redis-cli in distroless"
        workaround: "Use :latest-dev variant"
    migration_effort: medium
```

### Migration Outcomes Log

Location: `~/.homelab/knowledge/migration-log.csv`

```csv
timestamp,service,namespace,old_image,new_image,outcome,rollback_needed,failure_reason,recovery_time,cve_before,cve_after
2026-01-25T10:30:00Z,postgres,n8n,postgres:16-alpine,cgr.dev/chainguard/postgres:latest,SUCCESS,false,,,142,0
2026-01-26T14:15:00Z,redis,open-webui,redis:7-alpine,cgr.dev/chainguard/redis:latest,FAILED,true,health_check_missing_cli,3m,,
```

### Security Metrics History

Location: `~/.homelab/knowledge/security-metrics.yaml`

```yaml
cluster_score:
  current: 234
  baseline: 847
  target: 100

history:
  - date: 2026-01-22
    score: 847
    event: "baseline measurement"
  - date: 2026-01-25
    score: 512
    event: "migrated postgres to chainguard"
  - date: 2026-01-28
    score: 234
    event: "migrated redis, nginx"

by_namespace:
  n8n:
    score: 0
    chainguard_coverage: 100%
  open-webui:
    score: 89
    chainguard_coverage: 50%
  monitoring:
    score: 145
    chainguard_coverage: 0%
    notes: "vendor-managed images"
```

---

## Project Structure

```
/home/psimmons/projects/container-security-chainguard/
├── README.md
├── docs/
│   ├── compatibility-matrix.md      # Human-readable compatibility notes
│   ├── migration-runbook.md         # Step-by-step migration procedures
│   └── recovery-procedures.md       # Rollback and recovery documentation
├── manifests/
│   ├── trivy-operator/              # Trivy Operator deployment
│   ├── dashboards/                  # Grafana dashboard JSON
│   ├── alerts/                      # AlertManager rules
│   └── migrations/                  # Per-service migration manifests
├── dockerfiles/
│   ├── homepage/                    # Multi-stage Dockerfile for Homepage
│   ├── open-webui/                  # Multi-stage Dockerfile for Open-WebUI
│   └── custom-apps/                 # Custom app rebuilds
├── scripts/
│   ├── migrate-to-chainguard.sh    # Main migration script
│   ├── scan-cluster.sh             # Manual scan trigger
│   ├── export-metrics.sh           # Export to self-learning system
│   └── validate-migration.sh       # Post-migration validation
├── validations/
│   ├── postgres-validate.sh        # Postgres-specific validation
│   ├── redis-validate.sh           # Redis-specific validation
│   └── generic-validate.sh         # Generic HTTP health check
└── archive/
    └── previous-projects/          # Links to archived projects
```

---

## Success Criteria

### Phase 1 Complete When:
- [ ] Trivy Operator scanning all namespaces
- [ ] Grafana dashboard showing cluster security score
- [ ] Baseline exported to knowledge system
- [ ] AlertManager rules firing on test

### Project Complete When:
- [ ] All drop-in images migrated to Chainguard
- [ ] Multi-stage rebuilds completed for feasible apps
- [ ] Compatibility matrix documents all outcomes
- [ ] Security score reduced by >70% from baseline
- [ ] Zero unplanned rollbacks in final wave
- [ ] Self-learning system capturing all metrics

---

## Risk Mitigation

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Chainguard image incompatible | Medium | High | Canary testing, compatibility matrix |
| Data loss during migration | Low | Critical | Longhorn snapshots, pg_dump, tested restores |
| Performance regression | Low | Medium | Baseline metrics, performance gates |
| Cascading failures | Low | High | Dependency mapping, staged rollout |
| Trivy Operator resource impact | Medium | Low | Resource limits, dedicated node if needed |

---

## Timeline

| Phase | Dependencies | Deliverables |
|-------|--------------|--------------|
| Phase 1: Baseline | None | Dashboard, baseline metrics |
| Phase 2: Compatibility | Phase 1 | Compatibility matrix |
| Phase 3: Wave 1 | Phase 2 | Drop-in migrations complete |
| Phase 3: Wave 2-4 | Wave 1 success | All feasible migrations complete |
| Phase 4: Supply Chain | Phase 3 | SBOMs, blast radius map |
| Phase 5: Self-Learning | All phases | Automated metric tracking |

---

## Archive Actions

Upon project creation:
1. Move `/home/psimmons/projects/container-security-research` to `archive-k8s/`
2. Move `/home/psimmons/projects/container-security-upgrade` to `archive-k8s/`
3. Archive `/home/psimmons/projects/security-upgrade-plan.md`
4. Update PROJECTS-CATALOG.md with new project
5. Add reference links in new project to archived work

---

## Next Steps

1. **Approve this design**
2. **Create project directory structure**
3. **Begin Phase 1: Deploy Trivy Operator**
4. **Build baseline dashboard**

---

**Design Author:** Claude + User Collaboration
**Design Date:** 2026-01-22
**Version:** 1.0
