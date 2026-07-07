# Credential Rotation Runbook — engram-go

**Status**: REVISED 2026-05-17 after live attempt revealed the original draft was architecturally wrong (refs #716). Was: ".env is the source of truth." Reality: **Infisical is the source of truth** when configured.
**Last reviewed**: 2026-05-17
**Operator**: Single-developer homelab; no on-call rotation

## Read this first — what actually controls the running service

The container entrypoint is **`/starter`** (`cmd/starter/main.go`), not `/engram` directly. At every container start, starter:

1. Reads `INFISICAL_CLIENT_ID` from env. **If set** (current deployment):
   - Authenticates to Infisical with `INFISICAL_CLIENT_ID` + `INFISICAL_CLIENT_SECRET`
   - Fetches `ENGRAM_API_KEY` from Infisical at path `/apps/engram`, env `prod` — **overwrites** any value `.env` set
   - Fetches `POSTGRES_PASSWORD` from Infisical — **patches `DATABASE_URL`** with the fetched password
   - Strips Infisical creds + `POSTGRES_PASSWORD` from the process env
   - Execs `/engram server` with the patched env
2. **If `INFISICAL_CLIENT_ID` is absent**: starter requires `ENGRAM_API_KEY` to already be in env (from `.env`), skips Infisical entirely, execs `/engram` with the env as-is. `DATABASE_URL` is used unmodified.

So when Infisical is wired (default on this host):

| Credential | Source-of-truth | `.env` matters? |
|---|---|---|
| `POSTGRES_PASSWORD` | **Infisical** | Only for compose `${POSTGRES_PASSWORD}` substitution; starter overwrites the result |
| `ENGRAM_API_KEY` | **Infisical** | Only for the no-Infisical fallback path; ignored when Infisical is configured |
| `INFISICAL_CLIENT_SECRET` | `.env.machine-identity` (this IS the bootstrap secret) | Yes — this is the only one that can't be Infisical-sourced |

Forget the old mental model. **Update Infisical first; everything else is downstream.**

---

## Pre-Flight — read these in order

- [ ] Confirm you are on the host where engram-go runs (`hostname`, `pwd` → `~/projects/engram-go`).
- [ ] Take a Postgres logical backup. Rotation cannot lose data on its own, but a recent backup is cheap insurance:
  ```bash
  mkdir -p ~/backups
  docker exec engram-postgres pg_dump -U engram -d engram --format=custom \
    > ~/backups/engram-prerotation-$(date +%Y%m%d-%H%M%S).dump
  ```
- [ ] Note current bearer token (for the cache-invalidation step):
  ```bash
  CURRENT_KEY=$(grep '^ENGRAM_API_KEY=' .env | cut -d= -f2)
  echo "current key suffix: ...${CURRENT_KEY: -8}"
  ```
- [ ] **Confirm you can reach the Infisical UI**: open `https://infisical.petersimmons.com` and verify you can navigate to **Project → Access Control → Identities → engram-server** AND **Project → Secrets → /apps/engram (env: prod)**.
- [ ] **Schedule a ~5 min window** of unavailability. The MCP server will be down between the `restart` and "healthy" steps.

---

## Rotation 1 — `POSTGRES_PASSWORD` (Infisical → Postgres → restart)

### Steps

1. **Generate the new password**:
   ```bash
   NEW_PG=$(openssl rand -hex 32)
   echo "new password suffix: ...${NEW_PG: -8}"
   ```

2. **Update Infisical first** (UI). Open `https://infisical.petersimmons.com` → Secrets → Path `/apps/engram` → env `prod`:
   - Find `POSTGRES_PASSWORD`, click Edit, paste `$NEW_PG`, Save.
   - **Verify** by clicking Show Value — it should match `$NEW_PG`.

3. **Apply the same value to Postgres** (`ALTER USER` inside the live database):
   ```bash
   echo "ALTER USER engram WITH PASSWORD '$NEW_PG';" \
     | docker exec -i engram-postgres psql -U engram -d engram
   # Expect: ALTER ROLE
   ```

4. **Update `.env` for compose-substitution consistency** (optional but recommended — keeps `docker compose up --force-recreate` from breaking later):
   ```bash
   sed -i.rotation-$(date +%s).bak \
     -E "s|^POSTGRES_PASSWORD=.*|POSTGRES_PASSWORD=${NEW_PG}|" .env
   ```

5. **Restart engram-go and reembed workers** so starter re-fetches from Infisical:
   ```bash
   docker restart engram-go-app engram-reembed-7900xt engram-reembed-w6800
   ```
   (NOT engram-postgres — its password is now controlled by ALTER USER, not its compose env.)

6. **Wait for healthy + verify**:
   ```bash
   # Wait for engram-go to come back healthy
   until [ "$(docker inspect engram-go-app --format '{{.State.Health.Status}}')" = "healthy" ]; do sleep 2; done
   echo "✓ engram-go-app healthy"

   # Confirm no auth errors in the last minute
   docker logs engram-go-app --tail 30 --since 1m 2>&1 \
     | grep -iE "password authentication failed|fatal.*role" \
     && echo "FAIL: auth errors" || echo "✓ no auth errors"

   # Confirm reembed workers happy
   for c in engram-reembed-7900xt engram-reembed-w6800; do
     docker logs "$c" --tail 30 --since 1m 2>&1 \
       | grep -iE "password authentication failed|fatal.*role" \
       && echo "FAIL: $c" || echo "✓ $c clean"
   done
   ```

### Rollback (only if step 6 fails)

The new password is in BOTH Infisical and Postgres. If something else broke:
```bash
OLD_PG=$(grep '^POSTGRES_PASSWORD=' .env.rotation-*.bak | tail -1 | cut -d= -f2)
# Reverse Infisical UI update first (paste OLD_PG back)
# Then:
echo "ALTER USER engram WITH PASSWORD '$OLD_PG';" \
  | docker exec -i engram-postgres psql -U engram -d engram
sed -i -E "s|^POSTGRES_PASSWORD=.*|POSTGRES_PASSWORD=${OLD_PG}|" .env
docker restart engram-go-app engram-reembed-7900xt engram-reembed-w6800
```

---

## Rotation 2 — `ENGRAM_API_KEY` (Infisical → engram → MCP clients)

The bearer is consumed by every MCP client (Claude Code, Codex, Opencode). Rotation invalidates every cached client connection at once.

### Steps

1. **Identify every MCP client** with the current key cached:
   - `~/.claude.json` → `mcpServers.engram.headers.Authorization`
   - `~/.config/codex/config.toml` if used
   - `~/.config/opencode/*` if used

2. **Generate the new key**:
   ```bash
   NEW_KEY=$(openssl rand -hex 32)
   ```

3. **Update Infisical** (UI): Secrets → `/apps/engram` → env `prod` → `ENGRAM_API_KEY` → Edit → paste `$NEW_KEY` → Save.

4. **Update `.env` + local backup** (for the no-Infisical fallback path + tooling consistency):
   ```bash
   sed -i.rotation-$(date +%s).bak \
     -E "s|^ENGRAM_API_KEY=.*|ENGRAM_API_KEY=${NEW_KEY}|" .env
   echo "${NEW_KEY}" > ~/.config/engram/api_key && chmod 0600 ~/.config/engram/api_key
   ```

5. **Restart engram-go** so starter re-fetches:
   ```bash
   docker restart engram-go-app
   until [ "$(docker inspect engram-go-app --format '{{.State.Health.Status}}')" = "healthy" ]; do sleep 2; done
   ```

6. **Confirm the new key authenticates and the old one is rejected**:
   ```bash
   # New key works against /setup-token (bearer-gated)
   curl -s -o /dev/null -w "new key: HTTP %{http_code}\n" \
     -H "Authorization: Bearer ${NEW_KEY}" http://localhost:8788/setup-token
   # Expect: HTTP 200 (or HTTP 429 if rate-limited — wait 5 min and retry)

   # Old key rejected
   OLD_KEY=$(grep '^ENGRAM_API_KEY=' .env.rotation-*.bak | tail -1 | cut -d= -f2)
   curl -s -o /dev/null -w "old key: HTTP %{http_code}\n" \
     -H "Authorization: Bearer ${OLD_KEY}" http://localhost:8788/setup-token
   # Expect: HTTP 401
   ```

7. **Push the new key to MCP clients**:
   ```bash
   make setup
   # cmd/engram-setup probes /setup-token using the local .env bearer and
   # rewrites ~/.claude.json mcpServers.engram.headers.Authorization
   ```
   `/setup-token` is rate-limited to 3 req per 5 min per IP — don't re-run inside that window.

8. **In Claude Code** (and any other live IDE session): run `/mcp` to reconnect with the new key.

### Rollback (only if step 6 or 7 fails)

```bash
OLD_KEY=$(grep '^ENGRAM_API_KEY=' .env.rotation-*.bak | tail -1 | cut -d= -f2)
# Reverse Infisical UI update first
# Then:
sed -i -E "s|^ENGRAM_API_KEY=.*|ENGRAM_API_KEY=${OLD_KEY}|" .env
echo "${OLD_KEY}" > ~/.config/engram/api_key
docker restart engram-go-app
```

---

## Rotation 3 — `INFISICAL_CLIENT_SECRET` (bootstrap secret; irreversible step)

This authenticates engram-go's starter TO Infisical. It is the only secret that cannot itself be Infisical-sourced — it's the bootstrap. **Highest blast radius**; do this only when alert.

### Steps

1. **Log into Infisical UI**: `https://infisical.petersimmons.com`
2. **Navigate**: Project → Access Control → Identities → `engram-server`
3. **Add a NEW client secret**:
   - Click "Add Client Secret" — note the new value (it is shown once).
   - **Do not delete the old client secret yet.** Both are valid simultaneously.
4. **Update `.env.machine-identity`**:
   ```bash
   sed -i.rotation-$(date +%s).bak \
     -E "s|^INFISICAL_CLIENT_SECRET=.*|INFISICAL_CLIENT_SECRET=<paste-new-value>|" \
     .env.machine-identity
   ```
5. **Restart engram-go** so starter re-authenticates to Infisical with the new client secret:
   ```bash
   docker restart engram-go-app
   docker logs engram-go-app --tail 30 --since 1m 2>&1 | grep -i infisical
   # Expect: no auth errors. starter should succeed at fetching ENGRAM_API_KEY +
   # POSTGRES_PASSWORD using the new client secret.
   ```
6. **Verify end-to-end with a real Infisical-sourced operation**:
   ```bash
   # Bounce the container one more time and watch it start cleanly
   docker restart engram-go-app
   until [ "$(docker inspect engram-go-app --format '{{.State.Health.Status}}')" = "healthy" ]; do sleep 2; done
   ```
7. **Delete the OLD client secret in Infisical UI** ← **irreversible**:
   - Same Identity page → click the trash icon next to the old secret.
   - This is the irreversible step. Do not skip — leaving the old secret valid is the entire point of rotating.
8. **Re-verify** after the delete:
   ```bash
   docker restart engram-go-app
   until [ "$(docker inspect engram-go-app --format '{{.State.Health.Status}}')" = "healthy" ]; do sleep 2; done
   # If this fails, it means engram-go was running on the old secret cached
   # somewhere — generate a third secret in Infisical UI and paste in, restart.
   ```

### Rollback (only possible BEFORE step 7)

```bash
# Revert the file to the old secret
mv .env.machine-identity.rotation-*.bak .env.machine-identity
docker restart engram-go-app
# In Infisical UI: optionally delete the NEW secret you added in step 3
```

**After step 7 is irrecoverable.** Mitigation: generate a third secret in Infisical UI, paste into `.env.machine-identity`, restart. Treat as the new baseline.

---

## Invalidation — `.env.bak.*` files

After Rotations 1–2 complete and verify cleanly, the keys in `.env.bak.*` are stale.

### Steps

1. **Confirm the stale key in `.env.bak.1777969578` is rejected**:
   ```bash
   STALE=$(grep '^ENGRAM_API_KEY=' .env.bak.1777969578 | cut -d= -f2)
   curl -s -o /dev/null -w "stale key: HTTP %{http_code}\n" \
     -H "Authorization: Bearer ${STALE}" http://localhost:8788/setup-token
   # Expect: HTTP 401
   ```
2. **Shred** the file:
   ```bash
   shred -u .env.bak.1777969578
   ```
3. **Audit** the directory for any other stale `.env.bak.*`:
   ```bash
   ls -la .env.bak.* 2>/dev/null
   # For each, repeat steps 1-2
   ```

---

## Cleanup

After all rotations + invalidation succeed:

```bash
cd ~/projects/engram-go

# Remove one-shot rotation backups
rm -f .env.rotation-*.bak .env.machine-identity.rotation-*.bak

# Confirm no .env.bak.* files remain
ls -la .env.bak.* 2>/dev/null && echo "WARNING: backups still present"

# Confirm git working tree clean of secret files
git status -s | grep -E "\.env(\.|$)" && echo "WARNING: a .env file is tracked" || echo "✓ .env files properly ignored"
```

---

## Post-Rotation Verification (end-to-end)

All of these must pass before marking #657 closeable:

```bash
NEW_KEY=$(grep '^ENGRAM_API_KEY=' .env | cut -d= -f2)

# 1. New bearer accepted at /setup-token
curl -s -o /dev/null -w "HTTP %{http_code}\n" \
  -H "Authorization: Bearer ${NEW_KEY}" http://localhost:8788/setup-token
# Expect: 200 (or 429 if hit rate limit — wait 5 min and retry)

# 2. No auth errors in any container
for c in engram-go-app engram-postgres engram-reembed-7900xt engram-reembed-w6800; do
  docker logs "$c" --tail 50 --since 5m 2>&1 \
    | grep -iE "password authentication failed|fatal.*role|infisical.*(401|unauthorized)" \
    && echo "FAIL: $c" || echo "✓ $c clean"
done

# 3. Canary store + recall via MCP (in Claude Code, after running /mcp):
#    memory_quick_store "rotation canary $(date -u +%FT%TZ)" project=global
#    memory_recall "rotation canary" project=global
```

---

## Failure Mode Cheat Sheet

| Symptom | Likely cause | Action |
|---|---|---|
| engram-go-app crash-loops with `password authentication failed for user "engram"` after Rotation 1 | Infisical and Postgres password don't match | Reconcile: confirm Infisical's POSTGRES_PASSWORD matches the latest ALTER USER. Restart engram-go-app. |
| MCP client returns 401 after Rotation 2 | Client has the old key cached | Re-run `make setup`; in Claude Code run `/mcp` to reconnect. |
| starter fails at "infisical auth" after Rotation 3 step 7 | The old client secret was deleted before the running container had finished switching to the new one | Generate a third secret in Infisical UI, paste into `.env.machine-identity`, restart. The "two valid at once" window protects against this — don't skip it. |
| Reembed workers loop on auth errors | The container didn't pick up the new Infisical value (rare) | `docker compose up -d --force-recreate engram-reembed-*` |
| `make setup` returns 429 | `/setup-token` rate-limited (3 req per 5 min per IP) | Wait 5 minutes, retry. |
| Old bearer still authenticates after Rotation 2 | The starter cached the old value; container hasn't restarted with new Infisical state | `docker restart engram-go-app` and re-verify. |

---

## Schedule

- **POSTGRES_PASSWORD**: every 90 days, or immediately on any suspected leak.
- **ENGRAM_API_KEY**: every 90 days, or immediately on any suspected leak.
- **INFISICAL_CLIENT_SECRET**: every 180 days, or immediately on any suspected leak.

Track next rotation date in `~/projects/engram-go/docs/operations.md` "Maintenance Schedule" section.

---

## What this runbook does NOT cover

- **engram-postgres's own POSTGRES_PASSWORD env**: set via `environment:` in `docker-compose.yml`. Postgres only honors this on FIRST init (empty data dir); subsequent runs use whatever is stored in pg_authid. Normal rotation does NOT recreate the postgres container, so this env var is effectively cosmetic after first deploy.
- **Disaster recovery from a deleted Infisical project**: if Infisical itself is lost, the bootstrap secret in `.env.machine-identity` is your only path back; restore Infisical from its own backup before attempting any rotation.
- **Multi-host setups** (e.g., the oblivion reembed worker on a different machine). Each host has its own `.env.machine-identity` and must be rotated independently.

---

## History

- **2026-05-17**: Original draft assumed `.env` was source-of-truth. Live rotation attempt failed at Rotation 1 step 5 (engram-go crash-looped with `password authentication failed`) because the starter wrapper fetches the secret from Infisical, not from `.env`. Rolled back cleanly via ALTER USER. Runbook revised (this version). See issue #716.
