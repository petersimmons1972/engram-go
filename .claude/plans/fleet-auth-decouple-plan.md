# Plan — Decouple Fleet AI Consumers from Rolling-OAuth (FM-125 fix)

**Date:** 2026-06-26 · **Owner:** Claude-direct (founder-gated infra) · **Tracks:** FM-125
**Why Claude-direct:** touches Infisical secrets, container env, and fleet redeploy — all
founder-confirmation territory; the codex/grok containers lack the toolchain to self-verify.

## Auth path map

Codex has **TWO independent auth files**: (a) **HOST bare** `/home/psimmons/.codex/auth.json` on `codex.petersimmons.com` — used by the fleet-plan RELAY (`ssh host 'codex exec'`) and updated by interactive `codex login` on the host; (b) **CONTAINER volume** `/home/codex/.codex/auth.json` (Docker volume, user codex) — used by the codex-attach DAEMON, NOT updated by host/leviathan re-auth. Phase 1 must target the correct path per consumer: the relay reads the host file; the daemon reads the container volume. The API-key (or stopgap auth-sync) for the daemon must write the CONTAINER volume. Verify the host bare auth.json mtime before cutover (a prior diagnostic read it as Jun 23 despite a same-day host re-auth — confirm whether `codex login` wrote it or wrote elsewhere).

## Problem
All three Codex installs share ONE ChatGPT OAuth account (`account_id 9e886632…`,
`auth_mode: chatgpt`, no API key): leviathan (workstation), the `codex.petersimmons.com`
host bare path, and the `codex-agent` container — each with its own `auth.json`. ChatGPT
OAuth uses **rolling single-use refresh tokens**: any refresh anywhere rotates the token and
revokes the other two stores' copies. Re-authing leviathan today is what invalidated the
fleet host. Grok has the same disease via a different shape: the `grok-agent` container runs
the daemon (`GROK_ATTACH_ENABLED=1`) AND the relay `docker exec … grok --single` in the same
container, racing on one OAuth `auth.json`; the no-TTY loser hangs on the device-code flow.
`codex exec --ephemeral` compounds Codex by reading the rotated token but never writing it
back. (Full root cause: FM-125.)

## Goal
Fleet **service consumers** authenticate with **stable API keys** (`OPENAI_API_KEY` /
`XAI_API_KEY`) from Infisical, injected as container env — API keys do not rotate, so the
mutual-revocation race ends. **Leviathan keeps interactive ChatGPT OAuth, untouched.**

## Relevant
- `~/projects/aifleet/` codex-attach / grok-attach scripts + container env-files
- `codex-poll.sh` and the `fleet-plan` skill registry (both use `--ephemeral`)
- Infisical: Homelab/production, fleet secret paths
- FM-125 in `~/docs/failure-modes-standard.md`

## Phases (each ends with verification; do not advance until it passes)

### Phase 1 — Codex onto API key
- [ ] Provision an OpenAI **service-account** `OPENAI_API_KEY` (separate from your personal
      ChatGPT account) → store in Infisical (reference the key path, never the value).
- [ ] Inject into the `codex-agent` container env-file AND the `codex.petersimmons.com` host
      relay environment; confirm codex resolves `auth_mode` to API-key (not `chatgpt` OAuth).
- [ ] Drop `--ephemeral` from the `fleet-plan` skill registry relay command and `codex-poll.sh`
      (belt-and-suspenders; with API-key auth there is no refresh token to lose, but --ephemeral
      is no longer load-bearing and was masking the write-back issue).
- [ ] **Verify:** run a relay `codex exec` (no --ephemeral) TWICE in a row from the fleet host
      → second call must NOT return `refresh_token_invalidated`. Confirm leviathan
      `codex login status` still reports logged-in afterward (proves decoupling).

### Phase 2 — Grok onto API key + model pin
- [ ] Provision `XAI_API_KEY` → Infisical; inject into the `grok-agent` container env-file
      (`grok-attach` already supports `XAI_API_KEY`, currently `NOT_SET`).
- [ ] **Pin the model to `grok`, NOT the new Composer model** (founder reset this 2026-06-26;
      a silent swap to Composer changes every Grok review's behavior). Capture the model setting
      in the container env / config so a redeploy reproduces it.
- [ ] **Verify:** run a relay `grok --single` WHILE the daemon is processing a task → must
      return a result with no device-code prompt and no hang (API keys don't rotate, so daemon
      + relay no longer race). Confirm the responding model identifies as grok, not composer.

### Phase 3 — Hygiene + manifest-first
- [ ] If any OAuth path must remain anywhere, ensure the relay never co-runs in the same
      container as a daemon on a shared `auth.json` (separate container or serialize).
- [ ] Manifest-first: commit all env-file/manifest changes so a rebuild/redeploy reproduces
      the API-key state exactly; run `~/bin/check-manifest-drift.sh` → assert PASS.
- [ ] Update FM-125 with the resolution (API-key cutover landed).
- [ ] **Verify:** full round-trip — `fleet-canary.sh` (or a consult to each harness) green,
      twice, with no auth churn.

## Validation (whole-plan)
- [ ] Codex relay survives back-to-back calls (no token invalidation).
- [ ] Grok relay survives concurrent daemon activity (no device-flow hang) and answers as grok.
- [ ] leviathan interactive Codex unaffected.
- [ ] manifest-drift PASS; FM-125 marked resolved.

## Notes / open
- Founder-gated steps: Infisical secret creation, container env edits, fleet redeploy, any push.
- Open Q (Phase 1): does codex v0.140.0 cleanly prefer `OPENAI_API_KEY` over an existing
  `chatgpt` `auth.json` in the same `~/.codex`, or must the OAuth `auth.json` be removed/renamed
  in the container for API-key mode to take? Verify before cutover (don't delete leviathan's).
