# Longhorn → NFS Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrate K8s persistent storage from Longhorn 3x replication to NFS (TrueNAS then Proxmox-local), consolidate databases to TrueNAS, and eliminate Longhorn to reduce Proxmox CPU/IO overhead.

**Architecture:** Three-phase migration with checkpoints. Phase A moves non-database PVCs to TrueNAS NFS via democratic-csi. Phase B consolidates 8 Postgres + 1 MariaDB to TrueNAS Apps and decommissions Longhorn. Phase C moves NFS from TrueNAS to Proxmox-local ZFS. Each phase produces before/after metrics reports. All journal entries documented for LinkedIn content reuse.

**Tech Stack:** Python 3.12+ (prometheus-api-client, pandas, requests), shell scripts (kubectl, curl), democratic-csi (Helm), pv-migrate, TrueNAS API, Proxmox API, Prometheus/PromQL

**Spec:** `docs/superpowers/specs/2026-04-03-longhorn-nfs-migration-design.md`

---

## File Structure

```
longhorn-nfs/                          # New GitHub repo
├── scripts/
│   ├── baseline/
│   │   └── collect_baseline.py        # Query Prometheus for 7-day historical metrics, export to CSV
│   ├── migrate/
│   │   ├── migrate_pvc.sh             # Per-PVC migration: snapshot → pv-migrate → verify → cleanup
│   │   └── migrate_database.sh        # Per-database migration: pg_dump → restore → verify → cleanup
│   ├── provision/
│   │   ├── setup_truenas_nfs.sh       # Create NFS share on TrueNAS via API, configure ZFS snapshots
│   │   ├── setup_proxmox_nfs.sh       # Create ZFS dataset + NFS export on Proxmox
│   │   └── install_democratic_csi.sh  # Helm install democratic-csi + verify StorageClass
│   └── report/
│       └── compare_metrics.py         # Load baseline + post-migration CSVs, generate comparison report
├── config/
│   ├── workloads.yaml                 # PVC inventory: name, namespace, size, tier, migration order
│   ├── databases.yaml                 # Database inventory: name, namespace, version, size, connection info
│   ├── metrics.yaml                   # PromQL queries and metric definitions
│   └── credentials.env.example        # Template for API credentials (never committed with values)
├── k8s/
│   ├── democratic-csi-values.yaml     # Helm values for democratic-csi pointing to TrueNAS
│   ├── nfs-truenas-storageclass.yaml  # StorageClass definition for TrueNAS NFS
│   └── external-db-services/          # ExternalName Service manifests per namespace
│       ├── clearwatch-checkout.yaml
│       ├── clearwatch-research.yaml
│       ├── content-cache.yaml
│       ├── default.yaml
│       ├── infisical.yaml
│       ├── linkwarden.yaml
│       └── monitoring.yaml
├── docs/
│   ├── journal/                       # Session logs (LinkedIn content source material)
│   └── decisions/                     # ADR files
├── reports/                           # Generated metric reports (gitignored)
├── tests/
│   ├── test_baseline.py               # Tests for baseline collection scripts
│   ├── test_compare.py                # Tests for comparison report generation
│   └── test_config.py                 # Tests for config loading and validation
├── pyproject.toml                     # Python project config: deps, ruff, pytest
├── Makefile                           # make baseline, make report, make lint, make test
├── .github/
│   └── workflows/
│       └── lint.yaml                  # CI: ruff, shellcheck, yamllint, pytest
├── .gitignore
├── CLAUDE.md                          # Project-specific instructions for Claude
└── README.md
```

---

## Task 0: Create GitHub Repo and Project Scaffolding

**Files:**
- Create: all files in the repo root structure above

- [ ] **Step 0.1: Create GitHub repo**

```bash
gh repo create longhorn-nfs --private --clone --description "Longhorn to NFS storage migration tooling for K8s homelab"
cd longhorn-nfs
```

- [ ] **Step 0.2: Create .gitignore**

```gitignore
# Python
__pycache__/
*.pyc
.venv/
*.egg-info/

# Reports (generated, not committed)
reports/

# Credentials
credentials.env
.env

# OS
.DS_Store
```

- [ ] **Step 0.3: Create pyproject.toml**

```toml
[project]
name = "longhorn-nfs"
version = "0.1.0"
description = "Longhorn to NFS storage migration tooling"
requires-python = ">=3.12"
dependencies = [
    "prometheus-api-client>=0.5.0",
    "pandas>=2.0",
    "requests>=2.31",
    "pyyaml>=6.0",
    "tabulate>=0.9",
]

[project.optional-dependencies]
dev = [
    "pytest>=8.0",
    "ruff>=0.4",
]

[tool.ruff]
line-length = 120
target-version = "py312"

[tool.ruff.lint]
select = ["E", "F", "I", "W"]

[tool.pytest.ini_options]
testpaths = ["tests"]
```

- [ ] **Step 0.4: Create Makefile**

```makefile
.PHONY: baseline report lint test setup

setup:
	python3 -m venv .venv
	.venv/bin/pip install -e ".[dev]"

baseline:
	.venv/bin/python scripts/baseline/collect_baseline.py

report:
	.venv/bin/python scripts/report/compare_metrics.py

lint:
	.venv/bin/ruff check scripts/ tests/
	shellcheck scripts/**/*.sh
	yamllint config/ k8s/

test:
	.venv/bin/pytest tests/ -v
```

- [ ] **Step 0.5: Create GitHub Actions CI**

Create `.github/workflows/lint.yaml`:

```yaml
name: Lint & Test

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  lint-and-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Python
        uses: actions/setup-python@v5
        with:
          python-version: "3.12"

      - name: Install dependencies
        run: |
          python -m pip install --upgrade pip
          pip install -e ".[dev]"

      - name: Ruff lint
        run: ruff check scripts/ tests/

      - name: Shellcheck
        uses: ludeeus/action-shellcheck@2.0.0
        with:
          scandir: scripts/

      - name: Yamllint
        uses: ibiqlik/action-yamllint@v3
        with:
          file_or_dir: config/ k8s/

      - name: Pytest
        run: pytest tests/ -v
```

- [ ] **Step 0.6: Create credentials.env.example**

```bash
# TrueNAS API
TRUENAS_HOST=trunas.petersimmons.com
TRUENAS_API_KEY=your-api-key-here

# Proxmox API
PROXMOX_HOST=proxmox.petersimmons.com
PROXMOX_TOKEN_ID=root@pam!terraform
PROXMOX_TOKEN_SECRET=your-token-here

# Prometheus
PROMETHEUS_URL=http://prometheus.monitoring.svc.cluster.local:9090
```

- [ ] **Step 0.7: Create CLAUDE.md**

```markdown
# longhorn-nfs Project Instructions

## What This Is
Tooling for migrating K8s persistent storage from Longhorn to NFS and consolidating databases.
Design spec: `docs/superpowers/specs/2026-04-03-longhorn-nfs-migration-design.md`

## Key Rules
- Never commit credentials.env — use credentials.env.example as template
- All scripts must be idempotent — safe to re-run
- Every migration step must be logged in docs/journal/
- Run `make lint && make test` before every commit
- TDD: failing test first, then implementation

## API Access
- TrueNAS: Bearer token auth, creds in credentials.env
- Proxmox: PVEAPIToken auth, creds in credentials.env
- Prometheus: No auth (cluster-internal)

## Critical Safety
- NEVER touch Infisical PVCs without triple backup verified
- NEVER modify Traefik or CoreDNS
- Always snapshot Longhorn volume before migration
- Always retain old PVC for 48h after migration
```

- [ ] **Step 0.8: Create README.md**

```markdown
# longhorn-nfs

Tooling for migrating K8s persistent storage from Longhorn 3x replication to NFS, consolidating databases to TrueNAS, and eliminating Longhorn overhead.

## Quick Start

```bash
make setup           # Create venv, install deps
cp credentials.env.example credentials.env  # Fill in API keys
make baseline        # Collect Prometheus metrics
make report          # Generate comparison report
make lint            # Run all linters
make test            # Run tests
```

## Phases

- **Phase A:** Longhorn → TrueNAS NFS (non-database PVCs)
- **Phase B:** Database consolidation to TrueNAS Apps + Longhorn decommission
- **Phase C:** TrueNAS NFS → Proxmox-local NFS

See `docs/superpowers/specs/2026-04-03-longhorn-nfs-migration-design.md` for full design.

## Journal

Session logs in `docs/journal/` document every step for reproducibility and LinkedIn content reuse.
```

- [ ] **Step 0.9: Create directory stubs**

```bash
mkdir -p scripts/{baseline,migrate,provision,report}
mkdir -p config
mkdir -p k8s/external-db-services
mkdir -p docs/{journal,decisions}
mkdir -p reports
mkdir -p tests
touch scripts/baseline/__init__.py
touch scripts/report/__init__.py
touch tests/__init__.py
```

- [ ] **Step 0.10: Initial commit**

```bash
git add -A
git commit -m "feat: project scaffolding — CI, linting, Makefile, config templates"
git push -u origin main
```

---

## Task 1: Config Files — Workload and Database Inventory

**Files:**
- Create: `config/workloads.yaml`
- Create: `config/databases.yaml`
- Create: `config/metrics.yaml`
- Create: `tests/test_config.py`

- [ ] **Step 1.1: Write the test for config loading**

Create `tests/test_config.py`:

```python
import yaml
from pathlib import Path


def load_yaml(name: str) -> dict:
    path = Path(__file__).parent.parent / "config" / name
    with open(path) as f:
        return yaml.safe_load(f)


def test_workloads_has_required_fields():
    data = load_yaml("workloads.yaml")
    assert "pvcs" in data
    for pvc in data["pvcs"]:
        assert "name" in pvc
        assert "namespace" in pvc
        assert "size" in pvc
        assert "tier" in pvc, f"Missing tier for {pvc['name']}"
        assert pvc["tier"] in ("green", "yellow", "red", "skip")
        assert "phase" in pvc, f"Missing phase for {pvc['name']}"


def test_databases_has_required_fields():
    data = load_yaml("databases.yaml")
    assert "databases" in data
    for db in data["databases"]:
        assert "name" in db
        assert "namespace" in db
        assert "type" in db
        assert db["type"] in ("postgres", "mariadb", "redis", "qdrant")
        assert "tier" in db
        assert "phase" in db


def test_metrics_has_promql_queries():
    data = load_yaml("metrics.yaml")
    assert "metrics" in data
    for metric in data["metrics"]:
        assert "name" in metric
        assert "query" in metric
        assert "unit" in metric
```

- [ ] **Step 1.2: Run test to verify it fails**

```bash
cd longhorn-nfs
.venv/bin/pytest tests/test_config.py -v
```

Expected: FAIL — config files don't exist yet.

- [ ] **Step 1.3: Create config/workloads.yaml**

```yaml
# PVC inventory for migration planning
# tier: green (anytime), yellow (scheduled window), red (zero downtime), skip (not migrating)
# phase: A1, A2, A3, B1, B2, B3, B4, skip
pvcs:
  # Phase A.1 — Green tier (80 Gi)
  - name: registry-data-pvc
    namespace: container-registry
    size: 10Gi
    tier: green
    phase: A1
    workload: docker-registry
    type: deployment
    notes: "Degraded on Longhorn — good first candidate"

  - name: wordpress-content
    namespace: clearwatch
    size: 10Gi
    tier: green
    phase: A1
    workload: wordpress
    type: deployment
    notes: "Static content only"

  - name: plex-config-pvc
    namespace: plex
    size: 20Gi
    tier: green
    phase: A1
    workload: plex
    type: deployment
    notes: "Config only — media is external"

  - name: reports-pvc
    namespace: security-intelligence-business
    size: 20Gi
    tier: green
    phase: A1
    workload: security-reports
    type: deployment
    notes: "Static reports"

  - name: qdrant-storage-pvc
    namespace: content-cache
    size: 20Gi
    tier: green
    phase: A1
    workload: qdrant
    type: statefulset
    notes: "Vector DB — file-based storage, NFS works fine"

  # Phase A.2 — Yellow tier (81 Gi)
  - name: alertmanager-alertmanager-db-alertmanager-alertmanager-0
    namespace: monitoring
    size: 3Gi
    tier: yellow
    phase: A2
    workload: alertmanager
    type: statefulset
    notes: "Migrate with prometheus to minimize monitoring gap"

  - name: alertmanager-alertmanager-db-alertmanager-alertmanager-1
    namespace: monitoring
    size: 3Gi
    tier: yellow
    phase: A2
    workload: alertmanager
    type: statefulset
    notes: "Second alertmanager replica"

  - name: prometheus-prometheus-db-prometheus-prometheus-0
    namespace: monitoring
    size: 15Gi
    tier: yellow
    phase: A2
    workload: prometheus
    type: statefulset
    notes: "Time-series data — not a database"

  - name: common-crawl-raw-pvc
    namespace: clearwatch-research
    size: 30Gi
    tier: yellow
    phase: A2
    workload: clearwatch-research
    type: deployment
    notes: "Raw research data"

  - name: conference-raw-pvc
    namespace: clearwatch-research
    size: 20Gi
    tier: yellow
    phase: A2
    workload: clearwatch-research
    type: deployment
    notes: "Raw research data"

  - name: youtube-raw-pvc
    namespace: clearwatch-research
    size: 10Gi
    tier: yellow
    phase: A2
    workload: clearwatch-research
    type: deployment
    notes: "Raw research data"

  # Phase A.3 — Redis to emptyDir
  - name: infisical-redis-pvc
    namespace: infisical
    size: 5Gi
    tier: green
    phase: A3
    workload: infisical-redis
    type: deployment
    notes: "Ephemeral cache — swap to emptyDir, no NFS needed"

  - name: redis-pvc
    namespace: open-webui
    size: 5Gi
    tier: green
    phase: A3
    workload: open-webui-redis
    type: deployment
    notes: "Ephemeral cache — swap to emptyDir"

  # Database PVCs — stay on Longhorn until Phase B
  - name: postgres-pvc
    namespace: clearwatch-checkout
    size: 5Gi
    tier: yellow
    phase: B2
    workload: postgres-checkout
    type: deployment
    notes: "Database — migrated in Phase B"

  - name: research-db-pvc
    namespace: clearwatch-research
    size: 10Gi
    tier: yellow
    phase: B2
    workload: research-postgres
    type: statefulset
    notes: "Database — migrated in Phase B"

  - name: content-cache-db-pvc
    namespace: content-cache
    size: 50Gi
    tier: yellow
    phase: B2
    workload: content-cache-postgres
    type: statefulset
    notes: "Largest DB — test dump/restore timing first"

  - name: mariadb-data-mariadb-0
    namespace: clearwatch
    size: 5Gi
    tier: yellow
    phase: B3
    workload: mariadb
    type: statefulset
    notes: "WordPress MariaDB — migrated to TrueNAS MariaDB app"

  - name: infisical-postgres-pvc
    namespace: infisical
    size: 20Gi
    tier: red
    phase: B4
    workload: infisical-postgres
    type: statefulset
    notes: "CRITICAL — triple backup before touching. Last to migrate."

  - name: linkwarden-postgres-pvc
    namespace: linkwarden
    size: 5Gi
    tier: green
    phase: B1
    workload: linkwarden-postgres
    type: deployment
    notes: "Low-usage app database"

  - name: postgres-data-proxmox-monitor-postgres-0
    namespace: monitoring
    size: 20Gi
    tier: green
    phase: B1
    workload: proxmox-monitor-postgres
    type: statefulset
    notes: "Monitoring DB — can rebuild if needed"

  - name: ciso-tracker-db-pvc
    namespace: default
    size: 100Mi
    tier: green
    phase: B1
    workload: ciso-tracker
    type: deployment
    notes: "Tiny DB — lowest risk, first to migrate"

  # Not on Longhorn — skip
  - name: postgres-data-postgres-0
    namespace: n8n
    size: 5Gi
    tier: skip
    phase: skip
    workload: n8n-postgres
    type: statefulset
    notes: "On local-path, not Longhorn. Optional consolidation in Phase B."

  - name: open-webui-pvc
    namespace: open-webui
    size: 10Gi
    tier: skip
    phase: skip
    workload: open-webui
    type: deployment
    notes: "On local-path, not Longhorn."
```

- [ ] **Step 1.4: Create config/databases.yaml**

```yaml
# Database inventory for Phase B consolidation
# All connection strings are managed in Infisical
databases:
  # Phase B.1 — Green tier
  - name: ciso-tracker-db
    namespace: default
    type: postgres
    image: unknown
    size: 100Mi
    tier: green
    phase: B1
    pvc: ciso-tracker-db-pvc
    notes: "Tiny, lowest risk — first to migrate"

  - name: linkwarden-postgres
    namespace: linkwarden
    type: postgres
    image: unknown
    size: 5Gi
    tier: green
    phase: B1
    pvc: linkwarden-postgres-pvc
    notes: "Low-usage app"

  - name: proxmox-monitor-postgres
    namespace: monitoring
    type: postgres
    image: postgres:16-alpine
    size: 20Gi
    tier: green
    phase: B1
    pvc: postgres-data-proxmox-monitor-postgres-0
    notes: "Monitoring — can rebuild"

  # Phase B.2 — Yellow tier
  - name: postgres-checkout
    namespace: clearwatch-checkout
    type: postgres
    image: postgres:16-alpine
    size: 5Gi
    tier: yellow
    phase: B2
    pvc: postgres-pvc
    notes: "Clearwatch revenue app"

  - name: research-postgres
    namespace: clearwatch-research
    type: postgres
    image: cgr.dev/chainguard/postgres:latest
    size: 10Gi
    tier: yellow
    phase: B2
    pvc: research-db-pvc
    notes: "Research pipeline"

  - name: content-cache-postgres
    namespace: content-cache
    type: postgres
    image: cgr.dev/chainguard/postgres:latest
    size: 50Gi
    tier: yellow
    phase: B2
    pvc: content-cache-db-pvc
    notes: "Largest DB — test dump/restore timing before migration"

  # Phase B.3 — MariaDB
  - name: mariadb
    namespace: clearwatch
    type: mariadb
    image: cgr.dev/chainguard/mariadb:latest
    size: 5Gi
    tier: yellow
    phase: B3
    pvc: mariadb-data-mariadb-0
    notes: "WordPress DB — separate TrueNAS MariaDB app"

  # Phase B.4 — Infisical (CRITICAL)
  - name: infisical-postgres
    namespace: infisical
    type: postgres
    image: cgr.dev/chainguard/postgres:latest
    size: 20Gi
    tier: red
    phase: B4
    pvc: infisical-postgres-pvc
    notes: "CRITICAL — secrets store. Triple backup mandatory. Last to migrate."

  # Redis — emptyDir (Phase A.3, not B)
  - name: infisical-redis
    namespace: infisical
    type: redis
    image: cgr.dev/chainguard/redis:latest
    size: 5Gi
    tier: green
    phase: A3
    pvc: infisical-redis-pvc
    notes: "Ephemeral cache — swap to emptyDir"

  - name: open-webui-redis
    namespace: open-webui
    type: redis
    image: cgr.dev/chainguard/redis:latest
    size: 5Gi
    tier: green
    phase: A3
    pvc: redis-pvc
    notes: "Ephemeral cache — swap to emptyDir"

  # Qdrant — NFS (Phase A.1)
  - name: qdrant
    namespace: content-cache
    type: qdrant
    image: qdrant/qdrant:v1.7.4
    size: 20Gi
    tier: green
    phase: A1
    pvc: qdrant-storage-pvc
    notes: "Vector DB — file-based, migrated to NFS in Phase A"

  # Not on Longhorn — skip
  - name: n8n-postgres
    namespace: n8n
    type: postgres
    image: cgr.dev/chainguard/postgres:latest
    size: 5Gi
    tier: skip
    phase: skip
    pvc: postgres-data-postgres-0
    notes: "On local-path. Optional consolidation."
```

- [ ] **Step 1.5: Create config/metrics.yaml**

```yaml
# Prometheus metrics for baseline and comparison
# Each query returns a time series for the specified range
metrics:
  - name: proxmox_host_cpu_percent
    query: '100 * (1 - avg(rate(node_cpu_seconds_total{mode="idle"}[5m])))'
    unit: percent
    description: "Proxmox host CPU utilization"

  - name: proxmox_load_average_1m
    query: 'node_load1'
    unit: count
    description: "1-minute load average"

  - name: per_node_cpu_percent
    query: 'sum by (node) (rate(container_cpu_usage_seconds_total{namespace!=""}[5m])) / on(node) kube_node_status_capacity{resource="cpu"} * 100'
    unit: percent
    description: "Per K8s node CPU utilization"

  - name: longhorn_instance_manager_cpu
    query: 'sum(rate(container_cpu_usage_seconds_total{namespace="longhorn-system", pod=~"instance-manager.*"}[5m]))'
    unit: cores
    description: "Total Longhorn instance-manager CPU usage"

  - name: longhorn_manager_cpu
    query: 'sum(rate(container_cpu_usage_seconds_total{namespace="longhorn-system", pod=~"longhorn-manager.*"}[5m]))'
    unit: cores
    description: "Total Longhorn manager CPU usage"

  - name: longhorn_total_cpu
    query: 'sum(rate(container_cpu_usage_seconds_total{namespace="longhorn-system"}[5m]))'
    unit: cores
    description: "Total CPU used by all Longhorn pods"

  - name: disk_read_bytes
    query: 'sum(rate(node_disk_read_bytes_total[5m]))'
    unit: bytes_per_sec
    description: "Total disk read throughput"

  - name: disk_write_bytes
    query: 'sum(rate(node_disk_written_bytes_total[5m]))'
    unit: bytes_per_sec
    description: "Total disk write throughput"

  - name: network_receive_bytes
    query: 'sum(rate(node_network_receive_bytes_total{device!="lo"}[5m]))'
    unit: bytes_per_sec
    description: "Total network receive throughput"

  - name: network_transmit_bytes
    query: 'sum(rate(node_network_transmit_bytes_total{device!="lo"}[5m]))'
    unit: bytes_per_sec
    description: "Total network transmit throughput"

  - name: iowait_percent
    query: 'avg(rate(node_cpu_seconds_total{mode="iowait"}[5m])) * 100'
    unit: percent
    description: "Average I/O wait percentage"

  - name: pod_restart_count
    query: 'sum(increase(kube_pod_container_status_restarts_total[1h]))'
    unit: count
    description: "Total pod restarts in last hour"
```

- [ ] **Step 1.6: Run tests to verify they pass**

```bash
.venv/bin/pytest tests/test_config.py -v
```

Expected: PASS — all three config tests pass.

- [ ] **Step 1.7: Commit**

```bash
git add config/ tests/test_config.py
git commit -m "feat: workload, database, and metrics config with validation tests"
```

---

## Task 2: Baseline Collection Script

**Files:**
- Create: `scripts/baseline/collect_baseline.py`
- Create: `tests/test_baseline.py`

- [ ] **Step 2.1: Write the test**

Create `tests/test_baseline.py`:

```python
import json
from unittest.mock import patch, MagicMock
from pathlib import Path
import yaml

# We test the core logic without hitting Prometheus
import importlib.util

SCRIPT_PATH = Path(__file__).parent.parent / "scripts" / "baseline" / "collect_baseline.py"


def load_metrics_config() -> list[dict]:
    config_path = Path(__file__).parent.parent / "config" / "metrics.yaml"
    with open(config_path) as f:
        return yaml.safe_load(f)["metrics"]


def test_metrics_config_has_all_required_queries():
    metrics = load_metrics_config()
    names = [m["name"] for m in metrics]
    required = [
        "proxmox_host_cpu_percent",
        "longhorn_total_cpu",
        "disk_read_bytes",
        "disk_write_bytes",
        "network_receive_bytes",
        "network_transmit_bytes",
    ]
    for name in required:
        assert name in names, f"Missing required metric: {name}"


def test_metrics_config_all_have_query_and_unit():
    metrics = load_metrics_config()
    for m in metrics:
        assert m.get("query"), f"Metric {m['name']} missing query"
        assert m.get("unit"), f"Metric {m['name']} missing unit"
```

- [ ] **Step 2.2: Run test to verify it passes** (these test config, not the script yet)

```bash
.venv/bin/pytest tests/test_baseline.py -v
```

Expected: PASS

- [ ] **Step 2.3: Write collect_baseline.py**

Create `scripts/baseline/collect_baseline.py`:

```python
#!/usr/bin/env python3
"""Collect Prometheus baseline metrics and export to CSV.

Queries Prometheus for 7 days of historical data using the metrics
defined in config/metrics.yaml. Outputs one CSV per metric to reports/baseline/.

Usage:
    python scripts/baseline/collect_baseline.py [--prometheus-url URL] [--days DAYS] [--output-dir DIR]
"""

import argparse
import csv
import os
import sys
from datetime import datetime, timedelta, timezone
from pathlib import Path

import requests
import yaml


def load_metrics_config(config_path: Path) -> list[dict]:
    with open(config_path) as f:
        return yaml.safe_load(f)["metrics"]


def query_prometheus_range(
    base_url: str, query: str, start: datetime, end: datetime, step: str = "5m"
) -> list[dict]:
    """Query Prometheus range API and return list of {timestamp, value} dicts."""
    resp = requests.get(
        f"{base_url}/api/v1/query_range",
        params={
            "query": query,
            "start": start.timestamp(),
            "end": end.timestamp(),
            "step": step,
        },
        timeout=60,
    )
    resp.raise_for_status()
    data = resp.json()

    if data["status"] != "success":
        print(f"  WARNING: Query returned status={data['status']}: {query}", file=sys.stderr)
        return []

    results = []
    for series in data.get("data", {}).get("result", []):
        labels = series.get("metric", {})
        for timestamp, value in series.get("values", []):
            results.append({
                "timestamp": datetime.fromtimestamp(timestamp, tz=timezone.utc).isoformat(),
                "value": float(value),
                "labels": str(labels) if labels else "",
            })
    return results


def write_csv(output_path: Path, rows: list[dict]) -> None:
    if not rows:
        print(f"  No data to write for {output_path.name}")
        return
    output_path.parent.mkdir(parents=True, exist_ok=True)
    with open(output_path, "w", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=["timestamp", "value", "labels"])
        writer.writeheader()
        writer.writerows(rows)
    print(f"  Wrote {len(rows)} rows to {output_path}")


def main():
    parser = argparse.ArgumentParser(description="Collect Prometheus baseline metrics")
    parser.add_argument(
        "--prometheus-url",
        default=os.environ.get("PROMETHEUS_URL", "http://prometheus.monitoring.svc.cluster.local:9090"),
        help="Prometheus base URL",
    )
    parser.add_argument("--days", type=int, default=7, help="Days of history to collect")
    parser.add_argument("--output-dir", type=Path, default=Path("reports/baseline"), help="Output directory")
    parser.add_argument("--config", type=Path, default=Path("config/metrics.yaml"), help="Metrics config file")
    args = parser.parse_args()

    metrics = load_metrics_config(args.config)
    end = datetime.now(tz=timezone.utc)
    start = end - timedelta(days=args.days)

    print(f"Collecting {len(metrics)} metrics from {start.date()} to {end.date()}")
    print(f"Prometheus: {args.prometheus_url}")
    print(f"Output: {args.output_dir}")
    print()

    for metric in metrics:
        print(f"Querying: {metric['name']} ({metric['description']})")
        rows = query_prometheus_range(args.prometheus_url, metric["query"], start, end)
        output_path = args.output_dir / f"{metric['name']}.csv"
        write_csv(output_path, rows)

    print(f"\nBaseline collection complete. {len(metrics)} metrics exported to {args.output_dir}/")


if __name__ == "__main__":
    main()
```

- [ ] **Step 2.4: Run linter**

```bash
.venv/bin/ruff check scripts/baseline/collect_baseline.py
```

Expected: PASS

- [ ] **Step 2.5: Commit**

```bash
git add scripts/baseline/collect_baseline.py tests/test_baseline.py
git commit -m "feat: baseline metrics collection script with Prometheus range queries"
```

---

## Task 3: Comparison Report Script

**Files:**
- Create: `scripts/report/compare_metrics.py`
- Create: `tests/test_compare.py`

- [ ] **Step 3.1: Write the test**

Create `tests/test_compare.py`:

```python
import csv
import tempfile
from pathlib import Path


def create_csv(path: Path, rows: list[dict]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with open(path, "w", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=["timestamp", "value", "labels"])
        writer.writeheader()
        writer.writerows(rows)


def test_load_csv_and_compute_stats():
    """Test that we can load CSVs and compute basic statistics."""
    from scripts.report.compare_metrics import load_metric_csv, compute_stats

    with tempfile.TemporaryDirectory() as tmpdir:
        csv_path = Path(tmpdir) / "test_metric.csv"
        create_csv(csv_path, [
            {"timestamp": "2026-04-01T00:00:00+00:00", "value": "10.0", "labels": ""},
            {"timestamp": "2026-04-01T01:00:00+00:00", "value": "20.0", "labels": ""},
            {"timestamp": "2026-04-01T02:00:00+00:00", "value": "30.0", "labels": ""},
        ])
        values = load_metric_csv(csv_path)
        stats = compute_stats(values)

        assert stats["mean"] == 20.0
        assert stats["min"] == 10.0
        assert stats["max"] == 30.0
        assert stats["count"] == 3


def test_compute_delta():
    from scripts.report.compare_metrics import compute_delta

    baseline = {"mean": 30.0, "min": 20.0, "max": 40.0}
    current = {"mean": 20.0, "min": 15.0, "max": 25.0}
    delta = compute_delta(baseline, current)

    assert delta["mean_delta"] == -10.0
    assert delta["mean_percent_change"] == -33.33
```

- [ ] **Step 3.2: Run test to verify it fails**

```bash
.venv/bin/pytest tests/test_compare.py -v
```

Expected: FAIL — module doesn't exist yet.

- [ ] **Step 3.3: Write compare_metrics.py**

Create `scripts/report/compare_metrics.py`:

```python
#!/usr/bin/env python3
"""Compare baseline and post-migration Prometheus metrics.

Loads CSV files from two directories (baseline and current), computes
statistics, and generates a comparison report.

Usage:
    python scripts/report/compare_metrics.py --baseline reports/baseline --current reports/post-a1 [--output reports/comparison-a1.md]
"""

import argparse
import csv
from pathlib import Path

import yaml


def load_metric_csv(path: Path) -> list[float]:
    """Load a metric CSV and return list of float values."""
    values = []
    with open(path) as f:
        reader = csv.DictReader(f)
        for row in reader:
            values.append(float(row["value"]))
    return values


def compute_stats(values: list[float]) -> dict:
    """Compute basic statistics for a list of values."""
    if not values:
        return {"mean": 0.0, "min": 0.0, "max": 0.0, "count": 0}
    return {
        "mean": round(sum(values) / len(values), 2),
        "min": round(min(values), 2),
        "max": round(max(values), 2),
        "count": len(values),
    }


def compute_delta(baseline: dict, current: dict) -> dict:
    """Compute the difference between baseline and current stats."""
    mean_delta = round(current["mean"] - baseline["mean"], 2)
    if baseline["mean"] != 0:
        pct = round((mean_delta / baseline["mean"]) * 100, 2)
    else:
        pct = 0.0
    return {
        "mean_delta": mean_delta,
        "mean_percent_change": pct,
    }


def load_metrics_config(config_path: Path) -> list[dict]:
    with open(config_path) as f:
        return yaml.safe_load(f)["metrics"]


def generate_report(
    baseline_dir: Path, current_dir: Path, metrics: list[dict]
) -> str:
    """Generate a markdown comparison report."""
    lines = [
        "# Metrics Comparison Report",
        "",
        f"**Baseline:** `{baseline_dir}`",
        f"**Current:** `{current_dir}`",
        "",
        "| Metric | Unit | Baseline (mean) | Current (mean) | Delta | Change |",
        "|--------|------|-----------------|----------------|-------|--------|",
    ]

    for metric in metrics:
        name = metric["name"]
        unit = metric["unit"]
        baseline_csv = baseline_dir / f"{name}.csv"
        current_csv = current_dir / f"{name}.csv"

        if not baseline_csv.exists() or not current_csv.exists():
            lines.append(f"| {name} | {unit} | — | — | — | missing data |")
            continue

        baseline_stats = compute_stats(load_metric_csv(baseline_csv))
        current_stats = compute_stats(load_metric_csv(current_csv))
        delta = compute_delta(baseline_stats, current_stats)

        emoji = "🟢" if delta["mean_percent_change"] < 0 else "🔴" if delta["mean_percent_change"] > 5 else "⚪"
        lines.append(
            f"| {name} | {unit} | {baseline_stats['mean']} | {current_stats['mean']} "
            f"| {delta['mean_delta']:+} | {emoji} {delta['mean_percent_change']:+}% |"
        )

    lines.extend(["", f"*Generated: report covers {metrics[0]['name']} through {metrics[-1]['name']}*"])
    return "\n".join(lines)


def main():
    parser = argparse.ArgumentParser(description="Compare baseline and post-migration metrics")
    parser.add_argument("--baseline", type=Path, required=True, help="Baseline metrics directory")
    parser.add_argument("--current", type=Path, required=True, help="Current metrics directory")
    parser.add_argument("--output", type=Path, default=None, help="Output report path (default: stdout)")
    parser.add_argument("--config", type=Path, default=Path("config/metrics.yaml"), help="Metrics config")
    args = parser.parse_args()

    metrics = load_metrics_config(args.config)
    report = generate_report(args.baseline, args.current, metrics)

    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(report)
        print(f"Report written to {args.output}")
    else:
        print(report)


if __name__ == "__main__":
    main()
```

- [ ] **Step 3.4: Run tests**

```bash
.venv/bin/pytest tests/test_compare.py -v
```

Expected: PASS

- [ ] **Step 3.5: Run linter**

```bash
.venv/bin/ruff check scripts/report/compare_metrics.py
```

Expected: PASS

- [ ] **Step 3.6: Commit**

```bash
git add scripts/report/compare_metrics.py tests/test_compare.py
git commit -m "feat: metrics comparison report generator with delta and percent change"
```

---

## Task 4: TrueNAS NFS Provisioning Script

**Files:**
- Create: `scripts/provision/setup_truenas_nfs.sh`

- [ ] **Step 4.1: Write setup_truenas_nfs.sh**

```bash
#!/usr/bin/env bash
# Setup TrueNAS NFS share for K8s storage
# Prerequisites: TRUENAS_HOST and TRUENAS_API_KEY set in environment or credentials.env
# Idempotent: safe to re-run

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CREDS_FILE="${SCRIPT_DIR}/../../credentials.env"

# Load credentials if file exists
if [[ -f "$CREDS_FILE" ]]; then
    # shellcheck source=/dev/null
    source "$CREDS_FILE"
fi

: "${TRUENAS_HOST:?TRUENAS_HOST not set}"
: "${TRUENAS_API_KEY:?TRUENAS_API_KEY not set}"

BASE_URL="https://${TRUENAS_HOST}/api/v2.0"
AUTH_HEADER="Authorization: Bearer ${TRUENAS_API_KEY}"
DATASET_NAME="zp1/k8s-nfs"
NFS_PATH="/mnt/zp1/k8s-nfs"

echo "=== TrueNAS NFS Setup for K8s ==="
echo "Host: ${TRUENAS_HOST}"
echo "Dataset: ${DATASET_NAME}"
echo "NFS Path: ${NFS_PATH}"
echo ""

# Step 1: Check if dataset exists
echo "Checking if dataset ${DATASET_NAME} exists..."
DATASET_CHECK=$(curl -sk -o /dev/null -w "%{http_code}" \
    -H "${AUTH_HEADER}" \
    "${BASE_URL}/pool/dataset/id/$(echo "${DATASET_NAME}" | sed 's|/|%2F|g')")

if [[ "$DATASET_CHECK" == "200" ]]; then
    echo "  Dataset already exists — skipping creation"
else
    echo "  Creating dataset ${DATASET_NAME}..."
    curl -sk -X POST \
        -H "${AUTH_HEADER}" \
        -H "Content-Type: application/json" \
        -d '{
            "name": "'"${DATASET_NAME}"'",
            "sync": "STANDARD",
            "deduplication": "OFF",
            "compression": "LZ4",
            "atime": "OFF"
        }' \
        "${BASE_URL}/pool/dataset" | python3 -m json.tool
    echo "  Dataset created"
fi

# Step 2: Verify dataset properties
echo ""
echo "Verifying dataset properties..."
PROPS=$(curl -sk \
    -H "${AUTH_HEADER}" \
    "${BASE_URL}/pool/dataset/id/$(echo "${DATASET_NAME}" | sed 's|/|%2F|g')")

SYNC=$(echo "$PROPS" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['sync']['value'])")
DEDUP=$(echo "$PROPS" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['deduplication']['value'])")
COMPRESS=$(echo "$PROPS" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['compression']['value'])")

echo "  sync=${SYNC} (expected: STANDARD)"
echo "  dedup=${DEDUP} (expected: OFF)"
echo "  compression=${COMPRESS} (expected: LZ4)"

if [[ "$SYNC" != "STANDARD" ]] || [[ "$DEDUP" != "OFF" ]]; then
    echo "  WARNING: Properties don't match expected values!"
    exit 1
fi

# Step 3: Check if NFS share exists
echo ""
echo "Checking for existing NFS share at ${NFS_PATH}..."
NFS_SHARES=$(curl -sk \
    -H "${AUTH_HEADER}" \
    "${BASE_URL}/sharing/nfs")

EXISTING_SHARE=$(echo "$NFS_SHARES" | python3 -c "
import sys, json
shares = json.load(sys.stdin)
for s in shares:
    if s.get('path') == '${NFS_PATH}':
        print(s['id'])
        break
else:
    print('NONE')
")

if [[ "$EXISTING_SHARE" != "NONE" ]]; then
    echo "  NFS share already exists (id=${EXISTING_SHARE}) — skipping creation"
else
    echo "  Creating NFS share..."
    curl -sk -X POST \
        -H "${AUTH_HEADER}" \
        -H "Content-Type: application/json" \
        -d '{
            "path": "'"${NFS_PATH}"'",
            "comment": "K8s persistent storage via democratic-csi",
            "networks": ["192.168.0.0/24", "10.42.0.0/16"],
            "maproot_user": "root",
            "maproot_group": "wheel"
        }' \
        "${BASE_URL}/sharing/nfs" | python3 -m json.tool
    echo "  NFS share created"
fi

# Step 4: Configure ZFS periodic snapshots
echo ""
echo "Checking for periodic snapshot task..."
SNAP_TASKS=$(curl -sk \
    -H "${AUTH_HEADER}" \
    "${BASE_URL}/pool/snapshottask")

EXISTING_SNAP=$(echo "$SNAP_TASKS" | python3 -c "
import sys, json
tasks = json.load(sys.stdin)
for t in tasks:
    if t.get('dataset') == '${DATASET_NAME}':
        print(t['id'])
        break
else:
    print('NONE')
")

if [[ "$EXISTING_SNAP" != "NONE" ]]; then
    echo "  Snapshot task already exists (id=${EXISTING_SNAP}) — skipping"
else
    echo "  Creating hourly snapshot task (retain 24)..."
    curl -sk -X POST \
        -H "${AUTH_HEADER}" \
        -H "Content-Type: application/json" \
        -d '{
            "dataset": "'"${DATASET_NAME}"'",
            "recursive": true,
            "lifetime_value": 24,
            "lifetime_unit": "HOUR",
            "naming_schema": "auto-%Y-%m-%d_%H-%M",
            "schedule": {
                "minute": "0",
                "hour": "*",
                "dom": "*",
                "month": "*",
                "dow": "*"
            },
            "enabled": true
        }' \
        "${BASE_URL}/pool/snapshottask" | python3 -m json.tool
    echo "  Snapshot task created"
fi

echo ""
echo "=== TrueNAS NFS Setup Complete ==="
echo ""
echo "Next steps:"
echo "  1. Install democratic-csi: bash scripts/provision/install_democratic_csi.sh"
echo "  2. Verify NFS mount from a K8s node: mount -t nfs4 ${TRUENAS_HOST}:${NFS_PATH} /mnt/test"
```

- [ ] **Step 4.2: Make executable and lint**

```bash
chmod +x scripts/provision/setup_truenas_nfs.sh
shellcheck scripts/provision/setup_truenas_nfs.sh
```

Expected: PASS (or minor warnings to fix)

- [ ] **Step 4.3: Commit**

```bash
git add scripts/provision/setup_truenas_nfs.sh
git commit -m "feat: TrueNAS NFS provisioning script — dataset, share, ZFS snapshots"
```

---

## Task 5: Democratic-CSI Installation Script

**Files:**
- Create: `scripts/provision/install_democratic_csi.sh`
- Create: `k8s/democratic-csi-values.yaml`
- Create: `k8s/nfs-truenas-storageclass.yaml`

- [ ] **Step 5.1: Create Helm values file**

Create `k8s/democratic-csi-values.yaml`:

```yaml
# democratic-csi Helm values for TrueNAS NFS
# Docs: https://github.com/democratic-csi/democratic-csi
csiDriver:
  name: "org.democratic-csi.nfs-truenas"

storageClasses:
  - name: nfs-truenas
    defaultClass: false
    reclaimPolicy: Delete
    volumeBindingMode: Immediate
    allowVolumeExpansion: true
    parameters:
      fsType: nfs
    mountOptions:
      - hard
      - proto=tcp
      - timeo=600
      - retrans=2
      - nfsvers=4.1

driver:
  config:
    driver: freenas-nfs
    instance_id: truenas-nfs
    httpConnection:
      protocol: https
      host: trunas.petersimmons.com
      port: 443
      allowInsecure: true
      # apiKey injected via secret
    sshConnection:
      host: trunas.petersimmons.com
      port: 22
      # username and privateKey injected via secret
    zfs:
      datasetParentName: zp1/k8s-nfs/volumes
      detachedSnapshotsDatasetParentName: zp1/k8s-nfs/snapshots
      datasetProperties:
        "org.freenas:description": "{{ parameters.[csi.storage.k8s.io/pvc/namespace] }}/{{ parameters.[csi.storage.k8s.io/pvc/name] }}"
    nfs:
      shareHost: trunas.petersimmons.com
      shareAlldirs: false
      shareAllowedHosts: []
      shareAllowedNetworks:
        - 192.168.0.0/24
        - 10.42.0.0/16
      shareMaprootUser: root
      shareMaprootGroup: wheel
```

- [ ] **Step 5.2: Create install script**

Create `scripts/provision/install_democratic_csi.sh`:

```bash
#!/usr/bin/env bash
# Install democratic-csi for TrueNAS NFS provisioning
# Prerequisites: helm, kubectl, TrueNAS API key
# Idempotent: safe to re-run

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="${SCRIPT_DIR}/../.."
CREDS_FILE="${PROJECT_DIR}/credentials.env"

# Load credentials
if [[ -f "$CREDS_FILE" ]]; then
    # shellcheck source=/dev/null
    source "$CREDS_FILE"
fi

: "${TRUENAS_API_KEY:?TRUENAS_API_KEY not set}"

NAMESPACE="democratic-csi"
RELEASE_NAME="truenas-nfs"
HELM_REPO="https://democratic-csi.github.io/charts/"
VALUES_FILE="${PROJECT_DIR}/k8s/democratic-csi-values.yaml"

echo "=== Installing democratic-csi ==="
echo ""

# Step 1: Add Helm repo
echo "Adding democratic-csi Helm repo..."
helm repo add democratic-csi "${HELM_REPO}" 2>/dev/null || true
helm repo update

# Step 2: Create namespace
echo "Creating namespace ${NAMESPACE}..."
kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

# Step 3: Create secret with TrueNAS API key
echo "Creating TrueNAS API key secret..."
kubectl create secret generic truenas-api-key \
    --namespace="${NAMESPACE}" \
    --from-literal=apiKey="${TRUENAS_API_KEY}" \
    --dry-run=client -o yaml | kubectl apply -f -

# Step 4: Install/upgrade democratic-csi
echo "Installing democratic-csi Helm chart..."
helm upgrade --install "${RELEASE_NAME}" democratic-csi/democratic-csi \
    --namespace="${NAMESPACE}" \
    --values="${VALUES_FILE}" \
    --set driver.config.httpConnection.apiKey="${TRUENAS_API_KEY}" \
    --wait \
    --timeout=5m

# Step 5: Verify StorageClass exists
echo ""
echo "Verifying StorageClass..."
kubectl get sc nfs-truenas -o wide
echo ""

# Step 6: Test PVC creation
echo "Creating test PVC..."
kubectl apply -f - <<'EOF'
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: democratic-csi-test
  namespace: default
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: nfs-truenas
EOF

echo "Waiting for test PVC to bind..."
kubectl wait --for=condition=Bound pvc/democratic-csi-test -n default --timeout=60s

echo ""
echo "Test PVC status:"
kubectl get pvc democratic-csi-test -n default

echo ""
echo "Cleaning up test PVC..."
kubectl delete pvc democratic-csi-test -n default

echo ""
echo "=== democratic-csi Installation Complete ==="
echo ""
echo "StorageClass 'nfs-truenas' is ready for use."
echo "Next: run Phase A.1 migrations with scripts/migrate/migrate_pvc.sh"
```

- [ ] **Step 5.3: Make executable and lint**

```bash
chmod +x scripts/provision/install_democratic_csi.sh
shellcheck scripts/provision/install_democratic_csi.sh
```

- [ ] **Step 5.4: Commit**

```bash
git add k8s/democratic-csi-values.yaml scripts/provision/install_democratic_csi.sh
git commit -m "feat: democratic-csi install script with Helm values for TrueNAS NFS"
```

---

## Task 6: PVC Migration Script

**Files:**
- Create: `scripts/migrate/migrate_pvc.sh`

**Prerequisite:** Install pv-migrate before running this script:
```bash
# Option A: Go install
go install github.com/utkuozdemir/pv-migrate@latest

# Option B: Download binary
curl -LO https://github.com/utkuozdemir/pv-migrate/releases/latest/download/pv-migrate_linux_amd64.tar.gz
tar xzf pv-migrate_linux_amd64.tar.gz
sudo mv pv-migrate /usr/local/bin/
```

- [ ] **Step 6.1: Write migrate_pvc.sh**

```bash
#!/usr/bin/env bash
# Migrate a single PVC from Longhorn to NFS using pv-migrate
# Usage: ./migrate_pvc.sh <namespace> <pvc-name> <workload-name> <workload-type>
# Example: ./migrate_pvc.sh container-registry registry-data-pvc docker-registry deployment
#
# Idempotent: checks state before each step. Safe to re-run after partial failure.
# The old Longhorn PVC is renamed with -old suffix and retained for 48h.

set -euo pipefail

NAMESPACE="${1:?Usage: $0 <namespace> <pvc-name> <workload-name> <workload-type>}"
PVC_NAME="${2:?Usage: $0 <namespace> <pvc-name> <workload-name> <workload-type>}"
WORKLOAD_NAME="${3:?Usage: $0 <namespace> <pvc-name> <workload-name> <workload-type>}"
WORKLOAD_TYPE="${4:?Usage: $0 <namespace> <pvc-name> <workload-name> <workload-type>}"

NEW_PVC_NAME="${PVC_NAME}-nfs"
TARGET_SC="nfs-truenas"

echo "=== PVC Migration: ${NAMESPACE}/${PVC_NAME} ==="
echo "Workload: ${WORKLOAD_NAME} (${WORKLOAD_TYPE})"
echo "Source: Longhorn → Target: ${TARGET_SC}"
echo ""

# Step 1: Verify source PVC exists
echo "[1/7] Verifying source PVC..."
kubectl get pvc "${PVC_NAME}" -n "${NAMESPACE}" || {
    echo "ERROR: Source PVC ${PVC_NAME} not found in namespace ${NAMESPACE}"
    exit 1
}

# Get PVC size
PVC_SIZE=$(kubectl get pvc "${PVC_NAME}" -n "${NAMESPACE}" -o jsonpath='{.status.capacity.storage}')
echo "  Size: ${PVC_SIZE}"

# Step 2: Snapshot Longhorn volume (safety net)
echo ""
echo "[2/7] Creating Longhorn snapshot..."
VOLUME_NAME=$(kubectl get pvc "${PVC_NAME}" -n "${NAMESPACE}" -o jsonpath='{.spec.volumeName}')
if [[ -n "$VOLUME_NAME" ]]; then
    # Create snapshot via Longhorn API
    kubectl -n longhorn-system exec deploy/longhorn-driver-deployer -- \
        curl -s -X POST "http://longhorn-backend:9500/v1/volumes/${VOLUME_NAME}?action=snapshotCreate" \
        -H "Content-Type: application/json" \
        -d '{"name": "pre-migration-'"$(date +%Y%m%d-%H%M%S)"'"}' 2>/dev/null || \
        echo "  WARNING: Could not create Longhorn snapshot (non-fatal, continuing)"
    echo "  Snapshot requested for volume ${VOLUME_NAME}"
else
    echo "  WARNING: No volume name found — skipping snapshot"
fi

# Step 3: Scale workload to 0
echo ""
echo "[3/7] Scaling ${WORKLOAD_TYPE}/${WORKLOAD_NAME} to 0..."
CURRENT_REPLICAS=$(kubectl get "${WORKLOAD_TYPE}/${WORKLOAD_NAME}" -n "${NAMESPACE}" -o jsonpath='{.spec.replicas}')
echo "  Current replicas: ${CURRENT_REPLICAS}"
kubectl scale "${WORKLOAD_TYPE}/${WORKLOAD_NAME}" -n "${NAMESPACE}" --replicas=0
kubectl rollout status "${WORKLOAD_TYPE}/${WORKLOAD_NAME}" -n "${NAMESPACE}" --timeout=120s 2>/dev/null || true
echo "  Scaled to 0"

# Step 4: Run pv-migrate
echo ""
echo "[4/7] Migrating data with pv-migrate..."
echo "  Source: ${PVC_NAME} → Dest: ${NEW_PVC_NAME} (${TARGET_SC})"
pv-migrate migrate \
    --source-namespace "${NAMESPACE}" \
    --source "${PVC_NAME}" \
    --dest-namespace "${NAMESPACE}" \
    --dest "${NEW_PVC_NAME}" \
    --dest-storage-class "${TARGET_SC}" \
    --dest-access-mode ReadWriteOnce \
    --no-progress-bar=false

echo "  Migration complete"

# Step 5: Verify new PVC
echo ""
echo "[5/7] Verifying new PVC..."
kubectl get pvc "${NEW_PVC_NAME}" -n "${NAMESPACE}"
NEW_PVC_STATUS=$(kubectl get pvc "${NEW_PVC_NAME}" -n "${NAMESPACE}" -o jsonpath='{.status.phase}')
if [[ "$NEW_PVC_STATUS" != "Bound" ]]; then
    echo "ERROR: New PVC is not Bound (status: ${NEW_PVC_STATUS})"
    echo "Rolling back: scaling ${WORKLOAD_TYPE}/${WORKLOAD_NAME} back to ${CURRENT_REPLICAS}"
    kubectl scale "${WORKLOAD_TYPE}/${WORKLOAD_NAME}" -n "${NAMESPACE}" --replicas="${CURRENT_REPLICAS}"
    exit 1
fi

# Step 6: Update workload to use new PVC
echo ""
echo "[6/7] Updating ${WORKLOAD_TYPE}/${WORKLOAD_NAME} to use new PVC..."
echo "  MANUAL STEP REQUIRED:"
echo "  Update the manifest for ${WORKLOAD_TYPE}/${WORKLOAD_NAME} in namespace ${NAMESPACE}"
echo "  Change volume claim from '${PVC_NAME}' to '${NEW_PVC_NAME}'"
echo ""
echo "  Then run: kubectl scale ${WORKLOAD_TYPE}/${WORKLOAD_NAME} -n ${NAMESPACE} --replicas=${CURRENT_REPLICAS}"
echo ""
read -r -p "  Press Enter after updating the manifest and scaling back up..."

# Step 7: Verify workload is healthy
echo ""
echo "[7/7] Verifying workload health..."
kubectl get pods -n "${NAMESPACE}" -l "app=${WORKLOAD_NAME}" --no-headers 2>/dev/null || \
kubectl get pods -n "${NAMESPACE}" | grep "${WORKLOAD_NAME}" || true

echo ""
echo "=== Migration Complete ==="
echo "Old PVC '${PVC_NAME}' retained in ${NAMESPACE} — delete after 48h verification:"
echo "  kubectl delete pvc ${PVC_NAME} -n ${NAMESPACE}"
echo ""
echo "Journal entry:"
echo "  Date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo "  Phase: A.1"
echo "  Workload: ${WORKLOAD_NAME}"
echo "  PVC: ${PVC_NAME} → ${NEW_PVC_NAME}"
echo "  Size: ${PVC_SIZE}"
echo "  Status: MIGRATED (pending 48h verification)"
```

- [ ] **Step 6.2: Make executable and lint**

```bash
chmod +x scripts/migrate/migrate_pvc.sh
shellcheck scripts/migrate/migrate_pvc.sh
```

- [ ] **Step 6.3: Commit**

```bash
git add scripts/migrate/migrate_pvc.sh
git commit -m "feat: PVC migration script — Longhorn to NFS via pv-migrate with rollback"
```

---

## Task 7: Database Migration Script

**Files:**
- Create: `scripts/migrate/migrate_database.sh`

- [ ] **Step 7.1: Write migrate_database.sh**

```bash
#!/usr/bin/env bash
# Migrate a Postgres database from K8s container to TrueNAS consolidated Postgres
# Usage: ./migrate_database.sh <namespace> <source-pod> <database-name> <target-host>
# Example: ./migrate_database.sh linkwarden linkwarden-postgres-0 linkwarden trunas.petersimmons.com
#
# Prerequisites:
# - TrueNAS Postgres is running and accessible
# - Target role and database created on TrueNAS Postgres
# - pg_dump and pg_restore available (via kubectl exec into source pod)

set -euo pipefail

NAMESPACE="${1:?Usage: $0 <namespace> <source-pod> <database-name> <target-host>}"
SOURCE_POD="${2:?Usage: $0 <namespace> <source-pod> <database-name> <target-host>}"
DB_NAME="${3:?Usage: $0 <namespace> <source-pod> <database-name> <target-host>}"
TARGET_HOST="${4:?Usage: $0 <namespace> <source-pod> <database-name> <target-host>}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="${SCRIPT_DIR}/../.."
BACKUP_DIR="${PROJECT_DIR}/reports/db-backups"
TIMESTAMP="$(date +%Y%m%d-%H%M%S)"
DUMP_FILE="${BACKUP_DIR}/${DB_NAME}-${TIMESTAMP}.sql"

mkdir -p "${BACKUP_DIR}"

echo "=== Database Migration: ${DB_NAME} ==="
echo "Source: ${NAMESPACE}/${SOURCE_POD}"
echo "Target: ${TARGET_HOST}"
echo ""

# Step 1: Verify source pod is running
echo "[1/6] Verifying source pod..."
kubectl get pod "${SOURCE_POD}" -n "${NAMESPACE}" || {
    echo "ERROR: Source pod ${SOURCE_POD} not found"
    exit 1
}

# Step 2: pg_dump from source
echo ""
echo "[2/6] Running pg_dump..."
kubectl exec "${SOURCE_POD}" -n "${NAMESPACE}" -- \
    pg_dump -U postgres -d "${DB_NAME}" --format=custom --verbose \
    > "${DUMP_FILE}" 2>/dev/null

DUMP_SIZE=$(du -h "${DUMP_FILE}" | cut -f1)
echo "  Dump saved to ${DUMP_FILE} (${DUMP_SIZE})"

# Step 3: Verify dump integrity
echo ""
echo "[3/6] Verifying dump integrity..."
pg_restore --list "${DUMP_FILE}" > /dev/null 2>&1 || {
    echo "ERROR: Dump file is corrupt or unreadable"
    exit 1
}
OBJECT_COUNT=$(pg_restore --list "${DUMP_FILE}" 2>/dev/null | wc -l)
echo "  Dump contains ${OBJECT_COUNT} objects"

# Step 4: Create database and role on target (if not exists)
echo ""
echo "[4/6] Creating database on target..."
echo "  MANUAL STEP: Ensure the database '${DB_NAME}' and role exist on ${TARGET_HOST}"
echo "  Example:"
echo "    CREATE ROLE ${DB_NAME}_user WITH LOGIN PASSWORD 'from-infisical';"
echo "    CREATE DATABASE ${DB_NAME} OWNER ${DB_NAME}_user;"
echo ""
read -r -p "  Press Enter when target database is ready..."

# Step 5: pg_restore to target
echo ""
echo "[5/6] Running pg_restore to ${TARGET_HOST}..."
echo "  NOTE: You will need the target Postgres password"
pg_restore \
    --host="${TARGET_HOST}" \
    --port=5432 \
    --username=postgres \
    --dbname="${DB_NAME}" \
    --verbose \
    --no-owner \
    --no-acl \
    "${DUMP_FILE}" 2>&1 | tail -5

echo "  Restore complete"

# Step 6: Verify row counts
echo ""
echo "[6/6] Verifying migration..."
echo "  Comparing table counts between source and target..."

SOURCE_TABLES=$(kubectl exec "${SOURCE_POD}" -n "${NAMESPACE}" -- \
    psql -U postgres -d "${DB_NAME}" -t -c \
    "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'" 2>/dev/null | tr -d ' ')

TARGET_TABLES=$(PGPASSWORD="${PGPASSWORD:-}" psql \
    -h "${TARGET_HOST}" -U postgres -d "${DB_NAME}" -t -c \
    "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'" 2>/dev/null | tr -d ' ')

echo "  Source tables: ${SOURCE_TABLES}"
echo "  Target tables: ${TARGET_TABLES}"

if [[ "$SOURCE_TABLES" == "$TARGET_TABLES" ]]; then
    echo "  ✅ Table count matches"
else
    echo "  ⚠️  Table count mismatch — investigate before proceeding"
fi

echo ""
echo "=== Database Migration Complete ==="
echo ""
echo "Next steps:"
echo "  1. Update connection string in Infisical for ${DB_NAME}"
echo "  2. Create ExternalName Service: kubectl apply -f k8s/external-db-services/${NAMESPACE}.yaml"
echo "  3. Restart application pod in ${NAMESPACE}"
echo "  4. Verify application reads/writes work"
echo "  5. Keep source pod at scale=0 for 48h, then delete"
echo ""
echo "Journal entry:"
echo "  Date: ${TIMESTAMP}"
echo "  Phase: B"
echo "  Database: ${DB_NAME}"
echo "  Source: ${NAMESPACE}/${SOURCE_POD}"
echo "  Target: ${TARGET_HOST}"
echo "  Dump: ${DUMP_FILE} (${DUMP_SIZE}, ${OBJECT_COUNT} objects)"
echo "  Tables: source=${SOURCE_TABLES}, target=${TARGET_TABLES}"
```

- [ ] **Step 7.2: Make executable and lint**

```bash
chmod +x scripts/migrate/migrate_database.sh
shellcheck scripts/migrate/migrate_database.sh
```

- [ ] **Step 7.3: Commit**

```bash
git add scripts/migrate/migrate_database.sh
git commit -m "feat: database migration script — pg_dump/restore with verification"
```

---

## Task 8: ExternalName Service Manifests

**Files:**
- Create: `k8s/external-db-services/*.yaml` (7 files)

- [ ] **Step 8.1: Create ExternalName Service manifests**

Each namespace that has a database gets a Service pointing to TrueNAS Postgres.

Create `k8s/external-db-services/default.yaml`:

```yaml
# ExternalName Service for ciso-tracker-db → TrueNAS Postgres
apiVersion: v1
kind: Service
metadata:
  name: postgres
  namespace: default
spec:
  type: ExternalName
  externalName: trunas.petersimmons.com
  ports:
    - port: 5432
      targetPort: 5432
      protocol: TCP
```

Create `k8s/external-db-services/linkwarden.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: postgres
  namespace: linkwarden
spec:
  type: ExternalName
  externalName: trunas.petersimmons.com
  ports:
    - port: 5432
      targetPort: 5432
      protocol: TCP
```

Create `k8s/external-db-services/monitoring.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: postgres
  namespace: monitoring
spec:
  type: ExternalName
  externalName: trunas.petersimmons.com
  ports:
    - port: 5432
      targetPort: 5432
      protocol: TCP
```

Create `k8s/external-db-services/clearwatch-checkout.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: postgres
  namespace: clearwatch-checkout
spec:
  type: ExternalName
  externalName: trunas.petersimmons.com
  ports:
    - port: 5432
      targetPort: 5432
      protocol: TCP
```

Create `k8s/external-db-services/clearwatch-research.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: postgres
  namespace: clearwatch-research
spec:
  type: ExternalName
  externalName: trunas.petersimmons.com
  ports:
    - port: 5432
      targetPort: 5432
      protocol: TCP
```

Create `k8s/external-db-services/content-cache.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: postgres
  namespace: content-cache
spec:
  type: ExternalName
  externalName: trunas.petersimmons.com
  ports:
    - port: 5432
      targetPort: 5432
      protocol: TCP
```

Create `k8s/external-db-services/infisical.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: postgres
  namespace: infisical
spec:
  type: ExternalName
  externalName: trunas.petersimmons.com
  ports:
    - port: 5432
      targetPort: 5432
      protocol: TCP
```

- [ ] **Step 8.2: Lint YAML**

```bash
yamllint k8s/external-db-services/
```

- [ ] **Step 8.3: Commit**

```bash
git add k8s/external-db-services/
git commit -m "feat: ExternalName Service manifests for TrueNAS Postgres per namespace"
```

---

## Task 9: Proxmox NFS Provisioning Script (Phase C)

**Files:**
- Create: `scripts/provision/setup_proxmox_nfs.sh`

- [ ] **Step 9.1: Write setup_proxmox_nfs.sh**

```bash
#!/usr/bin/env bash
# Setup Proxmox-local NFS export for K8s storage (Phase C)
# Prerequisites: SSH access to Proxmox host, ZFS installed
# This script is run via SSH on the Proxmox host itself

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CREDS_FILE="${SCRIPT_DIR}/../../credentials.env"

if [[ -f "$CREDS_FILE" ]]; then
    # shellcheck source=/dev/null
    source "$CREDS_FILE"
fi

: "${PROXMOX_HOST:?PROXMOX_HOST not set}"

DATASET="zp3/k8s-nfs"
EXPORT_PATH="/zp3/k8s-nfs"
ALLOWED_NETWORK="192.168.0.0/24"

echo "=== Proxmox NFS Setup (Phase C) ==="
echo "Host: ${PROXMOX_HOST}"
echo "Dataset: ${DATASET}"
echo ""

# Run commands on Proxmox via SSH
ssh "root@${PROXMOX_HOST}" bash <<REMOTE
set -euo pipefail

# Step 1: Create ZFS dataset
echo "[1/4] Creating ZFS dataset ${DATASET}..."
if zfs list "${DATASET}" &>/dev/null; then
    echo "  Dataset already exists — skipping"
else
    zfs create \
        -o sync=standard \
        -o dedup=off \
        -o compression=lz4 \
        -o atime=off \
        "${DATASET}"
    echo "  Created"
fi

# Verify properties
echo "  Properties:"
zfs get sync,dedup,compression,atime "${DATASET}" | tail -4

# Step 2: Install NFS server if needed
echo ""
echo "[2/4] Ensuring NFS server is installed..."
if ! dpkg -l | grep -q nfs-kernel-server; then
    apt-get update && apt-get install -y nfs-kernel-server
    echo "  Installed nfs-kernel-server"
else
    echo "  Already installed"
fi

# Step 3: Configure NFS export
echo ""
echo "[3/4] Configuring NFS export..."
if grep -q "${EXPORT_PATH}" /etc/exports 2>/dev/null; then
    echo "  Export already configured — skipping"
else
    echo "${EXPORT_PATH} ${ALLOWED_NETWORK}(rw,sync,no_subtree_check,no_root_squash)" >> /etc/exports
    exportfs -ra
    echo "  Export added and applied"
fi

# Step 4: Verify
echo ""
echo "[4/4] Verifying NFS export..."
exportfs -v | grep "${EXPORT_PATH}" || echo "  WARNING: Export not visible"

echo ""
echo "=== Proxmox NFS Setup Complete ==="
echo "NFS export: ${EXPORT_PATH} → ${ALLOWED_NETWORK}"
REMOTE

echo ""
echo "Next: rsync data from TrueNAS to Proxmox, then update PVs"
```

- [ ] **Step 9.2: Make executable and lint**

```bash
chmod +x scripts/provision/setup_proxmox_nfs.sh
shellcheck scripts/provision/setup_proxmox_nfs.sh
```

- [ ] **Step 9.3: Commit**

```bash
git add scripts/provision/setup_proxmox_nfs.sh
git commit -m "feat: Proxmox NFS provisioning script for Phase C"
```

---

## Task 10: Journal Template and First Entry

**Files:**
- Create: `docs/journal/000-project-setup.md`

- [ ] **Step 10.1: Create first journal entry**

Create `docs/journal/000-project-setup.md`:

```markdown
# Journal: Project Setup

**Date:** 2026-04-03
**Phase:** Pre-migration
**Step:** Project scaffolding and baseline

## What We Did

- Created `longhorn-nfs` GitHub repo with CI (ruff, shellcheck, yamllint, pytest)
- Documented full PVC inventory (20 Longhorn volumes, 2 local-path, 1 external)
- Documented database inventory (8 Postgres, 1 MariaDB, 3 Redis, 1 Qdrant)
- Verified TrueNAS `zp1/k8s-data` dataset settings: `sync=standard`, `dedup=off` (confirmed LOCAL override)
- Built tooling: baseline collector, comparison reporter, migration scripts, provisioning scripts

## What We Measured

### Current State (Baseline)

| Metric | Value |
|--------|-------|
| Proxmox host CPU | ~30% (load avg 15.5/40 threads) |
| Longhorn instance-manager CPU | ~629m (top 5 engines) |
| Longhorn manager CPU | ~393m (9 nodes) |
| Total Longhorn CPU | ~1.3-1.5 cores direct overhead |
| Longhorn volumes | 20 (all 3x replication) |
| Unique storage | ~280 Gi |
| Replicated storage | ~840 Gi |
| Degraded volumes | 2 (container-registry, content-cache) |

### TrueNAS Target

| Metric | Value |
|--------|-------|
| Available storage | 33 TB free (60 TB RAIDZ2) |
| Network | 10GbE bonded failover |
| Load average | 1.24 (idle) |
| RAM | 252 GB |

## What We Learned

- Longhorn CPU overhead is a well-documented issue across versions (GitHub issues #6695, #3636)
- Longhorn achieves only 20-30% of native disk IOPS due to replication overhead
- democratic-csi is the recommended NFS provisioner for TrueNAS (active maintenance, full CSI spec)
- TrueNAS global `dedup=ON` and `sync=disabled` must be overridden per-dataset for K8s workloads
- Redis on Longhorn with 3x replication is wasteful for ephemeral cache — emptyDir is correct
- Postgres over NFS is suboptimal; TrueNAS App with direct ZFS access is the better path

## Decisions Made

- **ADR-1 through ADR-9** documented in design spec
- Phase B leading candidate: Postgres as TrueNAS App (Docker container) with Chainguard image
- Qdrant stays separate (no pgvector — avoiding supply chain risk from custom images)
- Three-phase plan with operational checkpoints between each phase
```

- [ ] **Step 10.2: Commit**

```bash
git add docs/journal/000-project-setup.md
git commit -m "docs: initial journal entry with baseline measurements and decisions"
```

---

## Task 11: Copy Design Spec to New Repo

**Files:**
- Copy: `docs/superpowers/specs/2026-04-03-longhorn-nfs-migration-design.md` from home repo

- [ ] **Step 11.1: Copy spec into repo**

```bash
mkdir -p docs/superpowers/specs
cp ~/docs/superpowers/specs/2026-04-03-longhorn-nfs-migration-design.md docs/superpowers/specs/
```

- [ ] **Step 11.2: Copy plan into repo**

```bash
mkdir -p docs/superpowers/plans
cp ~/docs/superpowers/plans/2026-04-03-longhorn-nfs-migration.md docs/superpowers/plans/
```

- [ ] **Step 11.3: Commit**

```bash
git add docs/superpowers/
git commit -m "docs: add design spec and implementation plan from home repo"
```

---

## Execution Order Summary

| Task | Phase | What it produces | Depends on |
|------|-------|-----------------|------------|
| 0 | Setup | GitHub repo, CI, scaffolding | Nothing |
| 1 | Setup | Config files (workloads, databases, metrics) | Task 0 |
| 2 | A.0 | Baseline collection script | Task 1 |
| 3 | A.0 | Comparison report script | Task 1 |
| 4 | A.0 | TrueNAS NFS share + ZFS snapshots | Task 0 |
| 5 | A.0 | democratic-csi + StorageClass | Task 4 |
| 6 | A.1+ | PVC migration script | Task 5 |
| 7 | B.0+ | Database migration script | Task 0 |
| 8 | B.0 | ExternalName Service manifests | Task 0 |
| 9 | C.1 | Proxmox NFS export | Task 0 |
| 10 | Setup | Journal template + first entry | Task 0 |
| 11 | Setup | Spec + plan copied to repo | Task 0 |

**After Task 5 completes:** Run `make baseline` to collect Prometheus data, then execute Phase A.1 migrations using Task 6's script.

**After Phase A completes:** Run `make report -- --baseline reports/baseline --current reports/post-a` to generate comparison.

**Phase B:** Use Task 7's script for each database, Task 8's manifests for connectivity.

**Phase C:** Use Task 9's script, then rsync + PV updates.
