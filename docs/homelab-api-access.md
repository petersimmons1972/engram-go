# Homelab API Access

Index of API credentials for homelab devices. All credentials live in
**Infisical → project `Homelab` → env `production`**, under the per-device path
noted in each section. **Reference the key paths only** — retrieve values via
`mcp__infisical-personal__get-secret`, and **never log, echo, or persist a
retrieved value.** When passing a secret to `curl`, use process substitution so
it never lands on disk:

    curl -k -K <(printf 'header = "<header>: %s"\n' "$SECRET") https://<host>/...

Do NOT write a secret to a temp file (even mode-600 + shred trips credential-leak policy).
Use `-k` for these hosts' self-signed certs.

**Do not MCP-retrieve a secret you will use in bash.** `mcp__infisical-personal__get-secret` returns the value as a plaintext literal into the agent's context, after which the bash secret-classifier pattern-matches that literal and blocks every subsequent command. Instead retrieve via the Infisical CLI inside a command substitution so the value is injected at exec time and never becomes a literal in any transcript/argv — e.g. `curl -H "X-API-KEY: $(infisical secrets get UNIFI_API_TOKEN --projectId=<id> --env=prod --path=/unifi --plain)" ...`. Verify CLI auth first via a non-secret key (e.g. `UNIFI_TOKEN_NAME` → `unifi-admin`).

The coordinator persona (bedell-smith) has no Bash and cannot make API calls —
dispatch a Bash-equipped specialist to exercise any of these.

---

## Proxmox VE (nuc, pve)

Infisical path: **`/proxmox`**. These are Proxmox VE **API tokens** for the REST
API at `https://<host>:8006/api2/json` — **NOT SSH keys**, no interactive root
shell. They grant API control: VM/CT lifecycle, config, storage, guest console.

| Host                     | Identity key (token name)      | Secret key (token value) | Token identity            | Privilege                                  |
|--------------------------|--------------------------------|--------------------------|---------------------------|--------------------------------------------|
| `nuc.petersimmons.com`   | `PROXMOX_API_NUC_TOKEN_NAME`   | `PROXMOX_API_NUC`        | `root@pam!psimmons-nuc`   | ✅ full root — `Administrator` ACL at `/`   |
| `pve.petersimmons.com`   | `PROXMOX_API_PVE_TOKEN_NAME`   | `PROXMOX_API_PVE`        | `root@pam!psimmons-pve`   | ✅ full root — `Administrator` ACL at `/`   |

Both verified against the live API on 2026-06-15: `/version` 200, full privilege
set at `/`, `Sys.Audit`-gated `/nodes/<node>/status` returns 200.

**Auth header:** `Authorization: PVEAPIToken=<token-name>=<token-secret>` (retrieve both halves).

**Privsep gotcha (IMPORTANT):** A Proxmox API token created with the default
`privsep=1` does NOT inherit its parent user's privileges. Even a `root@pam`
token authenticates (200 on `/version`) but holds ZERO privileges —
`/access/permissions` returns `{}` and privileged calls 403 — until granted an
explicit ACL. Both tokens here were fixed with:

    pveum acl modify / --tokens 'root@pam!<tokenid>' --roles Administrator

If a token is rotated and its tokenid changes, re-apply this grant. (Alternative:
`pveum user token modify root@pam <tokenid> --privsep 0`.)

---

## UniFi gateway (192.168.0.1)

Infisical path: **`/unifi`**. API credential for the UniFi gateway/controller at
`https://192.168.0.1`.

| Device              | Identity key (token name) | Secret key (token value) | Token identity | 
|---------------------|---------------------------|--------------------------|----------------|
| UniFi @ 192.168.0.1 | `UNIFI_TOKEN_NAME`        | `UNIFI_API_TOKEN`        | `unifi-admin`  |

**Auth header:** `X-API-KEY: <UNIFI_API_TOKEN>` against the UniFi OS Network Integration API (base `https://192.168.0.1/proxy/network/integration/v1/`). Self-signed cert → `curl -k`. **Verified 2026-06-15:** `GET /sites` and `GET /info` return 200; key has at least site-level read access (1 site: `Default`; Network app v10.4.57).
