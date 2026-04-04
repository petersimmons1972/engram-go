# Longhorn → NFS Migration & Database Consolidation

**Project:** `longhorn-nfs`
**Date:** 2026-04-03
**Status:** Design approved, pending implementation plan

## Problem Statement

The Proxmox hypervisor hosting a 9-node K3s cluster runs at ~30% CPU utilization (load average ~15.5/40 threads). Longhorn storage replicates all 20 persistent volumes at 3x across nodes, generating significant write amplification, inter-node network traffic, and CPU overhead from instance-manager processes (~1.3-1.5 CPU cores direct overhead, plus indirect I/O amplification).

### Hypothesis

Longhorn's 3x block replication is a significant contributor to elevated CPU and I/O on the Proxmox host. Replacing Longhorn PVCs with NFS-backed storage (initially on TrueNAS, ultimately on Proxmox-local ZFS) will measurably reduce overall resource consumption at equivalent workload.

## Infrastructure Inventory

### Proxmox Host (`proxmox.petersimmons.com`)

- CPU: 2x Xeon E5-2690 v2 — 20 cores / 40 threads @ 3.0 GHz
- RAM: 270 GB (123 GB used, 81 GB free)
- Storage: 965 GB local ext4 + 11.36 TB ZFS (zp3, 5.87 TB free)
- VMs: 24 running + 3 LXC containers
- Current CPU: ~30% (Proxmox API), load avg ~15.5

### K8s Cluster (K3s v1.33.5)

- 9 nodes: 3 control-plane (worker131-133) + 6 workers (worker134-139)
- Each node: 4 vCPU, 8 GB RAM (36 vCPU total of 40 host threads)
- 27 namespaces, 53 deployments, 9 StatefulSets
- Longhorn: 20 volumes, all 3x replication, ~280 Gi unique / ~840 Gi replicated

### TrueNAS (`trunas.petersimmons.com` / 192.168.0.189)

- Model: Dell PowerEdge R720xd
- CPU: Intel Xeon E5-2690 v2 @ 3.0 GHz (40 logical cores)
- RAM: 252 GB
- Pool: zp1 — 60 TB RAIDZ2 (6 disks), 33 TB free
- Network: 10GbE bonded failover
- Existing NFS shares: 8 (including plex for K8s)
- K8s dataset: `zp1/k8s-data` — `sync=standard`, `dedup=off` (locally overridden from parent)
- ZFS dedup table: 33.4 GB (global dedup=ON, but k8s-data is OFF)
- Load average: 1.24 (essentially idle)

### Longhorn Current State

- Default replica count: 3
- Replica auto-balance: disabled
- Instance-manager CPU: top 5 engines consume ~629m combined
- Longhorn manager CPU: ~393m across 9 nodes
- Total direct overhead: ~1.3-1.5 CPU cores
- 2 degraded volumes (container-registry, content-cache)
- 3 detached volumes (orphaned)

### Database Inventory (13 data services)

| Type | Count | Instances |
|------|-------|-----------|
| PostgreSQL | 8 | clearwatch-checkout, clearwatch-research, content-cache, infisical, job-search (not on Longhorn), linkwarden, proxmox-monitor, n8n (on local-path) |
| MariaDB | 1 | clearwatch (WordPress) |
| Redis | 3 | infisical, open-webui, open-webui-test |
| Qdrant | 1 | content-cache (vector DB) |

### Workload Tiers

| Tier | Meaning | Workloads |
|------|---------|-----------|
| Red — zero downtime | Cannot go down | Infisical (most precious data), cert-manager, Traefik (most operationally critical), CoreDNS, Pi-hole |
| Yellow — scheduled only | Maintenance window, minutes OK | Clearwatch (all variants), Prometheus/Grafana, n8n, Linkerd |
| Green — flexible | Can restart anytime | homepage, searxng, WordPress/www, docker-registry, open-webui, linkwarden, plex, security-reports |

## Architecture Decisions

### ADR-1: NFS Provisioner — democratic-csi

**Decision:** Use democratic-csi for NFS StorageClass provisioning.

**Why:** Active maintenance (releases every ~3 months), full CSI spec (snapshots, clones, resizing), native TrueNAS API integration, ZFS snapshot support as K8s VolumeSnapshots. nfs-subdir-external-provisioner is unmaintained (no releases since March 2023).

### ADR-2: NFS Mount Options

**Decision:** `hard,proto=tcp,timeo=600,retrans=2,nfsvers=4.1`

**Why:** Hard mounts block on NFS failure rather than returning errors (prevents data corruption). NFSv4.1 for modern security and performance. Research confirmed soft mounts are inappropriate for production.

### ADR-3: PVC Migration Tool — pv-migrate

**Decision:** Use pv-migrate for rsync-based PVC data migration between StorageClasses.

**Why:** Purpose-built for cross-StorageClass PVC moves. Handles job creation, cleanup, and verification. Alternative (Velero) is heavier and designed for cross-cluster, not cross-StorageClass.

### ADR-4: Database Consolidation Target — TrueNAS Postgres App

**Decision:** Run consolidated Postgres as a TrueNAS App (Docker container) rather than in K8s or a dedicated VM.

**Why:**
- TrueNAS has abundant spare RAM (252 GB, load avg 1.24 — essentially idle)
- Postgres gets direct ZFS access (no NFS, no hypervisor layer)
- TrueNAS App API allows programmatic install and management
- Eliminates database I/O from K8s storage layer entirely
- No OS provisioning needed — TrueNAS IS the OS
- K8s apps connect via ExternalName Service — transparent to applications

### ADR-5: Postgres Image — Chainguard (no pgvector)

**Decision:** Use `cgr.dev/chainguard/postgres:latest` (same as current cluster standard). Do not build custom images with pgvector.

**Why:** Maintaining a custom Dockerfile on top of Chainguard introduces supply chain risk and a build pipeline to secure/monitor. Qdrant stays as a separate service on NFS. The consolidation win of replacing Qdrant with pgvector is not worth the supply chain complexity for a homelab.

### ADR-6: Redis — emptyDir (no persistent storage)

**Decision:** Move all Redis instances from Longhorn PVCs to `emptyDir` volumes (no persistence).

**Why:** Both Redis use cases are ephemeral cache:
- Infisical Redis: session tokens + job queues. Crash = users re-login, jobs re-queue. Zero secret loss.
- Open WebUI Redis: chat session cache. Crash = page refresh. Conversations persisted in DB.

Replicating cache data 3x across nodes via Longhorn is wasteful. `emptyDir` is the correct storage for disposable data.

### ADR-7: MariaDB — TrueNAS App (separate from Postgres)

**Decision:** Install MariaDB as a separate TrueNAS App for WordPress. Do not convert WordPress to Postgres.

**Why:** WordPress-to-Postgres conversion is risky and tangential to the project goals. MariaDB as TrueNAS App follows the same pattern as Postgres and uses the existing Chainguard image (`cgr.dev/chainguard/mariadb:latest`).

### ADR-8: ZFS Dataset Settings for K8s NFS

**Decision:** `zp1/k8s-data` configured with `sync=standard`, `dedup=off` (already applied).

**Why:**
- `sync=standard` (overrides parent `disabled`): Required for data safety — disabled sync means writes acknowledged before on disk, risking data loss on power failure.
- `dedup=off` (overrides parent `on`): Dedup adds ~320 bytes per block to in-core tables. Database and general K8s I/O has low dedup ratio, making it wasteful and RAM-hungry.

### ADR-9: Backup Strategy — ZFS Snapshots

**Decision:** ZFS periodic snapshots replace Longhorn volume snapshots as the backup mechanism.

**Why:** Longhorn provides snapshots today. Moving to NFS eliminates that. ZFS snapshots on TrueNAS/Proxmox are the natural replacement. democratic-csi exposes ZFS snapshots as K8s VolumeSnapshots for seamless integration.

**Configuration:** Hourly snapshots, retain 24 (24 hours of rollback). Verify restore works before first migration.

## Phase A: Longhorn → TrueNAS NFS Migration

### A.0 — Prerequisites

1. **Backup strategy:** Configure ZFS periodic snapshots on `zp1/k8s-data` (hourly, retain 24). Verify restore with test dataset.
2. **Install democratic-csi:** Helm chart → `nfs-truenas` StorageClass. Mount options: `hard,proto=tcp,timeo=600,retrans=2,nfsvers=4.1`. Verify with test PVC.
3. **Install pv-migrate:** Test with throwaway PVC.
4. **NFS failure test:** Kill TrueNAS NFS export mid-operation, observe pod behavior. Document recovery time and behavior.
5. **Prometheus baseline:** Export 7-day historical metrics to `reports/baseline/`. Metrics: host CPU, per-node CPU, iowait, disk IOPS, network TX/RX, Longhorn instance-manager CPU, Longhorn volume IOPS.

### A.1 — Green Tier Migration (80 Gi)

**Candidates:**

| Workload | Namespace | PVC | Size |
|----------|-----------|-----|------|
| docker-registry | container-registry | registry-data-pvc | 10 Gi |
| wordpress | clearwatch | wordpress-content | 10 Gi |
| plex | plex | plex-config-pvc | 20 Gi |
| security-reports | security-intelligence-business | reports-pvc | 20 Gi |
| qdrant | content-cache | qdrant-storage-pvc | 20 Gi |

**Per-workload procedure:**
1. Snapshot Longhorn volume
2. Create NFS PVC via `nfs-truenas` StorageClass
3. Scale deployment to 0
4. `pv-migrate` Longhorn → NFS
5. Update deployment manifest to new PVC
6. Scale to 1, verify health
7. Retain old Longhorn PVC 48h, then delete

**Measurement:** 24h Prometheus collection post-migration. Journal entry in `docs/journal/`.

### A.2 — Yellow Tier Migration (81 Gi, scheduled maintenance window)

**Candidates:**

| Workload | Namespace | PVC | Size |
|----------|-----------|-----|------|
| alertmanager (x2) | monitoring | alertmanager-db (x2) | 3 Gi each |
| prometheus | monitoring | prometheus-db | 15 Gi |
| clearwatch-research | clearwatch-research | common-crawl-raw-pvc | 30 Gi |
| clearwatch-research | clearwatch-research | conference-raw-pvc | 20 Gi |
| clearwatch-research | clearwatch-research | youtube-raw-pvc | 10 Gi |

Prometheus and Alertmanager migrated together to minimize monitoring gap.

**Same per-workload procedure as A.1.** Measurement: 24h Prometheus collection.

### A.3 — Redis Swap to emptyDir (10 Gi freed)

Swap infisical-redis and open-webui-redis PVCs to `emptyDir`. One-line manifest change per deployment. Can run anytime during A.1-A.2.

### A.4 — Phase A Completion Report

- Full comparison: baseline → post-migration
- Calculate: CPU delta, I/O delta, network delta, per-volume Longhorn overhead removed
- Quantify remaining Longhorn overhead (database PVCs only)
- Project expected further reduction from Phase B
- Journal entry with all numbers for LinkedIn content

**CHECKPOINT — operational pause of any length**

## Phase B: Database Consolidation

### B.0 — Prerequisites

1. **Infisical triple backup:**
   - Longhorn snapshot of `infisical-postgres-pvc`
   - `pg_dump` to file on `zp1/backup`
   - ZFS snapshot of backup dataset
2. **Database audit:** Connect to all 8 Postgres instances. Catalog: version, extensions, schemas, table count, total size, connection strings. Map application → database dependencies.
3. **TrueNAS Postgres install:**
   - Install via TrueNAS App API — verify whether the app allows specifying `cgr.dev/chainguard/postgres:latest` as a custom image, or if it uses its own bundled image. If the TrueNAS App is too opinionated, fall back to a raw Docker Compose on TrueNAS.
   - Dedicated ZFS dataset: `recordsize=8K`, `sync=standard`, `dedup=off`, `compression=lz4`
   - Tune: `shared_buffers` (25% allocated RAM), `work_mem`, `effective_cache_size`, `statement_timeout`
   - Per-database roles with connection limits
   - Verify connectivity from K8s pods
4. **K8s ExternalName Services:** Create in each namespace, pointing to TrueNAS Postgres.

### B.1 — Green Tier Databases (25 Gi)

| Database | Namespace | Size |
|----------|-----------|------|
| ciso-tracker-db | default | 100 Mi |
| linkwarden-postgres | linkwarden | 5 Gi |
| proxmox-monitor-postgres | monitoring | 20 Gi |

**Per-database procedure:**
1. `pg_dump` from source → TrueNAS backup
2. Create logical database + role on TrueNAS Postgres
3. `pg_restore` into TrueNAS Postgres
4. Create ExternalName Service in namespace
5. Update connection string in Infisical
6. Restart application pod
7. Verify: spot check data, test writes
8. Keep source container at scale=0 for 48h
9. Delete Longhorn PVC after confirmation

### B.2 — Yellow Tier Databases (70 Gi, scheduled maintenance window)

| Database | Namespace | Size | Notes |
|----------|-----------|------|-------|
| postgres-checkout | clearwatch-checkout | 5 Gi | Clearwatch revenue app |
| research-postgres | clearwatch-research | 10 Gi | Research pipeline |
| content-cache-postgres | content-cache | 50 Gi | Largest — test dump/restore timing first |
| n8n-postgres | n8n | 5 Gi | On local-path, not Longhorn — optional consolidation |

Same procedure as B.1. `content-cache-postgres` gets extra timing validation before real migration.

**Decision point:** n8n-postgres is on `local-path`, not Longhorn. Include only if simplifying the database landscape is worth the effort.

### B.3 — MariaDB to TrueNAS App (5 Gi)

- Install MariaDB via TrueNAS App API (`cgr.dev/chainguard/mariadb:latest`)
- `mysqldump` → `mysql` restore on TrueNAS
- Update WordPress connection config in Infisical
- Same verification and rollback pattern

### B.4 — Infisical Postgres (20 Gi) — Most Critical Migration

**Pre-migration (days before):**
1. Triple backup verified (from B.0)
2. Dry-run: `pg_dump` → `pg_restore` to test database on TrueNAS, verify row counts
3. Spin up temporary Infisical pod pointed at test database, verify secret reads
4. Schedule dedicated maintenance window

**Migration (during window):**
1. Fresh `pg_dump` (captures changes since B.0)
2. Scale Infisical to 0 (secrets cached by apps — Infisical API briefly unavailable, not secrets themselves)
3. `pg_restore` to TrueNAS Postgres (production database)
4. Update ExternalName Service + Infisical's own connection config
5. Scale Infisical to 1
6. Verify: Infisical reads secrets, other apps fetch via Infisical API
7. Monitor 24h before removing old PVC

**Infisical Redis:** Already moved to emptyDir in Phase A.3.

### B.5 — Longhorn Decommission

After all database PVCs are migrated, verify zero attached Longhorn volumes, then:

1. Delete all detached/orphaned volumes
2. Set deletion confirmation flag: `kubectl -n longhorn-system patch lhs deleting-confirmation-flag --type=merge -p '{"value": "true"}'`
3. Run Longhorn uninstall job
4. Remove Longhorn StorageClass
5. Verify no stuck namespaces
6. Measure: final CPU/IO with Longhorn fully removed

### B.6 — Phase B Completion Report

- Full comparison: baseline → post-A → post-B → post-Longhorn-removal
- Database inventory: before (13 instances in K8s) → after (1 Postgres + 1 MariaDB on TrueNAS, Qdrant on NFS, Redis as emptyDir)
- Resource recovery quantified
- Journal entry for LinkedIn content

**CHECKPOINT — operational pause of any length**

## Phase C: TrueNAS NFS → Proxmox-local NFS

### C.1 — Provision NFS on Proxmox

- ZFS dataset: `zp3/k8s-nfs` with `sync=standard`, `dedup=off`, `compression=lz4`, `atime=off`
- NFS export: `/zp3/k8s-nfs` to `192.168.0.0/24`, NFSv4.1
- Verify mount from K8s node

### C.2 — Data Migration

- `rsync` from TrueNAS → Proxmox (can run live, NFS still served from TrueNAS)
- Final sync with pods scaled to 0 to catch last writes

### C.3 — Switchover

- Update democratic-csi config or create `nfs-proxmox` StorageClass
- Update PVs to Proxmox NFS server IP
- Scale pods back up, verify

### C.4 — Cleanup

- Decommission `zp1/k8s-data` NFS share on TrueNAS
- Configure ZFS auto-snapshots on Proxmox (hourly, retain 24)

### C.5 — Final Measurement

- Full comparison: Longhorn baseline → TrueNAS NFS → Proxmox NFS
- Delta between TrueNAS NFS and Proxmox NFS isolates 10GbE network hop cost
- Journal entry for LinkedIn content

**Note:** Databases stay on TrueNAS. Only general PVCs (configs, static content, reports, Qdrant) move to Proxmox NFS.

## Final State

| Location | What lives there |
|----------|-----------------|
| Proxmox NFS (local ZFS zp3) | All non-database PVCs — configs, static content, reports, raw research data, Qdrant |
| TrueNAS Postgres app | 8 logical databases (Chainguard image) |
| TrueNAS MariaDB app | WordPress database (Chainguard image) |
| K8s emptyDir | Redis instances (ephemeral cache) |
| K8s local-path | n8n-postgres, open-webui (if not consolidated) |
| Longhorn | Nothing — fully decommissioned |

## Metrics & Success Criteria

### Metrics Collected

| Metric | Source |
|--------|--------|
| Proxmox host CPU % | Proxmox API rrddata |
| Proxmox load average | Proxmox API |
| Per-node K8s CPU | Prometheus |
| Longhorn instance-manager CPU | Prometheus |
| Disk I/O (read/write bytes) | Prometheus node_exporter |
| Network TX/RX between nodes | Prometheus node_exporter |
| NFS operation latency | democratic-csi / NFS client metrics |
| Pod restart count | Prometheus |

### Comparison Points

| Snapshot | When |
|----------|------|
| Baseline | Existing Prometheus history (7 days) |
| Post A.1 | After green-tier NFS migration |
| Post A.2 | After yellow-tier NFS migration |
| Post B | After database consolidation |
| Post Longhorn removal | After decommission |
| Post C | After Proxmox NFS switchover |

### Success Criteria

No hard pass/fail threshold. The project produces a data-driven dashboard showing the full impact of each phase. The decision to proceed at each checkpoint is based on the totality of metrics (CPU, I/O, network, memory) — not a single number.

### Rollback Procedures

| Phase | Rollback method | Time |
|-------|----------------|------|
| A (per-PVC) | Old Longhorn PVC retained 48h, revert manifest | ~2 min per workload |
| B (per-database) | Source container at scale=0 for 48h, revert connection string | ~5 min per database |
| B (Infisical) | Triple backup, three independent restore paths | ~10 min |
| C | TrueNAS NFS share still exists until cleanup, revert PV IPs | ~5 min all PVCs |

## Risk Register

| Risk | Severity | Mitigation |
|------|----------|-----------|
| NFS single point of failure (replaces Longhorn redundancy) | High | ZFS snapshots (hourly, retain 24) + verified restore procedure |
| TrueNAS sync=disabled (inherited) | High | Already overridden: `sync=standard` on `zp1/k8s-data` (verified LOCAL) |
| NFS stale file handle / mount hangs | Medium | Hard mounts with timeouts; explicit failure testing in A.0 |
| Backup strategy gap (Longhorn snapshots lost) | High | ZFS snapshots configured before first migration; democratic-csi VolumeSnapshot integration |
| Dedup bloat on K8s data | Medium | Already overridden: `dedup=off` on `zp1/k8s-data` (verified LOCAL) |
| Infisical data loss | Critical | Triple backup (Longhorn snapshot + pg_dump + ZFS snapshot), dry-run before real migration |
| Shared Postgres noisy neighbor | Medium | Per-database roles with connection limits, statement_timeout, monitoring |
| Postgres version mismatch | Low | Extension/version audit in B.0 before choosing target version |
| Longhorn uninstall stuck namespace | Medium | Follow strict decommission sequence; verify zero volumes before uninstall |
| Pods hang during NFS server outage | Medium | Hard mount options + documented recovery procedure |

## Project Structure

```
longhorn-nfs/
├── scripts/
│   ├── baseline/          # Prometheus query scripts (Python)
│   ├── migrate/           # PVC migration scripts (shell + Python)
│   ├── provision/         # TrueNAS/Proxmox NFS provisioning (shell)
│   └── report/            # Before/after comparison (Python)
├── config/
│   ├── workloads.yaml     # PVC inventory, tiers, migration order
│   ├── thresholds.yaml    # Metrics to collect
│   └── credentials.env.example
├── k8s/
│   ├── nfs-storageclass.yaml
│   └── nfs-pv-templates/
├── docs/
│   ├── journal/           # Session-by-session logs for LinkedIn content
│   │   ├── 000-baseline.md
│   │   ├── 001-nfs-provision.md
│   │   └── ...
│   ├── decisions/         # ADRs (also captured above)
│   └── superpowers/specs/ # This document
├── reports/               # Generated comparison reports (gitignored)
├── tests/                 # Validation scripts
├── pyproject.toml         # Python deps (prometheus-api-client, pandas, etc.)
├── Makefile               # make baseline, make migrate, make report
├── .github/
│   └── workflows/
│       └── lint.yaml      # ruff, shellcheck, yamllint
└── README.md
```

## Journal Template

Each session log in `docs/journal/` follows:

- **Date / Phase / Step**
- **What we did** (commands, configs)
- **What we measured** (before/after metrics with actual numbers)
- **What we learned** (surprises, gotchas, validations)
- **Decision made** (if any, with reasoning)

## Research Sources

### Longhorn CPU Overhead
- [GitHub Issue #6695 - Longhorn manager 100% CPU](https://github.com/longhorn/longhorn/issues/6695)
- [Fixing Longhorn Instance Manager CPU Issues](https://hashbang.nl/blog/fixing-longhorn-instance-manager-max-cpu-issues)
- [GitHub Issue #3636 - High CPU Usage](https://github.com/longhorn/longhorn/issues/3636)
- [Longhorn Performance Benchmark Wiki](https://github.com/longhorn/longhorn/wiki/Performance-Benchmark)

### NFS Best Practices
- [democratic-csi GitHub](https://github.com/democratic-csi/democratic-csi)
- [Moving to TrueNAS and Democratic CSI](https://www.lisenet.com/2021/moving-to-truenas-and-democratic-csi-for-kubernetes-persistent-storage/)
- [K3s and external NFS storage](https://marksharpley.co.uk/posts/k3s-nfs/)

### Postgres Consolidation
- [CloudNativePG consolidation guide](https://www.beyondwatts.com/posts/combining-multiple-postgres-databases-into-a-single-cloudnative-pg-instance/)
- [CNCF recommended architectures](https://www.cncf.io/blog/2023/09/29/recommended-architectures-for-postgresql-in-kubernetes/)
- [Postgres inside vs outside Kubernetes](https://www.glasskube.dev/blog/postgres-on-kubernetes/)

### ZFS & Backup
- [Optimizing Postgres on ZFS](https://vadosware.io/post/everything-ive-seen-on-optimizing-postgres-on-zfs-on-linux)
- [CNPG and ZFS Snapshots](https://www.ardanlabs.com/blog/2025/01/optimizing-databases-on-kubernetes-ep5-kubernetes-backup-and-recovery-with-cnpg-and-zfs-snapshots.html)
- [ZFS Deduplication](https://www.truenas.com/docs/references/zfsdeduplication/)

### Migration Stories
- [D3vtech: Longhorn to NFS case study](https://www.d3vtech.com/case-studies/transitioning-from-longhorn-block-storage-to-nfs-file-server-in-a-unique-kubernetes-environment/)
- [pv-migrate tool](https://github.com/utkuozdemir/pv-migrate)
- [Longhorn official uninstall guide](https://longhorn.io/docs/1.11.0/deploy/uninstall/)
