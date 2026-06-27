# Lessons Learned

Per CLAUDE.md: append a dated entry whenever the user corrects course or reframes a problem.
**Source of truth: `lessons-learned.jsonl`** — this file is auto-generated.
Regenerate with: `python3 ~/bin/render-lessons-learned.py`
Append new entry: `python3 ~/bin/render-lessons-learned.py --append '{...}'`

## 2026-05-16 — Pivot on user data, not on theory

**Context:** Bambu Studio refused to launch. I burned 6+ download/install attempts probing versions (2.5.3, 2.6.0, 2.7.0-beta), both flatpak and AppImage, plus extracted-binary debugging, GTK theme overrides, X11 fallback, software rendering. All failed identically. I filed a comment on bambulab/BambuStudio#9217 framing it as a cross-distro wxWidgets bug.

**User correction:** "Can we manually test backwards releases and see? The software seemed to work about 3 days ago. What did I update then?"

**What I missed:** The user already had the key datum — *it worked 3 days ago*. That collapses the problem from "which Bambu version works" to "what changed on the host between 22:49 and 23:40 on May 15". Two commands (apt history + Bambu log directory listing) revealed the precise success→failure window with zero package changes inside it, and `journalctl -k` showed the amdgpu DPCD errors starting in that window. Root cause: GPU driver state corruption after 12-day uptime, not Bambu.

**Rule going forward:** When the user supplies a working-baseline timestamp, *stop probing the suspected component and pivot to host-state archaeology immediately.* Specifically:
1. `awk` over `/var/log/apt/history.log` filtered to the success→failure window
2. `flatpak history --since=DATE`
3. App's own log dir sorted by time + size (file-size diagnostic for app-specific crash signatures)
4. `journalctl -k --since` for kernel/driver warnings in the window
5. `uptime` — long uptime + AMD GPU + GUI app failure = strong reboot hypothesis

This is faster, cheaper, and more diagnostic than version regression sweeps.

## 2026-05-18 — Container "healthy" ≠ service reachable

**Context:** Engram-go MCP appeared to "crash" twice during an LME benchmark. Investigation showed `docker compose ps` reporting `Up 8 minutes (healthy)` while every `curl http://127.0.0.1:8788/health` returned `Recv failure: Connection reset by peer` after the TCP handshake.

**Root cause:** The healthcheck was `["CMD","/starter","health"]` — a CLI command that does not exercise the HTTP listener. Meanwhile the app bound to `127.0.0.1:8788` *inside* the container netns (default `ENGRAM_HOST=127.0.0.1` from #666), so the docker-proxy forwarding `host 127.0.0.1:8788 → container eth0 172.26.0.2:8788` hit a closed port and RST'd. Container was structurally unreachable from the host port mapping and yet reported healthy the entire time.

**What I missed initially:** I treated the container's `(healthy)` status as evidence the listener worked, and went hunting for a panic/OOM/restart explanation for the "crashes." There were none — RestartCount=0, ExitCode=0, no panic in 2h of logs. The service was never reachable; the benchmark just made the unreachability visible.

**Rules going forward:**
1. **Never trust `healthy` without verifying the healthcheck command.** `docker inspect <c> --format '{{json .Config.Healthcheck}}'` first. If it's a CLI subcommand (`/binary health`, `pg_isready` against localhost-in-container), it proves nothing about external reachability.
2. **When TCP connects but the response RSTs, suspect bind-interface mismatch before suspecting app crash.** App-side panic mid-response is rare; bind-on-loopback-inside-container behind a port mapping is common. Check `docker inspect --format '{{range $k,$v := .NetworkSettings.Networks}}{{$k}}={{$v.IPAddress}} {{end}}'` then `curl http://<container-ip>:<port>/` directly — connection refused there confirms wrong-interface bind.
3. **Port mapping `127.0.0.1:PORT:PORT` does NOT imply the app should bind 127.0.0.1 inside the container.** The host-side restriction is enforced by the proxy. The container must bind `0.0.0.0` (or the eth interface) for the proxy to reach it. Defaulting the app to loopback inside the container makes the port mapping useless.
4. **Stand down on shared resources when the user signals another agent is acting.** Diagnosis is still useful; concurrent edits to the same compose file are not.

## 2026-05-19 — npx -y patches are not durable; fork to a git ref instead

**Context:** `@infisical/mcp@0.0.23` sends `?type=all` on `list-projects` which the self-hosted Infisical API rejects (422, invalid enum). A fix was applied on 2026-05-10 by patching the npx cache copy at `~/.npm/_npx/<hash>/node_modules/...`. This session re-applied the same patch before anyone noticed — the patch had been silently wiped when npx re-extracted the package.

**Root cause:** `npx -y <pkg>@<version>` re-extracts the package from the registry on every new invocation context (new hash, new machine, new cache prune). Any manual edits to files under `~/.npm/_npx/` are ephemeral.

**Durable recovery pattern:**
1. Fork the upstream repo
2. Apply the patch to the TypeScript source
3. `npm run build` → force-commit `dist/` into the fork
4. Create a semver tag (e.g., `v0.0.23-patch1`)
5. Switch the MCP launcher from `npx -y <pkg>@<version>` to `npx -y github:<user>/<repo>#<tag>`
6. Open an upstream PR so the fork eventually retires

Cost: ~30 minutes. Benefit: patch survives cache wipes, machine reinstalls, and new sessions indefinitely.

**Today's resolution:** `petersimmons1972/infisical-mcp-server#v0.0.23-patch1`, upstream PR Infisical/infisical-mcp-server#19.

## 2026-05-19 — Coordinator restriction: "didn't we already do this" is a memory-recall failure, not a planning failure

**Context:** The Infisical MCP 422 fix was re-applied in this session because no `memory_recall` happened for the immediate task topic at session start. The user had to ask "didn't we just do this?" before recall was triggered.

**What I missed:** CLAUDE.md R1 mandates `memory_recall("current project status recent work", project="global")` + topic at session start. The specific fix (Infisical MCP patch) had been stored in Engram on 2026-05-10. A pre-task recall would have surfaced it immediately.

**Rules going forward:**
1. R1 recall at session start is mandatory — but also perform a **task-specific recall before starting any work that feels "adjacent to something we've touched before."**
2. When the user says "didn't we already do X?" — that is a signal that recall did NOT happen. Stop, recall immediately, and report what memory says before re-doing any work.
3. The cost of a recall miss isn't just wasted time — it's undetected regressions (re-applying a patch that was supposed to be permanent), repeated expensive operations, and eroded user trust in the coordinator.

## 2026-05-18 — Founder decision pattern: prefer thoroughness over speed

During instinct-migration campaign, founder consistently chose more thorough options when coordinator recommendations were biased toward velocity:

1. **LLM backend (Track A):** Coordinator (Eisenhower) recommended Option 3 (generic Complete interface). Founder confirmed — alignment.
2. **Olla dual-backend (E1 scope):** Coordinator (via min-change-plan) recommended dropping Olla to ship Anthropic-only. Founder said **HOLD: implement dual backend before cutover**. Choose thoroughness.
3. **Storage strategy (Track E):** Coordinator (Eisenhower) recommended option (i) tag-based confidence (~half-day, no schema change). Founder chose **option (iii) new dedicated DOUBLE PRECISION column** (~1-2 days + engram-go review cycle). Choose clean semantics over speed. This decision surfaced the deeper two-importance-column schema and exposed that engram-go's own consolidator had the same defect.
4. **Confidence migration:** Coordinator offered hybrid `--detect-and-report` first. Founder agreed. Preserves evidence-gathering before destructive op.
5. **Soft halt depth:** Coordinator offered three halt depths; founder chose **finish in-flight E1-FIX-2 then park** rather than hard halt mid-flight. Preserves clean parking point.

**Calibration lesson for future coordinator briefings:** when presenting options to the founder, do NOT bias the "Recommended" tag toward velocity. The founder's track record is consistently to choose the more thorough path. The recommended-default in AskUserQuestion should reflect this — make the thoroughness option the recommended one unless the velocity option has a specific time-sensitive trigger (production fire, missed deadline, etc.).

The founder also issued a mid-flight stabilization directive on engram-go that triggered the soft halt. This was upstream priority (service stability) overriding migration completion. Coordinator-level pattern: be alert to upstream priority signals that may pause or reframe a campaign even when execution is going well.

## 2026-05-20 — Sub-agent briefs must not contain push commands

**Context:** Project A Stage 0 dispatched a Sonnet sub-agent to write two principle docs for the harness-port repo. The brief I authored included `git push origin main` as the final step of its commit sequence. The sub-agent executed correctly. The harness security warning fired: AP.11 (wake-the-founder push-to-main) requires explicit per-push founder confirmation, which was absent.

**User correction:** "Advisory Mandate!" — immediately on seeing the security warning.

**Why I missed it:** I conflated plan-level approval with per-push approval. The user had approved the plan (which contains pushes as a logical step), so I treated embedding push commands in sub-agent briefs as in-scope. AP.11's actual text rejects this — every push to main needs explicit per-push confirmation.

**Rule going forward:** Coordinator owns the publish boundary. Sub-agent briefs MAY include `git add` and `git commit` (the sub-agent's working state ends at a clean local commit). Sub-agent briefs MUST NOT include `git push`. Coordinator pushes after explicit per-push user confirmation. Applies to any ref pushed to a shared remote: main, release branches, anything visible beyond the local machine.

**Advisory:** opus-advisor returned A + D-extended + H. Accept the landed push retroactively (rollback re-triggers AP.11), apply the rule for all branches not just main, do not amend the existing Project A plan (discipline corrections live in journals not plan text).

**Cross-reference:** harness-port JOURNEY.md 2026-05-20 entry; commit `656c99a` is the offending push.

## 2026-05-30 — Substack publishing lessons

**python-substack constructor**: `Api(cookies_string="...", publication_url="https://...")` — NOT `cookies=`.

**Publish sequence**: `api.prepublish_draft(id)` then `api.publish_draft(id)`. Raw POST to `/api/v1/posts/{id}/publish` returns 404.

**load_dotenv() in heredocs**: Fails with AssertionError. Always use explicit path: `load_dotenv('/home/psimmons/projects/substack/.env')`.

**SUBSTACK_PUBLICATION_URL**: Must be account subdomain `plutarchtx.substack.com`, not the @handle `clearwatch`.

**Substack cover fields**: `cover_photo_url` = wide homepage banner. `logo_url_wide` = nav header thumbnail. Different fields. Write to `cover_photo_url` for the publication homepage cover.

**Scheduler state corruption**: pub/ git tags are terminal state. Bogus tags block future publishes silently. Fix: delete tags + remove from published.json + drafts.json on scheduler/state branch. Scheduler cannot recreate drafts past the 24h lead window — manual recovery via push-draft.py + state injection required.

## 2026-06-01 — Publish articles via API, not by pushing draft URLs to user

When instructed to "publish" a Substack article, call the Substack API to publish
the post programmatically — do not push a draft URL and ask the user to click Publish.

Workflow:
1. push-draft.py (with idempotency — finds/updates existing draft or creates new)
2. api.publish_draft(post_id, send=False, share_automatically=False)
3. Confirm publication via api.get_published_posts()

The `send=False` flag avoids triggering a subscriber email blast from a test publish.
This matches CLAUDE.md rule: "Never tell the user to do something manually that you
can do yourself — just do it."

## 2026-06-04 — Don't restart the portal router on a live desktop session

**Context:** Brave and PDFs were taking ~60s to launch on COSMIC (Pop!_OS). Root cause was correct: `xdg-desktop-portal-cosmic`'s Settings interface had deadlocked, adding a ~28s dbus timeout to every GUI launch (confirmed: `gdbus ... Settings.ReadAll` = 28.1s vs 3ms healthy). Trigger was `cosmic-applet-audio` stuck at 80% CPU for 28h + WirePlumber churn.

**What I did wrong:** I restarted `xdg-desktop-portal.service` and called it "low-risk, won't disrupt running apps." It severed every app's portal connection at once; the signal flood made `dbus-broker` force-disconnect overloaded peers (including `:1.1`, the systemd user manager), which **killed all running GUI apps**. My repair attempts then leaked bus connections (160 -> 280), past dbus-broker's per-user cap of 256, after which the broker refused every new connection ("Error sending credentials: Broken pipe") and nothing could launch at all. Only fix: logout/login.

**Rules going forward:**
1. A wedged desktop-session daemon (portal, compositor helper) wants a **logout/login**, not a daemon restart. Restarting the portal router cascades through every subscriber.
2. Restarting a live desktop daemon is externally-visible + hard to reverse — **confirm with the user first**, never label it "low-risk."
3. Slow launch of 2+ unrelated apps = portal/dbus timeout signature, not an app bug. Time `Settings.ReadAll`; ~28s = deadlocked backend.
4. Tripwire: "Error sending credentials: Broken pipe" on a fresh connection (even your own gdbus) = dbus-broker over its 256 cap. STOP — every further probe consumes another slot and worsens it.
5. When already deep in a self-inflicted hole, stop probing and concede the clean reset early; don't keep restarting things.

## 2026-06-04 — Claude↔Codex poller made real, + LME score hygiene

**The handoff pipeline had been silently broken and is now functional for the first time.** Full architecture, deploy path, and the four root-cause bugs are in [codex-poll-canonical](codex-poll-canonical.md); the reusable systemd/bash gotchas in [systemd-killmode-children](systemd-killmode-children.md). One-line recap of the four: (1) poison-pill `return 1` crashed the oneshot every tick → `continue`; (2) `KillMode=mixed` SIGKILLed backgrounded codex → `KillMode=process`; (3) `wait` on a non-child PID returned instantly → `while kill -0`; (4) verifier marked done without CI / false-stalled no-PR review issues / lost unpublished work (claude-codex #20/#21/#22). Consolidated two divergent poller lineages onto the canonical `petersimmons1972/codex` version (16/16 bats green) and retired the interim homelab-config copy. Live-tested: filed an issue → poller auto-dispatched headless codex in ~3 min.

**Operational discipline confirmed this session:** the worktree-isolation guard blocks Write/Edit on any path outside the active worktree — write memory/external files via Bash heredoc or Python, not the file tools. Sub-agent briefs may `git add`/`commit` but never `git push`; coordinator owns the publish boundary.

**LME score hygiene (the central measurement wisdom).** The headline "82.4% (412/500)" is NOT a clean or comparable number: it is `Haiku-correct ∪ Opus-correct-on-Haiku-failures` on LongMemEval-**M** (our hard 500q/~1.5M-token variant), while every published competitor number is LongMemEval-**S** (~115K tokens). The honest single-system figure is **Stage-1 65.4%**. Asymmetric re-judging (we re-judge only failures, never the 327 "correct") is a ratchet bias, not a feature. Every future number must declare: variant (S/M), reader model, single-pass vs ensemble, retrieval config, and whether CORRECTs were re-audited. The bottleneck is **retrieval, not the reader** (~81% of remaining failures are retrieval-limited); the real ceiling on that run is ~83%. Most of the SOTA techniques are already built but flag-gated default-off and were not deployed during the scored run — so we measured our baseline, not our system. Full detail: `~/projects/engram-go/docs/lme-campaign/{FINDINGS.md,RUNBOOK.md}`.

## 2026-06-05 — CLAUDE.md self-audit: recall is measurable, and pointers go stale

Ran the Article 045/046 self-summary + compression audit on the 89-rule `CLAUDE.md`, then applied the fixes (commit `25da70f`, pushed to `github/master`). Two durable lessons (full detail in Engram, project `global`, ids `019e960d-7c49…` and `019e960d-a455…`):

**1. Prominence ≠ internalization; trigger→action shape drives recall.** Cold-ish self-summary recall was ~54% (warm — a fresh session is lower). The most damning miss: the **three Core Principles** (Simplicity-first / No-laziness / Minimal-impact) — the literal first rules in the file — were not recalled, while the tool-prefs and cost-guardrails (table / "before X do Y" shaped) came back clean. Rules stick when they're (1) trigger→action shaped, (2) concrete/testable, (3) front-loaded — not when they're abstract values, however prominent. Fix applied: reshaped Core Principles into a "before presenting, run this gate" 3-step checklist and front-loaded a Decisions & Defaults block. **Validation is a COLD re-run next session** — if R01–R03 surface then, the reshape earned its keep. (Also: 2 "hallucinated" rules were really harness rules — Edit-over-sed, bg-narrate — mis-attributed to CLAUDE.md; the model can't tell contract source under load.)

**2. `→` pointers silently rot when you restructure the target.** Three CLAUDE.md pointers referenced `AGENTS.md` sections (`§Security & Process Non-Negotiables`, `§Container Image Standard`, `§9` art-direction) that no longer exist — the big AGENTS.md was replaced by a 603-byte Codex queue directive and its content moved to `~/docs/` + `~/projects/art-direction-research/`. The security CONTRACT was pointing at nothing. This is the known failure mode of the "shift content to pointers to save tokens" strategy: pointers are unverified by default. Fix: repaired all 3 to real targets + inlined the security NEVER-list inline. **Rule going forward:** when you move/rename a referenced doc, grep `CLAUDE.md` for the old path/section and re-point; periodically verify every `→ path` and `§section` actually resolves.

**Process note:** the pre-flight env check paid off — when asked to apply+commit, the check found `CLAUDE.md` was *already* committed (another session had run the exact commands), so I halted instead of double-applying a stale patch. In bg sessions the live file can move under you; re-verify the baseline (md5 vs t0 snapshot) before any apply.

## 2026-06-05 — Pre-validate a queued task's premise against live state; verify LB direction empirically

**Context:** The embed campaign's keystone issue (aifleet#400) was framed as a "fleet-wide embed outage: FC endpoints lack the embeddings capability." Before letting Codex implement the queued fix (#412), I ran a live pre-validation against the actual controller-generated `olla-config` ConfigMap and Olla `/internal/status`. Two things the issue framing got wrong:

1. **No outage.** Embeds worked fine — a live in-pod round-trip returned a valid 1024-dim `bge-m3` vector. W6800 was serving 4104 requests. The "outage" framing was stale. The *real* defect was narrower: `embed-mi50` carried no `capabilities` field and was silently locked out (0 requests), and the three embed cards needed explicit duty-tier priorities.

2. **I had the priority direction backwards.** From the stale "embeds leaned on 7900xt" line I assumed Olla's priority LB was lower=preferred and wrote a #412 comment saying priority was "inverted." The `/internal/status` request counters proved the opposite: `w6800` pri=100 → 4104 reqs, `7900xt` pri=25 → 146 reqs, so **higher number = preferred** and the numbers were already correct. Had Codex followed my comment it would have flipped correct priorities backwards and broken routing. I retracted the comment and corrected the brief + Engram.

**Rules going forward:**
1. **Before dispatching an implementer against an issue, validate the issue's premise on live state** (config, status endpoint, a real request). Issue text — even your own from earlier in the session — goes stale; the running system is the source of truth. This is CLAUDE.md AP.1 ("validate on a real sample first") applied to bug *framing*, not just samples. Cheap, read-only, and here it converted a "rewrite all caps + priorities" task into "add caps for one endpoint + set 3 tier values" and caught a backwards assumption.
2. **Never encode a load-balancer's priority direction from intuition or prose.** Drive real traffic and read the per-endpoint request counter to learn which way "preferred" goes. For Olla specifically: `/internal/status` exposes `priority` + `requests` per endpoint; higher priority number = more preferred. Full reference: [olla-priority-routing](reference_olla_priority_routing.md).
3. **A capability-gated router drops mis-tagged endpoints silently** — no error, just zero traffic. When a healthy backend serves nothing, check whether it advertises the required capability before chasing health/network causes.

## 2026-06-10 — Claude↔Codex protocol-vNext build + infra firefight

- **Repo split (founder clarification):** `claude-codex` = the communication/protocol layer (contracts, specs, `.agent-comms` semantics); `codex` = the Rust tools only (codex-mcp, codex-poll, codex-doctor, binaries). Route protocol/contract changes → claude-codex; tool implementation → codex. (Engram global 019eb1ff)
- **Don't tie service auth to the desktop keyring.** codex-poll authenticated to GitHub via the gnome-keyring `gh` token; a COSMIC desktop crash cut the systemd service's keyring access → git/gh "could not read Username" → poller silently found "no queued issues" while interactive agents worked fine. Fast fix: `systemctl --user restart codex-poll.timer` once the desktop is back. Durable fix: Infisical universal-auth MACHINE IDENTITY injection (non-interactive, survives crashes, works from a phone). Interactive `infisical login` is NOT durable — its session token is keyring-bound and expires (~7 days). (Engram homelab 019eb39e)
- **Check before you alarm.** Verify root cause with evidence before escalating: SMART was clean (not a dying disk — it was systemd sandbox EROFS); a gh 401 was keyring-context loss, not a dead token; a doctor "fail" was a cosmetic env-visibility check, not an outage. Almost raised a false "disk failing" alarm. (Engram homelab 019eb3b4-9cac)
- **Pace to founder merge throughput.** Surfacing one green draft PR at a time and letting the founder merge beats piling up unmerged PRs (WIP inventory). Hold large phases for an explicit checkpoint. (Engram global 019eb3b4-7afa)
- **Subagent quirk:** when queuing via a subagent, tell it explicitly to USE BASH for the heredoc + queue-agent — one agent wrongly refused, thinking the Write tool was blocked.

## 2026-06-14 — Ask the endpoint, don't infer by observation

Grepped a container filesystem to guess another agent's GitHub capabilities; founder: 'fail moment assuming by observation — directly ask the other endpoints.' Asking returned the truth (and my inference was wrong). Query the source of truth directly when it can answer.

## 2026-06-14 — Don't converge early in multi-agent review

Founder: 'don't agree too early, surface the differences, I want diverse opinions.' Three YES/YES sign-off rounds missed bugs a contrarian-persona reviewer caught immediately. Engineer dissent; ask 'what's overstated/what race remains', not 'do you agree'.

## 2026-06-14 — Track all workstreams durably across sessions

A task assigned in a prior session (plan Substack series) had zero record in this one and silently dropped until founder asked. Agent memory doesn't span sessions; persist every active workstream durably and restate at checkpoints; own continuity gaps as failures.

## 2026-06-14 — Never use an LLM agent to poll/monitor — use a shell loop

A monitoring agent re-invoked each poll replays its ENTIRE context (~125k tokens) every cycle. Watching a counter to 135 cost ~1M+ tokens. RULE: monitoring/waiting = cheap background bash until-loop (until [ cond ]; do sleep N; done) which costs ~0 tokens. Reserve LLM agents for work that needs judgment, not for watching state. When a long job needs supervised restart/stall-detection, the supervisor is a bash watchdog, never an agent that wakes and replays context.

## 2026-06-14 — GPU embed role separation is a HARD policy — MI-50=live, W6800+7900XT=reembed

Founder has stated 3+ times: MI-50 (precision:8007, bge-m3-live) serves Engram LIVE embed queries ONLY; W6800 (precision:8005) and 7900XT (leviathan) are reembed/batch ONLY. It kept getting LOST because it lived only in chat/volatile memory and was never (a) encoded in engram deployment env, (b) a CLAUDE.md hard rule, or (c) a deterministic check. ENGRAM_EMBED_URL must point directly at MI-50:8007, NOT through olla (olla load-balances live onto the wrong cards). Before ANY embed-routing change, run the GPU-role check. Recurring-policy-loss is itself the failure mode: a repeatedly-stated infra policy MUST be encoded in config + a verifier, never just remembered.

## 2026-06-14 — Test vs prod migration-bootstrap divergence (FM-33 instance)

A merged change added a migration file + dependent code; all tests passed because the test harness applies migrations from its OWN list, while the prod boot path (cmd/.../runMigrations) had a separate, unupdated list. Green tests certified nothing about prod (no column -> runtime failure). Fix: single shared migration list + a deterministic test asserting migrations/ dir == the boot list. Always hand-verify the prod boot/runtime path consumes a newly added artifact before deploying.

## 2026-06-14 — Canary needs reachability+creds staged BEFORE the fire (FM-35/36)

Firing a live end-to-end canary failed not on code but on access: the service name resolved only via a hosts-shim on one node (not the agent host); kubectl port-forward to the ClusterIP exits 144 (reaped by the agent harness); SSH-to-another-host to run it is correctly gated. Before attempting a live canary, stage reachability (portable DNS or an in-cluster one-shot runner) and credentials up front, and add a pre-fire reachability probe that fails fast with 'not reachable from here' instead of a confusing mid-enqueue dial error.

## 2026-06-15 — Trust AI direct observations; challenge AI conclusions

Calibrate trust by TYPE of claim. A direct observation (an agent reporting what it read in source/logs/output) is somewhat trustworthy — accept and act, do not re-derive it. A conclusion/assumption (a claim about how a system behaves) is what to challenge and verify, especially load-bearing premises. I over-verified Codex's direct read of the code (body not threaded into launch_coded_issue) the same way I rightly challenged the zero-context reviewer's behavioral premise (supervisord env inheritance). Re-confirming an observation wastes cycles; failing to verify a conclusion ships bugs.

## 2026-06-15 — fleet-dispatch: codex obeys the prefetched GitHub issue BODY over the enqueue ref-prompt

When a fleet-dispatch item runs codex, the harness prefetches the target GitHub issue body into the prompt, and codex treats THAT as the authoritative task definition — overriding a contradicting ref/enqueue prompt. A canary failed to produce its artifact for 3 runs because the target issue body still described the (old) design as analyze-only/no-mutation; the ref-prompt said create-the-file. codex followed the issue body. Fix: the issue body and the enqueue prompt must AGREE; for impl items the issue body must demand the artifact. Founder pinpointed it (previous prompts/limits steering codex), not a sandbox/credential block — codex had danger-full-access.

## 2026-06-16 — Coordinator must filter low-level decisions, not escalate them

During the credential-centralization campaign the founder corrected ~4 times (escalating frustration): stop routing trivial nondestructive decisions for approval; execute on sensible defaults; fan out parallel agents by default for independent work; escalate ONLY destructive/irreversible/prod/data-loss/cost/security-boundary calls. Quote: you are approved to touch all these secrets, that is the point of this plan. Asking permission per-command on already-authorized work is the failure mode for a coordinator persona.

## 2026-06-16 — Subagent used customer project binary (agent-send) in homelab infra script

When building homelab infrastructure scripts, subagents may reach for available binaries on the host without checking whether they belong to customer projects. The secret-sprawl-scan agent called agent-send (from the agent-gateway customer project) for alerting. Correct pattern: homelab cron alerting = exit code + cron mail + log file. Never depend on customer project binaries in homelab scripts. Review any subprocess calls in generated homelab scripts before accepting them.

## 2026-06-18 — Codex ChatGPT auth auto-refreshes — do not flag re-auth on access-token expiry

The Codex container uses ChatGPT device-flow auth (auth.json on the codex-auth volume) which holds a refresh token (tokens + last_refresh). The access_token short expiry (~6 days) auto-refreshes via the refresh token; an expired id_token is normal. Do NOT flag a manual interactive re-auth as a needed founder action based on access-token expiry — it is not required. Founder confirmed auth is already set up and working.

## 2026-06-20 — File GitHub issues for findings — not just plan files / FM catalog

Founder reminder ("Always issues"): bugs and backlog findings must be filed as GitHub Issues (the system of record) before/alongside reporting status, per the CLAUDE.md Bug Accountability rule. During the fleet campaign I leaned on plan files and the failure-mode catalog instead of opening issues; the founder corrected this. Standing behavior: when a bug or actionable finding is discovered, open a tracked GitHub issue in the owning repo, not only a plan-file note.

## 2026-06-21 — Search online when stuck

When debugging an unfamiliar system (e.g. opencode HTTP API), search the web EARLY rather than guessing through redeploy cycles. The x-opencode-directory header fix + the known ?directory= bug (upstream #12271) were found in one searxng query after several failed deploy iterations. Founder flagged this explicitly as a failure mode.

## 2026-06-21 — Never echo secrets even with inline redaction

A sed redaction of a GH_TOKEN in command output FAILED to match the format and leaked the full PAT into the transcript. Do not print env values for secrets at all (not even piped through sed); test presence with ${VAR:+present} ONLY when the surrounding echo cannot concatenate the value. Prefer a boolean check that cannot expand the secret.

## 2026-06-21 — Plan socialization must hit ALL reachable fleet AIs, not just codex+hermes

When socializing a plan (three-way-plan/fleet-plan), dispatch to every reachable fleet worker — codex, grok, AND hermes — not the historical "three-way" pair. Discover reachability via ssh <node> docker ps. Updated both skill rosters (fleet-plan + three-way-plan) to add grok and reframe to "all reachable workers". Grok added genuine value: it cast the lone dissent that turned out mechanistically correct (refuted evidence-first-pack).

## 2026-06-22 — Kill by explicit PID in a separate step; never pkill -f PATTERN in a command that also contains PATTERN

pkill -f PATTERN (and kill of ps|grep PATTERN) self-matches and kills the CURRENT shell — exit 144, command aborts mid-run, leaving the target state ambiguous (sometimes killed, relaunch skipped) — whenever the running command line itself contains PATTERN, e.g. a kill-and-relaunch one-liner (the relaunch carries the binary name) or even the literal grep pattern. Hit 3x this session on longmemeval-s and run_experiment.py. CORRECT METHOD: (1) obtain the PID in a SEPARATE read-only tool call (pgrep -f PATTERN, or ps -eo pid,cmd | grep with the bracket trick); (2) kill the explicit numeric PID; (3) NEVER combine kill-by-pattern with a relaunch (which contains PATTERN) in one command — split into separate tool calls; (4) the bracket trick alone is insufficient if the SAME command contains the literal string elsewhere.

## 2026-06-22 — Eisenhower-charter drift: the field-marshal session kept doing lieutenant work (hands-on Bash) instead of delegating

The orchestrating Opus/Eisenhower session repeatedly ran mechanical hands-on work itself — kill-process commands, fleet enqueues, experiment restarts, prod deploys, log-greps — instead of dispatching Sonnet/Haiku subagents, violating its own charter (coordinates/briefs/synthesizes/adjudicates; does NOT implement; disallowedTools [Edit,Bash]). ROOT CAUSE: Bash is enabled on the MAIN session, so the charter is unenforced and rides on discipline; under momentum the fast self-path beat the doctrinal delegate-path, and per-act this-one-is-quick rationalizations compounded into the commander operating as a lieutenant — burning Opus tokens on mechanical work and producing fumbles (pkill self-kills). GATE (default-to-delegate): before ANY Bash, classify — a genuinely trivial single pre-approved one-liner MAY run inline; anything multi-step, investigative, a deploy, experiment management, or doable by a Sonnet/Haiku agent MUST be dispatched to a subagent. Stand up Bradley (omar-bradley) as the operational shield so the marshal stays strategic. The marshal output is briefs, synthesis, and decisions — not shell commands.

## 2026-06-23 — Grok /imagine can render diagrams, not just photos

Grok /imagine output type is prompt-driven: ask explicitly for a flat-vector technical diagram (labeled boxes, arrows, no photorealism, palette hexes) and it produces clean diagrams. Do not assume it is photo-only or fall back to hand-authored SVG for schematics.

## 2026-06-25 — Default-deny egress is known for this cluster; apply it proactively + verify behavior not pod status

This k8s cluster is default-deny egress (previously discussed). When a pod needs outbound, proactively add the egress allow rule rather than diagnosing/handing off. Never call a service live on 1/1 Ready — verify logs/downstream effect. Symptom of the trap: Ready pod, silent per-cycle outbound failures.

## 2026-06-25 — Persist live cluster fixes to source manifests (GitOps), not just kubectl

Any live kubectl fix (NetworkPolicy, secret, scale, edit) must be written into the GitOps-watched source manifests and merged, else it reverts on reconcile/rebuild. Check for ArgoCD/Flux + ESO before applying live; edit source when managed. A fix is not done until it is in source on the watched branch.

## 2026-06-25 — Do not prompt user to handle Infisical token operations

After updating a secret in Infisical, complete the full deploy cycle (ESO force-sync + kubectl rollout restart) autonomously. Never prompt the user to retrieve, set, or act on an Infisical token — the MCP tool and kubectl are available and sufficient. Prompting is a failure mode.

## 2026-06-25 — Fleet deploy secret-handling: inject via infisical run, pre-name authorized paths in the brief

For claude-codex fleet deploys that need a secret (e.g. GH_TOKEN), inject it with `infisical run --path <p> --env prod -- <deploy-script>` so the value reaches the subprocess env without touching argv, shell history, or a temp file. In the dispatch brief, pre-name the exact authorized Infisical path(s) the deploy script expects so the agent fetches one known secret rather than searching across projects. Confirm deploy success only via non-secret signals (gh auth identity print, container health). Known path for the Grok worker GH_TOKEN: /homelab/fleet/codex.

## 2026-06-25 — Spark (oblivion GB10) has no embed service

The 47-day-old project_oblivion_spark_state.md recorded BAAI/bge-m3 via Infinity on Spark. This is wrong per current infra: embed lives on precision (MI-50:8007 live queries, W6800:8005 batch). Spark is inference-only. Full 128GB is available for the LLM — do not subtract embed budget when sizing models for oblivion.

## 2026-06-26 — Capture the live response before concluding a server-side root cause

On engram #1185 I diagnosed the deployed server as stale and nearly redeployed a container, when the actual bug was a one-line client URL-normalization miss (RestClient kept the /mcp suffix -> POSTed to /mcp/quick-store -> bare-number response). I only confirmed by finally capturing the live /quick-store response, which was correct {ok,id}. Rule: before attributing a failure to a deployed service (and especially before any restart/redeploy), capture the actual response from the exact URL the failing client uses. A cryptic decode error (cannot unmarshal number) points at a wrong endpoint/contract, not necessarily a stale server.

## 2026-06-26 — Manifest-first deploy discipline

Never leave a running container ahead of its source/manifest/image; commit + update the image/manifest before or with any deploy so a rebuild reproduces running state; verify running==manifest after deploy by running ~/bin/check-manifest-drift.sh; cp-into-running-container is banned; drift is a wake-the-founder incident.

## 2026-06-26 — Truncation-to-fit gutted accuracy (LME)

--max-block-chars 2500 forced all items to fit Spark 40960 but dropped user30 strict 87->21%. A suspiciously low score on an easy type implicates context truncation, not model capability. Cap context by lowering topk or use generous max-block-chars (full-session ~8000), never aggressive truncation.

## 2026-06-26 — LME Spark exclusive-lock collisions from overlapping bg jobs

longmemeval run takes an exclusive backend lock; a second overlapping run exits 75 and writes 0/0 garbage. An orchestrator killed mid-flight falls through to its next stage and grabs the lock. Run ONE Spark job at a time; verify-free-before-launch; kill the orchestrator parent, not just its child.

## 2026-06-26 — Biased-subset trap in partial benchmark runs

When items error out (context overflow), scoring only survivors overstates accuracy (survivors are the easier small-context items). Always check scored-total vs N; denominator < N = silent item loss, numbers untrustworthy.

## 2026-06-26 — Codex host auth vs container-volume auth are separate

Re-authing codex on the host or leviathan via 'codex login' updates /home/<user>/.codex/auth.json on that host only. The codex-agent container uses a SEPARATE Docker volume (/home/codex/.codex/auth.json) that host/leviathan re-auth never touches. The fleet-plan relay (ssh host 'codex exec') uses the host bare auth; the codex-attach daemon uses the container volume. Fix the path the failing consumer actually uses.

## 2026-06-26 — Idle install on a shared OAuth account looks healthy but isn't

A ChatGPT/x.ai OAuth account shared across hosts uses rolling single-use refresh tokens; any refresh revokes the others' stored copy. An idle install appears to 'never fail' only because it never exercises (and thus never discovers) its now-stale token. Co-locating a fleet daemon and an ad-hoc relay in one container makes them race on one auth.json.

## 2026-06-26 — Pin Grok CLI model to grok-build, not Composer

Grok CLI offers grok-build (Grok 4.x, xAI) and grok-composer-2.5-fast (Cursor's Composer). A silent default-model swap to Composer changes every Grok review's behavior. Pin default=grok-build in the container config and verify the self-reported model after any Grok update.
