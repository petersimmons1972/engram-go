# Team Management Workflow

## CRITICAL: Required Workflow Before Team Shutdown

**NEVER shut down a team without completing this workflow.**

### Step 1: Document Service Records

Write comprehensive records for each team member:

**Required elements:**
- **Accomplishments** - What succeeded, why it worked, what was learned
- **Failures/Issues** - What failed, root causes, lessons learned, how to prevent
- **Behavioral observations** - Personality traits demonstrated, consistency with profile
- **XP earned** - Calculate based on task completion and quality
- **Competence progress** - N/5 deployments toward next star
- **Campaign ribbons and medals** - Based on user feedback and achievements

**Format:** Structured markdown in team member's profile or service record file

### Step 2: Commit to GitHub

All service records must be committed and pushed:

1. Update commander profiles with deployment experience
2. Stage changes: `git add profiles/*.md` (or relevant files)
3. Commit with descriptive message explaining what was learned
4. Push to remote repository

**For generals project:**
- Repository: `https://github.com/petersimmons1972/generals.git`
- Profiles location: `profiles/*.md`
- Include deployment summary in service record section

### Step 3: Verify Commit

Confirm git push succeeded:

```bash
git log --oneline -1  # Check commit exists
git status           # Should show "up to date with origin"
```

### Step 4: Shutdown Team

Only after steps 1-3 complete:

```bash
rm -rf ~/.claude/teams/{team-name}
rm -rf ~/.claude/tasks/{team-name}
```

Or use TeamDelete tool if available.

---

## Why This Matters

**This is the self-learning mechanism.**

Without service records:
- We lose all operational knowledge
- We repeat the same mistakes
- Commanders don't improve across deployments
- No learning curve or skill progression

Service records are how the system:
- Captures what works and what doesn't
- Tracks commander development over time
- Builds institutional knowledge
- Improves performance across sessions

**Penalty for Skipping:** Regression to zero knowledge = wasted effort

---

## Quick Checklist

Before team shutdown, verify:
- [ ] Service records written for all team members
- [ ] Records include accomplishments + failures + observations
- [ ] XP/competence/ribbons updated
- [ ] Changes committed to git
- [ ] Changes pushed to remote
- [ ] `git status` shows up to date
- [ ] THEN shutdown team directories

---

## Integration with Generals System

See `GENERALS-INTEGRATION.md` for detailed guidance on:
- XP calculation methodology
- Ribbon and medal criteria
- Competence progression system
- Profile update formats
