# Hermes Agent Installation Plan — hermes.petersimmons.com

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deploy Hermes Agent (Nous Research v0.14.0) on hermes.petersimmons.com inside a Chainguard-hardened Docker container, running as UID 65532 (nonroot), with Discord gateway, Docker-socket sandbox execution, and all credentials stored in Infisical project `hermes`.

**Architecture:** Hermes Agent runs as a Chainguard wolfi-base container (nonroot UID 65532). For code execution sandboxes, it mounts the host's Docker socket so it can spawn isolated sub-containers (`terminal.backend: docker`). The host `hermes` Linux user (UID 65532) owns all config volumes and is in the `docker` group — this aligns the host UID and container UID so volume permissions and Docker socket GID work without root tricks. All gateway-facing secrets live in Infisical personal/hermes; deployment context in Engram project `hermes`.

**Tech Stack:** Ubuntu 24.04.4 LTS · Docker CE (host daemon) · Chainguard `python:latest-dev` (build) + `wolfi-base` (runtime) · hermes-agent[messaging] 0.14.0 (pip) · docker-compose · ed25519 SSH key · Infisical MCP (personal) · Engram MCP

---

## Target Host Facts (confirmed)
- **Host**: hermes.petersimmons.com
- **OS**: Ubuntu 24.04.4 LTS (Noble), kernel 6.8.0-117-generic
- **CPU**: 8× Intel Xeon E5-2690 v2 @ 3.00GHz | **RAM**: 15 GiB | **Disk**: 989 GiB free
- **Python**: 3.12.3 installed · pip3 NOT installed · Docker NOT installed
- **psimmons**: NOPASSWD sudo (our install account)
- **Listening**: SSH :22 only

---

## Security Architecture

```
Internet
  │
  ▼ (Discord only — no inbound ports opened)
hermes.petersimmons.com (VM — dedicated, no other services)
  │
  └─ docker daemon (host, runs as root BUT rootless-mode-opt available)
       │
       └─ hermes-agent container (UID 65532, no new privs, caps: dropped ALL)
            │  reads ~/.hermes/.env  (chmod 600, owned 65532)
            │  reads ~/.hermes/config.yaml
            │  mounts /var/run/docker.sock (Docker socket — security tradeoff, see §Notes)
            │
            └─ sandbox containers (spawned per task, cap_drop ALL, no-new-privs,
                                   pids-limit 256, tmpfs /tmp, resource-limited)
```

**Security tradeoff documented:** The Docker socket mount gives the hermes container root-equivalent access to the host Docker daemon. This is mitigated by: (1) the VM is dedicated — no other workloads; (2) UFW blocks all inbound ports; (3) the hermes user cannot SSH or escalate beyond Docker; (4) the container itself has all capabilities dropped and no-new-privileges. This is the standard Hermes production pattern per official docs.

---

## File Map

| Location | Purpose |
|----------|---------|
| `/home/hermes/.hermes/config.yaml` | Hermes Agent config (Docker backend, Discord, limits) |
| `/home/hermes/.hermes/.env` | Secrets (Discord token, LLM API key) — chmod 600 |
| `/home/hermes/.hermes/logs/` | Agent logs |
| `/home/hermes/.hermes/sandboxes/` | Docker sandbox workspaces (bind-mounted into sub-containers) |
| `/home/hermes/workspace/` | Default working directory for agent tasks |
| `/home/hermes/deploy/Dockerfile` | Chainguard image definition |
| `/home/hermes/deploy/docker-compose.yaml` | Compose stack definition |
| `/home/hermes/deploy/.env` | Compose env (DOCKER_GID) |
| `/home/hermes/.ssh/authorized_keys` | hermes SSH access |

---

## Task 1: Create Infisical Project "hermes" + Seed Engram

Create the credential store before generating any secrets.

- [ ] **Step 1.1: Create Infisical project**

  Use `mcp__infisical-personal__create-project`:
  ```
  name: "hermes"
  slug: "hermes"
  ```
  Note the returned project ID.

- [ ] **Step 1.2: Create production environment**

  Use `mcp__infisical-personal__create-environment`:
  ```
  projectId: <id from 1.1>
  name: "production"
  slug: "prod"
  ```

- [ ] **Step 1.3: Create placeholder secrets (filled during later tasks)**

  Use `mcp__infisical-personal__create-secret` for each (environment=prod, secretPath=/):
  ```
  HERMES_LINUX_PASSWORD      = "PLACEHOLDER"
  HERMES_SSH_PRIVATE_KEY     = "PLACEHOLDER"
  HERMES_DISCORD_TOKEN       = "PLACEHOLDER — user fills before gateway setup"
  HERMES_LLM_API_KEY         = "PLACEHOLDER — user fills before hermes model setup"
  HERMES_DISCORD_USER_ID     = "PLACEHOLDER — numeric Discord user ID for allowlist"
  ```

- [ ] **Step 1.4: Seed Engram project "hermes"**

  Use `mcp__engram__memory_store`:
  ```json
  {
    "project": "hermes",
    "type": "context",
    "content": "Hermes Agent deployed on hermes.petersimmons.com (Ubuntu 24.04.4, 8 CPU, 15GiB RAM, 989GiB disk). Chainguard wolfi-base container, UID 65532. Terminal backend: Docker socket. Discord gateway, DISCORD_ALLOWED_USERS allowlist (fail-closed). Credentials in Infisical personal/hermes/prod. Compose stack at /home/hermes/deploy/. hermes Linux user UID 65532.",
    "tags": ["hermes-agent", "discord", "docker", "chainguard", "hermes.petersimmons.com"]
  }
  ```

- [ ] **Step 1.5: Verify**

  Use `mcp__infisical-personal__list-projects` — confirm "hermes" appears.

---

## Task 2: Create hermes System User (UID 65532)

Using UID 65532 (Chainguard nonroot standard) ensures the container UID and host UID match, so volume mounts have correct ownership without root manipulation.

- [ ] **Step 2.1: Create user with explicit UID 65532**

  ```bash
  ssh psimmons@hermes.petersimmons.com "sudo adduser --uid 65532 --disabled-password --gecos 'Hermes Agent' hermes"
  ```
  Expected: `Adding user 'hermes'...` — home at `/home/hermes`.

- [ ] **Step 2.2: Generate a secure random password**

  ```bash
  HERMES_PASS=$(openssl rand -base64 32)
  echo "Generated password (store this): $HERMES_PASS"
  ```

- [ ] **Step 2.3: Set the password**

  ```bash
  ssh psimmons@hermes.petersimmons.com "echo 'hermes:${HERMES_PASS}' | sudo chpasswd"
  ```

- [ ] **Step 2.4: TEST — hermes must have NO sudo access**

  ```bash
  ssh psimmons@hermes.petersimmons.com "sudo -u hermes sudo -l 2>&1"
  ```
  Expected output contains: `not allowed to run sudo` or `Sorry, user hermes may not run sudo`.
  **If hermes CAN sudo — STOP. Fix before continuing.**

- [ ] **Step 2.5: Verify UID is 65532**

  ```bash
  ssh psimmons@hermes.petersimmons.com "id hermes"
  ```
  Expected: `uid=65532(hermes) gid=65532(hermes)`.

- [ ] **Step 2.6: Store password in Infisical**

  Use `mcp__infisical-personal__update-secret`:
  ```
  projectId: hermes, environment: prod, secretPath: /
  secretKey: HERMES_LINUX_PASSWORD
  secretValue: <value from step 2.2>
  ```

---

## Task 3: Generate SSH Keypair for hermes User

- [ ] **Step 3.1: Generate ed25519 keypair locally**

  ```bash
  ssh-keygen -t ed25519 -f ~/.ssh/hermes_agent_key -N "" -C "hermes@hermes.petersimmons.com"
  ```
  Creates: `~/.ssh/hermes_agent_key` (private) and `~/.ssh/hermes_agent_key.pub`.

- [ ] **Step 3.2: Install public key**

  ```bash
  PUBKEY=$(cat ~/.ssh/hermes_agent_key.pub)
  ssh psimmons@hermes.petersimmons.com \
    "sudo -u hermes mkdir -p /home/hermes/.ssh && \
     echo '$PUBKEY' | sudo -u hermes tee /home/hermes/.ssh/authorized_keys && \
     sudo -u hermes chmod 700 /home/hermes/.ssh && \
     sudo -u hermes chmod 600 /home/hermes/.ssh/authorized_keys"
  ```

- [ ] **Step 3.3: TEST — SSH as hermes works**

  ```bash
  ssh -i ~/.ssh/hermes_agent_key hermes@hermes.petersimmons.com "id && echo SSH_OK"
  ```
  Expected: `uid=65532(hermes)...` and `SSH_OK`.
  **If SSH fails — stop. Check authorized_keys ownership before continuing.**

- [ ] **Step 3.4: Store private key in Infisical**

  ```bash
  PRIVKEY=$(cat ~/.ssh/hermes_agent_key)
  ```
  Use `mcp__infisical-personal__update-secret`:
  ```
  secretKey: HERMES_SSH_PRIVATE_KEY
  secretValue: <full contents of ~/.ssh/hermes_agent_key>
  ```

---

## Task 4: Install Docker CE on hermes.petersimmons.com

Executed as psimmons (has sudo).

- [ ] **Step 4.1: Install Docker prerequisites**

  ```bash
  ssh psimmons@hermes.petersimmons.com \
    "sudo apt-get update -q && sudo apt-get install -y ca-certificates curl"
  ```

- [ ] **Step 4.2: Add Docker GPG key and repository**

  ```bash
  ssh psimmons@hermes.petersimmons.com "sudo install -m 0755 -d /etc/apt/keyrings && \
    sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc && \
    sudo chmod a+r /etc/apt/keyrings/docker.asc && \
    echo \"deb [arch=\$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu \$(. /etc/os-release && echo \"\$VERSION_CODENAME\") stable\" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null"
  ```

- [ ] **Step 4.3: Install Docker CE and Compose plugin**

  ```bash
  ssh psimmons@hermes.petersimmons.com \
    "sudo apt-get update -q && sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin"
  ```

- [ ] **Step 4.4: Add hermes (UID 65532) to docker group**

  ```bash
  ssh psimmons@hermes.petersimmons.com "sudo usermod -aG docker hermes"
  ```

- [ ] **Step 4.5: TEST — Docker daemon running**

  ```bash
  ssh psimmons@hermes.petersimmons.com "sudo docker run --rm hello-world 2>&1 | grep 'Hello from Docker'"
  ```
  Expected: `Hello from Docker!`

- [ ] **Step 4.6: TEST — hermes user can run Docker**

  ```bash
  ssh -i ~/.ssh/hermes_agent_key hermes@hermes.petersimmons.com \
    "docker run --rm hello-world 2>&1 | grep 'Hello from Docker'"
  ```
  Expected: `Hello from Docker!`
  If `permission denied on /var/run/docker.sock`: run `newgrp docker` in the SSH session — the group membership requires a new login. Log out and back in.

- [ ] **Step 4.7: Note Docker GID (needed for Compose)**

  ```bash
  DOCKER_GID=$(ssh psimmons@hermes.petersimmons.com "getent group docker | cut -d: -f3")
  echo "Docker GID: $DOCKER_GID"
  ```

---

## Task 5: Write Dockerfile (Chainguard-Hardened)

Files go in `/home/hermes/deploy/` on the remote host.

- [ ] **Step 5.1: Create deploy directory**

  ```bash
  ssh -i ~/.ssh/hermes_agent_key hermes@hermes.petersimmons.com "mkdir -p ~/deploy"
  ```

- [ ] **Step 5.2: Write Dockerfile**

  ```bash
  ssh -i ~/.ssh/hermes_agent_key hermes@hermes.petersimmons.com "cat > ~/deploy/Dockerfile" << 'DOCKERFILE'
  # ──────────────────────────────────────────────────────────────────────────────
  # Stage 1: Build — install hermes-agent into a venv
  # ──────────────────────────────────────────────────────────────────────────────
  FROM cgr.dev/chainguard/python:latest-dev AS build

  WORKDIR /app

  # Create venv
  RUN python -m venv /app/venv
  ENV PATH="/app/venv/bin:$PATH"

  # Install hermes-agent with messaging support (Discord/Telegram/Slack)
  RUN pip install --no-cache-dir "hermes-agent[messaging]==0.14.0"

  # ──────────────────────────────────────────────────────────────────────────────
  # Stage 2: Runtime — minimal Chainguard wolfi-base
  # ──────────────────────────────────────────────────────────────────────────────
  FROM cgr.dev/chainguard/wolfi-base:latest AS runtime

  # Runtime deps: Python 3.12, git (for hermes update/skills), openssh-client
  # (SSH backend fallback), Node.js 22 (hermes web features), tini (PID 1),
  # ripgrep (hermes code search tools)
  RUN apk add --no-cache \
      python-3.12 \
      git \
      openssh-client \
      nodejs-22 \
      tini \
      ca-certificates-bundle \
      ripgrep

  # Copy venv from build stage
  COPY --from=build /app/venv /app/venv
  ENV PATH="/app/venv/bin:$PATH"
  ENV HOME=/home/hermes

  # Config and workspace dirs will be volume-mounted; create mount points
  # with nonroot ownership so the container can write to them
  RUN mkdir -p /home/hermes/.hermes /home/hermes/workspace \
      && chown -R 65532:65532 /home/hermes

  # Chainguard nonroot UID
  USER 65532

  # Health check — verifies hermes binary responds
  HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
      CMD ["hermes", "--version"]

  ENTRYPOINT ["/sbin/tini", "--"]
  CMD ["hermes", "gateway", "start"]
  DOCKERFILE
  ```

- [ ] **Step 5.3: TEST — Dockerfile syntax (dry build, no push)**

  ```bash
  ssh -i ~/.ssh/hermes_agent_key hermes@hermes.petersimmons.com \
    "cd ~/deploy && docker build --no-cache -t hermes-agent:latest . 2>&1 | tail -20"
  ```
  Expected: `Successfully built <id>` or `writing image sha256:...` — no errors.

- [ ] **Step 5.4: TEST — Container runs as UID 65532**

  ```bash
  ssh -i ~/.ssh/hermes_agent_key hermes@hermes.petersimmons.com \
    "docker run --rm hermes-agent:latest id"
  ```
  Expected: `uid=65532 gid=65532`

- [ ] **Step 5.5: TEST — hermes binary present in image**

  ```bash
  ssh -i ~/.ssh/hermes_agent_key hermes@hermes.petersimmons.com \
    "docker run --rm hermes-agent:latest hermes --version"
  ```
  Expected: Version string.

---

## Task 6: Write docker-compose.yaml

- [ ] **Step 6.1: Create compose .env with Docker GID**

  ```bash
  # Replace <DOCKER_GID> with the value from Task 4.7
  ssh -i ~/.ssh/hermes_agent_key hermes@hermes.petersimmons.com \
    "echo 'DOCKER_GID=<DOCKER_GID>' > ~/deploy/.env && chmod 600 ~/deploy/.env"
  ```

- [ ] **Step 6.2: Write docker-compose.yaml**

  ```bash
  ssh -i ~/.ssh/hermes_agent_key hermes@hermes.petersimmons.com "cat > ~/deploy/docker-compose.yaml" << 'COMPOSE'
  services:
    hermes-agent:
      image: hermes-agent:latest
      build:
        context: .
        dockerfile: Dockerfile
      container_name: hermes-agent
      restart: unless-stopped

      # Run as Chainguard nonroot UID, supplemented with Docker GID for socket access
      user: "65532:65532"
      group_add:
        - "${DOCKER_GID}"

      # Volume mounts — all owned by UID 65532 on host
      volumes:
        - /home/hermes/.hermes:/home/hermes/.hermes
        - /home/hermes/workspace:/home/hermes/workspace
        # Docker socket for terminal.backend: docker
        # SECURITY NOTE: socket access = root-equivalent on host Docker daemon.
        # Mitigated by: dedicated VM, UFW blocks inbound, container caps dropped.
        - /var/run/docker.sock:/var/run/docker.sock

      # Secrets from hermes .env file
      env_file:
        - /home/hermes/.hermes/.env

      # Hardened security context
      security_opt:
        - no-new-privileges:true
      cap_drop:
        - ALL
      read_only: false    # hermes writes logs/sandboxes under ~/.hermes/

      # Resource limits (leaves headroom for sandbox containers)
      deploy:
        resources:
          limits:
            cpus: '4.0'
            memory: 6G
          reservations:
            cpus: '0.5'
            memory: 512M

      # Only accessible from host loopback (no inbound ports exposed externally)
      # Dashboard on 127.0.0.1:7788 if enabled
      ports:
        - "127.0.0.1:7788:7788"

      logging:
        driver: journald
        options:
          tag: hermes-agent
  COMPOSE
  ```

---

## Task 7: Configure ~/.hermes/config.yaml and .env

The user must provide the Discord token and LLM API key before this step. Update them in Infisical first.

- [ ] **Step 7.1: Gate — verify secrets are no longer PLACEHOLDER**

  Use `mcp__infisical-personal__get-secret` for:
  - `HERMES_DISCORD_TOKEN` — must not be "PLACEHOLDER"
  - `HERMES_LLM_API_KEY` — must not be "PLACEHOLDER"
  - `HERMES_DISCORD_USER_ID` — must not be "PLACEHOLDER"

  **If any are still PLACEHOLDER — STOP. Ask user to update Infisical before continuing.**

- [ ] **Step 7.2: Create ~/.hermes directory with correct permissions**

  ```bash
  ssh -i ~/.ssh/hermes_agent_key hermes@hermes.petersimmons.com \
    "mkdir -p ~/.hermes/logs ~/.hermes/sandboxes && chmod 700 ~/.hermes"
  ```

- [ ] **Step 7.3: Write config.yaml**

  ```bash
  ssh -i ~/.ssh/hermes_agent_key hermes@hermes.petersimmons.com "cat > ~/.hermes/config.yaml" << 'CONFIG'
  terminal:
    backend: docker
    container_cpu: 2
    container_memory: 4096    # MB per sandbox
    container_disk: 20480     # MB per sandbox
    container_persistent: true

  approvals:
    mode: manual              # Always prompt for dangerous commands

  security:
    allow_private_urls: false # Block access to RFC1918 / cloud metadata
    tirith_enabled: true
    tirith_fail_open: true
    website_blocklist:
      enabled: true
      domains: []             # Add internal homelab domains here if needed

  gateway:
    unauthorized_dm_behavior: ignore   # Silently ignore unknown users
  CONFIG
  ```

- [ ] **Step 7.4: Write .env with secrets (values retrieved from Infisical)**

  Replace the angle-bracket values with actual secrets from Infisical:
  ```bash
  ssh -i ~/.ssh/hermes_agent_key hermes@hermes.petersimmons.com "cat > ~/.hermes/.env" << 'ENVFILE'
  DISCORD_TOKEN=<HERMES_DISCORD_TOKEN from Infisical>
  DISCORD_ALLOWED_USERS=<HERMES_DISCORD_USER_ID from Infisical>
  GATEWAY_ALLOWED_USERS=<HERMES_DISCORD_USER_ID from Infisical>
  ANTHROPIC_API_KEY=<HERMES_LLM_API_KEY from Infisical>
  MESSAGING_CWD=/home/hermes/workspace
  ENVFILE
  ```

- [ ] **Step 7.5: Lock down .env permissions**

  ```bash
  ssh -i ~/.ssh/hermes_agent_key hermes@hermes.petersimmons.com "chmod 600 ~/.hermes/.env"
  ```

- [ ] **Step 7.6: TEST — .env permissions are 600**

  ```bash
  ssh -i ~/.ssh/hermes_agent_key hermes@hermes.petersimmons.com "stat -c '%a %U' ~/.hermes/.env"
  ```
  Expected: `600 hermes`. **If not 600 — stop and fix before starting container.**

---

## Task 8: Create Systemd Service (container wrapper)

Manages the Docker Compose stack as a service with autostart.

- [ ] **Step 8.1: Write systemd service unit (as psimmons)**

  ```bash
  ssh psimmons@hermes.petersimmons.com "sudo tee /etc/systemd/system/hermes-agent.service" << 'UNIT'
  [Unit]
  Description=Hermes Agent Container Stack
  After=network.target docker.service
  Requires=docker.service

  [Service]
  User=hermes
  Group=hermes
  WorkingDirectory=/home/hermes/deploy
  ExecStart=/usr/bin/docker compose up
  ExecStop=/usr/bin/docker compose down
  Restart=on-failure
  RestartSec=15
  StandardOutput=journal
  StandardError=journal
  SyslogIdentifier=hermes-agent
  # Don't kill subprocesses on service stop (let compose handle it)
  KillMode=process

  [Install]
  WantedBy=multi-user.target
  UNIT
  ```

- [ ] **Step 8.2: Enable the service (don't start yet)**

  ```bash
  ssh psimmons@hermes.petersimmons.com \
    "sudo systemctl daemon-reload && sudo systemctl enable hermes-agent.service"
  ```

---

## Task 9: First Start and Manual Smoke Test

The user (Peter) does the gateway configuration wizard by hand via SSH because `hermes model` and `hermes gateway setup` are interactive.

- [ ] **Step 9.1: Start container in foreground to run interactive setup**

  ```bash
  # SSH in as hermes
  ssh -i ~/.ssh/hermes_agent_key hermes@hermes.petersimmons.com

  # Start container interactively
  cd ~/deploy
  docker run -it --rm \
    --user 65532:65532 \
    -v /home/hermes/.hermes:/home/hermes/.hermes \
    -v /home/hermes/workspace:/home/hermes/workspace \
    --env-file /home/hermes/.hermes/.env \
    hermes-agent:latest \
    bash
  ```

- [ ] **Step 9.2: Run LLM model setup inside container**

  ```bash
  hermes model
  # Select provider and confirm API key
  ```

- [ ] **Step 9.3: Run gateway setup for Discord**

  ```bash
  hermes gateway setup
  # Select Discord, confirm bot token
  ```

- [ ] **Step 9.4: Run doctor**

  ```bash
  hermes doctor
  # Review advisories; ack known ones with: hermes doctor --ack <id>
  ```

- [ ] **Step 9.5: Exit container shell, start via systemd**

  ```bash
  exit   # leave container
  exit   # leave SSH session

  # Via psimmons or as hermes with sudo access to systemctl for own unit:
  ssh psimmons@hermes.petersimmons.com "sudo systemctl start hermes-agent.service"
  ```

- [ ] **Step 9.6: TEST — service running**

  ```bash
  ssh psimmons@hermes.petersimmons.com "sudo systemctl status hermes-agent.service"
  ```
  Expected: `Active: active (running)`.

- [ ] **Step 9.7: TEST — container health**

  ```bash
  ssh psimmons@hermes.petersimmons.com "docker ps --filter name=hermes-agent --format 'table {{.Names}}\t{{.Status}}'"
  ```
  Expected: `hermes-agent   Up X minutes (healthy)`.

- [ ] **Step 9.8: TEST — container running as UID 65532**

  ```bash
  ssh psimmons@hermes.petersimmons.com "docker exec hermes-agent id"
  ```
  Expected: `uid=65532 gid=65532`.

- [ ] **Step 9.9: TEST — Discord DM (user sends a message)**

  Send a DM to the Hermes bot on Discord. Expected: response received (or gateway log shows request processed).
  If no response: `journalctl -u hermes-agent.service -f`

---

## Task 10: Firewall Hardening

- [ ] **Step 10.1: TEST — check ports before firewall (baseline)**

  ```bash
  ssh psimmons@hermes.petersimmons.com "sudo ss -tlnp"
  ```
  Expected: `:22` (SSH) only. Hermes dashboard binds to `127.0.0.1:7788` — must NOT appear on `0.0.0.0`.

- [ ] **Step 10.2: Install and configure UFW**

  ```bash
  ssh psimmons@hermes.petersimmons.com \
    "sudo apt-get install -y ufw && \
     sudo ufw default deny incoming && \
     sudo ufw default allow outgoing && \
     sudo ufw allow ssh && \
     sudo ufw --force enable"
  ```

- [ ] **Step 10.3: TEST — UFW status**

  ```bash
  ssh psimmons@hermes.petersimmons.com "sudo ufw status verbose"
  ```
  Expected: `Status: active`, `Default: deny (incoming)`, SSH rule present.

- [ ] **Step 10.4: TEST — SSH still works after UFW**

  ```bash
  ssh -i ~/.ssh/hermes_agent_key hermes@hermes.petersimmons.com "echo FIREWALL_OK"
  ```
  Expected: `FIREWALL_OK`.

---

## Task 11: Final Infisical + Engram Record

- [ ] **Step 11.1: Confirm all Infisical secrets are populated (not PLACEHOLDER)**

  Use `mcp__infisical-personal__list-secrets`:
  ```
  projectId: hermes, environment: prod, secretPath: /
  ```
  All 5 secrets should have real values.

- [ ] **Step 11.2: Store final deployment record in Engram**

  Use `mcp__engram__memory_store`:
  ```json
  {
    "project": "hermes",
    "type": "decision",
    "content": "Hermes Agent v0.14.0 deployed on hermes.petersimmons.com. Container: Chainguard wolfi-base, UID 65532, caps dropped ALL, no-new-privileges. Docker CE host daemon, hermes user (UID 65532) in docker group. Terminal backend: docker (2 CPU, 4GB RAM, 20GB disk per sandbox). Gateway: Discord, explicit DISCORD_ALLOWED_USERS, GATEWAY_ALLOW_ALL_USERS unset (fail-closed). Approval mode: manual. allow_private_urls=false. Systemd: hermes-agent.service (compose wrapper). UFW: deny inbound, SSH only. All creds in Infisical personal/hermes/prod. Logs: journalctl -u hermes-agent.service. Docker dashboard: 127.0.0.1:7788 only.",
    "tags": ["hermes-agent", "deployment", "production", "discord", "chainguard", "docker"]
  }
  ```

---

## Verification Matrix

| # | Check | Command | Expected |
|---|-------|---------|---------|
| 1 | hermes user UID 65532 | `id hermes` | `uid=65532(hermes)` |
| 2 | hermes has NO sudo | `sudo -u hermes sudo -l` | Denied |
| 3 | hermes SSH works | `ssh -i ~/.ssh/hermes_agent_key hermes@...` | Shell |
| 4 | Docker running | `docker run --rm hello-world` (as hermes) | `Hello from Docker!` |
| 5 | Image built | `docker images hermes-agent` | Image present |
| 6 | Container UID | `docker exec hermes-agent id` | `uid=65532` |
| 7 | Container caps | `docker exec hermes-agent cat /proc/1/status \| grep CapEff` | All zeros |
| 8 | .env permissions | `stat -c '%a' ~/.hermes/.env` | `600` |
| 9 | No external ports | `sudo ss -tlnp` | Only `:22` |
| 10 | UFW active | `sudo ufw status` | active, deny incoming |
| 11 | Service running | `systemctl status hermes-agent` | `active (running)` |
| 12 | Container healthy | `docker ps --filter name=hermes-agent` | `(healthy)` |
| 13 | Infisical secrets | `list-secrets hermes/prod` | 5 secrets, no PLACEHOLDERs |
| 14 | Engram record | `memory_query "hermes agent deployment"` | Record found |
| 15 | Discord responds | Send DM to bot | Response received |

---

## What User Does vs What Is Automated

| Step | Who | Notes |
|------|-----|-------|
| Tasks 1–6 | **Automated** | Full remote execution via psimmons/hermes SSH |
| Task 7 (secrets) | **User fills in Infisical first** | `HERMES_DISCORD_TOKEN`, `HERMES_LLM_API_KEY`, `HERMES_DISCORD_USER_ID` |
| Task 7 (write files) | **Automated** | After user confirms secrets are set |
| Tasks 8–10 | **Automated** | systemd, first container start, firewall |
| Task 9 Steps 9.2–9.4 | **User (interactive)** | `hermes model` and `hermes gateway setup` are interactive wizards inside the container |
| Task 11 | **Automated** | Final records |

---

## Operational Reference (post-install)

```bash
# View logs
journalctl -u hermes-agent.service -f

# Restart service
sudo systemctl restart hermes-agent.service

# Rebuild image (after hermes update)
cd /home/hermes/deploy
docker build -t hermes-agent:latest .
sudo systemctl restart hermes-agent.service

# Run hermes commands
docker exec -it hermes-agent hermes doctor
docker exec -it hermes-agent hermes pairing list

# Check container security
docker inspect hermes-agent | jq '.[0].HostConfig.CapDrop'
docker inspect hermes-agent | jq '.[0].HostConfig.SecurityOpt'
```
