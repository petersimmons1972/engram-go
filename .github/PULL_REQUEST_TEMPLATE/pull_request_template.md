## What

<!-- One sentence describing what this PR does. -->

## Why

<!-- What problem does it solve? Link to the issue if one exists (Closes #N). -->

## How

<!-- Key implementation decisions. What changed and why that approach? -->

## Test plan

- [ ] `go test ./... -count=1 -race` passes
- [ ] New code has ≥ 60% statement coverage (CI gate)
- [ ] New MCP tools have: happy path, empty input, and one boundary condition test
- [ ] Integration tests pass (`TEST_DATABASE_URL=... go test ./internal/... -run Integration`)
- [ ] Manually verified with a running stack (`docker compose ps` all healthy)

## Checklist

- [ ] No secrets committed (`.env`, credentials, API keys)
- [ ] `git diff --staged` reviewed before push
- [ ] Documentation updated if behavior changed (docs/, README, connecting.md)
- [ ] AI-generated PRs labeled `ai-generated` (required by CLAUDE.md)
