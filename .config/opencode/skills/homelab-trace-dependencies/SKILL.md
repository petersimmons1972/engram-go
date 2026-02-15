---
name: homelab-trace-dependencies
description: Trace service dependency chain to identify root cause location when a service is down. Walks dependency tree from leaf to root, testing each dependency to find first failure point and blast radius.
---

# Trace Service Dependencies

## Overview

Systematic dependency chain tracing to identify root cause when services fail. Instead of guessing, trace the dependency tree from the failed service back to the infrastructure foundation, testing each layer to find the first failure.

**Purpose**: Find the actual root cause, not just the symptom. If Homepage is down because Traefik is down, fix Traefik.

## When to Use

**Triggers**:
- User reports "X is down" or "X not working"
- Service returning 502/503/504 errors
- Multiple services failing simultaneously
- Before starting debugging to identify correct target
- When unsure if problem is the service itself or upstream

**When NOT to use**:
- Service-specific bugs (app crashes, config errors within the service)
- Slow performance (use performance debugging instead)
- Intermittent issues (may need different approach)

## Dependency Configuration

Load service dependencies from:
```
/home/psimmons/.homelab/config/service-dependencies.yaml
```

**Critical path** (most common cascade):
```
Unifi Router -> Pi-holes -> Network -> Proxmox -> K8s Control Plane -> Traefik -> Apps
```

## Process

### Step 1: Identify Failed Service

**Ask user if not clear**:
```
Which service is down?
Examples: homepage, nextcloud, traefik, kubernetes-api, open-webui, linkwarden, searxng, n8n
```

**Auto-detect if possible**:
- Check recent conversation context
- Check recent error messages

### Step 2: Load Dependency Chain

Load from `service-dependencies.yaml` and build chain for the service.

**Example for Homepage**:
```yaml
dependency_chain:
  - homepage           # The failing service (leaf)
  - traefik            # Reverse proxy
  - kubernetes-api     # K8s API
  - control-plane-nodes # K8s masters
  - proxmox            # Hypervisor
  - network            # DNS/routing
  - pihole-primary     # Primary DNS
  - unifi-router       # Network foundation (root)
```

### Step 3: Test Dependencies (Root to Leaf)

Test each dependency starting from the root (most foundational) working up to the leaf (failing service). This order finds the root cause first.

**Execute health checks in order**:

| Order | Service              | Health Check Command                                              |
|-------|----------------------|-------------------------------------------------------------------|
| 1     | unifi-router         | `ping -c 1 192.168.0.1`                                           |
| 2     | pihole-primary       | `dig @192.168.0.231 google.com +short`                            |
| 3     | pihole-secondary     | `dig @192.168.0.232 google.com +short`                            |
| 4     | network              | `dig @192.168.0.231 google.com`                                   |
| 5     | proxmox              | `ssh psimmons@192.168.0.100 'uptime'`                             |
| 6     | control-plane-nodes  | `kubectl get nodes \| grep control-plane \| grep Ready`           |
| 7     | kubernetes-api       | `kubectl get nodes`                                               |
| 8     | cert-manager         | `kubectl get pods -n cert-manager -l app=cert-manager \| grep Running` |
| 9     | traefik              | `kubectl get pods -n kube-system -l app.kubernetes.io/name=traefik \| grep Running` |
| 10    | longhorn             | `kubectl get pods -n longhorn-system -l app=longhorn-manager \| grep Running` |
| 11    | [application]        | Application-specific check (curl, kubectl get pods, etc.)         |

**Stop at first failure** - that's the root cause.

### Step 4: Identify Root Cause

First failure in the chain = root cause.

**Record**:
- Which service failed
- What the health check returned
- Error message if any

### Step 5: Calculate Blast Radius

Look up blast radius from config for the failed service.

**Blast radius examples**:

| Failed Service   | Affected Services                                              |
|------------------|----------------------------------------------------------------|
| unifi-router     | EVERYTHING - complete homelab outage                           |
| pihole-primary   | All services if secondary also down; degraded if secondary OK  |
| proxmox          | All VMs (Kubernetes, Nextcloud) - entire homelab               |
| traefik          | All K8s web services: Homepage, Linkwarden, Open-WebUI, SearXNG, N8N, Registry |
| kubernetes-api   | All K8s workloads                                              |
| longhorn         | Stateful workloads: Linkwarden, Open-WebUI, N8N (not Homepage) |

### Step 6: Recommend Fix

Based on root cause, suggest fix from known remediation.

**Common fixes**:

| Root Cause           | Recommended Fix                                            |
|----------------------|------------------------------------------------------------|
| traefik              | `kubectl rollout restart -n kube-system deployment/traefik` |
| kubernetes-api       | Check control plane nodes: `kubectl get nodes`             |
| control-plane-nodes  | SSH to nodes, check kubelet: `systemctl status kubelet`    |
| proxmox              | Access via console, check storage: zp3 usage               |
| pihole-primary       | SSH to 192.168.0.231, check pihole-FTL service             |
| network              | Check both Pi-holes, check router connectivity             |
| longhorn             | `kubectl get pods -n longhorn-system`, check node disks    |
| cert-manager         | Check certificate status, webhook pods                     |

## Output Format

**Standard output**:
```
Tracing dependencies for: [SERVICE]

[CHECKING DEPENDENCIES - ROOT TO LEAF]

Router (192.168.0.1)           [OK] ping successful
Pi-hole Primary (.231)         [OK] DNS resolving
Pi-hole Secondary (.232)       [OK] DNS resolving
Proxmox (pve)                  [OK] uptime: 42 days
K8s Control Plane              [OK] 3/3 nodes Ready
K8s API                        [OK] cluster accessible
Cert-Manager                   [OK] pod running
Traefik                        [FAILED] pod not running

---

ROOT CAUSE: Traefik pod is not running

BLAST RADIUS: All K8s web services affected
  - Homepage
  - Open-WebUI
  - Linkwarden
  - SearXNG
  - N8N
  - Container Registry

RECOMMENDED ACTION:
  kubectl rollout restart -n kube-system deployment/traefik

NEXT STEPS:
  1. Run recommended action
  2. Wait 30 seconds for pod to start
  3. Re-test original service: curl -I https://[service].petersimmons.com
```

**All dependencies healthy output**:
```
Tracing dependencies for: [SERVICE]

[CHECKING DEPENDENCIES - ROOT TO LEAF]

Router (192.168.0.1)           [OK] ping successful
Pi-hole Primary (.231)         [OK] DNS resolving
Pi-hole Secondary (.232)       [OK] DNS resolving
Proxmox (pve)                  [OK] uptime: 42 days
K8s Control Plane              [OK] 3/3 nodes Ready
K8s API                        [OK] cluster accessible
Traefik                        [OK] pod running
[SERVICE]                      [OK] health check passed

---

ALL DEPENDENCIES HEALTHY

The problem is likely within [SERVICE] itself, not upstream.

RECOMMENDED: Use superpowers:systematic-debugging for service-specific issues
Check:
  1. Pod logs: kubectl logs -n [namespace] -l [label-selector]
  2. Pod status: kubectl describe pod [pod-name] -n [namespace]
  3. Network policies: kubectl get networkpolicy -n [namespace]
  4. Service config: kubectl get svc,ingress -n [namespace]
```

## Quick Reference: Service to Namespace

| Service          | Namespace           | Label Selector                        |
|------------------|---------------------|---------------------------------------|
| homepage         | default             | app.kubernetes.io/name=homepage       |
| traefik          | kube-system         | app.kubernetes.io/name=traefik        |
| cert-manager     | cert-manager        | app=cert-manager                      |
| longhorn         | longhorn-system     | app=longhorn-manager                  |
| linkwarden       | linkwarden          | app=linkwarden                        |
| open-webui       | open-webui          | app=open-webui                        |
| searxng          | searxng             | app=searxng                           |
| n8n              | n8n                 | app=n8n                               |
| container-registry | container-registry | app=docker-registry                   |

## Integration with Other Skills

**Triggers other skills**:
- If root cause found -> suggest `superpowers:systematic-debugging` for that component
- If incident resolved -> suggest `homelab:log-incident` to capture learnings

**Used by**:
- `homelab:incident-response` - uses this for initial triage
- `superpowers:systematic-debugging` - may call this to rule out upstream issues

## Example Sessions

### Example 1: Traefik is the root cause

```
User: "Homepage is down"

Assistant runs skill:

Tracing dependencies for: Homepage

Router (192.168.0.1)           [OK] ping successful
Pi-hole Primary (.231)         [OK] DNS resolving
Pi-hole Secondary (.232)       [OK] DNS resolving
Proxmox (pve)                  [OK] uptime: 42 days
K8s Control Plane              [OK] 3/3 nodes Ready
K8s API                        [OK] cluster accessible
Traefik                        [FAILED] 0/1 pods running

---

ROOT CAUSE: Traefik pod is not running

BLAST RADIUS: All K8s web services affected
  - Homepage
  - Open-WebUI
  - Linkwarden
  - SearXNG
  - N8N
  - Container Registry

RECOMMENDED ACTION:
  kubectl rollout restart -n kube-system deployment/traefik
```

### Example 2: Service-specific issue (all dependencies OK)

```
User: "Linkwarden not loading"

Assistant runs skill:

Tracing dependencies for: Linkwarden

Router (192.168.0.1)           [OK]
Pi-hole Primary (.231)         [OK]
Proxmox (pve)                  [OK]
K8s Control Plane              [OK]
K8s API                        [OK]
Longhorn                       [OK]
Traefik                        [OK]
Linkwarden                     [FAILED] curl returned 502

---

ALL UPSTREAM DEPENDENCIES HEALTHY

The problem is within Linkwarden itself, not upstream infrastructure.

RECOMMENDED: Use superpowers:systematic-debugging
Check:
  1. Pod logs: kubectl logs -n linkwarden -l app=linkwarden
  2. Pod status: kubectl describe pod -n linkwarden -l app=linkwarden
  3. Database connectivity (if applicable)
```

### Example 3: DNS failure causing cascade

```
User: "Multiple services are down"

Assistant runs skill (picks Homepage as test service):

Tracing dependencies for: Homepage

Router (192.168.0.1)           [OK] ping successful
Pi-hole Primary (.231)         [FAILED] no response to dig
Pi-hole Secondary (.232)       [FAILED] no response to dig

---

ROOT CAUSE: Both Pi-hole DNS servers are down

BLAST RADIUS: CRITICAL - ALL services requiring DNS resolution
  - All Kubernetes services
  - All external connectivity
  - All internal name resolution

RECOMMENDED ACTION:
  1. SSH to 192.168.0.231: ssh pi@192.168.0.231
  2. Check Pi-hole status: systemctl status pihole-FTL
  3. Check if Pi-hole VM is running in Proxmox
  4. Repeat for secondary (192.168.0.232)

NOTE: Use IP addresses for SSH since DNS is down
```

## Maintenance

**Config file location**: `/home/psimmons/.homelab/config/service-dependencies.yaml`

**Update config when**:
- New services added to cluster
- Service URLs change
- Health check commands change
- Dependency relationships change

**Test the skill**:
- Run against known-healthy service (should show all OK)
- Temporarily stop a dependency and verify detection
