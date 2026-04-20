# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest  | ✅        |

engram-go follows a rolling release model — only the latest version receives security fixes. If you are running an older build, update before reporting.

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Report vulnerabilities privately via GitHub's [Security Advisories](../../security/advisories/new) feature (Advisories → Report a vulnerability).

Include:
- Description of the issue and its impact
- Steps to reproduce or proof-of-concept
- Affected component (e.g., SSE transport, authentication, embed client, migrations)
- Any suggested fix, if you have one

### Response Timeline

| Stage | Target |
|-------|--------|
| Acknowledgement | Within 48 hours |
| Initial assessment | Within 7 days |
| Fix or mitigation | Depends on severity; critical issues prioritized |

### What to Expect

- I will confirm receipt and assess severity.
- For confirmed vulnerabilities I will coordinate a fix and release before any public disclosure.
- Credit in the release notes and advisory if desired.

## Scope

In-scope:
- Authentication bypass (bearer token validation)
- SSRF vulnerabilities in the Ollama embed client
- SQL injection or unsafe query construction
- Privilege escalation through the MCP tool interface
- Information disclosure through error messages or API responses

Out of scope:
- Vulnerabilities requiring physical access to the host
- Issues in third-party dependencies that have upstream fixes available
- Denial-of-service through resource exhaustion on a single-user local deployment

## Security Design Notes

engram-go is designed to run locally on trusted infrastructure:

- **Bearer authentication required** — the server refuses to start without `ENGRAM_API_KEY` and rejects all connections without a valid token
- **Loopback binding by default** — the MCP port binds to `127.0.0.1` (not `0.0.0.0`) to limit exposure to the local machine
- **SSRF protection on Ollama client** — the embed client re-resolves hostnames on every dial and rejects private IPs to prevent DNS-rebinding attacks; the operator-configured host is allow-listed
- **Container hardening** — Docker Compose sets `mem_limit`, `pids_limit`, and `no-new-privileges` by default
- **No secrets in environment at runtime** — `DATABASE_URL` is unset after the DB connection is established
