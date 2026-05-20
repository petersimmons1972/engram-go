# Lessons Learned

Per CLAUDE.md: append a dated entry whenever the user corrects course or reframes a problem.

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
