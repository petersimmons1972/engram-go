# Design: Fleet Controller Failure Reporting + Self-Healing

**Date:** 2026-05-29
**Status:** Draft — awaiting founder review
**Scope:** `aifleet` repo only — `watcher/` and `controller/` modules
**Feature label:** `feature/fc-failure-reporting`

---

## 1. Context and Problem

### 1.1 Incident History

Four classes of production incident have made the gaps concrete:

| Issue | Incident | Root Cause of Missed Detection |
|-------|----------|-------------------------------|
| **#304** | Disk-full cascade on `precision` — overlay2 ENOSPC → GPU_INIT_FAILED (exit 42) → degraded-suppression → host reboot | Watcher classifies disk-full crash as a GPU init failure. No disk evidence captured. No remediation. Degraded-suppression buries the signal. |
| **#310** | Stale `degradedContainers` — CRD status still lists removed/healthy containers as degraded | Controller's CRD status writer never clears degraded entries on model removal or recovery. False-degraded signal persists indefinitely. |
| **#312** | oblivion vLLM inference down 6h+ with FC still reporting `running:true` | Watcher silence for 6h (crash or partition) was invisible. No heartbeat-staleness gating on the FC side. FC republished stale `running:true`. |
| **#309** | VRAM self-healer silently non-functional — `docker exec rocm-smi` fails (docker binary absent from watcher container PATH) | Self-healer probe path fails silently; `AssertProbeReady()` error at boot is logged but not propagated to status. |

There is also an **unbounded `pullFailed` retry path** (referenced as the #910 gap in the brief): `reconcileDrift` retries a model whose `Ensure()` failed indefinitely, with backoff capped at `pullFailedRetryInterval = 5m`. There is no bound on total attempts, no escalation path, and no controller-visible failure record until the operator manually sends SIGHUP.

The **W6800 archetype** (precision, 2026-05): `fast-inference` (vLLM, Tier 2) entered a crash loop, cycling hundreds of times and consuming GPU VRAM / disk on each restart. The co-located `embed-w6800` (Infinity, Tier 1) was degraded by the contention. The FC had no mechanism to yank the failing model to protect the healthy one; the outage required manual operator intervention and a host reboot.

### 1.2 The Existing Channel

`report()` in `watcher/internal/watcher.go` already calls `PostStatus` every 30 seconds (`reportInterval`), pushing a `StatusReport` to `POST /status/{hostname}` on the controller. The payload already carries `DegradedContainers []string` and `FailedModels []ModelFailure`. The controller's `postStatus` handler calls `recordStatusAnomalies`, writes to the `Store`, patches the GpuHost CRD status subresource, and reconciles the Olla ConfigMap.

**This feature does not add a new transport.** It enriches the payload and adds controller-side classification, bounded self-correction, incident recording, and GitHub issue escalation.

### 1.3 What "Plan A" Means

This spec is **Plan A**: capture → classify → bounded self-correct (preferred) → else record Incident + raise GitHub issue → Codex repairs.

**Plan B** (autonomous hypothesis testing — runtime experiments, multi-step diagnosis) is explicitly a future concern and is out of scope.

---

## 2. Goals and Non-Goals

### Goals

- **G1** — Capture a rich, structured `FailureEvent` at the watcher when a container fails, so the failure class and evidence are visible controller-side.
- **G2** — Classify failures into a deterministic enum so the controller can apply per-class self-correction rules without natural language parsing.
- **G3** — Execute bounded self-correction for failures that have a known fix (disk_full, oom_vram, image_pull_failed, stale degraded entries, heartbeat staleness, crash-loop ceiling).
- **G4** — Guarantee every self-correct action is recorded in history so loops are impossible.
- **G5** — Record a first-class **Incident** in FC state (in-memory store + CRD + `/status` API) when a bound is exhausted or no rule matches.
- **G6** — Raise a deduplicated GitHub issue carrying the full diagnostic payload when an Incident is created, so Codex can pick it up as a work item.
- **G7** — Fix the `#310` stale-degraded-containers bug as a direct side-effect of the self-correct rule engine.
- **G8** — Fix the `#312` heartbeat-staleness-oblivion gap by adding a controller-side heartbeat staleness gate.
- **G9** — Fix the `#304` disk-full mis-classification by distinguishing `disk_full` from `gpu_init_failed`.
- **G10** — Cap the unbounded `pullFailed` retry path by adding a maximum-attempt escalation bound.
- **G11** — Protect higher-criticality (Tier 1) models from sustained lower-tier contention by authorizing the FC to autonomously yank a failing model that is degrading its co-located neighbors.

### Non-Goals

- **NG1** — Runtime hypothesis testing or autonomous multi-step diagnosis (Plan B, future work).
- **NG2** — Replacing the existing `degraded` map with a database.
- **NG3** — Any transport change — the existing `POST /status/{hostname}` 30-second channel is the sole payload path.
- **NG4** — Any UI or dashboard change.
- **NG5** — Replacing the SIGHUP operator escape hatch; it remains the manual recovery path for operators.
- **NG6** — Changing the existing `restartWindow=10m / maxRestartsInWindow=3` health check loop — it is correct and stays unchanged.
- **NG7** — Automatic re-admission of a yanked model. Re-admission is a deliberate post-repair action (Codex/founder); a yanked model stays out of active desired state until manually cleared.

---

## 3. Workload Criticality Tiers

The FC uses criticality tiers for contention adjudication and yank decisions. Tiers are assigned per model and encoded in `ModelSpec`.

| Tier | Description | Examples | Rationale |
|------|-------------|----------|-----------|
| **1** | EMBEDDING models — protect above all | `embed-w6800` (bge-m3 on W6800), `embed-7900xt` (bge-m3 on 7900 XT) | Back engram-go/recall. The entire recall stack is inoperative without a healthy embedder. The FC must never sacrifice an embedder for a lower-tier model. |
| **2** | Network-available INFERENCE models | `fast-inference` (vLLM on W6800), oblivion Qwen3-32B | Wanted and kept available, but yields to embedders under resource contention. After a yank, tier-2 models are candidates for re-deployment post-repair. |
| **3** | Experimental / background models | oblivion experimental workloads | First to be throttled, deprioritized, or sacrificed under resource pressure. |

**Contention rule:** under shared-resource contention (GPU VRAM, host disk, host memory) the FC protects the higher-tier model and yanks or throttles the lower-tier model. In a tie within the same tier the failing model (the one breaching its ceiling) is yanked first.

**Spec extension:** add `criticality int` (1/2/3) to `ModelSpec` in both `watcher/internal/types.go` and `controller/internal/types.go`, and to the GPUHost CRD schema. Default (zero value) = Tier 2 for backward compatibility. The CRD schema enum is `[1, 2, 3]`. Operator sets this in the GPUHost spec YAML.

---

## 4. Architecture

### 4.1 Component Map

```
GPU Host (watcher)
├── checkHealth() / sampleAnomaly() / reconcileDrift()
│   └─ [failure detected] → FailureClassifier.Classify(evidence) → FailureEvent
│                        → SelfCorrectEngine.Apply(event) → action or ESCALATE
│                           ├─ action taken → record in FailureEvent.CorrectiveActions
│                           └─ ESCALATE → PostStatus carries FailureEvent in payload
│
│   watcher/internal/watcher.go     (failure detection, existing + extended)
│   watcher/internal/types.go       (FailureEvent struct + enum + Criticality — NEW/extended)
│   watcher/internal/classifier.go  (FailureClassifier — NEW)
│   watcher/internal/selfcorrect.go (SelfCorrectEngine — NEW)
│   watcher/internal/policy.go      (PostStatus — unchanged, payload extended)

Controller
├── POST /status/{hostname} → postStatus handler
│   ├─ existing: Store.Set, recordStatusAnomalies, updateGpuHostStatus, reconcileOllaConfigMap
│   └─ NEW: FailureEventProcessor.Process(events)
│       ├─ SelfCorrectRuleEngine.Apply(event)
│       │   ├─ clear stale degradedContainers (#310 fix)
│       │   ├─ heartbeat staleness gate (#312 fix)
│       │   ├─ collateral-impact detection → yank offending model
│       │   └─ crash-loop ceiling → yank offending model
│       ├─ [bound exhausted / no rule / yank] → IncidentStore.Record(incident)
│       │   ├─ Store.AddIncident (in-memory ring buffer)
│       │   ├─ PatchGpuHostStatus (CRD status, adds incidents[])
│       │   └─ GitHubIssueEscalator.Raise(incident) [deduped, non-blocking]
│       └─ yank path: remove model from GPUHost spec via K8s patch
│
│   controller/internal/handlers.go   (postStatus — add FailureEvent processing)
│   controller/internal/store.go      (AddIncident, ListIncidents — NEW)
│   controller/internal/status.go     (Incident type — NEW)
│   controller/internal/types.go      (StatusReport + GpuHostStatus extended; Criticality field)
│   controller/internal/escalator.go  (GitHubIssueEscalator — NEW)
│   controller/deploy/crd.yaml        (status.incidents[], spec.models[].criticality — NEW)
```

### 4.2 Lifecycle Sequence

```
1. Failure event on watcher
   checkHealth / sampleAnomaly / recordEnsureFailure
         │
         ▼
2. Classify
   FailureClassifier: exit code, logs, disk%, VRAM, probe error, restart history
   → FailureClass enum
   → "normal churn baseline" check: if restarted AND then healthy > stabilization window → NOT a problem; record rate only, no escalation
         │
         ▼
3. Self-correct attempt (watcher-side, bounded)
   SelfCorrectEngine.Apply(class, history, spec.Criticality)
   ├─ Corrective action taken → record in FailureEvent.CorrectiveActions; retry
   └─ Bound exhausted OR class has no watcher-side rule → NeedsEscalation = true
         │
         ▼
4. PostStatus (next 30s tick)
   StatusReport.FailureEvents[] populated
   POST /status/{hostname}
         │
         ▼
5. Controller: FailureEventProcessor.Process
   ├─ R6 stale-clear pass
   ├─ R7 heartbeat gate
   ├─ Collateral-impact detection: any co-located model degraded?
   │   └─ Yes + failing model is lower tier → yank (remove from GPUHost spec) immediately
   ├─ Crash-loop ceiling: sustained loop without recovery, ceiling crossed?
   │   └─ Yes → yank + NeedsEscalation = true
   └─ NeedsEscalation → IncidentStore.Record → GH issue raised
         │
         ▼
6. Yank path (when triggered)
   K8sClient.PatchGpuHostSpec: remove failing model from spec.models[]
   → Watcher reconciles: detects model absent from policy → removes container
   → Incident records that the model was yanked; GH issue body notes re-admission requires explicit operator action
         │
         ▼
7. GitHub issue (async, non-blocking)
   Dedup: ONE open issue per (host, model, failure_class)
   Body: FailureEvent + actions tried + why escalated
   Labels: fc-incident, severity, agent/codex/queued or agent/hermes
```

---

## 5. Data Model

### 5.1 FailureClass Enum

```go
// watcher/internal/types.go (new)
type FailureClass string

const (
    FailureClassDiskFull              FailureClass = "disk_full"
    FailureClassOOMVRAM               FailureClass = "oom_vram"
    FailureClassOOMHost               FailureClass = "oom_host"
    FailureClassImagePullFailed       FailureClass = "image_pull_failed"
    FailureClassGPUInitFailed         FailureClass = "gpu_init_failed"
    FailureClassHealthcheckUnhealthy  FailureClass = "healthcheck_unhealthy"
    FailureClassUnreachableButHealthy FailureClass = "unreachable_but_healthy"
    FailureClassCrashLoop             FailureClass = "crash_loop"
    FailureClassExitNonZero           FailureClass = "exit_nonzero"
    FailureClassCollateralImpact      FailureClass = "collateral_impact"
    FailureClassUnknown               FailureClass = "unknown"
)
```

### 5.2 FailureEvent

```go
// watcher/internal/types.go (new)
type FailureEvent struct {
    // Identity
    Host          string `json:"host"`
    ContainerName string `json:"containerName"`
    ModelName     string `json:"modelName"`
    Image         string `json:"image"`
    ImageTag      string `json:"imageTag,omitempty"`
    PolicyVersion string `json:"policyVersion"`
    Criticality   int    `json:"criticality"`  // 1/2/3 from ModelSpec

    // Lifecycle
    ModelAgeSec         int64     `json:"modelAgeSec"`         // seconds since container last started
    IsNewModel          bool      `json:"isNewModel"`          // true if < 5 min old at time of failure
    CumulativeAttempts  int       `json:"cumulativeAttempts"`  // total Ensure() calls this session
    RestartCount        int       `json:"restartCount"`        // restarts in current window
    FirstFailedAt       time.Time `json:"firstFailedAt"`
    LastFailedAt        time.Time `json:"lastFailedAt"`
    LastHealthyAt       *time.Time `json:"lastHealthyAt,omitempty"` // last time container was healthy

    // Classification
    Class                    FailureClass `json:"class"`
    IsNormalChurn            bool         `json:"isNormalChurn"`            // restarted + ran healthy; NOT escalated
    IsSustainedLoop          bool         `json:"isSustainedLoop"`          // restarts without recovery
    IsCollateralImpactSignal bool         `json:"isCollateralImpactSignal"` // co-located model degraded

    // Evidence
    ExitCode           int      `json:"exitCode,omitempty"`
    ExitSignal         string   `json:"exitSignal,omitempty"`
    LastLogLines       []string `json:"lastLogLines,omitempty"`  // last 20 lines
    VRAMUsedMB         int      `json:"vramUsedMB,omitempty"`
    VRAMFreeMB         int      `json:"vramFreeMB,omitempty"`
    HostDiskPct        float64  `json:"hostDiskPct,omitempty"`
    HostMemUsedPct     float64  `json:"hostMemUsedPct,omitempty"`
    ProbeFailedType    string   `json:"probeFailedType,omitempty"`  // "healthcheck" | "tcp_readiness" | "vram_oversubscribed"
    ProbeError         string   `json:"probeError,omitempty"`
    CollidingModelName string   `json:"collidingModelName,omitempty"` // name of the co-located model being harmed

    // History
    ConsecutiveFailures int                `json:"consecutiveFailures"`
    CorrectiveActions   []CorrectiveAction `json:"correctiveActions,omitempty"`
    WasYanked           bool               `json:"wasYanked,omitempty"`
    NeedsEscalation     bool               `json:"needsEscalation"`
    EscalationReason    string             `json:"escalationReason,omitempty"`
}

type CorrectiveAction struct {
    At      time.Time `json:"at"`
    Action  string    `json:"action"`   // e.g. "pruned_build_cache", "restarted", "backoff_retry", "yanked_from_spec"
    Outcome string    `json:"outcome"`  // "success" | "failed" | "pending"
    Detail  string    `json:"detail,omitempty"`
}
```

### 5.3 ModelSpec Extension

```go
// watcher/internal/types.go (extend existing)
// controller/internal/types.go (extend existing)
type ModelSpec struct {
    // ... existing fields unchanged ...
    // Criticality declares this model's protection tier for contention adjudication.
    // 1 = embeddings (highest, protect first), 2 = inference (default), 3 = experimental.
    // 0 is treated as 2 (backward-compatible default).
    Criticality int `json:"criticality,omitempty"`
}
```

### 5.4 StatusReport Extension

```go
// watcher/internal/types.go (extend existing struct)
type StatusReport struct {
    // ... existing fields unchanged ...
    FailureEvents []FailureEvent `json:"failureEvents,omitempty"` // NEW
}
```

### 5.5 Incident

```go
// controller/internal/status.go (new)
type Incident struct {
    ID             string       `json:"id"`          // "<host>/<model>/<class>"
    Host           string       `json:"host"`
    ModelName      string       `json:"modelName"`
    Class          string       `json:"class"`
    Criticality    int          `json:"criticality"`
    OpenedAt       time.Time    `json:"openedAt"`
    LastUpdatedAt  time.Time    `json:"lastUpdatedAt"`
    FailureEvent   FailureEvent `json:"failureEvent"` // most recent
    Yanked         bool         `json:"yanked"`
    YankedAt       *time.Time   `json:"yankedAt,omitempty"`
    GitHubIssueURL string       `json:"githubIssueUrl,omitempty"`
    GitHubIssueNum int          `json:"githubIssueNum,omitempty"`
    Resolved       bool         `json:"resolved"`
    ResolvedAt     *time.Time   `json:"resolvedAt,omitempty"`
}
```

### 5.6 GpuHostStatus Extension

```go
// controller/internal/types.go (extend existing)
type GpuHostStatus struct {
    // ... existing fields unchanged ...
    Incidents []IncidentRef `json:"incidents,omitempty"` // NEW
}

type IncidentRef struct {
    ID             string `json:"id"`
    Class          string `json:"class"`
    ModelName      string `json:"modelName"`
    Yanked         bool   `json:"yanked,omitempty"`
    GitHubIssueURL string `json:"githubIssueUrl,omitempty"`
}
```

### 5.7 CRD Extensions (`controller/deploy/crd.yaml`)

Under `spec.properties.models.items.properties` add:
```yaml
criticality:
  type: integer
  enum: [1, 2, 3]
  description: "Model criticality tier: 1=embedding (highest), 2=inference, 3=experimental."
```

Under `status.properties` add:
```yaml
incidents:
  type: array
  description: "Active or recently resolved escalated incidents for this host."
  items:
    type: object
    properties:
      id:
        type: string
      class:
        type: string
      modelName:
        type: string
      yanked:
        type: boolean
      githubIssueUrl:
        type: string
```

---

## 6. Detection Philosophy

### 6.1 Normal Churn Baseline — Do NOT Over-Trigger

A container crash followed by a successful restart that then runs **healthy for at least the stabilization window** (default: `2 × reportInterval = 60s`) is **normal operational churn**. This event:

- Is **not** a problem.
- Is **not** escalated. No Incident is created. No GitHub issue is raised.
- Is recorded for rate/trend history only (raw event count in `FailureEvent.RestartCount`).

The classifier must explicitly set `IsNormalChurn = true` and return early for this case.

**Example:** watcher restarts a vLLM container due to an unhealthy health check. The container comes back up and reports `healthy` on the next two 30s report cycles. This is normal. The FC is silent.

### 6.2 Real Problem Signals

There are exactly two real triggers:

**Signal A — Sustained Crash-Loop (loop-without-recovery)**

The container has restarted N times within the window AND has NOT held a healthy state for at least the stabilization window between restarts. The classifier sets `IsSustainedLoop = true`. The hard ceiling is `maxRestartsInWindow = 3` within `restartWindow = 10m` (existing bounds, unchanged). After the ceiling is exhausted the model is already marked `degraded` by the existing logic; the new path adds the `FailureEvent` and triggers escalation/yank logic.

A "hundreds of restarts" scenario (the W6800 archetype) is the extreme manifestation of this signal — it means the existing `maxRestartsInWindow` ceiling was not being acted on escalation-wise. After this feature, hitting the ceiling → FailureEvent + escalation/yank, immediately.

**Signal B — Collateral Impact (blast radius)**

A failing model's problems **cause or correlate with degradation in a co-located or dependent service**. This is an independent trigger. The FC acts on collateral impact WITHOUT waiting for the failing model to hit its own ceiling, and acts faster when the harmed service is higher-tier.

Detection: on each `FailureEventProcessor.Process` call, if model A is in a failure state (any `class`, including still within its restart window) AND a co-located model B on the same host has transitioned to `degraded` or `failed` state within the same 10-minute window, and model A's `Criticality >= model B's Criticality` (A is lower or equal tier, B is higher or equal tier), this is classified as collateral impact.

The classifier sets `IsCollateralImpactSignal = true` on the failing model's FailureEvent and populates `CollidingModelName`.

**W6800 archetype, re-examined:**
- `fast-inference` (Tier 2) was crashing and consuming disk/VRAM.
- `embed-w6800` (Tier 1) degraded.
- Collateral impact: Tier 1 harmed by Tier 2 failure → immediate yank of `fast-inference`, no ceiling wait required.

### 6.3 Threshold Framing

Raw restart count is a backstop ceiling, not the primary trigger. Primary signals are:
1. Loop-without-recovery (pattern, not count).
2. Collateral impact on a co-located service (blast radius, especially cross-tier).

~10 restarts with full recovery between each is normal churn. ~10 restarts in a tight window without recovery is Signal A.

---

## 7. Failure-Class → Action Table

Rules are evaluated in priority order. First matching rule fires. Every action is bounded. Bounds exhausted → escalate.

| # | Class / Trigger | Condition | Action | Bound | Anchor | Guardrail Class |
|---|----------------|-----------|--------|-------|--------|-----------------|
| R1 | `disk_full` | `hostDiskPct >= 90` OR logs contain `"no space left on device"` OR exit code 28 | Prune build cache + dangling images: `docker system prune --filter=until=24h --volumes=false` | **1 attempt** then escalate | #304 | ALLOWED: prune build cache, dangling, re-pullable images. FORBIDDEN: volumes, model weights, `--all` flag. |
| R2 | `oom_vram` | VRAM anomaly confirmed: 3 consecutive samples > 1.5× `reservedMemoryGB` | Restart (`Remove` + `Ensure`) | **2 restarts / 30m** (existing SelfHealer — unchanged) | #309 | ALLOWED: restart. |
| R3 | `image_pull_failed` | `pullFailedAttempts` incremented; Ensure() error is not `GPUInitError` | Exponential backoff retry (existing) | **`maxPullAttempts = 10`** then escalate | #910 gap | ALLOWED: retry. Must not prune the image being pulled. |
| R4 | `healthcheck_unhealthy` / `crash_loop` (normal range) | Restart count < `maxRestartsInWindow` AND container previously recovered (not sustained loop) | Restart (existing `checkHealth` path) | **3 restarts / 10m** (existing — unchanged) | Existing | ALLOWED: restart. |
| R5 | `gpu_init_failed` | `errors.As(err, &gpuErr)` in `recordEnsureFailure` | Mark degraded immediately (existing) + capture `FailureEvent` | **No retry** | #304 | FORBIDDEN: any prune. Escalate immediately. |
| R6 | stale `degradedContainers` (controller-side) | Container in CRD `degradedContainers` but: (a) absent from current spec, OR (b) last StatusReport shows it healthy | Remove from `GpuHostStatus.DegradedContainers` on next patch | Fires on every status receipt — no bound | #310 | ALLOWED: clear status entry only. No host-side action. |
| R7 | Heartbeat stale (controller-side) | `time.Since(LastHeartbeat) > heartbeatStaleThreshold (default: 90s)` | Transition host to `degraded`; after `heartbeatFailThreshold (default: 10m)` mark `failed` + Incident | Stale→degraded at 90s; degraded→failed+escalate at 10m | #312 | FORBIDDEN: any host-side action from controller. Controller records only; watcher recovers. |
| R8 | Crash-loop ceiling exhausted | Sustained loop: `restartCount >= maxRestartsInWindow` AND `IsSustainedLoop == true` (no recovery between restarts) | **Yank**: remove model from GPUHost spec → watcher reconciles container away. Escalate (GH issue). | One yank per model per session | W6800 archetype | ALLOWED: remove model from desired spec. Image is re-pullable; no data destroyed. Re-admission requires explicit operator action. |
| R9 | Collateral impact (controller-side) | `IsCollateralImpactSignal == true` AND harmed model tier < failing model tier (or same tier, failing model hits ceiling faster) | **Immediate yank** of the failing (lower-tier) model. Escalate. | Fires immediately on detection, no count required | W6800 archetype (Tier-2 inference → Tier-1 embedder degraded) | ALLOWED: remove failing model from spec. FORBIDDEN: touching the healthy co-located model. |
| R10 | `unreachable_but_healthy` | Container passes Docker HEALTHCHECK but TCP probe fails | Capture `FailureEvent`; create Incident. No self-correct action until #308 (NetworkPolicy) lands. | N/A | #308 | Placeholder; no autonomous action yet. |

### Yank Mechanism (R8, R9)

The yank reuses the **authoritative desired-state removal path** — the same mechanism proven on `precision` (`fast-inference`) and `leviathan` (`embed-7900xt`):

1. Controller calls `K8sClient.PatchGpuHostSpec(ctx, hostname, modelName)`: removes the named model from `spec.models[]` on the GPUHost CRD.
2. Watcher detects `PolicyVersion` change on next poll (60s max).
3. `applyPolicy()` calls `docker.Remove` for the model absent from desired state.
4. Container is stopped and removed. No data destroyed — image is re-pullable from the registry; model weights are on the NFS mount.

The CorrectiveAction record for a yank is:
```
Action: "yanked_from_spec"
Detail: "model removed from GPUHost.spec.models by FC (reason: <escalation reason>). Re-admission requires explicit operator action."
```

**GUARDRAIL (applies to all rules):**

> Self-correct may take ANY action EXCEPT data-destroying ones.
>
> ALLOWED: restart/recreate, prune build-cache/dangling/re-pullable images, clear stale status entries, bounded retry, yank/remove a model from desired state (container removal + spec patch), regenerate NetworkPolicy, scale, mark degraded/failed.
>
> Yanking a model is explicitly ALLOWED because: the container is removed (ephemeral), but the image is re-pullable from the registry and the model weights are on the NFS mount (persistent, untouched). No data is destroyed. The action is reversible by adding the model back to spec.
>
> FORBIDDEN (→ escalate immediately): delete named Docker volumes, destructive DB operations, remove images that are the sole local copy of a non-re-pullable artifact, any operation that loses model weights or persistent NFS data. The `docker system prune` rule (R1) explicitly uses `--volumes=false` and never uses `--all`.
>
> Every action taken is appended to `FailureEvent.CorrectiveActions` before the next `PostStatus`. This record is the reason the engine never loops — a bound check reads `len(CorrectiveActions)`.

---

## 8. FailureClassifier Design

**File:** `watcher/internal/classifier.go` (new)

The classifier is a pure function: `Classify(evidence ClassifierInput) FailureClass`. Called from `recordEnsureFailure`, `checkHealth`, and `sampleAnomaly`. No external process calls.

```go
type ClassifierInput struct {
    ExitCode         int
    ExitSignal       string
    Logs             string    // last 20 lines
    DiskPct          float64
    VRAMUsedMB       int
    VRAMFreeMB       int
    GPUInitError     bool
    PullFailed       bool
    RestartCount     int
    LastHealthyAt    *time.Time
    NowHealthy       bool      // container currently healthy (for normal-churn check)
}
```

**Classification order** (first matching rule wins):

1. `NowHealthy == true` AND `RestartCount >= 1` AND `time.Since(*LastHealthyAt) < 2*reportInterval` → `IsNormalChurn = true`; return without setting a failure class.
2. `GPUInitError == true` → `gpu_init_failed`
3. `DiskPct >= 90` OR logs contain `"no space left on device"` OR `ExitCode == 28` → `disk_full`
4. `PullFailed == true` → `image_pull_failed`
5. VRAM anomaly (SelfHealer confirmed) → `oom_vram`
6. `ExitCode == 137` (SIGKILL) AND VRAM anomaly signal → `oom_vram`
7. `ExitCode == 137` without VRAM signal → `oom_host`
8. `ExitCode != 0 && ExitCode != 137` → `exit_nonzero`
9. Restart count >= 2 within window WITHOUT recovery → `crash_loop` (+ `IsSustainedLoop = true`)
10. Probe failed → `healthcheck_unhealthy`
11. Default → `unknown`

The **sustained-loop check** (step 9) requires: `RestartCount >= 2` AND `LastHealthyAt == nil OR time.Since(*LastHealthyAt) > restartWindow`. This distinguishes the restart-and-recovered case from the loop-without-recovery case.

---

## 9. SelfCorrectEngine Design

**File:** `watcher/internal/selfcorrect.go` (new)

```go
type SelfCorrectEngine struct {
    hostname        string
    docker          DockerClient
    maxPullAttempts int  // default 10
    stabilizationWindow time.Duration // default 2*reportInterval = 60s
}

// Apply decides and takes the bounded watcher-side self-correct action.
// Returns the action taken (for recording) and whether controller-side escalation is needed.
// For crash-loop ceiling and collateral impact, only marks NeedsEscalation — the controller owns the yank path.
func (e *SelfCorrectEngine) Apply(
    ctx context.Context,
    class FailureClass,
    event *FailureEvent,
    spec ModelSpec,
) (actionTaken string, escalate bool)
```

The engine checks `len(event.CorrectiveActions)` for the same action type before acting — this is the loop-prevention check. It never acts without first appending the action record.

**Yank is a controller-side action.** The watcher does not directly patch the CRD. When `NeedsEscalation = true` and the escalation reason includes `crash_loop_ceiling` or `collateral_impact`, the controller's `FailureEventProcessor` owns the yank decision.

---

## 10. Controller: FailureEventProcessor

**File:** `controller/internal/handlers.go` (extend `postStatus`)

On receipt of a `StatusReport`:

1. **R6 (stale-degraded clear):** for each entry in `report.DegradedContainers`, check current policy spec. If model absent → clear. If model present and reported healthy → clear.

2. **Collateral-impact scan:** for each host, if any model A is in failure state and any model B on the same host is in `degraded` or `failed` state, and `A.Criticality >= B.Criticality` (A no more critical than B), classify as collateral impact, set `IsCollateralImpactSignal = true` on A's FailureEvent, populate `CollidingModelName`.

3. **Per-FailureEvent processing:** for each `FailureEvent` with `NeedsEscalation == true` OR `IsSustainedLoop == true` OR `IsCollateralImpactSignal == true`:
   - Determine if yank is warranted (R8: sustained loop at ceiling; R9: collateral impact on higher-tier model).
   - If yank: call `K8sClient.PatchGpuHostSpec(ctx, hostname, modelName)` to remove model from spec. Record `WasYanked = true`.
   - Call `IncidentStore.Record(incident)`.
   - Call `PatchGpuHostStatus` with updated `incidents[]`.
   - Dispatch `GitHubIssueEscalator.Raise(incident)` in a goroutine.

4. **R7 (heartbeat gate):** separate background goroutine, checks all hosts every 30s. Stale → degraded marker. Failed → Incident + GH issue. Controller never sends commands to the watcher.

---

## 11. IncidentStore

**File:** `controller/internal/store.go` (extend) and `controller/internal/status.go` (new types)

```go
// Store (extend)
incidents map[string]Incident  // keyed by "<host>/<model>/<class>"

func (s *Store) AddIncident(i Incident) (isNew bool)
func (s *Store) GetIncident(id string) (Incident, bool)
func (s *Store) ListIncidents(resolved bool) []Incident
func (s *Store) ResolveIncident(id string)
```

**Auto-resolution:** when the controller receives a `StatusReport` where the (host, model) combination no longer appears in `FailureEvents` with `NeedsEscalation` and the container is healthy, the incident is marked resolved. Resolved incidents are retained for 7 days. Memory bound: 500 active incidents (ring buffer evicting oldest resolved first).

---

## 12. GitHubIssueEscalator

**File:** `controller/internal/escalator.go` (new)

**Authentication:** Claude PAT from Infisical (`homelab/prod/coordinator/gh-pat-claude`). Read at controller startup, held in memory. Never in config files or env vars in plaintext.

**Deduplication:** query `petersimmons1972/aifleet` for open issues with title matching `fc-incident: <host>/<model>/<class>`. If found: add comment with updated event JSON. If not found: create new issue.

**Issue format:**

```
Title: fc-incident: <host>/<model>/<class> — <human-readable summary>

Labels: fc-incident, severity/<label>, agent/codex/queued (or agent/hermes)

Body:
## FC Incident — <class> on <host>/<model>

**Opened:** <timestamp>
**Class:** <FailureClass>
**Host/Model:** <host> / <model> (Criticality Tier <N>)
**Yanked:** <yes/no>

### Failure Evidence
- Exit code: <n>
- Probe failed: <type> — <error>
- Last log lines:
  ```
  <last 20 lines>
  ```
- VRAM used/free: <X>/<Y> MB
- Host disk: <Z>%
- Co-located model affected: <name or N/A>

### Corrective Actions Tried
| Attempt | Action | Outcome |
|---------|--------|---------|
| <1>     | <action> | <outcome> |

### Why Escalated
<EscalationReason>

### Re-Admission Note
[If yanked] This model has been removed from GPUHost spec.
Re-admission requires: (1) root-cause fix confirmed, (2) explicit operator spec patch.
Do NOT add back to spec as an automated step.

### Diagnostic Payload (machine-readable)
<FailureEvent JSON>
```

**Severity label assignment:**
- `disk_full`, `oom_vram`, `crash_loop`, `collateral_impact`, `heartbeat_stale` → `severity/blocker`
- `gpu_init_failed` → `severity/blocker`
- `image_pull_failed` (after max attempts) → `severity/serious`
- `exit_nonzero`, `unknown` → `severity/nice-to-have`

**Agent label assignment:**
- Repair-type incidents (disk, pull, crash, gpu) → `agent/codex/queued`
- Observability-type (heartbeat, stale-degraded that needs human review) → `agent/hermes`

---

## 13. New API Endpoints

Add to `controller/internal/handlers.go`:

```
GET /status/incidents               — list all active incidents
GET /status/incidents/{id}          — get one incident by ID
POST /status/incidents/{id}/resolve — manually resolve an incident
```

---

## 14. Error Handling

- **GitHub issue creation failure:** `slog.Warn`. Incident is already in the store and CRD. The escalator retries on the next `PostStatus` tick for the same incident (idempotent dedup).
- **Classifier insufficient evidence:** returns `FailureClassUnknown`. No self-correct rule; escalate immediately.
- **R1 prune fails:** `slog.Error`, record action with `outcome: "failed"`, escalate. Never retry prune on the same FailureEvent.
- **Yank (K8s spec patch) fails:** `slog.Error`, record action with `outcome: "failed"`, escalate with additional reason `"yank_failed"`. Do not retry the yank automatically; surface via Incident and GH issue.
- **Heartbeat goroutine panics:** recover and restart with 5s delay. Goroutine liveness surfaced via `/health`.
- **IncidentStore overflow:** 500 incident cap. Evict oldest resolved first; oldest unresolved last with `slog.Error`.

---

## 15. Testing Approach

### Unit Tests (per-rule)

| Test | Rule | Input | Expected Outcome |
|------|------|-------|------------------|
| `TestClassifier_NormalChurn` | baseline | container healthy, 1 restart, `LastHealthyAt` < 60s ago | `IsNormalChurn = true`, no escalation, no GH issue |
| `TestClassifier_SustainedLoop` | R8 | 3 restarts in 10m, no recovery interval | `IsSustainedLoop = true`, `class = crash_loop` |
| `TestClassifier_DiskFull` | R1 | `DiskPct=95, logs="no space left on device"` | `class = disk_full`, prune action taken |
| `TestClassifier_DiskFull_ExitCode28` | R1 | `ExitCode=28` | `class = disk_full` |
| `TestClassifier_GPUInit_TakesPrecedence` | R5 | `GPUInitError=true, DiskPct=95` | `class = gpu_init_failed` (GPU init wins) |
| `TestPullFailed_BoundExhausted` | R3 | `pullFailedAttempts=10` | `NeedsEscalation=true`, no further increment |
| `TestStaleDegraded_ModelRemoved` | R6 | no spec entry for `ai-fleet-fast-inference` | cleared from next `GpuHostStatus.DegradedContainers` |
| `TestStaleDegraded_ContainerHealthy` | R6 | container appears healthy in report | cleared |
| `TestHeartbeatStale` | R7 | `LastHeartbeat = 95s ago` | host marked degraded |
| `TestHeartbeatFailed` | R7 | `LastHeartbeat = 11m ago` | Incident created, GH issue triggered |
| `TestCollateralImpact_Tier2HarmsTier1` | R9 | Tier-2 model failing, Tier-1 co-located model degraded | immediate yank of Tier-2, Incident, GH issue |
| `TestCollateralImpact_NoYankHigherTier` | R9 | Tier-1 model failing, Tier-2 co-located model degraded | no yank (don't sacrifice higher tier for lower) |
| `TestYankMechanism` | R8/R9 | yank triggered | `K8sClient.PatchGpuHostSpec` called; `WasYanked=true` in Incident |
| `TestIncidentDedup` | escalator | second `NeedsEscalation` for same (host, model, class) | comment added to existing issue, no new issue |
| `TestGuardrail_NoVolumePrune` | R1 | disk_full, container has named volumes | prune called with `--volumes=false`; confirmed via docker mock |

### Integration Tests

- `TestFullLifecycle_DiskFull_W6800`: simulates #304 chain using fake DockerClient. Asserts: `disk_full` classified, prune fires once, second failure escalates, Incident created.
- `TestFullLifecycle_PullFailedCap`: 10 fake `Ensure()` failures → watcher stops retrying, escalates, controller creates GH issue stub.
- `TestFullLifecycle_HeartbeatStale_Oblivion`: fake host stops reporting → controller degrades at T+90s, creates Incident + GH issue at T+10m.
- `TestFullLifecycle_CollateralImpact_W6800Archetype`: simulates W6800 crash-loop with co-located Tier-1 embedder degrading → Tier-2 inference model yanked, Incident + GH issue carry correct labels.
- `TestFullLifecycle_NormalChurn_Silent`: 2 restarts each followed by healthy recovery → no Incident, no GH issue, no escalation.

### Existing Tests

No existing test names change. `handlers_test.go:TestPostStatus*` gains cases for `FailureEvents[]` field. `watcher_test.go:TestCheckHealth*` gains cases for classifier wiring.

---

## 16. Rollout

### Phase 1 — Watcher: Classifier + Evidence Capture (read-only)

- Add `FailureEvent`, `FailureClass`, `Criticality` to `watcher/internal/types.go`
- Add `FailureClassifier` (`classifier.go`)
- Wire classifier into `recordEnsureFailure`, `checkHealth`, `sampleAnomaly`
- Extend `StatusReport.FailureEvents` and populate in `report()`
- Controller receives `FailureEvents` and logs them (no processing yet)
- **Validation:** observe enriched payloads; confirm classifications match live degraded state on precision and leviathan

### Phase 2 — Watcher: Self-Correct Actions (R1, R3)

- Add `SelfCorrectEngine` (`selfcorrect.go`)
- Wire R1 (disk_full prune) and R3 (pullFailed cap at 10 attempts)
- **Validation:** reproduce #304 scenario; confirm prune fires once, escalation on second failure

### Phase 3 — Controller: Incident Store + Stale-Clear + Heartbeat Gate

- Add `IncidentStore` to `store.go`
- Add R6 (stale-degraded clear) in `postStatus`
- Add R7 (heartbeat gate) background goroutine
- Extend `GpuHostStatus` and CRD schema
- Add `/status/incidents` endpoints
- **Validation:** confirm #310 fix (removed model → cleared within 30s); confirm #312 fix (stop watcher on oblivion → degraded→failed at correct intervals)

### Phase 4 — Controller: Yank (R8, R9) + GitHub Escalation

- Add `GitHubIssueEscalator` (`escalator.go`)
- Add yank path (`K8sClient.PatchGpuHostSpec`)
- Wire collateral-impact detection and crash-loop ceiling → yank + escalate
- Add `Criticality` field to GPUHost spec YAML for precision, leviathan, oblivion
- **Validation:** trigger max-attempts crash-loop in staging → confirm yank fires, exactly one GH issue created. Trigger collateral-impact scenario (Tier-2 failing, Tier-1 degraded) → confirm Tier-2 yanked, Tier-1 recovers.

### Phase 5 — CRD + kubectl

- Apply updated `controller/deploy/crd.yaml` with `status.incidents[]` and `spec.models[].criticality`
- Confirm `kubectl get gpuhost` shows `Incidents` printer column

---

## 17. Open Questions

| # | Question | Assumed Default | Impact if Wrong |
|---|----------|-----------------|-----------------|
| OQ1 | `heartbeatStaleThreshold`: 3× reportInterval = 90s. Should it be 5× (150s) to avoid false positives on a watcher restart? | 90s | A watcher restart (~5–10s) shouldn't trip 90s. 150s is safer if watcher restarts take longer on some hosts. |
| OQ2 | `heartbeatFailThreshold`: 10m before Incident. Right balance between noise and the 6h oblivion gap? | 10m | At 10m, a transient network blip resolves well before the threshold. Seems correct. |
| OQ3 | `maxPullAttempts = 10`. oblivion pulls 30+ GB models — 10 attempts × 5m = 50min before escalation. Is this sufficient or should oblivion get a per-host override? | 10 (global); per-host override as follow-on | A large model legitimately needing 20+ attempts is worth addressing in a follow-on. |
| OQ4 | Stabilization window for normal-churn baseline: `2 × reportInterval = 60s`. Is 60s enough to confirm "really healthy" after a restart? | 60s (2 report cycles) | A model that takes 10m to load (vLLM) might report healthy only after the load finishes. The existing `vllmReadinessTimeout` handles this — after that timeout the container is considered ready, and the stabilization window applies from that point. |
| OQ5 | Collateral-impact detection: scan all co-located models on the same host, or only same-GPU models? | Same host (all models on the same physical host) | On precision the W6800 and MI-50 share one host record. Both GPU domains share disk. Host-level scan is the correct scope. |
| OQ6 | Should `collateral_impact` always trigger an immediate yank, or should there be a brief confirmation window (e.g., 2 report cycles) to avoid acting on transient co-degradation? | Immediate yank when Tier-1 is harmed by Tier-2 failure; 2-cycle confirmation window when same-tier | Tier-1 harm is high-stakes enough to justify immediate action. Same-tier contention benefits from a brief confirmation to filter transients. |
| OQ7 | Incident dedup key: `<host>/<model>/<class>`. Should yanked incidents use a separate dedup bucket so re-admission creates a fresh issue rather than commenting on the old one? | Separate dedup bucket for yanked incidents (`<host>/<model>/<class>/yanked`) | Makes the re-admission failure audit trail cleaner. Low risk. |
| OQ8 | Should the GH issue escalator use `agent/codex/queued` for yank incidents immediately, or require a `decision/needs-founder` label first since re-admission is a founder call? | `decision/needs-founder` on yank incidents; Codex handles the diagnostic repair work only | Yanking is an FC action; re-admitting is a founder/operator call. The issue should signal both: Codex diagnoses, founder approves re-admission. |

---

## 18. Host Roles and Human-Priority Overlay

### 18.1 Host Role Classification

Not all GPU hosts are equivalent. The FC must classify hosts by role:

| Role | `hostRole` value | Description | Fleet usage |
|------|-----------------|-------------|-------------|
| **Fleet host** | `fleet` | Dedicated GPU compute: precision (W6800 + MI-50), oblivion (Grace-Blackwell) | Fleet-exclusive; full-time fleet workloads |
| **Workstation** | `workstation` | Human desktop with spare GPU capacity: leviathan (RX 7900 XT, 4-monitor desktop, also where the coordinating Claude runs) | Fleet is a GUEST — capacity is borrowable but preemptible by the human |

**Design principle:** Critical Tier-1 work (embedders) belongs on **fleet hosts**, not on workstations. Workstations host fleet work only as borrowable/yieldable guests. The human owns the workstation; the fleet borrows spare capacity.

Add `hostRole string` (`"workstation"` | `"fleet"`, default `"fleet"`) to the `GPUHostSpec` in both `watcher/internal/types.go`, `controller/internal/types.go`, and the CRD spec schema.

### 18.2 Human-Priority Overlay (Highest, Host-Scoped, Dynamic)

**The human on their workstation is the highest-priority consumer, above all criticality tiers.** When the human needs their workstation, desktop responsiveness preempts fleet workloads on that host. This is a distinct overlay that sits above the criticality-tier system:

- **Criticality tiers** govern contention between fleet workloads on **dedicated fleet hosts**.
- **Human-priority overlay** governs **workstation hosts** — the human wins, fleet yields.

The overlay is **context-sensitive and dynamic**: normally the fleet may use a workstation's spare capacity freely. The instant the human signals need (important Zoom call, rendering, heavy local compute), the bar shifts immediately.

### 18.3 Explicit Human Signal — Top-Priority Trigger

"Human says desktop is slow" / "I need my workstation" is an **explicit human-in-the-loop input** to the FC. It is distinct from automated detection (unlike all other triggers in this spec, this one requires a human action to initiate). It has the highest priority of any FC trigger.

**Input channel:** the human sends a signal via the FC CLI or a dedicated endpoint:

```
# CLI (via ai-fleet CLI or direct call)
ai-fleet workstation-priority raise --host leviathan [--reason "zoom"]
ai-fleet workstation-priority clear --host leviathan

# HTTP API (controller)
POST /hosts/{hostname}/human-need      # raise — body: {"reason": "zoom"}
DELETE /hosts/{hostname}/human-need    # clear — need has passed
```

**Controller state:**

```go
// controller/internal/types.go (new)
type HumanNeedState struct {
    Active    bool      `json:"active"`
    RaisedAt  time.Time `json:"raisedAt,omitempty"`
    Reason    string    `json:"reason,omitempty"`
    ClearedAt *time.Time `json:"clearedAt,omitempty"`
}
```

Stored in-memory in the `Store` keyed by hostname. Persisted to CRD status so it survives controller restarts.

**Auto-expiry:** if a human-need signal is not cleared within `humanNeedAutoExpiry` (default: 4h), the controller clears it automatically and logs a warning. This prevents stale signals from permanently evicting fleet workloads.

### 18.4 Graduated Relief Ladder

When a human-need signal is raised for a workstation host, the FC applies the following ladder in order — least disruptive first, all within the no-data-destruction guardrail:

**Step 1 — Assess**
The controller checks the workstation's current status: container count, VRAM usage, CPU/mem load (from the last StatusReport resource snapshot), and active Olla request rate if available.

**Step 2a — Throttle/Rate-Limit (least disruptive)**
If a fleet model on the workstation is receiving heavy request load (Olla is dispatching many requests to it), the FC requests Olla to cap/slow dispatch to that backend. This sheds load without stopping the container.

Action type: `"throttle_olla_requests"` — a new self-correct action distinct from restart or yank.

Implementation: the FC calls the Olla admin API to reduce the weight or apply a rate cap on the affected route. This is reversible when the human-need signal is cleared.

**Step 2b — Pause Batch Work**
Pause or throttle any `engram-reembed` or other background batch containers on the workstation. These are the highest-resource non-interactive workloads and the most expendable.

Action type: `"pause_batch_workload"`.

**Step 2c — Migrate / Yank**
If Steps 2a–2b do not sufficiently relieve the workstation (assessed by subsequent resource snapshot: VRAM / mem still high), yank fleet models off the workstation using the authoritative removal path (same as R8/R9). The yanked models' spec patches are tagged with reason `"human_need"`.

Action type: `"yank_for_human_need"`.

**Step 3 — Restore after Clear**
When the human-need signal is cleared, restore workloads in reverse order:
1. Un-throttle Olla routes.
2. Resume batch work.
3. Re-add yanked models to spec (this is the one case where **automatic re-admission is permitted** — the yank was not a failure; it was a temporary preemption for a known, time-limited reason). Re-admission is gated on the signal being cleared explicitly by the human (not by auto-expiry).

The restore path is the only case in this spec where automatic re-admission is permitted. All failure-driven yanks (R8, R9) still require explicit operator action.

### 18.5 Connection to the Consumer-Health Model

The human is the **ultimate consumer**. "Desktop is slow" is their consumer-health signal — it reports perceived degradation of the workstation as a dependency of their work. This connects the consumer-health model (§19) to the human-priority overlay:

- Consumer-health signals come from automated consumers (engram-go) via `POST /consumer-health/`.
- The human signal comes via the explicit `POST /hosts/{hostname}/human-need` channel.
- Both are processed by `FailureEventProcessor`, but the human-need trigger fires the graduated-relief ladder (§18.4) immediately, not the generic incident path.

---

## 19. Two-Perspective Health Model: Provider vs. Consumer (the §18 overlay applies; this section covers automated consumers)

### 18.1 The Divergence Problem

The W6800/MI-50 session (2026-05-29) exposed a gap that raw container health cannot close: **a service can be provider-healthy while consumer-degraded.** The embedders' `/health` endpoint returned 200 throughout the incident. What the provider health surface did not show: engram-go's embed success rate was falling, recall was silently falling back to BM25, and the MI-50 embed service was saturated with 500ms timeouts (#917). From the provider's perspective, everything was fine. From the consumer's perspective, recall quality was degraded.

This is a distinct failure mode, not addressed by restarts, not addressed by heartbeat gating. It requires a second health perspective.

### 18.2 The Two Perspectives

**Perspective 1 — Provider Health** (what the FC already tracks)
- Container running state (Docker)
- Docker HEALTHCHECK / TCP probe pass/fail
- VRAM usage vs. declared
- Watcher heartbeat freshness

**Perspective 2 — Consumer Health** (what this section adds)
Consumer health is the dependency's health as perceived by the consuming service. For Tier-1 embedding services the primary consumer is engram-go. Consumer health signals, sourced from engram-go (see issues #912 and #917):
- Embed success rate (calls / successes / timeouts per window)
- Recall embed-timeout rate
- BM25-fallback rate (proxy for embed unavailability: high rate = consumer is degrading)
- Circuit-breaker state (`open` = consumer has given up on the dependency)
- The `degraded.embed` flag in engram-go's internal health state

**Key principle:** for Tier-1 services, consumer-perceived health is **as important as** (and in user-facing terms, **more important than**) provider self-report. Consumer health is the user-facing truth.

### 18.3 Divergence as a First-Class Detection Signal

**Provider-healthy / consumer-degraded divergence** is a new `FailureClass`:

```go
FailureClassConsumerDegraded FailureClass = "consumer_degraded"
```

This class is NOT triggered by a container failure. It is triggered when:
- Provider health: all containers healthy (no `degraded`/`failed` state, no probe failures)
- Consumer health: consuming service reports degraded embed success rate, elevated timeout rate, or BM25-fallback > threshold, OR circuit-breaker is `open`

The divergence is a problem on its own. It must surface as an Incident even when every container in the fleet is green.

**Why it differs from other classes:**
- A restart will NOT fix it. The container is healthy; the problem is latency/contention/throughput mismatch (e.g. MI-50 saturation under load, #917).
- The correct self-correct actions are: investigate (escalate to GH issue), potentially rebalance load across available embedders (shift weight in Olla routes toward the non-saturated embedder), NOT restart the provider.
- It must not be silently ignored because the provider is green.

### 18.4 Consumer-Health Ingestion Interface

**Design:** the FC defines a lightweight consumer-health ingestion endpoint. Consumers (engram-go) POST their perceived dependency health on a periodic basis (suggested: same 30s interval as watcher status).

```
POST /consumer-health/{consumer-name}
Body: ConsumerHealthReport
```

```go
// controller/internal/types.go (new)
type ConsumerHealthReport struct {
    ConsumerName  string                      `json:"consumerName"`  // e.g. "engram-go"
    ReportedAt    time.Time                   `json:"reportedAt"`
    Dependencies  []DependencyHealthObservation `json:"dependencies"`
}

type DependencyHealthObservation struct {
    DependencyName  string  `json:"dependencyName"` // matches a model name in the fleet, e.g. "embed-w6800"
    Host            string  `json:"host"`
    SuccessRate     float64 `json:"successRate"`     // 0.0–1.0 over the last window
    TimeoutRatePct  float64 `json:"timeoutRatePct"`  // percentage of calls that timed out
    FallbackRatePct float64 `json:"fallbackRatePct"` // e.g. BM25-fallback rate
    CircuitState    string  `json:"circuitState"`    // "closed" | "half-open" | "open"
    DegradedFlag    bool    `json:"degradedFlag"`    // consumer's own assessment (e.g. degraded.embed)
}
```

**engram-go integration (MVP path):** engram-go already has the `degraded.embed` flag and the timeout/fallback counters per #912/#917. MVP is: engram-go calls `POST /consumer-health/engram-go` with the current values. The FC does not need engram-go to instrument new metrics — it needs engram-go to expose existing ones to the FC ingestion endpoint.

**Controller-side correlation (new method in `FailureEventProcessor`):**
On each consumer-health report ingestion, for each `DependencyHealthObservation`:
1. Look up the named model in the current fleet state.
2. Check provider health: is the provider healthy?
3. If provider healthy AND (`SuccessRate < consumerDegradedThreshold` OR `CircuitState == "open"` OR `DegradedFlag == true`): create a `FailureEvent` with `Class = consumer_degraded` and `IsCollateralImpactSignal = false` (this is not a provider failure causing consumer harm; it is a provider-healthy/consumer-degraded divergence).
4. Process via `FailureEventProcessor`: no watcher-side action, escalate directly to Incident + GH issue.

**Thresholds (configurable, suggested defaults):**
- `consumerDegradedThreshold` (success rate floor): 0.95 (95%)
- BM25-fallback threshold: 10% (if 10% of recalls are falling back, the embed service is not keeping up)

### 18.5 Rule Table Addition (R11)

Add to the failure-class → action table:

| R11 | `consumer_degraded` | Provider healthy + consumer reports degraded embed success rate / open circuit-breaker / BM25-fallback > threshold | Escalate immediately. No restart. Optional: rebalance Olla routes toward healthier embedder if one exists (Tier-1 protection). | No count bound — fires on first confirmed divergence | #917 MI-50 saturation / BM25 silent fallback | FORBIDDEN: restart the provider. The provider is green; restarting adds noise without fixing the saturation. Escalate + investigate. |
| R12 | Human-need signal on a workstation host | `POST /hosts/{hostname}/human-need` received | Graduated relief ladder (§18.4): (a) throttle Olla request dispatch → (b) pause batch workloads → (c) yank fleet models. Auto-restore when signal cleared. | 1 cycle of assessment before action; auto-expire after 4h | Leviathan desktop contention | ALLOWED: throttle, pause, yank for preemption. Re-admission on clear is AUTOMATIC (not a failure-driven yank). |

### 18.6 Data Model Addition

Add `ConsumerHealthObservations` to the controller's Store:

```go
// controller/internal/store.go
consumerHealth map[string]ConsumerHealthReport  // keyed by consumerName
```

And add the ingestion handler:

```
POST /consumer-health/{consumer-name} → s.postConsumerHealth
```

### 18.7 Rollout Phase for Consumer Health

**Phase 5 (new, after Phase 4):** Consumer health ingestion.
- Add `ConsumerHealthReport` type and `/consumer-health/` endpoint to controller.
- Implement divergence detection in `FailureEventProcessor`.
- Wire engram-go to call `POST /consumer-health/engram-go` every 30s using existing `degraded.embed` flag and timeout counters.
- **Validation:** simulate MI-50 saturation (artificially throttle embed service); confirm FC creates `consumer_degraded` Incident even though provider `/health` is green; confirm GH issue carries BM25-fallback rate and timeout rate.

---

## Appendix A — Relationship to Existing Code Paths

| Existing code | How this feature touches it |
|---------------|----------------------------|
| `watcher.go:recordEnsureFailure` | Extended: calls `FailureClassifier.Classify`, populates `FailureEvent` |
| `watcher.go:reconcileDrift` | Extended: R3 pullFailed cap check; skips increment when `pullFailedAttempts >= maxPullAttempts` |
| `watcher.go:checkHealth` | Extended: on restart-limit path, attaches `FailureEvent` to degraded record; normal-churn detection added |
| `watcher.go:report` | Extended: includes `FailureEvents` in `StatusReport` |
| `selfheal.go:SelfHealer` | Unchanged. Classifier reads `oom_vram` signal via existing `IsFailed()` / `Events()` |
| `policy.go:PostStatus` | Unchanged (payload enriched by caller) |
| `handlers.go:postStatus` | Extended: calls `FailureEventProcessor.Process` after existing side-effects |
| `handlers.go:recordStatusAnomalies` | Unchanged. `FailureEventProcessor` is a parallel path, not a replacement |
| `store.go:Store` | Extended: adds `incidents map[string]Incident` and associated methods |
| `status.go:Anomaly` | Unchanged. `Incident` is a new, richer type alongside `Anomaly` |
| `types.go:ModelSpec` (both modules) | Extended: adds `Criticality int` field (omitempty, backward-compatible) |
| `types.go:StatusReport` | Extended: adds `FailureEvents []FailureEvent` (omitempty, backward-compatible) |
| `types.go:GpuHostStatus` | Extended: adds `Incidents []IncidentRef` |
| `crd.yaml` | Extended: adds `status.incidents[]` and `spec.models[].criticality` |
