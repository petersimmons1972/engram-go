# Failure Modes Catalog

Known failure patterns with impact (blast radius) and recovery time (MTTR).

**Purpose:**
- Quickly understand failure severity
- Set realistic recovery expectations
- Prioritize which failures to prevent

---

## Tier 1: Critical (Complete Loss of Functionality)

| Failure                        | Symptoms                           | Blast Radius              | MTTR   | Last Occurrence | Recovery Procedure                                          |
|--------------------------------|------------------------------------|---------------------------|--------|-----------------|-------------------------------------------------------------|
| **Both Pi-holes down**         | DNS resolution fails globally      | Complete internet outage  | 1 min  | Not documented  | Check Pi-hole status at .231 and .232                       |
| **Proxmox storage (zp3) full** | VM freeze, deployment failures     | Entire homelab freezes    | 30 min | Not documented  | Run cleanup, extend storage                                 |
| **Mouse driver crash**         | No cursor movement after idle      | Complete desktop unusable | 30 sec | Ongoing issue   | `/home/psimmons/bin/mouse.sh` or logout/login               |
| **Traefik pod down**           | All Kubernetes ingress returns 503 | All K8s web services down | 5 min  | Not documented  | `kubectl rollout restart -n kube-system deployment/traefik` |

---

## Tier 2: Major (Single Service Down)

| Failure                       | Symptoms                                | Blast Radius            | MTTR   | Last Occurrence | Recovery Procedure                                    |
|-------------------------------|-----------------------------------------|-------------------------|--------|-----------------|-------------------------------------------------------|
| **Nextcloud Apache down**     | 503 error on nextcloud.petersimmons.com | Nextcloud only          | 1 min  | Not documented  | `ssh 192.168.0.200 "sudo systemctl restart apache2"`  |
| **Homepage pod crashloop**    | 404, "API Error", widgets not loading   | Homepage dashboard only | 2 min  | Not documented  | `kubectl rollout restart deployment/homepage`         |
| **Longhorn volume faulted**   | PVC stuck Pending, I/O errors           | Affected pods only      | 30 min | Not documented  | Check Longhorn UI, verify node disk space             |
| **Kubernetes node not ready** | Pods evicted, scheduling failures       | Workloads on that node  | 10 min | Not documented  | Check node: `kubectl describe node <name>`            |

---

## Tier 3: Moderate (Degraded/Partial Functionality)

| Failure                              | Symptoms                              | Blast Radius                 | MTTR   | Last Occurrence | Recovery Procedure                                    |
|--------------------------------------|---------------------------------------|------------------------------|--------|-----------------|-------------------------------------------------------|
| **Homepage network policy mismatch** | 200 OK but "Something went wrong"     | Homepage dashboard only      | 5 min  | Not documented  | Verify labels match `app.kubernetes.io/name=homepage` |
| **Nextcloud maintenance mode stuck** | Maintenance mode page shown           | Nextcloud only               | 1 min  | Not documented  | `occ maintenance:mode --off`                          |
| **Traefik routing conflict**         | Specific service 404/503              | Single service               | 10 min | Not documented  | Check for both Ingress + IngressRoute (must use one)  |
| **AD DNS malformed CNAME**           | DNS lookup returns wrong/empty result | Internal hostname resolution | 5 min  | Not documented  | Check for trailing dots in DNS records                |
| **Cert-manager certificate issues**  | HTTPS errors, invalid cert warnings   | Services using that cert     | 10 min | Not documented  | `kubectl get certificate -A`, check cert-manager logs |
| **Pod GC gaps after node issues**    | 100+ Error/Failed pods in health check | Monitoring noise only      | 5 min  | 2026-01-13      | Verify services healthy, cleanup orphaned pods        |
| **Node disk pressure**               | Pod evictions, scheduling failures    | Pods on affected node        | 30 min | 2026-01-13      | Check node storage, cleanup unused images/volumes     |
| **K8s imagePullPolicy caching**      | Rebuilt app shows old content/features | Affected deployments        | 10 min | 2026-02-14      | Set `imagePullPolicy: Always` for :latest tags, delete pods |
| **Service/Deployment port mismatch** | Pods CrashLoopBackOff, probes fail    | Affected deployment         | 5 min  | 2026-02-14      | Verify containerPort, livenessProbe.port, readinessProbe.port, Service.targetPort all match |
| **Traefik IngressRoute API version** | `kubectl apply` fails, "no matches for kind" | Deployment blocked     | 2 min  | 2026-02-14      | Use `traefik.io/v1alpha1` not `traefik.containo.us/v1alpha1` |
| **Conflicting IngressRoute hostnames** | Site serves stale content or 404    | Affected hostname           | 5 min  | 2026-02-14      | Check `kubectl get ingressroute` for duplicates, delete old routes |
| **Stuck rolling update (CrashLoopBackOff)** | Two pods from different RSes, new one crashing 50-500+ times | Double resource consumption, wasted CPU | 2 min | 2026-02-17 | `kubectl rollout undo -n <ns> deployment/<name>` |
| **Missing registry image (ImagePullBackOff >1h)** | Pod in ImagePullBackOff, registry returns NAME_UNKNOWN | Service down until image rebuilt | 10 min | 2026-02-17 | Verify image: `curl -sk https://registry.petersimmons.com/v2/<img>/tags/list`; scale to 0 then rebuild |
| **ReplicaSet bloat (empty RSes)** | 50+ empty ReplicaSets cluster-wide, slow kubectl responses | Etcd bloat, monitoring noise | 10 min | 2026-02-17 | Delete empty RSes; set `revisionHistoryLimit: 3` on all deployments |
| **Stateless deployment without HPA** | Fixed replica count regardless of load (e.g. 20 replicas at 0% CPU) | Resource waste on idle workloads | 5 min | 2026-02-17 | Create HPA: min replicas, CPU/memory targets, scale-down stabilization window |

---

## Legend

| Term               | Meaning                                                                    |
|--------------------|----------------------------------------------------------------------------|
| **MTTR**           | Mean Time To Recovery (based on documented procedures)                     |
| **Blast Radius**   | What stops working when this failure occurs                                |
| **Last Occurrence**| When this failure last happened (UPDATE after each incident)               |
| **Tier 1**         | Complete loss of major functionality - highest prevention priority         |
| **Tier 2**         | Single service down, no cascade                                            |
| **Tier 3**         | Degraded functionality, services remain partially functional               |

---

## Maintenance

- **Update Last Occurrence** after each incident to track frequency
- **Add new failure modes** as they're discovered
- **Source**: Based on actual failures from `/home/psimmons/RUNBOOKS-INDEX.md`
- Focus monitoring/automation on preventing Tier 1 failures first

---

**Last Updated:** 2026-02-17

## Tier 5: Report Generation Failures (Business Intelligence)

| Failure | Symptoms | Blast Radius | MTTR | Last Occurrence | Recovery Procedure |
|---------|----------|--------------|------|-----------------|-------------------|
| **Technical accuracy without human usability** | Report is factually correct but unreadable by humans - wall of text, no narrative flow, information scattered, no visual engagement | Report unusable despite being accurate | 8-12 hrs | 2026-02-12 (Report 197) | Redesign with: narrative structure, visual breaks, pull quotes, sidebars, strategic chart placement, smaller chunks (400-600 words), max length 15-18K words |
| **Multi-stage verification failure (charts)** | Code generates charts but final output missing - placeholder mismatch | Charts absent from report | 2-3 hrs | 2026-02-12 (Report 197: 5/10 charts) | Verify Stage 2 (integration) and Stage 3 (delivery), not just Stage 1 (generation) |
| **Multi-stage verification failure (citations)** | Code generates citations but endnotes not rendered | 90% broken citations, no source verification | 2-4 hrs | 2026-02-12 (Report 197: 4/39 endnotes) | Verify endnote section exists with full bibliographic info, not just inline markers |
| **Data contradiction between sections** | Chart shows different numbers than text | Financial credibility undermined | 15 min | 2026-02-12 (Report 197: TCO 58% gap) | Cross-reference verification between chart data sources and prose calculations |
| **Robot assembly without narrative** | Content reads like "6 robots cut-and-paste" - no unified voice despite multiple authors | Readers abandon document, $500 value not delivered | 8-12 hrs | 2026-02-12 (Report 197 user grade: C-) | Add narrative assembly phase with single editor pass for voice/flow consistency |

---

### Report Generation Lessons (2026-02-12)

**Context**: Report 197 validation team gave 77.4/100 (C+) but user gave C- because validation missed **human usability** dimension entirely.

**What validation team checked:**
- ✅ Technical accuracy (88/100)
- ✅ Citation count (failed: 23/100)
- ✅ Chart count (failed: 54/100)
- ✅ Content depth (88/100)
- ✅ Structure & cohesion (91/100)

**What validation team MISSED:**
- ❌ Readability - Can humans actually read 31,733 words of walls of text?
- ❌ Engagement - Does this draw readers in with visual hierarchy?
- ❌ Information architecture - Can you find related info or is TCO scattered across 12 pages?
- ❌ Visual rhythm - Pull quotes, sidebars, charts placed strategically?
- ❌ Narrative flow - Does this tell a story or just list facts?
- ❌ Length appropriateness - Is 31,733 words (2.8x target) reasonable?
- ❌ Practical usability - Will a CISO actually read this or treat as spam?
- ❌ $500 value proposition - Worth keeping vs searchable document vs spam?

**Critical insight**: Even if citations and charts were fixed (Phase 1 remediation → 85-87/100), report would still be C- grade because fundamental document architecture is wrong for human consumption.

**Requirements for Report 198 (Army Orders v5.0):**

1. **Length constraint**: 15,000-18,000 words maximum (not 31,733)
2. **Narrative structure**: Story with chapters, not sections thrown together
3. **Visual engagement strategy**:
   - Pull quotes (key insights highlighted)
   - Sidebars (related facts, case studies, "Did you know?" moments)
   - Charts/graphs placed near relevant text (not random)
   - Illustrations that reinforce concepts
   - Info-graphics for complex data
   - Visual breaks every 1-2 pages
4. **Information architecture**:
   - Related concepts grouped together (TCO in ONE place, not 12 pages)
   - Cross-references when needed
   - Clear section breaks with visual hierarchy
   - Scannable headings and subheadings
5. **Reading experience design**:
   - Smaller chunks (400-600 word sections maximum)
   - Visual rhythm (text → chart → text → sidebar → text)
   - Eye-catching elements to stop skimmers
   - "Reasons to dig deeper" moments
   - White space and breathing room
6. **Value proposition for $500 document**:
   - Immediately actionable
   - Visually engaging (not wall of text)
   - Easy to navigate
   - Worth keeping and referencing
   - Not just searchable, but discoverable
   - Designed to be read, not skimmed out of desperation

**New validation requirement**: Add "Reader Experience Reviewer" role to evaluate human usability, not just technical correctness.

**Key quote from user**: "We have to give them a reason to want to dig into this $500 document. Otherwise, we have given them a worthless document that is just a searchable document... at best. At worst it might be something you leave as spam in your mailbox."

---

## Tier 4: Operational Hygiene (Non-Critical)

| Failure                               | Symptoms                                   | Blast Radius           | MTTR   | Last Occurrence | Recovery Procedure                                            |
|---------------------------------------|--------------------------------------------|------------------------|--------|-----------------|---------------------------------------------------------------|
| **Failed pod accumulation**           | >3 Failed pods in cluster, monitoring noise | Monitoring clarity only | 5 min  | 2026-02-12      | Review pods, delete completed job pods after verification     |
| **Certificate issuer name mismatch**  | Cert stuck "Issuing", no order/challenge   | Affected services      | 17 min | 2026-02-12      | Update ingress annotation to correct ClusterIssuer name       |

