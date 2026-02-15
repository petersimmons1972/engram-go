---
name: homelab:create-runbook
description: Use when user asks to document a procedure or workflow - ensures proper runbook vs playbook format
---

# Create Documentation Procedure

**Announce:** "Using create-runbook skill to document this procedure properly..."

## Step 1: Determine Document Type

Ask user: "Is this procedure for:"

**A) A specific, known task** (restart a service, restore a backup, reset something)
→ Create a **Runbook** (single-process workflow)

**B) Diagnosing an unknown problem** (service is down, something isn't working)
→ Create a **Playbook** (incident response / diagnostic)

## Runbook Template

**Location:** `/home/psimmons/RUNBOOKS/[procedure-name].md`

```markdown
# Runbook: [Task Name]

**When to use:** [Specific situation that triggers this runbook]

**Prerequisites:**
- [Any required access or tools]

**Procedure:**

1. [Step 1 with exact command]
   ```bash
   exact command here
   ```

2. [Step 2 with exact command]
   ```bash
   exact command here
   ```

3. [Continue with all steps...]

**Verification:**
```bash
command to verify success
```
Expected output: [what success looks like]

**Troubleshooting:**
- If [X happens]: [Do Y]
- If [Z happens]: [Do W]

**Tags:** #[category] #[keywords]
**Type:** Runbook (single-process)
**Last Updated:** [YYYY-MM-DD]
```

## Playbook Template

**Location:** `/home/psimmons/PLAYBOOKS/[issue-name].md`

```markdown
# Playbook: [Issue Name]

**Symptoms:**
- [Observable problem 1]
- [Observable problem 2]

**Quick Checks:**
```bash
# Commands to run immediately
command1
command2
```

**Diagnostic Steps:**

### Step 1: [Check Category]
```bash
diagnostic command
```
- If [result A]: Go to Step 2
- If [result B]: Go to Step 3
- If [result C]: Problem is [X], run Runbook [Y]

### Step 2: [Next Check]
[Continue decision tree...]

**Common Causes:**
| Cause | Solution | Runbook |
|-------|----------|---------|
| [Cause 1] | [Quick fix] | [Link to runbook if exists] |
| [Cause 2] | [Quick fix] | [Link to runbook if exists] |

**Escalation:**
If not resolved after [X] minutes, escalate per incident response procedure.

**Tags:** #[category] #[keywords]
**Type:** Playbook (incident response)
**Last Updated:** [YYYY-MM-DD]
```

## Step 2: Update Index

After creating the document, update RUNBOOKS-INDEX.md:

```bash
# Add entry to appropriate section
echo "- [Procedure Name](RUNBOOKS/procedure-name.md) - Brief description" >> ~/RUNBOOKS-INDEX.md
```

## Step 3: Verify

Confirm with user:
- Document created in correct location
- Format follows template
- Added to RUNBOOKS-INDEX.md
