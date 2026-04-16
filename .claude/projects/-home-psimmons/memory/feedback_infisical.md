---
name: Use Infisical for all secrets
description: Never suggest plaintext env var exports for secrets — always point to Infisical
type: feedback
Category: feedback
---

Never document or suggest `DATABASE_URL="..."` or any secret as a shell export, `.env` file, or inline value. Always use Infisical at `https://infisical.petersimmons.com`.

**Why:** CLAUDE.md is explicit: "use Infisical". Plaintext secrets in shell history, CI configs, and docs are exactly what Infisical prevents. The user shouldn't have to say this.

**How to apply:** Whenever showing a usage example that involves a secret (DB URL, API key, token), show the Infisical retrieval pattern instead of a raw value. If unsure of the exact Infisical path, note that the secret should be fetched from Infisical rather than set as a literal.
