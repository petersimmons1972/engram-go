# /home/psimmons AGENTS

Global rules for Codex and Claude Code. Repo-local `AGENTS.md` overrides only as long as it does not conflict with user instructions.

## Project-level handoff
- For work inside `/home/psimmons/projects`, inherit `/home/psimmons/projects/AGENTS.md`.
- Keep this file for workspace-global triggers only (security/recoverability).

## Unsafe command guard
- Ask first before destructive operations: wildcard deletes (`rm -rf`, `find ... -delete`), data-volume resets, destructive DB resets, or mass migration/data wipes.
- Treat Engram changes with extra caution: identify container/db/schema/table and confirm no irreversible state before destructive actions.

## Confirmed secret in git history
- Rotate credential and revoke dependent sessions/tokens.
- Rewrite history with `/home/psimmons/bin/expunge-secrets-from-git.sh` (or approved equivalent).
- Verify: `gitleaks detect --source . --log-opts='--all'`.
- Push with lease (`--force-with-lease`) and notify affected users to reclone.

Keep this file concise and executable; this is a trigger sheet, not a full reference manual.
