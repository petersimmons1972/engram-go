# Failure-Mode Standard (Universal)

**Scope:** All projects. Per-repo checklists INHERIT from this catalog and may append repo-specific checks.
**Process:** Every time a bug burns a cycle → append a catalog row + a check entry in the relevant class below.
**Per-repo pattern:** `<repo>/bin/checkin-lint.sh` (automates 🤖 items) + `<repo>/docs/CHECKIN-CHECKLIST.md` (inherits + extends).
**First seeded:** 2026-06-13 from the aifleet embedder-identity and poller-relocation sagas.

---

## Bug Classes

### A. Identity vs. Routing

A routing/serving label (alias, tag, selector, role suffix) leaked into a field that a downstream system persists and compares as a **stable identity** → false mismatch, blocked writes, or spurious reprocessing.

- 🤖 **No routing/role word encoded in a stable identity field.** Flag names containing `-live`, `-reembed`, `-bulk`, `-burst`, `-fast`, `-primary`, host names, ports, or role descriptors wherever they feed a persisted identity comparison.
- **The advertised name == the authoritative identity stored downstream.** If any system persists `<thing>_name` / `<thing>_id`, every producer must advertise the exact same string.
- **Same artifact ⇒ same identity.** Two endpoints/versions serving identical artifacts MUST advertise the identical identity. Different name for the same artifact = a future false mismatch.
- **Different artifact ⇒ deliberately different identity** (never reuse a name across distinct artifacts).
- **Role is expressed in routing, not in the name.** primary/bulk/fast/etc. = which endpoint/URL/queue a consumer targets, or an explicit routing field — never the artifact identity string.
- **No field carries two orthogonal meanings.** If one string is both the identity AND the routing selector, split them before merge.
- **A re-process/re-index operation fires ONLY on a real content-or-schema change or explicit force flag.** An identity rename or label change must short-circuit to no-op rather than triggering a mass re-process. Guard: check whether a genuine underlying change occurred before enqueueing work.

- 🤖 **Validate model name casing in ConfigMap matches exactly what the embedding backend registers**
### B. Install / Config Layout Integrity

A unit, manifest, or wrapper references a path that the installer does not actually produce — wrong filename, missing `.sh` extension, binary never vendored, or ordering race between `daemon-reload` and `enable`.

- 🤖 **Every `ExecStart`/`ExecStartPre`/wrapped path resolves to what the installer actually produces** (exact filename, including `.sh`-or-not, and exact location). Render with real substitutions; confirm the target exists post-install. Offline linters (`systemd-analyze verify`) cannot see runtime specifiers — verify against a staged install.
- **Every referenced binary/script is actually vendored AND installed.** Grep each unit's targets; confirm a source file exists in-repo and an install line places it at the expected path.
- **Wrappers that `exit 0` don't mask a failing child.** Any run/notify wrapper must propagate the real exit code.
- **`daemon-reload` before `enable`.** Enabling a unit before the daemon has reloaded the new unit file can self-disable a timer or cause stale-unit behavior. Sequence: write unit → `daemon-reload` → `enable` → `start`.

### C. Portability — No Machine/User Literals

Hardcoded user home paths, version-pinned tool paths, or specifier tokens used outside their valid expansion context break on any other user, machine, or upgrade.

- 🤖 **No `/home/<user>` literal in any unit/drop-in/installer/config.** Use `%h`, `__HOME__`, `__REPO_ROOT__`, or equivalent install-time tokens only. `rg -n '/home/[a-z]' .` must return zero hits.
- **Know where template specifiers expand.** `%h` expands in `ExecStart`, `ReadWritePaths`, etc. — NOT inside `Environment=` strings. Use install-time `sed` substitution tokens in `Environment=`.
- **No version-pinned absolute tool paths** (e.g. `~/.nvm/versions/node/vX.Y.Z/bin/...`). Resolve dynamically (`command -v`) or via install-time template — never pin a path that dies on the next tool upgrade.

### D. Fail-Loud — No Silent Masking

A missing dependency, empty result, failed child process, or degraded fallback is swallowed and reported as success — masking the failure for hours or weeks.

- **Critical/gating paths fail CLOSED, not open.** A missing dependency, empty result, or fallback must surface as a non-zero exit or an explicit error flag, never silently swallowed.
- **A `Result=success` that could mask a failed child is suspect.** Verify the leaf process actually ran and produced output.
- **Fallback/degraded modes must emit a warning-level signal,** not silently score as a normal result.

- 🤖 **Always use `set -e` at top of K8s Job scripts so any command failure propagates**
- 🤖 **Before releasing chunk claims or assuming reembed will pick up all chunks, verify the claim predicate covers the actual stuck state**
### E. Thermal / Resource-Role Safety

A change to naming, routing, or configuration removes or bypasses a safety pin that restricts sustained/bulk workloads to hardware that can handle them.

- **Resource-role pinning survives identity/routing changes.** Constrained devices (fanless, low-memory, shared CPU) must never receive sustained/bulk load. If a change alters how a consumer selects a resource, confirm the safety pin still holds by an independent selector (URL, label, affinity) that cannot be removed by a rename.
- **Bulk workloads target the correct resource by a selector that survives a name collapse.**
- **Sequence routing changes resource-safely:** assign the device/resource pin BEFORE collapsing whatever selector currently routes to it.

- 🤖 **Run per-table DELETEs in separate autocommit statements; scale the live service to 0 first**
- 🤖 **Verify embedding endpoint returns 200 on /v1/models before scaling reembed or releasing claims**
### F. Repo Hygiene / Canonical Home

Infra or code living in the wrong repo, look-alike stubs installed in place of the real artifact, or dual install paths creating ambiguity about which version is live.

- **Artifacts live in their owning repo.** Do not add infra to a repo whose stated purpose is something else.
- **One canonical artifact — no look-alike stubs.** No stub that can be installed as the real implementation; no dual install paths for the same service.
- 🤖 **`git remote -v` matches the intended repo before editing.** A session's cwd may belong to a different repo or worktree than the one you mean to change — verify before every cross-repo edit.

- 🤖 **Never TRUNCATE a shared-project table; use scoped DELETE or dump/restore only the target project rows**
### G. Deploy / Verify Discipline

Skipping staged verification before enabling a live artifact, or linting offline instead of against the live install output.

- **Prod deploys are staged and reversible.** Pattern: backup → stage with `NO_ENABLE` (or equivalent dry-run) → verify against live artifact → enable → auto-rollback on any verify miss.
- **Verify against the LIVE artifact, not just an offline lint.** Offline tools (`systemd-analyze`, schema linters) can pass an artifact that the live executor rejects. Staged install-layout checks catch what offline lints miss.
- **Prod deploys are founder-gated** (see `~/CLAUDE.md` §Cost Guardrails).

### H. Ephemeral Resource Leakage → Host Exhaustion (GLOBAL RISK)

**Universal directive (founder, 2026-06-14): any testing that creates ephemeral infrastructure MUST clean up after itself — Docker *and* Kubernetes, and any other ephemeral runtime.** The agent that created the resource owns its teardown; "the test passed" is not done until the resources it spun up are gone.

Ephemeral test/build/dev artifacts (containers, **anonymous volumes**, images, build cache; k8s test namespaces, PVCs, pods, jobs, LoadBalancers) accumulate on a **shared host/cluster** until they exhaust disk or memory and crash *unrelated* workloads. On the workstation this OOMs the **COSMIC desktop** (and any co-located containers) — a known failure mode. On a cluster, orphaned PVCs and test namespaces exhaust storage/quota and strand finalizers. The agent that ran the tests is responsible for the cleanup.

- **Clean up every Docker resource you create for a test/build.** After any `go test` with testcontainers, `docker build`, or ad-hoc `docker run`, remove the containers AND their **anonymous volumes**. Ryuk reaps containers but anonymous Postgres/data volumes routinely persist (observed: weeks of leaked volumes).
- **Clean up every Kubernetes resource you create for a test.** Tear down test namespaces (`kubectl delete ns <test-ns>` cascades pods/PVCs/jobs), and verify **PVCs are actually released** (PVCs can outlive their namespace's pods and pin backing storage; check `kubectl get pvc -A` for orphans, and watch for stuck `Terminating` finalizers). Prefer ephemeral namespaces per test run so teardown is a single delete. Never leave a test LoadBalancer/Ingress allocated. Same delta-only discipline: remove only what your run created, never blanket-delete a shared cluster's resources.
- 🤖 **Verify-after-test (delta-only):** snapshot `docker volume ls -qf dangling=true` before a run; after, remove **only the volumes your session created** (filter by `CreatedAt` = today/this session). Assert zero session-created dangling volumes remain.
- **NEVER global-force-prune a shared host.** `docker volume prune -f`, `docker system prune -a` on a multi-tenant box can destroy *other* workloads' detached data. Targeted `docker rm` / `docker volume rm <id>` of your own artifacts only. (The auto-mode classifier correctly blocks global prunes — do not work around it.)
- **Watch host headroom as the early signal.** `docker system df` reclaimable creeping into tens of GB, or dangling-volume count climbing, precedes the OOM. The desktop crashing is the *late* signal — by then it is already too late.
- **Long-running ≠ wanted.** A container "Up N days" may be stale/forgotten (e.g. a gateway that "shouldn't be running"); confirm intent before assuming it's load-bearing, and stop reversibly (`docker stop`, not `rm`).

---

## Failure-Mode Catalog

*Append a row every time a bug burns a cycle. Per-repo checklists reference these IDs.*

| ID    | Bug class                        | Concrete instance                                                                                             | Check that catches it                                                                      | Auto? |
|-------|----------------------------------|---------------------------------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------|-------|
| FM-01 | Identity vs. routing             | olla `--alias bge-m3-live/reembed` read as Engram identity → `embedder_mismatch`, `memory_store` blocked     | Grep aliases for role/routing words; confirm advertised name == persisted identity         | 🤖    |
| FM-02 | Identity vs. routing             | Role (live/bulk) encoded in model name                                                                        | Same as FM-01                                                                              | 🤖    |
| FM-03 | Identity vs. routing             | Model name doubled as identity AND routing selector                                                           | Split fields; confirm no field carries two orthogonal meanings                             | Judgment |
| FM-04 | Install layout                   | `ExecStart=…codex-poll.sh` vs installed binary named `codex-poll` → 203/EXEC                                 | Render ExecStart with real substitutions; diff against post-install listing                | 🤖    |
| FM-05 | Install layout + silent mask     | `codex-liveness-check` binary never vendored → silent exit-127 for weeks                                     | Grep all unit targets; confirm source + install line exist in-repo                        | 🤖    |
| FM-06 | Portability literal              | `/home/psimmons` hardcoded in systemd units and drop-ins                                                      | `rg -n '/home/[a-z]' .` must return zero hits                                             | 🤖    |
| FM-07 | Portability / specifier scope    | `%h` not expanding in `Environment=` line                                                                     | Document expansion contexts; use sed tokens in `Environment=`                             | Judgment |
| FM-08 | Portability / version pin        | Node-version-pinned `CODEX_BIN` path breaks on nvm upgrade                                                   | Replace pinned paths with `command -v` or install-time template                           | 🤖    |
| FM-09 | Thermal / resource-role safety   | Bulk 259 GB re-embed could land on fanless MI-50 after name collapse                                          | Confirm bulk selector is URL/affinity-based, not name-only, before renaming               | Judgment |
| FM-10 | Repo hygiene / canonical home    | Poller infra in tools repo; 128-line stub installed as real 2798-line poller; dual install paths             | `git remote -v` before edit; one canonical artifact per service                           | 🤖 / Judgment |
| FM-11 | Install layout / ordering        | `daemon-reload`-before-`enable` race self-disabled a timer                                                   | Sequence: write → `daemon-reload` → `enable` → `start`                                    | Judgment |
| FM-12 | Repo hygiene / wrong repo        | Worktree/wrong-repo edits landed in unintended repo                                                           | `git remote -v` + branch check before every cross-repo edit                               | 🤖    |
| FM-13 | Identity vs. routing             | Spurious mass re-process fired by a label/identity rename — no real content/schema change occurred            | Re-process fires ONLY on real content-or-schema change or explicit `--force`; identity rename short-circuits to no-op | Judgment |
| FM-14 | Stateless-vs-stateful health probe | engram-go `/ready` returns 200 once warm even when Postgres is down — k8s restarts never fire, traffic continues to a broken pod | Health probe must query the real dependency (e.g. `SELECT 1`); return 503 on any required dep failure | Judgment |
| FM-15 | `:latest` image drift            | k8s engram deployments used unpinned `:latest` tags — a node restart pulled a different/broken image silently | Pin every container image to a digest or immutable tag; CI lint blocks `:latest` in manifests              | 🤖    |
| FM-16 | Ephemeral resource leakage / host OOM (GLOBAL) | testcontainers left 272 anonymous Postgres volumes (152 GB, weeks old) on a shared dev host → risk of OOM crashing the COSMIC desktop + unrelated containers | Clean up session-created containers+volumes after every test/build (delta-only); watch `docker system df` headroom; NEVER global-prune a shared host | 🤖 / Judgment |
| FM-28 | Hardcoded ClusterIP in NetworkPolicy | engram + aifleet netpols encoded a service's ClusterIP — service recreate silently broke egress with no alert | NetworkPolicy egress must use `podSelector`/`namespaceSelector`, never a hardcoded IP                     | 🤖    |
| FM-17 | Missing namespace / non-idempotent apply | manifests referenced a namespace with no `00-namespace.yaml`; `kubectl apply -f .` fails cold on a fresh cluster | Every namespace used by a manifest set must have a `00-namespace.yaml` in the same apply bundle; CI dry-run on a clean cluster | 🤖    |
| FM-18 | exit-0 masks failure             | `codex-poll.sh` always `exit 0`; liveness wrappers swallowed child failures — systemd/CI/alerting saw green | Any script/wrapper that calls a child must propagate the child's exit code; wrappers are audited for unconditional `exit 0` | 🤖    |
| FM-19 | Doc drift re-teaches a fixed bug | olla-runbook Docker commands, `embedders.md` wrong env var, cutover rollback referencing wrong filename — each re-introduced a bug we had already fixed | After any bug fix, grep docs/runbooks for the old pattern and update in the same PR; doc review is a required fix step | Judgment |
| FM-20 | Dry-run bypasses validation      | `memory_migrate_embedder dry_run` skipped the dimension/G2 check — gave a false-OK before the real run failed | Dry-run and real paths must share the same validation gates; divergence is a bug, not a feature              | Judgment |
| FM-21 | Public destructive method with caller-side guard only | `NullAllEmbeddings`/`DeleteProject`/`prune` — safety lives only in the caller; a future caller bypasses it silently | Destructive DB/API methods must enforce their own guard (require explicit force flag or confirmation token at the method boundary) | Judgment |
| FM-22 | Two aliases for one model        | `bge-m3-live` vs `bge-m3-reembed` — same weights served under two routing names; identity/routing conflated downstream → `embedder_mismatch` on every write (tonight's root cause) | Same artifact must advertise exactly one canonical identity name; routing is expressed in URL/selector, not in the name | Judgment |
| FM-23 | Fail-Loud | K8s Job exit-code masking by `echo` | Always use `set -e` at top of K8s Job scripts so any command failure propagates | 🤖 |
| FM-24 | Thermal / Resource-Role Safety | Large single-transaction DELETE blocks live service | Run per-table DELETEs in separate autocommit statements; scale the live service to 0 first | 🤖 |
| FM-25 | Repo Hygiene / Canonical Home | TRUNCATE on shared table causes data loss risk | Never TRUNCATE a shared-project table; use scoped DELETE or dump/restore only the target project rows | 🤖 |
| FM-26 | Fail-Loud | Reembed claim predicate invisible to pre-embedded chunks | Before releasing chunk claims or assuming reembed will pick up all chunks, verify the claim predicate covers the actual stuck state | 🤖 |
| FM-27 | Thermal / Resource-Role Safety | Releasing chunk claims without verifying embedding endpoint health | Verify embedding endpoint returns 200 on /v1/models before scaling reembed or releasing claims | 🤖 |
| FM-29 | Capability misdiagnosis via non-login shell | `go`/`docker` reported `command not found` over a non-login SSH shell though Go was installed at `/usr/local/go` — nearly drove a wrong "node lacks toolchain → don't hand off" decision (2026-06-14, codex node) | Probe a remote toolchain in a fresh LOGIN shell (`ssh host bash -lc 'command -v X'`); a binary on disk but off the login `PATH` is a `/etc/profile.d` gap, not an absent tool | 🤖 |
| FM-30 | Handoff to executor lacking build/test toolchain | routing a Go + testcontainers TDD task to a node with no Go and no Docker would have produced unverified code (TDD Protocol 10 violated) — caught before queueing | Before queueing a handoff for repo R to node N, assert N has R's build toolchain AND test runtime (e.g. Go+Docker for testcontainers repos); gate queue-agent/poller on a per-repo-class node-capability check (claude-codex#114) | 🤖 |
| FM-31 | Target repo absent from handoff allowlist | a full Codex handoff plan was authored before discovering the repo was not in `config/target-repos.txt` → silently unroutable by queue-agent/poller | Validate target repo ∈ `config/target-repos.txt` AND node toolchain matches BEFORE authoring the handoff plan, not after | 🤖 |
| FM-32 | Non-unique failure-mode catalog IDs | two rows were both labeled `FM-16` (ephemeral-leak + NetworkPolicy ClusterIP) → per-repo checklists referencing "FM-16" are ambiguous | FM IDs must be unique: `grep -oP 'FM-\d+' failure-modes-standard.md \| sort \| uniq -d` must return empty (pre-commit/CI guard) | 🤖 |
| FM-34 | Harness update breaks a poller assumption (silently) | The third-party harnesses (codex CLI, hermes) update WEEKLY by design — they are deliberately NOT pinned (currency is wanted). The risk is not drift per se but a weekly update silently changing an interface the poller depends on (an `codex exec` flag renamed/removed, the `hermes chat` invocation changing) with no signal — the loop stalls or misbehaves quietly. | Do NOT pin the harnesses (founder directive: keep them current). Instead make the integration RESILIENT: (1) a tiny harness-contract smoke test exercising the exact `codex exec`/`hermes chat` invocation the poller uses, run on a schedule + after any detected version change; (2) fail-LOUD on interface breakage (non-zero exit / changed flags surface an alert, never a silent exit-0 — see FM-18); (3) re-probe capabilities/version rather than assuming (ASK-don't-infer); (4) record the observed harness version each cycle so a break correlates to an update. | 🤖 |
| FM-33 | Implemented-but-undispatched maintenance routine | fleet-dispatch `ReapOrphanPayloads` (orphan-payload GC) was correct + tested but never CALLED in the reaper loop — and a comment falsely claimed it ran → payloads leak unbounded (caught by adversarial QA, not by tests, because no test asserted the daemon dispatches it) | Every periodic-maintenance method (reaper/GC/sweeper) must have a call site in the daemon loop AND a test that asserts the loop invokes it (or that its effect occurs end-to-end). Grep: each exported `Reap*`/`GC*`/`Sweep*` has a non-test caller. Distrust comments that claim a routine "also runs" — verify the call. | 🤖 |
| FM-28 | Identity vs. Routing | Embedding model name case mismatch causes 503 | Validate model name casing in ConfigMap matches exactly what the embedding backend registers | 🤖 |
| FM-35 | Service reachable only via a host-local DNS shim | The fleet-dispatch canary could not fire: `fleet-dispatch.petersimmons.com` resolves only on the consumer node (codex, `/etc/hosts` shim — fleet-dispatch#6), so a producer on any other host gets `no such host`. The producer client + live endpoint both existed; the path was un-exercisable from the build host purely due to name resolution. | A service that multi-host producers must reach needs a PORTABLE resolvable endpoint (split-horizon DNS / a real A record — fleet-dispatch#6) before it is treated as the coordination path. Pre-fire reachability probe: resolve + TCP-connect the endpoint and fail fast with "not reachable from this host" rather than a confusing dial error mid-enqueue. | 🤖 |
| FM-36 | `kubectl port-forward` unusable from the agent harness | Repeated attempts to tunnel to the fleet-dispatch ClusterIP via `kubectl port-forward` exited 144 (reaped by the agent sandbox) — even detached via `nohup`/`setsid`. Burned several cycles trying variants. | To reach an in-cluster (ClusterIP) service from an agent session, do NOT rely on `kubectl port-forward` (the harness reaps long-lived listeners). Use an in-cluster one-shot (`kubectl run --rm` / a Job) that resolves the cluster DNS `svc.<ns>.svc.cluster.local` and carries the credential via a mounted secret, or run the call from a host already on the cluster/ingress network. Check: if a canary needs network reachability the agent host lacks, stage the runner in-cluster up front. | Judgment |
| FM-37 | Container Runtime — Design (FM-RC-01) | systemd timer / container scheduler fires nothing inside container; HEALTHCHECK returns 0 = false-healthy | Surface last-successful-cycle timestamp to HEALTHCHECK; assert freshness, not just process-RUNNING | Judgment |
| FM-38 | Container Runtime — Design (FM-RC-02) | Inner bwrap sandbox needs userns/seccomp; naive privilege probes invert on modern kernels → crash loop | Drop inner sandbox when container is the boundary; assert confinement via /proc/1/status (fail when unconfined), never via sandbox-success | Judgment |
| FM-39 | Container Runtime — Design (FM-RC-03) | Lease/heartbeat/cache/state paths default to HOME, not volume-backed → lost on container restart; cache re-clones each boot | Startup asserts every stateful path is a mounted volume; restart-persistence test must pass | 🤖 |
| FM-40 | Container Runtime — Design (FM-RC-04) | Installer-created vs compose-managed volumes diverge → orphaned volumes + wrong mount on start | Single source of volume names; assert the running container mounts the expected (compose) set | 🤖 |
| FM-41 | Container Runtime — Design (FM-RC-05) | Service assumed at localhost/host-gateway is actually LAN/remote → silent connection failures on startup | Startup probe of each declared endpoint; no localhost/host-gateway assumption without verification | 🤖 |
| FM-42 | Container Runtime — Design (FM-RC-06) | Default-allow egress permits lateral movement from within the container | Default-deny egress; see FM-43 (FM-RC-07) for the hostname-filtering proxy requirement | 🤖 |
| FM-43 | Container Runtime — Design (FM-RC-07) | Apply-time /32 IP pins break when CDN/SaaS host (e.g. GitHub) rotates IPs; container-resolved ≠ host-resolved → egress silently blocked | Hostname-filtering egress proxy (re-resolves per request); test: allowed host survives a simulated A-record change | 🤖 |
| FM-44 | Container Runtime — Design (FM-RC-08) | Host scheduler and container scheduler both poll simultaneously → double-claim race, duplicate work | Poll-gate flag (default OFF) until cutover; mask (not disable) host scheduler at cutover; single-claimant fleet guard | 🤖 |
| FM-45 | Container Runtime — Design (FM-RC-09) | Single-container blast radius: one crashing component kills all co-located services; health probe meaningless | Supervisor restart + backoff + circuit-breaker; per-component health + last-run timestamps + tagged logs | Judgment |
| FM-46 | Container Runtime — Design (FM-RC-10) | Device-flow token persists on a volume as crown-jewel: concurrent-write corruption; recovery tied to a human ceremony | Atomic credential write; documented rotation/revoke path; restart-persistence test | Judgment |
| FM-47 | Container Runtime — Operational (FM-RC-11) | Pinned-SHA config file or script not COPY'd into image → silent cycle-1 abort at runtime | Test every runtime-referenced file exists at its installed path in the image | 🤖 |
| FM-48 | Container Runtime — Operational (FM-RC-13) | Supervisor program command path doesn't exist; `|| true` masks it; process reports RUNNING while doing nothing | Assert every supervisor program command resolves; grep for `|| true` in program blocks | 🤖 |
| FM-49 | Container Runtime — Operational (FM-RC-14) | supervisord config `%` format sigil on a literal string → crash on start; lint passes, boot test required | Config-parse-reaches-RUNNING boot test; no `%` literals in supervisord config strings | 🤖 |
| FM-50 | Container Runtime — Operational (FM-RC-15) | Missing `[unix_http_server]` block → `supervisorctl` healthcheck always unhealthy; k8s restarts fire constantly | Healthcheck actually returns healthy in the boot test; assert `[unix_http_server]` present in supervisord config | 🤖 |
| FM-51 | Container Runtime — Operational (FM-RC-16) | `env_file required:false` → container boots without secrets; partial-credential run with no error surfaced | `env_file required:true`; required-vars list complete including memory and API keys | 🤖 |
| FM-52 | Container Runtime — Operational (FM-RC-17) | Installer missing `set -e` → deploys stale image after failed build step; no signal, no abort (cross-ref: FM-18 — same fail-loud class, installer-specific instance) | Installer aborts on any step failure (`set -e`); genuine `--dry-run`; idempotent | 🤖 |
| FM-53 | Container Runtime — Operational (FM-RC-18) | Device-flow login process detached without `nohup </dev/null` → killed on session teardown; code expires unused | `nohup </dev/null` durable launch for device-flow login; verify process survives session exit | Judgment |
| FM-54 | Container Runtime — Operational (FM-RC-19) | CLI polls device-flow token endpoint too aggressively → self-429, rate-limited, login dies | Backoff wrapper, fresh code per retry, bounded by deadline; sanctioned API-key path is the zero-poll alternative | Judgment |
| FM-55 | Container Runtime — Operational (FM-RC-20) | Runtime-only nft/iptables egress rules vanish on reboot → silent open egress after any host restart | Persistence hook ordered after docker.service; reboot tested; documented in runbook | 🤖 |
| FM-56 | Container Runtime — Operational (FM-RC-21) | Compose-prefixed docker network name (`<project>_<net>`) ≠ script default → egress fail-closes or mis-scopes silently | Resolve actual network name; parameterize; assert network exists with expected name at startup | 🤖 |
| FM-57 | Agent Runtime — Process / Coordination (FM-RC-22) | Executor refused a legitimate coordinator-relayed go-ahead, conflating artifact generation with authorization — blocked real work while human waited | Briefs must separate the human-only act (approval) from the non-authorization act (artifact generation); name what the executor may vs may not originate | Judgment |
| FM-58 | Agent Runtime — Process / Coordination (FM-RC-23) | Value resolved on host at apply time (DNS lookup, IP) diverges from what the container sees at runtime → silent mismatch (generalisation of FM-RC-07 and FM-28) | Resolve where the value is consumed, or re-resolve continuously; never pass a host-resolved ephemeral value to a container that will use it later | Judgment |
| FM-59 | Container Runtime — Operational (FM-RC-29) | Host PATH/runtime vars (PROJECTS_ROOT, CODEX_POLL_STATE_DIR) leaked via copied env-file → non-root container crash (paths point at /home/psimmons/... which doesn't exist in container) | Installer env-file denylist + `sanitize_env_file`; test asserts no host-path vars in env-file skeleton | 🤖 |
| FM-60 | Agent Runtime — Process / Coordination (FM-RC-30) | Lease-owner vs report-actor identity mismatch: post-verify guard compared report actor (hardcoded "codex") to lease owner (POLLER_ID). Container POLLER_ID=codex-container ≠ "codex" → completed work rejected at post-verify, bounced to queued, no PR | Guard compares `lease_owner == POLLER_ID` (not actor); test asserts container identity passes and host identity passes; foreign poller rejected | 🤖 |
| FM-61 | Container Runtime — Design (FM-RC-31) | Codex CLI inner bwrap sandbox cannot create a user namespace inside a cap-dropped/no-new-privileges container → workspace-write tasks fail silently; no draft PR | Container context disables inner sandbox (`--dangerously-bypass-approvals-and-sandbox`, gated on CODEX_INNER_SANDBOX=off set only in Dockerfile); container hardening NOT weakened; tests assert container→bypass-on, host→bypass-off | 🤖 |
| FM-62 | Agent Runtime — Process / Coordination (FM-RC-32) | FM-002 fleet guard ships disarmed — `fleet-hosts.txt` is comment-only so the guard passes silently, giving NO double-claim protection; two pollers on the same namespace both claim the same issue | Arm the guard: add the active host's `host:namespace` line to `fleet-hosts.txt`; a same-namespace second claimant must be rejected loudly (tests: guard armed + intruder rejected) | 🤖 |
| FM-63 | Agent Runtime — Process / Coordination (FM-RC-33) | Host leases live on the VM filesystem; container leases live inside a Docker volume — separate stores, so `lease_reclaimable` cross-boundary arbitration physically cannot fire; the container can claim an issue the host already holds | Design constraint: use split namespaces so host and container never poll the same label (host→`agent/codex/queued`, container→`agent/codex/bake`); documented as permanent design constraint, not a code fix | Judgment |
| FM-64 | Fail-Loud — host-side prefetch silent-empty | Host-side artifact/issue-body prefetch silently falls back to empty (`gh … 2>/dev/null \|\| echo ""`) → the runner is invoked with an EMPTY prompt → a full ~75s LLM run is burned and ends in a misleading `needs-input` ("paste the body"), not a clear failure (2026-06-15, fleet-dispatch F2; fixed in claude-codex `d822121`) | A REQUIRED-input prefetch must fail LOUD on error OR empty: emit `failed` (retryable) with the real stderr and NEVER invoke the runner on an empty prompt. Ban `2>/dev/null \|\| echo ""` for required inputs. Test: empty/errored prefetch → failed disposition, runner never launched | 🤖 |
| FM-65 | Fail-Loud — failure-shape promotion | Inferring a SUCCESS deliverable from the ABSENCE of an artifact: routing `done + notes + no-PR + no-commits` to plan-only delivery promotes a missing-artifact FAILURE (a run that narrated intent but produced nothing) to success — it is the identical shape (Hermes red-team, 2026-06-15, claude-codex#132) | Deliverable mode must be an EXPLICIT contract (producer-set `expected_delivery` / flag / label), never inferred from report shape. "Absence is not evidence." Adversarial test: an impl/default task reporting done with notes but no PR/commits → FAILED, not delivered | Judgment |
| FM-66 | Identity vs. Routing — side-channel delivery key | The runner's delivery/routing decision is gated on a signal the producer/queue never sets: fleet-dispatch items are keyed by CAPABILITY, but plan-only delivery was gated on a GitHub `PLAN_ONLY_LABEL` → legitimate plan/review deliverables hit the no-artifact guard and were discarded (2026-06-15, claude-codex#132) | The delivery/routing decision must read the SAME key the producer set (capability / explicit enqueue field), not a side-channel (GitHub label) the producing path doesn't populate. Check: the signal the consumer branches on is one the producer is contractually required to set | 🤖 |
| FM-67 | Fail-Loud / Observability — truncated agent output | Piping an agent/LLM consult's output through `\| tail`/`\| head` IN the invocation discarded the substantive head/body (lost both Codex's and Hermes's review bodies, twice in one session) → acted on a partial answer (2026-06-15) | Capture FULL agent/consult output to a file (no inline `tail`/`head` in the consult command); read back with offset/limit. For background tasks the redirect target holds the full stream — never pre-truncate at the source | 🤖 |
| FM-68 | Deploy / Verify Discipline — headless-agent stdin hang | A headless agent (`codex exec`) given its prompt as a positional arg via base64/env indirection fell back to reading STDIN and HUNG ~2h with zero output and no timeout; the watcher "waited" indefinitely (2026-06-15) | Feed headless-agent prompts via STDIN (heredoc/pipe), not a positional arg through env-decode. Wrap every remote agent invocation in `timeout`. Never block on a background agent task without a bounded deadline + a no-output tripwire | 🤖 |
| FM-69 | Implemented-but-undispatched — mitigation flag set but not wired (cross-ref FM-33, FM-61) | A documented mitigation exists as a FLAG but is not threaded into the invocation: `CODEX_INNER_SANDBOX=off` is set in the image (per FM-61) but the `--dangerously-bypass-approvals-and-sandbox` it is meant to gate was not actually passed to `codex exec`, so impl tasks still hit the bwrap namespace failure ~20min in (2026-06-15, claude-codex#134) | A disable/mitigation flag must be VERIFIED wired by a preflight probe (a no-op exec proving the inner sandbox is bypassed) that fails LOUD at boot — not discovered mid-task. Grep: the flag has a real consumer that changes the invocation | 🤖 |
| FM-70 | Process — trust calibration (observation vs conclusion) | Shipped a fix derived from an AI's unverified causal CONCLUSION ("the empty body is why codex bailed") instead of its verified OBSERVATION ("the code double-fetches and silently empties") — the fix did not resolve the symptom; the real cause was elsewhere (2026-06-15) | Calibrate trust by claim TYPE: act on direct observations; VERIFY conclusions (esp. load-bearing premises) before building on them; after a root-cause fix, REPRODUCE to confirm the symptom is gone before declaring done | Judgment |
| FM-71 | Reconciler — arity collision | `lease_release(brief_id, owner)` in codex-lease-interface.sh silently overwrites the same-named `lease_release(brief_id)` in codex-poll.sh when both are sourced; all one-arg poller calls get `$2: unbound` under `set -euo pipefail` | Rename 2-arg form to `lease_release_owned`; verified by integration test sourcing both scripts | 🤖 |
| FM-72 | Reconciler — mutable join-key | brief_id derived from mutable issue body hash → body edit changes the hash → new hash misses the existing lease → ORPHAN misdetection → spurious re-queue on every body edit | Use stable `sha256("${repo}#${number}")` for brief_id; `decide()` always matches by `issue_ref` (stable) not brief_id | 🤖 |
| FM-73 | Reconciler — strike ordering | `emit_strike()` called AFTER successful requeue, not before attempt → a failed requeue (e.g. 403) never increments the counter → quarantine never triggers → infinite requeue loop possible | Call `emit_strike()` before `requeue_issue()`; call `reset_strike()` only on confirmed success | 🤖 |
| FM-74 | Reconciler — label projection gap | Reconciler first pass only scans issues WITH the in-progress label; a poller that holds a live lease but whose in-progress label was silently dropped (race/API error) is never re-asserted | Second pass scans all live lease files; for each live lease whose `issue_ref` was NOT seen in first pass → re-assert in-progress label via `upsert_label` | 🤖 |
| FM-75 | Agent Dispatch — Permission Isolation | Background/worktree-spawned agents auto-deny interactive Bash (ssh) permission prompts, burning a full agent spawn per attempt. A foreground-valid ssh transport (e.g. ssh codex.petersimmons.com socialization) is silently DENIED when run by a background or post-EnterWorktree agent — the interactive grant prompt has no channel to approve, so it auto-declines; re-dispatching repeats the token waste. | grep ~/.claude/settings.json (and settings.local.json) for an allowlist entry covering ssh codex/hermes socialization transports; absence = unmitigated. ABORT dispatch after first permission-denied result — never retry. | Judgment |

### I. Container Runtime — Design

A container runtime design assumption fails silently: schedulers never fire inside the container, inner sandboxes crash on modern kernels, stateful paths write to the ephemeral layer instead of a volume, or a topology assumption about service locality is wrong. Failures in this class were catalogued during the Codex containerisation pilot (FM-RC-01 through FM-RC-10).

- **Active liveness, not process-RUNNING.** Scheduler, timer, and periodic tasks inside a container must surface a last-successful-cycle timestamp to HEALTHCHECK; freshness is asserted, not just process presence.
- **Drop the inner sandbox when the container is the boundary.** `bwrap`/userns/seccomp inside a container inverts privilege probes on modern kernels. Assert confinement via `/proc/1/status` (fail when unconfined), never via sandbox-success.
- **All stateful paths on named volumes; startup asserts mounts.** Lease/heartbeat/cache/state paths that default to HOME write to the ephemeral layer and are lost on restart. Startup must verify every stateful path is volume-backed; a restart-persistence test must pass.
- **Single source of volume names; assert the running container mounts the expected set.** Installer-created vs compose-managed volumes diverge → orphans + wrong mount.
- **Startup probe of each declared endpoint; no localhost/host-gateway assumption without verification.** A depended service assumed local may be LAN/remote; the startup probe catches topology mismatches before the first job.
- **Default-deny egress; hostname-filtering proxy re-resolves per request.** Static IP pins break when CDN/SaaS hosts rotate IPs; container-resolved ≠ host-resolved. Test: allowed host must survive a simulated A-record change.
- **Poll-gate flag (default OFF) until cutover; mask host scheduler at cutover.** Prevents double-claim when both host and container scheduler run concurrently.
- **Supervisor restart + backoff + circuit-breaker; per-component health + last-run timestamps + tagged logs.** Single-container blast radius: one crash must not silently kill all co-located services.
- **Atomic credential write; documented rotation/revoke path; restart-persistence test.** Device-flow/bearer tokens on a volume are the crown-jewel: concurrent-write corruption or undocumented recovery path blocks all operations.

### J. Container Runtime — Operational

Live-discovered failures during container install: missing image assets, cross-script name drift, phantom supervisor paths, config parse crashes, healthcheck connectivity gaps, silent missing-secrets boot, and egress-rule persistence after reboot. Failures in this class were catalogued during the Codex pilot (FM-RC-11 through FM-RC-21).

- 🤖 **Test every runtime-referenced file exists at its installed path in the image.** Missing pinned-SHA config or script causes a silent cycle-1 abort.
- 🤖 **Every sibling-script invocation in the container resolves to an executable in the image (name vs name.sh).** Cross-script name/extension drift causes silent failure. Cross-ref: FM-04, FM-05 (same class, container-specific context; FM-RC-12 is not assigned a separate FM-ID as it is covered by the existing entries).
- 🤖 **Assert every supervisor `program command` path resolves; no `|| true` masking.** Phantom paths report RUNNING while doing nothing.
- 🤖 **Config-parse-reaches-RUNNING boot test.** `supervisord` format sigils (`%`) on a literal value crash on start — caught only by a real boot test, not a lint.
- 🤖 **Healthcheck actually returns healthy in the boot test.** Missing `[unix_http_server]` block means `supervisorctl` healthcheck is always unhealthy.
- 🤖 **`env_file required:true`; required-vars list complete (incl. memory/API keys).** `required:false` lets the container boot without secrets → partial-credential run, no error.
- 🤖 **Installer aborts on any step failure; idempotent; genuine `--dry-run`.** Missing `set -e` in installer deploys a stale image after a failed build with no signal. Cross-ref: FM-18 (same fail-loud class; FM-RC-17 is the installer-script instance).
- **`nohup </dev/null` durable launch for device-flow login.** Detached login process killed on session teardown → code expires unused.
- **Backoff wrapper, fresh code per retry, bounded by deadline for device-flow polling.** CLI polls token endpoint too hard → self-429, login dies.
- 🤖 **Egress persistence hook ordered after `docker.service`; documented in runbook.** Runtime-only nft rules vanish on reboot → silent open egress.
- 🤖 **Resolve the actual docker network name (compose-prefixed `<project>_<net>`); parameterize.** Script-default network name mismatch → egress fail-closes or mis-scopes.
- 🤖 **Container context disables the Codex CLI inner bwrap sandbox; host context unchanged.** bwrap cannot create a user namespace inside cap_drop=ALL + no-new-privileges → workspace-write tasks fail. Fix: gate `--dangerously-bypass-approvals-and-sandbox` on `CODEX_INNER_SANDBOX=off` (Dockerfile ENV only). Tests assert container→bypass-on, host→bypass-off. Cross-ref: FM-RC-31.

### K. Agent Runtime — Process / Coordination

Process failures at the human-agent coordination boundary: executor refuses a legitimate coordinator-relayed authorization, or a value resolved at apply time diverges from what the container sees at runtime. From FM-RC-22 and FM-RC-23.

- **Briefs must separate the human-only act (approval) from the non-authorization act (artifact generation).** An executor that conflates "issue a code" with "grant access" blocks real work while the human waits. Name what the executor may vs may not originate.
- **Resolve where the value is consumed, or re-resolve continuously.** Any value resolved on the host at apply time but used by the container at runtime can diverge (DNS, IPs, tokens). Generalisation of FM-RC-07 / FM-28.
- **Pre-authorize fleet socialization transports in settings.json; ABORT after the first permission-denied result — never retry.** Background/worktree agents have no interactive channel to approve Bash permission prompts; an allowlist entry for `ssh codex.petersimmons.com` / `ssh hermes.petersimmons.com` is required or the agent silently burns tokens for zero progress. (FM-75)

---

## Case Studies

### Case Study 1 — Embedder Identity vs. Role Leak (aifleet / Engram, 2026-06-12)

**What happened:** aifleet's olla per-role routing commit encoded each GPU's role into the olla `--alias` (`bge-m3-live`, `bge-m3-reembed`). Engram-go reads that alias as `project_meta.embedder_name` — a persisted identity it compares on every write. Because the alias no longer matched the stored identity `BAAI/bge-m3`, every `memory_store` returned `embedder_mismatch` and was blocked. The model weights were identical on both endpoints; only the advertised name differed. No re-embed was needed.

A secondary hazard: collapsing the names without first giving the bulk worker a URL-based selector would have sent a 259 GB bulk re-embed to the fanless MI-50, which has no active cooling.

**Class:** FM-01, FM-02, FM-03, FM-09.

**The checks that would have caught it:**
- Grep `--alias` values for role words (`-live`, `-reembed`, `-bulk`) before merging the routing commit. (🤖)
- Confirm the advertised alias == the value persisted in the downstream system's identity field. (Judgment, pre-merge)
- Before collapsing names, confirm bulk-workload routing uses a selector other than the name being changed. (Judgment, pre-deploy)

**Fix applied:** Three ordered prod steps — (1) fix primary alias to `BAAI/bge-m3` (stops the bleed), (2) give bulk worker a direct URL pin (thermal safety independent of name), (3) unify bulk alias. Full runbook: `~/.claude/worktrees/advisory-protocol-package/.claude/plans/RUNBOOK-embedder-identity-vs-role.md`.

---

### Case Study 2 — Poller Install-Layout / Repo Relocation (claude-codex, 2026-06-12)

**What happened:** The codex poller service had accumulated three compounding problems: (a) the systemd unit's `ExecStart` referenced `codex-poll.sh` (with `.sh`) but the installer produced a binary named `codex-poll` — every start attempt returned 203/EXEC; (b) `codex-liveness-check` was referenced in the unit but had never been vendored or installed — it had been silently failing with exit-127 every 15 minutes for weeks, with the wrapper masking it as success; (c) the poller infra lived in the `codex` tools repo (Rust CLIs) rather than its owning repo `claude-codex`, and a 128-line stub had been deployed in place of the real 2798-line poller.

**Class:** FM-04, FM-05, FM-06, FM-08, FM-10, FM-11, FM-12.

**The checks that would have caught it:**
- Staged install-layout check: render `ExecStart` with real specifiers, diff against post-install file listing. (🤖 — the offline `systemd-analyze` passed the bug; the staged check caught it)
- Grep all unit targets; confirm source file exists in-repo and an install line places it. (🤖 — would have caught the missing liveness binary before deployment)
- `git remote -v` before editing: verifies you are in the owning repo, not a sibling worktree. (🤖)

**Fix applied:** Six-phase migration — real poller replaces stub, liveness binary vendored, paths de-hardcoded, single portable installer, VM sha symlink repointed, tools repo stripped of poller files. Verified via live fire (`ExecMainStatus=0`). Full record: `~/.claude/worktrees/advisory-protocol-package/.claude/plans/MIGRATION-poller-infra-COMPLETE.md`.

- **Missing-HTTP-timeout hang** — external-dependency HTTP call with no per-request read deadline wedges all workers indefinitely when an upstream gateway stalls a connection (zero bytes, ESTAB held for hours). Backends may be healthy; the bug is the missing client timeout. Deterministic check: every HTTP client against an external dependency sets `http.Client{Timeout}` or per-call `context.WithTimeout` + bounded retry. Ref: engram-go#1107.

---

### E. Lease/Reconciler Failure Modes

The in-progress orphan saga (engram-go#1108): an issue labeled `in-progress` with no live lease is invisible to the poll loop and to `lease_reclaimable()`. The active expiry-sweeper (codex-reconciler.sh) closes this gap by independently querying for all `in-progress` issues and reconciling against the lease store.

| ID | Name | Symptom | Fix |
|----|------|---------|-----|
| FM-REC-01 | stale-lease-wrong-owner | Issue labeled `in-progress` + lease exists but expired + different owner | Reconciler detects via `decide()` → `STALE_LEASE_WRONG_OWNER` → re-queue |
| FM-REC-02 | reconciler-ABA | Second reconciler sees same lease generation, re-queues already-recovered issue | Idempotency guard: CAS on lease generation before acting |
| FM-REC-03 | strike-bounds-missing | Unbounded recovery strikes on persistently failing issue burn loops forever | Self-quarantine after `CODEX_RECONCILER_MAX_STRIKES` (default 3) |
| FM-REC-04 | 403-silent-release | 403 on label write silently releases a live lease, orphaning the work | `requeue_issue()` fails-loud on non-zero gh exit, NEVER releases on 403 |
| FM-REC-05 | duplicate-host-identity | Two processes with same POLLER_ID both claim; second double-executes | Lease identity must carry process generation; `decide()` detects `DUAL_CLAIM` → `freeze_issue()` |
| FM-REC-06 | clock-skew | Clock jump ahead causes mass premature lease expiry and mass re-queue | Lease expiry should have a grace window; noted for future hardening (not implemented) |
| FM-REC-07 | reconciler-crash-mid-heal | Crash between re-queue label write and strike decrement; rerun duplicates | Idempotent rerun: requeue is always retried; strike counter only increments |
| FM-REC-08 | label-projection-staleness | Dead projector means in-progress label stale while lease store is correct | Reconciler heartbeat file (`CODEX_RECONCILER_HEARTBEAT_FILE`) must alarm if stale |
| FM-REC-09 | toctou-reap-deletes-renewed-lease | Foreign poller RENEWS its lease between `decide()` reading it as expired and `reap_stale_foreign_lease()` deleting it; reap removes a now-LIVE lease → issue re-queued while foreign host still runs it → DUAL-EXECUTE | Reap re-reads `expires_at`+`owner` under `flock` immediately before `rm`; SKIPs delete if now live (`now < expires_at`) or owner changed |
| FM-REC-10 | silent-unreadable-store-mass-orphan | Lease dir unreadable/unwritable (e.g. systemd `ProtectHome=read-only` with default `CODEX_POLL_LEASE_DIR=${HOME}/.cache/...` not overridden to a ReadWritePath); reconciler scans empty store → classifies EVERY in-progress issue as `ORPHAN_NO_LEASE` → mass force-requeue of all live work | `assert_lease_store_usable()` fails CLOSED at startup: probes read+write, logs FATAL and EXITs non-zero BEFORE any `decide()`/requeue; systemd unit pins lease dir under `ReadWritePaths` |
| FM-REC-11 | label-reasserted-on-closed-issue | `reconcile_label_missing()` second pass re-asserts `in-progress` on a completed+CLOSED issue whose lease hasn't expired (closed issues absent from `--state open` first pass) every sweep until TTL → misleading label + API-hammer loop | Verify issue is OPEN (`gh issue view --json state`) before `upsert_label`; skip if closed/merged or state unconfirmable |
