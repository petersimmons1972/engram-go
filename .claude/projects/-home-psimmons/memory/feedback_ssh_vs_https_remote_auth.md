---
name: feedback-ssh-vs-https-remote-auth
description: When a git remote fails with "Repository not found," verify BOTH SSH and HTTPS-via-gh channels before declaring the repo dead; deploy-key vs user-key is the usual mismatch
metadata:
  type: feedback
originSessionId: home-dir-untangle-2026-05-20
---
**Rule:** Before concluding that a private GitHub remote is dead/renamed/inaccessible, test BOTH auth channels:
1. SSH: `ssh -T git@github.com` — the response line shows what account the SSH key authenticates as. If it says `Hi <user>/<repo-name>`, that's a **deploy key** scoped to a single repo, not a user-account key.
2. HTTPS via gh: `gh repo view <owner>/<repo> --json visibility` succeeds when the gh CLI's HTTPS token has `repo` scope. `gh auth status` should show `Git operations protocol: https`.

If SSH is deploy-key-only and gh works: switch the remote URL to HTTPS. `git remote set-url <name> https://github.com/<owner>/<repo>.git`. The gh credential helper will provide auth automatically.

**Why:** During home-dir untangle 2026-05-20, both `git push` and `git fetch` returned "Repository not found" on `petersimmons1972/homelab-config`. First diagnostic concluded "repo deleted." Second-pass `gh repo view` confirmed it existed (private, alive). Root cause: local SSH key is a deploy key for `substack-prompts`, no access to other repos. Fix: HTTPS URL + gh token.

**How to apply:**
- "Repository not found" on git: don't assume deletion until both channels fail
- `gh repo list <owner> --visibility all --limit 200` (not the default filter) to confirm existence — see [[feedback-gh-repo-list-visibility-filter]]
- `ssh -T git@github.com` is the cheapest first diagnostic — it tells you WHICH key/identity is being offered
- Deploy keys are common in CI machines and forgotten quickly; "I authenticated as `user/some-other-repo`" is the signature
