---
name: armies:award-experience
description: Update commander service record with deployment experience, XP, and lessons learned after a campaign
---

# Award Experience

Update commander service record after deployment with XP, deployment history, behavioral observations, and lessons learned.

**This closes the self-learning cycle.** Each deployment makes commanders more experienced.

⚠️ **CRITICAL**: Updates MUST be committed to the `camp-david` repo (`https://github.com/petersimmons1972/camp-david`). If you don't commit, future sessions will have stale XP/deployment data.

---

## Usage

```
/armies:award-experience <commander-name> <xp> <deployment-summary>
```

**Example**:
```
/armies:award-experience grace-hopper 100 "Implemented engram-go retrieval pipeline — excellent accessibility translation"
```

---

## How It Works

### Step 1: Read Current Profile

Load `~/.armies/profiles/<name>.md` and extract:
- Current XP total
- Deployment count
- Competence progress
- Existing ribbons/medals

### Step 2: Calculate Updates

**XP Awards**:
- Base task: 50-100 XP
- Complex task: 100-150 XP
- Campaign leadership: 150-200 XP
- Exceptional performance: +50 bonus
- Lessons documented: +25 bonus

**Competence Progress**:
- Stars: 10 deployments = ⭐, 25 = ⭐⭐, 50 = ⭐⭐⭐, 100 = ⭐⭐⭐⭐, 250 = ⭐⭐⭐⭐⭐

**Ribbons/Medals**:
- Campaign Participation: Completed deployment
- Excellence: Exceptional quality/innovation
- Strategic Impact: Significant mission contribution

### Step 3: Write Service Record

Create or update `~/.armies/service-records/<commander-name>-<date>.md`:

```markdown
### Deployment N: [Name] (YYYY-MM-DD)

**Mission**: [Task description]
**Role**: [coordinator | implementer | designer | reviewer | observer]
**Deliverable**: [What they produced]
**Outcome**: [SUCCESS/FAILURE with details]
**XP Earned**: [Amount] ([reason])

**Execution Details**:
[How they approached the task]

**Behavioral Observations**:
- [Trait 1]: [How it manifested]

**Lessons Learned**:
[What this taught about commander/task type]
```

### Step 4: Update Profile

Edit `~/.armies/profiles/<commander-name>.md`:
- Increment XP total
- Increment deployment count
- Update competence table
- Add ribbon if earned

### Step 5: Commit to camp-david

```bash
cd ~/.armies
git add profiles/<commander-name>.md service-records/
git commit -m "docs: [Commander] Deployment N - [brief description]

- XP: +[amount] (total: [new total])
- Deployment: [task summary]
- Lessons: [key lesson]

Operation: [Operation-Name]
Team: [Name](role), [Name](role)

Co-Authored-By: [Commander] <commander@armies.ai>
Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"

git push origin main
```

---

## Callability Update (if applicable)

If this deployment warrants making the commander callable as a named subagent, add `status: active` to their profile frontmatter, then run:

```bash
~/.armies/bin/sync-to-claude-agents.sh
```

This syncs the profile to `~/.claude/agents/` so future sessions can call them as `subagent_type="<name>"`.

---

## Output

```
═══════════════════════════════════════════════════════════════
ARMIES: AWARD EXPERIENCE
═══════════════════════════════════════════════════════════════

Commander: [Name]
Deployment: [Task]

EXPERIENCE AWARDED
XP: +[amount]  Total XP: [old] → [new]
Deployments: [N] → [N+1]

PROFILE UPDATED
✅ Service record written to ~/.armies/service-records/
✅ Profile XP updated
✅ Competence progress incremented
✅ Committed and pushed to camp-david

Self-learning cycle complete.
═══════════════════════════════════════════════════════════════
```

---

## Dependencies

**Required**:
- `Read` — Load profile from `~/.armies/profiles/`
- `Edit` / `Write` — Update profile and service record
- `Bash` — Git commit/push to camp-david
