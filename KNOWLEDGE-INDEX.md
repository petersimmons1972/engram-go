# Knowledge Base Index

**Purpose**: Comprehensive index of all troubleshooting logs, lessons learned, and session documentation across the homelab infrastructure.

**Last Updated**: 2026-01-25

**Total Documents Indexed**: 60+ knowledge files across hardware, software, and infrastructure

---

## Quick Navigation

- [By Technology](#by-technology)
  - [Hardware](#hardware)
  - [Kubernetes](#kubernetes)
  - [Nextcloud](#nextcloud)
  - [Proxmox](#proxmox)
  - [Infrastructure as Code](#infrastructure-as-code)
  - [Self-Learning System](#self-learning-system)
- [By Problem Type](#by-problem-type)
  - [Critical Production Issues](#critical-production-issues)
  - [Deployment Issues](#deployment-issues)
  - [Configuration Issues](#configuration-issues)
  - [Performance Issues](#performance-issues)
- [By Date](#by-date)

---

## By Technology

### Hardware

#### Logitech Mouse Issues
- **File**: `/home/psimmons/MOUSE-TROUBLESHOOTING-LOG.md`
- **Last Updated**: 2025-12-27
- **Status**: Active troubleshooting in progress
- **Topics**:
  - Mouse stops responding after idle
  - Kernel driver regression (hid_logitech_hidpp, hid_logitech_dj)
  - USB autosuspend issues
  - Permanent fix options evaluated
- **Key Lessons**:
  - Module reload requires interactive terminal
  - USB autosuspend is root cause
  - Scripts requiring sudo cannot run through Claude CLI
- **Tags**: #hardware #logitech #usb #driver #troubleshooting
- **Critical Commands**:
  ```bash
  /home/psimmons/mouse.sh  # Manual fix (requires sudo password)
  /home/psimmons/fix-mouse-permanent.sh  # Permanent fix (not yet applied)
  ```

#### AMD GPU Complete System Freeze (CRITICAL)
- **File**: `/home/psimmons/AMD-GPU-FREEZE-TROUBLESHOOTING-LOG.md`
- **Last Updated**: 2026-01-03
- **Status**: ⚠️ ROOT CAUSE IDENTIFIED - FIX READY TO APPLY
- **Severity**: CRITICAL - Requires hard power off to recover
- **Topics**:
  - Complete system freeze after idle period
  - AMD Radeon RX 7900 XT/XTX DisplayPort power management failures
  - COSMIC desktop compositor crash
  - DPCD (DisplayPort Configuration Data) communication errors
  - GFXOFF power feature causing crashes
- **Key Lessons**:
  - AMD RDNA3 + Linux 6.17 + COSMIC + Multi-monitor DisplayPort = known problematic combination
  - GFXOFF feature causes DisplayPort link failures during idle
  - Pop_OS 24.04 COSMIC is beta quality - expect stability issues
  - Systematic debugging revealed exact 3-second failure cascade
  - This is NOT the same as mouse-only issue - this is GPU driver crash
- **Root Cause**: AMD PowerPlay GFXOFF feature fails DisplayPort DPCD communication during power state transition, causing COSMIC compositor crash
- **Solution**: Disable GFXOFF with kernel parameter `amdgpu.ppfeaturemask=0xffff7fff`
- **Tags**: #hardware #amd #gpu #displayport #power-management #cosmic #critical #freeze
- **Emergency Fix Commands**:
  ```bash
  # PERMANENT FIX (requires reboot):
  sudo kernelstub --add-options "amdgpu.ppfeaturemask=0xffff7fff"
  gsettings set org.gnome.desktop.session idle-delay 1800  # 30 min
  sudo reboot

  # TEMPORARY TEST (no reboot):
  echo "0xffff7fff" | sudo tee /sys/module/amdgpu/parameters/ppfeaturemask
  ```
- **Affected Hardware**: AMD Radeon RX 7900 XT/XTX (RDNA3), 4x 27" DisplayPort monitors
- **Differences from Mouse Issue**:
  - Mouse issue: Only `hid_logitech_hidpp` driver, mouse-specific, easy fix
  - GPU issue: AMD GPU driver + COSMIC compositor crash, system-wide freeze, requires kernel parameter

---

### Kubernetes

#### Local Container Registry (Production)
- **File**: `/home/psimmons/projects/infrastructure/container-registry/README.md`
- **Last Updated**: 2026-01-13
- **Status**: Production - fully operational
- **URL**: https://registry.petersimmons.com
- **Topics**:
  - Eliminated ImagePullBackOff issues across cluster
  - Docker Hub caching for faster image pulls
  - TLS via Traefik with Let's Encrypt
  - All 9 nodes configured automatically
- **Key Lessons**:
  - Control-plane nodes use `k3s.service`, workers use `k3s-agent.service`
  - emptyDir storage is fine for registries when images can be rebuilt
  - Node-level registry config more reliable than imagePullSecrets
  - Automated configuration script saved significant manual work
  - DNS CNAME works perfectly for service routing
- **Performance**:
  - Image push: 2-3 seconds
  - Image pull (cached): 695ms
  - Node configuration: ~60 seconds for all 9 nodes
- **Tags**: #kubernetes #registry #docker #image-distribution #traefik #production
- **Critical Commands**:
  ```bash
  # Push image
  docker tag IMAGE registry.petersimmons.com/IMAGE
  docker push registry.petersimmons.com/IMAGE

  # Health check
  curl -I https://registry.petersimmons.com/v2/

  # Reconfigure all nodes
  /home/psimmons/projects/infrastructure/container-registry/configure-nodes.sh
  ```

#### Homepage Application (Most Problematic Pod)
- **File**: `/home/psimmons/projects/custom-homepage/HOMEPAGE-INSTRUCTIONS.md`
- **Last Updated**: 2025-12-22
- **Status**: Production - requires special handling
- **Topics**:
  - Emergency restoration procedures
  - Network policy requirements (uses `app.kubernetes.io/name=homepage`)
  - Critical testing requirements
  - Common failure modes
- **Key Lessons**:
  - HTTP 200 does NOT mean Homepage is working
  - Must check HTML content for "API Error" and "Something went wrong"
  - Widget functionality can break silently
  - Favicon loading requires icon libraries (mdi-*, si-*)
  - Browser caching can show stale demo content
- **Related Files**:
  - `/home/psimmons/projects/kubernetes/homepage/LESSONS_LEARNED.md` - Comprehensive lessons
  - `/home/psimmons/projects/kubernetes/homepage/SESSION-2025-12-20-IMPROVEMENTS.md` - Recent fixes
  - `/home/psimmons/projects/kubernetes/homepage/GOLDEN-CONFIG-BASELINE.md` - Known-good config
  - `/home/psimmons/projects/kubernetes/homepage/QUICK-REFERENCE.md` - Emergency procedures
  - `/home/psimmons/projects/kubernetes/homepage/TESTING.md` - Testing procedures
- **Tags**: #kubernetes #homepage #networking #testing #critical
- **Emergency Restore**:
  ```bash
  kubectl apply -f /home/psimmons/projects/kubernetes/homepage/configmap-updated.yaml
  kubectl rollout restart deployment/homepage -n default
  ```

#### Homepage Lessons Learned
- **File**: `/home/psimmons/projects/kubernetes/homepage/LESSONS_LEARNED.md`
- **Last Updated**: 2025-12-20
- **Status**: Living document
- **Topics**:
  - Preventing recurring API errors
  - Using :latest tag vs pinned versions
  - Kubernetes auto-discovery issues
  - Widget configuration dependencies
  - Systematic debugging approach
- **Key Lessons**:
  - NEVER use :latest tags (caused silent breaking changes)
  - Kubernetes mode should be explicitly disabled if not needed
  - Widget configs must match mounted files
  - HTTP 200 ≠ Working application
  - Always analyze error patterns before fixing
- **Tags**: #kubernetes #homepage #lessons-learned #api-errors #debugging

#### Certificate Management & Renewal (TLS)
- **File**: `/home/psimmons/LESSONS-LEARNED-CERT-MANAGER.md` (Primary reference)
- **Runbook**: `/home/psimmons/RUNBOOKS/TROUBLESHOOT-CERTIFICATE-RENEWAL.md`
- **Last Updated**: 2026-01-25
- **Status**: Active - Recent incident resolved
- **Severity**: High (affects all HTTPS services)
- **Topics**:
  - cert-manager DNS-01 ACME challenge failures
  - Cloudflare API token IP filtering blocking pod network (10.42.x.x)
  - Silent failure mode: Challenge "Presented: true" but DNS record never created
  - Network heterogeneity: homelab IPs (192.168.x.x) vs K8s overlay (10.42.x.x)
  - Health check improvements for certificate renewal monitoring
- **Critical Incident**:
  - **Symptom**: Certificate `petersimmons-com-wildcard-tls` stuck in IncorrectIssuer for 4+ months
  - **Root Cause**: Cloudflare token IP filter restricted to 192.168.x.x, but cert-manager pod uses 10.42.2.247
  - **Duration**: Nov 22, 2025 (issue start) to Jan 25, 2026 (diagnosis)
  - **Expiry Risk**: Certificate expires Feb 20, 2026 (26 days remaining at diagnosis)
  - **Resolution**: Remove IP filtering from Cloudflare token or add 10.42.0.0/16 to whitelist
- **Key Lessons**:
  - Kubernetes pods use overlay network (10.42.x.x), NOT external homelab IPs (192.168.x.x)
  - IP filtering on API tokens must account for pod network ranges
  - Defense in depth can break legitimate use cases if not properly thought through
  - Silent failure modes (API call succeeds, but request silently rejected) are dangerous
  - Zone + permission scoping on tokens is sufficient security without IP filtering
  - Health checks must validate end-to-end behavior, not just "API reachable"
- **Files Updated**:
  - `/home/psimmons/.homelab/knowledge/failure-history.yaml` - Incident documentation
  - `/home/psimmons/.homelab/knowledge/warning-patterns.yaml` - New patterns for stuck certs
  - `/home/psimmons/bin/health-check.sh` - Added certificate renewal checks
  - `/home/psimmons/CLAUDE.md` - Added Certificate Management section
- **Related Warning Patterns**:
  - `cert-renewal-failure-dns` - Certificate renewal stuck >7 days
  - `cloudflare-dns-challenge-propagation-failure` - DNS TXT record not appearing
- **Tags**: #kubernetes #tls #cert-manager #cloudflare #acme #dns #ip-filtering #network
- **Quick Fixes**:
  ```bash
  # Check certificate status
  kubectl get certificate -A

  # Troubleshoot stuck certificate
  kubectl describe certificate <name> -n <namespace>

  # Verify DNS propagation
  nslookup _acme-challenge.<domain> 1.1.1.1

  # Fix Cloudflare IP filtering
  # Dashboard > API Tokens > "Kubernetes - cert-manager" > Remove IP filter OR add 10.42.0.0/16
  ```

#### Linkwarden Security Vulnerabilities (CRITICAL)
- **File**: `/home/psimmons/LINKWARDEN-SECURITY-FINDINGS.md`
- **Last Updated**: 2026-01-13
- **Status**: ⚠️ ACTIVE SECURITY CONCERN - Decision pending
- **Priority**: HIGH - Immediate awareness required
- **Quick Summary**: Postgres version unknown (51+ days old), ~120 CVEs vs. ~5 with Chainguard, no independent patching
- **Impact**: 24x more vulnerabilities than necessary, 2+ month security patch lag
- **Recommendation**: Migrate to hardened architecture within 1-2 months
- **Tags**: #security #cve #vulnerability #critical #postgres #chainguard

#### Linkwarden Scaling & Security Architecture Issues
- **File**: `/home/psimmons/LINKWARDEN-SCALING-ANALYSIS.md`
- **Last Updated**: 2026-01-13 (Security analysis added)
- **Status**: Decision pending - Security concerns documented
- **Priority**: ⚠️ HIGH - Security vulnerabilities identified (upgraded from Medium)
- **Topics**:
  - Unusual postgres-as-sidecar architecture
  - RWO storage constraint preventing horizontal scaling
  - Three options for achieving redundancy
  - Migration planning for proper architecture
- **Key Issues**:
  - **SECURITY**: Postgres version unknown (~51 days old), ~120 CVEs exposed, no independent patching
  - **ARCHITECTURE**: Postgres in same pod as app (anti-pattern)
  - **SCALING**: Can't scale beyond 1 replica due to RWO PVC
  - **COUPLING**: App and database tightly coupled, violates separation of concerns
- **Current State (Post-Jan 13)**:
  - nodeSelector removed (was pinned to worker132)
  - Can now run on any node (better resilience)
  - Still 1 replica only (architectural limitation)
- **Security Findings (CRITICAL)**:
  - Current: ~120 CVEs (Debian base + postgres), unknown version, no independent patching
  - Recommended: Chainguard postgres (~5 CVEs, distroless, independent patching)
  - Risk: 24x more vulnerabilities than necessary, postgres version 51+ days old
  - Impact: Security updates delayed by app upgrade coupling
- **Options Documented**:
  1. ⭐ Separate postgres into StatefulSet + Chainguard (RECOMMENDED for security, 2-3 hours)
  2. ❌ Use RWX storage (data corruption risk - DO NOT DO)
  3. ✅ External postgres database (enterprise pattern, 3-4 hours)
- **Recommendation**: Migrate to Option 1 within 1-2 months for security hardening (even ignoring redundancy benefits)
- **Decision Needed**: User to choose architecture approach - Security vs. Simplicity tradeoff
- **Tags**: #linkwarden #scaling #architecture #technical-debt #postgres #statefulset #rwo-limitation #decision-pending #security #cve #chainguard #hardening
- **Related**: K8S-POD-GARBAGE-COLLECTION-LESSONS.md (193 failed pods from worker132 pinning)

#### Pod Garbage Collection & Orphaned Pods
- **File**: `/home/psimmons/K8S-POD-GARBAGE-COLLECTION-LESSONS.md`
- **Last Updated**: 2026-01-13
- **Status**: Active learning - ongoing investigation
- **Severity**: Moderate - Operational noise, not service-impacting
- **Topics**:
  - Kubernetes pod garbage collection gaps
  - Exit code 137 (SIGKILL) patterns
  - Storage incident cascading effects
  - Orphaned pods from rolling updates
  - Health check false positives
- **Key Lessons**:
  - Kubernetes doesn't auto-cleanup all Failed/Error/Evicted pods
  - Exit code 137 is often infrastructure-related, not application bugs
  - Storage incidents create long-tail orphaned pod problems
  - Check deployment health, not just pod counts
  - Orphaned pods can persist for weeks (cleanup took 31 pods on Jan 13)
- **Decision Tree**: Error pod → Check deployment health → Check service functionality → Check age → Cleanup if orphaned
- **Root Causes Identified**:
  - worker137 storage incident (Jan 11-12) caused 18 SearXNG evictions
  - Rolling updates leave orphaned pods (4 Open-WebUI, 8-38d old)
  - Test pods never auto-cleaned (9 pods, 42d old)
- **Preventive Measures**:
  - Monitor deployment health, not pod error counts
  - Weekly cleanup routine for pods in Error >7 days
  - Post-incident cleanup pass 24h after resolution
- **Tags**: #kubernetes #pod-lifecycle #garbage-collection #storage #monitoring #troubleshooting #exit-137 #worker137
- **Cleanup Commands**:
  ```bash
  # Check deployment health first
  kubectl get deployment -n <namespace> <name> -o jsonpath='{.status}'

  # Safe cleanup if service healthy
  kubectl delete pod -n <namespace> <failed-pod-name>
  ```

#### Longhorn Storage Issues (CRITICAL - RECURRING)
- **File**: `/home/psimmons/LONGHORN-STORAGE-TROUBLESHOOTING.md`
- **Last Updated**: 2026-01-10
- **Status**: ⚠️ ACTIVE RECURRING ISSUE - Known Problem
- **Severity**: HIGH - Blocks stateful workload deployment
- **Topics**:
  - Volume attachment failures ("not ready for workloads")
  - CSI driver false positives
  - Node affinity conflicts with local-path
  - Disk pressure taints preventing scheduling
  - Stuck volumes in "Attaching" state
- **Key Lessons**:
  - **Use emptyDir for development databases** - avoid Longhorn until production ready
  - Longhorn PVCs frequently fail to attach despite healthy Longhorn system
  - local-path storage class more reliable for single-node workloads
  - Volume attachment can get stuck even with proper CSI drivers
  - StatefulSet PVCs don't auto-delete - manual cleanup required
- **Root Causes**:
  - Longhorn volume stuck in attaching state
  - Stale volume attachments from previous pods
  - Node taints (disk-pressure) preventing scheduling
  - Replica rebuilding blocking new attachments
- **Quick Solutions**:
  - **Development**: Use emptyDir (data loss acceptable)
  - **Single-node**: Switch to local-path storage class
  - **Stuck volumes**: Delete StatefulSet + PVC, recreate fresh
  - **Taints**: Remove false disk-pressure taints if verified
- **Decision Matrix**:
  - Development DB → emptyDir (fast, reliable)
  - Single-node persistent → local-path (better than Longhorn)
  - Production critical → longhorn (when working, has replication)
- **Related Issues**: Image distribution across K8s nodes compounds problem
- **Tags**: #kubernetes #longhorn #storage #pvc #critical #recurring #troubleshooting
- **Time Cost**: ~45 minutes per occurrence if not recognized immediately
- **Prevention**: Start with emptyDir, upgrade to persistent storage only when needed
- **Emergency Commands**:
  ```bash
  # Quick switch to emptyDir (edit manifest, replace PVC volume with):
  volumes:
    - name: data-storage
      emptyDir: {}

  # Delete stuck PVC and StatefulSet:
  kubectl delete statefulset <name> -n <namespace>
  kubectl delete pvc <pvc-name> -n <namespace>
  ```
- **Incident History**:
  - 2026-01-10: Job Search PostgreSQL - switched to emptyDir after 45min troubleshooting
  - 2026-02-17: job-search fastapi + gmail-tracker ImagePullBackOff (6 days) - images missing from registry, scaled to 0

#### Kubernetes Cluster Maintenance (February 17, 2026)
- **File**: `/home/psimmons/INCIDENTS/2026-02-17-kubernetes-maintenance.md`
- **Last Updated**: 2026-02-17
- **Status**: Partially resolved (image rebuilds deferred)
- **Topics**: #kubernetes #cleanup #configuration #performance
- **Issues Found & Fixed**:
  - 278 empty ReplicaSets deleted; `revisionHistoryLimit: 3` set on all 69 deployments
  - 3 clearwatch stuck rollouts (uss-constitution/enterprise/tang, 370-547 restarts) rolled back
  - 3 ImagePullBackOff deployments scaled to 0 (images missing from registry)
  - searxng HPA created (min 2, max 20 replicas, CPU 70% / memory 80% targets)
- **Key Lessons**:
  - Two pods from different RSes for same deployment = stuck rollout → `kubectl rollout undo`
  - `revisionHistoryLimit` defaults to 10 — always set to 3 in new deployment manifests
  - ImagePullBackOff >1h means image is missing — verify with `curl -sk https://registry.petersimmons.com/v2/<image>/tags/list`
  - Stateless multi-replica deployments should always have an HPA
- **TODO**:
  - Rebuild `job-search-api`, `gmail-job-tracker`, `proxmox-monitor` images
  - Investigate why clearwatch uss-* new versions crash on startup

#### Homepage Session Improvements (December 20, 2025)
- **File**: `/home/psimmons/projects/kubernetes/homepage/SESSION-2025-12-20-IMPROVEMENTS.md`
- **Last Updated**: 2025-12-20
- **Status**: Complete
- **Topics**:
  - Widget functionality restoration (Proxmox, Pihole, Tailscale)
  - Search bar fix
  - Favicon resolution
  - Testing procedure improvements
- **Key Lessons**:
  - Widgets must be configured inline with services
  - LinkedIn requires `mdi-linkedin` (URL-based icons fail)
  - Search widget needs `provider: custom` for SearXNG
  - Visual verification is mandatory
- **Tags**: #kubernetes #homepage #widgets #icons #troubleshooting

#### Longhorn Storage Issues
- **File**: `/home/psimmons/projects/kubernetes/LONGHORN-TROUBLESHOOTING-LOG.md`
- **Last Updated**: 2025-12-28
- **Status**: CRITICAL - Active issue
- **Topics**:
  - Prometheus volume faulted (pvc-7429abe6-34f2-4b56-8c97-129f2df0bb2d)
  - Replica scheduling failures
  - Insufficient storage errors
  - Volume health monitoring
- **Current Issues**:
  - Issue #1: Prometheus volume faulted - all cluster metrics lost
  - Symptom: `input/output error` writing to WAL
  - Root cause: Longhorn cannot schedule replicas
- **Key Lessons**:
  - Volumes can fail silently (appear "bound" but be "faulted")
  - I/O errors usually mean storage backend issues, not application issues
  - Always check volume status in longhorn-system namespace
- **Tags**: #kubernetes #longhorn #storage #critical #prometheus
- **Health Check**:
  ```bash
  kubectl get volumes -n longhorn-system
  kubectl get volumes -n longhorn-system -o json | jq -r '.items[] | select(.status.state=="faulted") | .metadata.name'
  ```

#### K3s HA Cluster - Lessons Learned
- **File**: `/home/psimmons/projects/k3s-ha-cluster-rebuild/docs/reference/LESSONS-LEARNED.md`
- **Last Updated**: 2025-11-22
- **Status**: Reference document
- **Topics**:
  - Embedded etcd vs external database
  - k3sup deployment tool
  - Network policies
  - Monitoring and backup strategies
  - Technology validation
- **Key Lessons**:
  - Embedded etcd is correct for <100 node clusters
  - k3sup saved 3+ hours vs manual deployment
  - Monitoring should be deployed Day 1
  - Backups BEFORE applications
  - Network policies easier to design upfront than retrofit
  - Research time (12 hours) saved 40+ hours of rework
- **Technology Decisions Validated**:
  - ✅ 3 control planes (industry standard)
  - ✅ Embedded etcd (~5ms latency)
  - ✅ Static IPs for control plane
  - ✅ Flannel CNI (sufficient for private network)
- **Tags**: #kubernetes #k3s #ha #lessons-learned #architecture #best-practices

#### K3s Setup Lessons (Infrastructure as Code)
- **File**: `/home/psimmons/projects/iac-homelab/K3S_SETUP_LESSONS.md`
- **Status**: Reference document
- **Topics**:
  - SSH key mismatches during VM provisioning
  - Pending hardware changes in Proxmox
  - DHCP vs static IP issues
  - Cloud-init boot times
  - Proxmox API troubleshooting
- **Key Lessons**:
  - Always regenerate/verify SSH keys for new VMs
  - Check for pending hardware changes before VM start
  - Use static IPs for critical infrastructure
  - Account for cloud-init execution time
  - Implement retry logic with exponential backoff
- **Tags**: #kubernetes #k3s #iac #proxmox #cloud-init #ssh

---

### Nextcloud

#### Nextcloud Deployment - Lessons Learned
- **File**: `/home/psimmons/projects/nextcloud-deployment/LESSONS-LEARNED.md`
- **Last Updated**: 2025-12-08
- **Status**: Production deployment complete
- **Topics**:
  - Multi-domain certificate strategy
  - Apache plugin dependencies
  - DNS resolution (Tailscale MagicDNS)
  - Technology architecture decisions
- **Key Lessons**:
  - Let's Encrypt cannot validate Tailscale domains
  - Dual certificate approach (Let's Encrypt + Tailscale) is best
  - Tailscale certs stored in `/var/lib/tailscale/certs/`
  - Incremental deployment benefits
  - Security-first implementation
- **Technology Validation**:
  - ✅ PostgreSQL for performance
  - ✅ Apache for Nextcloud integration
  - ✅ Redis + APCu caching
  - ✅ Tailscale for private network access
- **Tags**: #nextcloud #deployment #ssl #certificates #architecture #lessons-learned

#### Nextcloud Maintenance Automation Session
- **File**: `/home/psimmons/projects/nextcloud-deployment/SESSION-2025-12-23-MAINTENANCE-AUTOMATION.md`
- **Date**: 2025-12-23
- **Status**: Complete - Production Ready
- **Topics**:
  - Automated health monitoring (every 5 minutes)
  - Daily backup verification (2:30 AM)
  - Weekly maintenance (Sunday 3 AM)
  - Monthly maintenance (1st of month, 4 AM)
- **Critical Mistakes Documented**:
  - Mistake #1: Relied on documentation instead of checking live environment
  - Mistake #2: Didn't question "Not Deployed" status
  - Mistake #3: Assumed maintenance plan was implemented (it wasn't)
  - Mistake #4: Incomplete initial implementation
- **Key Lessons**:
  - ALWAYS verify live environment FIRST before reading docs
  - Documentation represents a point in time, not current reality
  - Question "not yet implemented" status if context suggests otherwise
  - Distinguish planning from implementation clearly
  - Verify permissions match use case (cron vs www-data)
  - Document "silent when healthy" behavior
- **Automation Created**:
  - Health monitoring script
  - Daily backup check script
  - Weekly maintenance script (app updates, DB optimization, cleanup)
  - Monthly maintenance script (security scans, system updates)
- **Tags**: #nextcloud #automation #maintenance #lessons-learned #mistakes #cron

#### Nextcloud Maintenance Status
- **File**: `/home/psimmons/projects/nextcloud-deployment/MAINTENANCE-AUTOMATION-STATUS.md`
- **Last Updated**: 2025-12-23
- **Status**: Fully Operational
- **Topics**:
  - Automated maintenance schedules
  - Script locations and purposes
  - Log monitoring procedures
  - Troubleshooting guide
- **Current System Health**:
  - HTTP: 302 (redirect) ✓
  - Disk: 3% usage ✓
  - Memory: 10% usage ✓
  - SSL: 84 days remaining ✓
  - Backups: 66 files, latest Dec 22
- **Tags**: #nextcloud #maintenance #automation #monitoring #status

---

### Proxmox

#### Grafana Dashboard Troubleshooting
- **File**: `/home/psimmons/projects/proxmox-performance-measurement/GRAFANA-TROUBLESHOOTING-LOG.md`
- **Last Updated**: 2025-12-28
- **Status**: Issues resolved, monitoring active
- **Topics**:
  - Conflicting Ingress resources (RESOLVED)
  - DNS configuration error (RESOLVED)
  - Prometheus storage failure (workaround in place)
  - Dashboard datasource corruption
  - Authentication issues
- **Key Issues Resolved**:
  - Issue 1: Two routing resources (Ingress + IngressRoute) caused port mismatch
  - Issue 2: Active Directory DNS had malformed CNAME (`traefik.` instead of `traefik.petersimmons.com`)
  - Issue 3: Dashboard datasource UID corrupted after Prometheus restart
- **Key Lessons**:
  - Don't delete/restart pods as first troubleshooting step
  - DNS issues can cause oscillating behavior
  - Restarting Prometheus can corrupt dashboard datasource references
  - Always preserve user data and state
  - Fix root cause, not symptoms
- **Critical User Requirements**:
  - DO NOT delete/restart Grafana pods without preserving data
  - DO NOT delete/recreate user accounts
  - DO investigate logs and configuration first
- **Tags**: #grafana #proxmox #troubleshooting #dns #ingress #prometheus

---

### Infrastructure as Code

#### IaC Homelab Lessons
- **File**: `/home/psimmons/projects/iac-homelab/K3S_SETUP_LESSONS.md`
- **Status**: Reference document
- **Topics**: SSH, Proxmox API, cloud-init, networking
- **Key Lessons**: See K3s Setup Lessons section above
- **Tags**: #iac #terraform #proxmox #automation

---

### Self-Learning System

#### CLAUDE.md Optimization & Self-Learning Architecture
- **Design Document**: `/home/psimmons/docs/plans/2026-01-17-claude-md-optimization-design.md`
- **Test Results**: `/home/psimmons/.homelab/docs/phase6-integration-test-results.md`
- **Last Updated**: 2026-01-18
- **Status**: Production - Phase 6 testing complete
- **Topics**:
  - CLAUDE.md token optimization (2400 → 600 tokens)
  - Self-learning infrastructure with YAML knowledge base
  - 17 homelab skills for troubleshooting, learning, and prediction
  - Zero auto-invocation startup policy
- **Key Lessons**:
  - Extract procedural content to skills, keep CLAUDE.md as navigation
  - Track fix effectiveness to evolve runbooks data-driven
  - Capture corrections silently for meta-learning
  - Predict failures from historical warning patterns
- **Data Structure**:
  ```
  ~/.homelab/
  ├── knowledge/           # Learning data
  │   ├── failure-history.yaml
  │   ├── fix-effectiveness.yaml
  │   ├── warning-patterns.yaml
  │   └── assistant-learning.yaml
  ├── config/              # Static configuration
  │   ├── service-dependencies.yaml
  │   ├── anti-patterns.yaml
  │   └── assistant-behavior.yaml
  └── docs/                # Generated reports
  ```
- **Skills Created** (17 total):
  - Troubleshooting: `troubleshoot-common-issues`, `troubleshoot-homepage`, `troubleshoot-nextcloud`, `troubleshoot-hardware`
  - Learning: `log-incident`, `log-fix-result`, `capture-learning`, `monthly-review`
  - Predictive: `predict-failure`, `trace-dependencies`, `check-anti-patterns`
  - Process: `evolve-runbook`, `create-runbook`, `incident-response`, `session-startup`, `production-readiness`, `test-backups`
- **Tags**: #self-learning #skills #optimization #automation #meta-learning #architecture

---

## By Problem Type

### Critical Production Issues

#### Complete System Freeze After Idle (Hardware - CRITICAL)
- **File**: `/home/psimmons/AMD-GPU-FREEZE-TROUBLESHOOTING-LOG.md`
- **Severity**: ⚠️ CRITICAL - Requires hard power off to recover
- **Status**: ROOT CAUSE IDENTIFIED - FIX READY TO APPLY
- **Impact**: Entire system freezes after idle, mouse/keyboard unresponsive, monitors stuck on
- **Root Cause**: AMD PowerPlay GFXOFF feature fails during DisplayPort power management, crashes COSMIC compositor
- **Hardware Affected**: AMD Radeon RX 7900 XT/XTX (RDNA3), 4x 27" DisplayPort monitors
- **Fix**: Disable GFXOFF with `sudo kernelstub --add-options "amdgpu.ppfeaturemask=0xffff7fff"` then reboot
- **Tags**: #hardware #amd #gpu #critical #freeze #displayport #cosmic

#### Mouse Unresponsive After Idle (Hardware)
- **File**: `/home/psimmons/MOUSE-TROUBLESHOOTING-LOG.md`
- **Severity**: High - Blocks work when idle
- **Status**: Workaround available, permanent fix ready
- **Impact**: Mouse stops responding after system idle
- **Workaround**: `/home/psimmons/mouse.sh` (requires manual execution)
- **Permanent Fix**: `/home/psimmons/fix-mouse-permanent.sh` (disables USB autosuspend)
- **Tags**: #hardware #critical #workaround

#### Prometheus Volume Faulted (Storage)
- **File**: `/home/psimmons/projects/kubernetes/LONGHORN-TROUBLESHOOTING-LOG.md`
- **Severity**: CRITICAL - All cluster metrics lost
- **Status**: Active investigation required
- **Impact**: Prometheus cannot write metrics, cluster-wide monitoring down
- **Root Cause**: Longhorn replica scheduling failure
- **Next Steps**: Check disk space on all nodes, investigate Longhorn health
- **Tags**: #kubernetes #longhorn #critical #storage

#### Active Directory DNS Misconfiguration (Network)
- **File**: `/home/psimmons/projects/proxmox-performance-measurement/GRAFANA-TROUBLESHOOTING-LOG.md`
- **Severity**: High - Causes service outages
- **Status**: RESOLVED (2025-12-28)
- **Impact**: grafana.petersimmons.com DNS SERVFAIL, breaking Homepage links
- **Root Cause**: AD server had malformed CNAME record (`traefik.` instead of FQDN)
- **Fix Applied**: Corrected CNAME in Active Directory DNS on 192.168.0.249
- **Tags**: #dns #active-directory #networking #resolved

---

### Deployment Issues

#### Homepage Widget Configuration Lost
- **File**: `/home/psimmons/projects/kubernetes/homepage/SESSION-2025-12-20-IMPROVEMENTS.md`
- **Severity**: Medium - Functionality degraded
- **Status**: Resolved (2025-12-20)
- **Impact**: Proxmox and Pihole widgets showing no dynamic data
- **Root Cause**: Widget configurations removed during previous config change
- **Fix**: Restored inline widget configs for all affected services
- **Prevention**: Always backup configs before changes, verify widget functionality
- **Tags**: #kubernetes #homepage #deployment #configuration

#### Nextcloud Maintenance Not Implemented
- **File**: `/home/psimmons/projects/nextcloud-deployment/SESSION-2025-12-23-MAINTENANCE-AUTOMATION.md`
- **Severity**: Medium - Gap between planning and implementation
- **Status**: Resolved (2025-12-23)
- **Impact**: Comprehensive maintenance plan documented but never built
- **Root Cause**: Assumed documentation = implementation
- **Fix**: Created all maintenance scripts, configured cron jobs
- **Prevention**: Always verify live state before trusting documentation
- **Tags**: #nextcloud #deployment #automation #documentation

---

### Configuration Issues

#### Homepage Using :latest Tag
- **File**: `/home/psimmons/projects/kubernetes/homepage/LESSONS_LEARNED.md`
- **Severity**: High - Silent breaking changes
- **Status**: Resolved - Now pinned to v0.9.5
- **Impact**: Homepage auto-updated to buggy versions, breaking functionality
- **Root Cause**: Used `:latest` instead of pinned version
- **Fix**: Pinned to `ghcr.io/gethomepage/homepage:v0.9.5`
- **Prevention**: NEVER use :latest in production
- **Tags**: #kubernetes #homepage #configuration #best-practice

#### Grafana Datasource UID Corruption
- **File**: `/home/psimmons/projects/proxmox-performance-measurement/GRAFANA-TROUBLESHOOTING-LOG.md`
- **Severity**: Medium - Dashboard shows "No data"
- **Status**: Resolved (2025-12-28)
- **Impact**: All dashboard panels showing "No data" after Prometheus restart
- **Root Cause**: Dashboard datasource UID changed to invalid value "prometheus"
- **Fix**: Used jq to replace all datasource UIDs with correct value
- **Prevention**: Verify dashboard datasources after Prometheus restarts
- **Tags**: #grafana #configuration #prometheus

---

### Performance Issues

#### Embedded etcd Performance
- **File**: `/home/psimmons/projects/k3s-ha-cluster-rebuild/docs/reference/LESSONS-LEARNED.md`
- **Severity**: N/A - Performance is excellent
- **Status**: Validated
- **Finding**: Embedded etcd performs excellently (~5ms latency)
- **Recommendation**: Use embedded etcd for clusters <100 nodes in private network
- **Tags**: #kubernetes #k3s #performance #etcd

---

## By Date

### 2026-01-18
- **CLAUDE.md Self-Learning System**: COMPLETE - Implemented intelligent assistant learning architecture
- **Skills Created**: 17 homelab skills for troubleshooting, learning, and prediction
- **Phase 6 Testing**: All 4 integration test scenarios passed
- **Files**:
  - `/home/psimmons/docs/plans/2026-01-17-claude-md-optimization-design.md` - Design document
  - `/home/psimmons/.homelab/docs/phase6-integration-test-results.md` - Test results
  - `/home/psimmons/.homelab/knowledge/` - Self-learning data files
  - `/home/psimmons/.claude/commands/homelab-*.md` - 17 homelab skills
- **Key Achievements**:
  - CLAUDE.md reduced from ~2400 to ~600 tokens (75% reduction)
  - Self-learning system captures incidents, tracks fix effectiveness
  - Predictive warnings based on historical failure patterns
  - Runbook evolution based on real-world success rates
  - Meta-learning captures workflow mistakes for skill improvement
- **Tags**: #self-learning #skills #optimization #automation #meta-learning

### 2026-01-13
- **Container Registry Setup**: COMPLETE - Local Docker registry with HTTPS access, all nodes configured
- **Job Search System**: Identified image distribution blocker, now unblocked by registry
- **Files**:
  - `/home/psimmons/projects/infrastructure/container-registry/README.md`
  - `/home/psimmons/projects/job-search-system/PROJECT-STATUS.md`
- **Impact**: Eliminated ImagePullBackOff issues cluster-wide, unblocked job-search-system project

### 2025-12-28
- **Grafana Dashboard Issues**: RESOLVED - DNS and datasource problems fixed
- **Longhorn Storage**: CRITICAL - Prometheus volume faulted, investigation ongoing
- **Files**:
  - `/home/psimmons/projects/proxmox-performance-measurement/GRAFANA-TROUBLESHOOTING-LOG.md`
  - `/home/psimmons/projects/kubernetes/LONGHORN-TROUBLESHOOTING-LOG.md`

### 2025-12-27
- **Mouse Troubleshooting**: Permanent fix created, ready for implementation
- **Files**: `/home/psimmons/MOUSE-TROUBLESHOOTING-LOG.md`

### 2025-12-23
- **Nextcloud Maintenance Automation**: Complete - All scripts deployed
- **Files**:
  - `/home/psimmons/projects/nextcloud-deployment/SESSION-2025-12-23-MAINTENANCE-AUTOMATION.md`
  - `/home/psimmons/projects/nextcloud-deployment/MAINTENANCE-AUTOMATION-STATUS.md`

### 2025-12-22
- **Homepage Instructions**: Updated with critical documentation links
- **Files**: `/home/psimmons/projects/custom-homepage/HOMEPAGE-INSTRUCTIONS.md`

### 2025-12-20
- **Homepage Improvements**: Widget restoration, icon fixes, search bar fixed
- **Files**:
  - `/home/psimmons/projects/kubernetes/homepage/SESSION-2025-12-20-IMPROVEMENTS.md`
  - `/home/psimmons/projects/kubernetes/homepage/LESSONS_LEARNED.md`

### 2025-12-08
- **Nextcloud Deployment**: Production deployment complete
- **Files**: `/home/psimmons/projects/nextcloud-deployment/LESSONS-LEARNED.md`

### 2025-11-22
- **K3s HA Cluster**: Deployment complete, lessons documented
- **Files**: `/home/psimmons/projects/k3s-ha-cluster-rebuild/docs/reference/LESSONS-LEARNED.md`

---

## Critical Commands Reference

### Homepage
```bash
# Emergency restore
kubectl apply -f /home/psimmons/projects/kubernetes/homepage/configmap-updated.yaml
kubectl rollout restart deployment/homepage -n default

# Health check
curl -s https://homepage.petersimmons.com | grep -i "api error"
curl -s https://homepage.petersimmons.com | grep -i "something went wrong"
```

### Longhorn Storage
```bash
# Check for faulted volumes
kubectl get volumes -n longhorn-system
kubectl get volumes -n longhorn-system -o json | jq -r '.items[] | select(.status.state=="faulted") | .metadata.name'

# Access Longhorn UI
kubectl port-forward -n longhorn-system svc/longhorn-frontend 8080:80
# Then: http://localhost:8080
```

### Nextcloud
```bash
# Check live status
curl -I https://nextcloud.petersimmons.com
ssh psimmons@192.168.0.200 "systemctl status apache2 postgresql redis-server"

# View maintenance logs
ssh psimmons@192.168.0.200
tail -f /var/log/nextcloud/monitoring.log
tail -f /var/log/nextcloud/backup-check.log
```

### Mouse Fix
```bash
# Temporary fix (requires sudo password)
/home/psimmons/mouse.sh

# Permanent fix (not yet applied)
/home/psimmons/fix-mouse-permanent.sh
```

---

## Documentation Standards

### When Creating New Troubleshooting Logs

**Required Sections:**
1. **Overview** - Purpose, scope, last updated
2. **Current State** - What's working, what's not
3. **Critical Issues** - Active problems with severity
4. **Lessons Learned** - Key takeaways
5. **Quick Reference** - Common commands
6. **Related Documentation** - Links to other relevant files

**Naming Convention:**
- `*TROUBLESHOOTING-LOG.md` - Active troubleshooting
- `*LESSONS-LEARNED.md` - Lessons from completed work
- `SESSION-*.md` - Session-specific documentation
- `*INSTRUCTIONS.md` - How-to guides
- `*MAINTENANCE*.md` - Maintenance procedures

### When to Update This Index

**Update this index when:**
- Creating new troubleshooting logs
- Resolving critical issues
- Documenting new lessons learned
- Starting major troubleshooting sessions
- Completing deployment sessions

---

## Index Maintenance

**Review Frequency**: Monthly or after significant troubleshooting sessions

**Maintainer**: Peter Simmons

**Last Major Review**: 2025-12-29

**Next Scheduled Review**: 2026-01-29

---

## Search Tips

**Finding specific issues:**
```bash
# Search for specific technology
grep -i "kubernetes" /home/psimmons/KNOWLEDGE-INDEX.md

# Find all critical issues
grep -i "severity: critical" /home/psimmons/KNOWLEDGE-INDEX.md

# Find recent updates
grep -i "2025-12" /home/psimmons/KNOWLEDGE-INDEX.md

# Find by tag
grep -i "#troubleshooting" /home/psimmons/KNOWLEDGE-INDEX.md
```

**Common tags:**
- `#hardware` - Hardware issues
- `#kubernetes` - K8s related
- `#critical` - Critical production issues
- `#troubleshooting` - Active troubleshooting
- `#lessons-learned` - Documented lessons
- `#resolved` - Issues that have been fixed
- `#workaround` - Temporary fixes in place
- `#automation` - Automation and maintenance
- `#deployment` - Deployment issues
- `#configuration` - Configuration problems
- `#networking` - Network issues
- `#storage` - Storage issues
- `#performance` - Performance topics
- `#self-learning` - Self-learning system components
- `#skills` - Claude skills and workflows
- `#meta-learning` - Assistant improvement tracking
- `#optimization` - Token/performance optimization

---

## Quick Access by Service

| Service | Primary Documentation | Troubleshooting Log | Status |
|---------|----------------------|---------------------|--------|
| Homepage | `/home/psimmons/projects/custom-homepage/HOMEPAGE-INSTRUCTIONS.md` | `/home/psimmons/projects/kubernetes/homepage/LESSONS_LEARNED.md` | Production |
| Nextcloud | `/home/psimmons/projects/nextcloud-deployment/MAINTENANCE-AUTOMATION-STATUS.md` | `/home/psimmons/projects/nextcloud-deployment/SESSION-2025-12-23-MAINTENANCE-AUTOMATION.md` | Production |
| Longhorn | N/A | `/home/psimmons/projects/kubernetes/LONGHORN-TROUBLESHOOTING-LOG.md` | Critical Issue |
| Grafana | `/home/psimmons/projects/proxmox-performance-measurement/PROJECT_SUMMARY.md` | `/home/psimmons/projects/proxmox-performance-measurement/GRAFANA-TROUBLESHOOTING-LOG.md` | Resolved |
| K3s Cluster | `/home/psimmons/projects/k3s-ha-cluster-rebuild/README.md` | `/home/psimmons/projects/k3s-ha-cluster-rebuild/docs/reference/LESSONS-LEARNED.md` | Production |
| Mouse (Hardware) | N/A | `/home/psimmons/MOUSE-TROUBLESHOOTING-LOG.md` | Workaround Available |

---

**Remember**: This index is a living document. Update it whenever you create new troubleshooting logs or resolve major issues. The more comprehensive this index, the faster you can find solutions to recurring problems.

**Pro Tip**: When starting a troubleshooting session, search this index first. Many problems have been solved before and documented here.
