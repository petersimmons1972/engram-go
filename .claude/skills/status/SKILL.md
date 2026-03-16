---
name: status
description: Use when user types "/status" or asks for workspace health, project state, or a quick overview of git, issues, tests, and cleanliness.
---

# Status Dashboard

**Purpose:** One-command workspace health check. Runs four checks and outputs a concise dashboard.

## Checks

Run all four in parallel where possible:

### 1. Git State
```bash
pwd
git branch --show-current
git status --short
git log --oneline -3
```

Report: current directory, branch, count of modified/untracked/staged files, last 3 commits.

### 2. Open GitHub Issues
```bash
gh issue list --state open --limit 100 --json number,title,labels,updatedAt
```

Report: total open count, grouped by label if >5 issues. If no repo or gh not authenticated, report "N/A".

### 3. Test Status
Detect and run the most relevant test suite:

| Indicator                  | Command                          |
|---------------------------|----------------------------------|
| `package.json` with test  | `npm test 2>&1 \| tail -20`     |
| `pytest.ini` / `pyproject.toml` / `tests/` | `python -m pytest --tb=short -q 2>&1 \| tail -20` |
| `Makefile` with test target | `make test 2>&1 \| tail -20`  |
| `go.mod`                  | `go test ./... 2>&1 \| tail -20` |
| None found                | Report "No test suite detected"  |

Report: pass/fail count, or "No test suite detected". Do NOT run tests if the suite takes >2 minutes — report "Skipped (long-running)" and note how to run manually.

### 4. Workspace Cleanliness
```bash
# Untracked files outside .claude/ and node_modules/
git ls-files --others --exclude-standard | grep -v -E '^\.(claude|cache|vscode)/' | grep -v node_modules | head -20

# Large uncommitted files (>1MB)
git ls-files --others --exclude-standard -z | xargs -0 -r stat --format='%s %n' 2>/dev/null | awk '$1 > 1048576 {print $2, int($1/1048576)"MB"}'

# Stale branches (merged into main)
git branch --merged main 2>/dev/null | grep -v -E '^\*|main|master' | head -10
```

Report: untracked file count, any large files, stale branches.

## Output Format

Render as a compact dashboard. Example:

```
## Workspace Status

| Check              | Status | Detail                              |
|--------------------|--------|-------------------------------------|
| Git branch         | `main` | 3 modified, 1 untracked, 0 staged  |
| Open issues        | 12     | 5 bug, 4 enhancement, 3 unlabeled  |
| Tests              | PASS   | 47 passed, 0 failed                 |
| Workspace clean    | WARN   | 2 large files, 1 stale branch       |

**Last 3 commits:**
- `a1b2c3d` fix: resolve auth redirect loop
- `d4e5f6g` feat: add status dashboard skill
- `h7i8j9k` docs: update CLAUDE.md pre-flight protocol
```

## Rules

- **No side effects.** This skill is read-only — never modify files, branches, or issues.
- **Fast.** If any check takes >10 seconds, skip it and note why.
- **Concise.** Dashboard fits in one screen. Details only on problems.
- **Honest.** Report "N/A" or "Skipped" rather than guessing.
