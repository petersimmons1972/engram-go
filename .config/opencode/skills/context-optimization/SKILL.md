---
name: context-optimization
description: Use when hitting context limits, experiencing slow startups, or managing stale context - systematically optimize token usage while increasing information density
---

# Context Optimization

Methodology for optimizing Claude Code context to provide more relevant information with fewer tokens.

## When to Use

**Triggers:**
- Hitting context limits during sessions
- Session startup feels slow (>5 seconds)
- Context contains stale or irrelevant information
- Major changes to workflow or infrastructure
- Context files haven't been reviewed in 3+ months

**Signs you need optimization:**
- CLAUDE.md or MEMORY.md over 3,000 tokens
- Static information that rarely changes
- Duplicate information across files
- Historical data not used in decisions
- Long sections you skip when reading

## Core Principle

**Make context ACTIVE instead of PASSIVE**

- ❌ **Passive:** Static lessons that get stale
- ✅ **Active:** Dynamic data generated at session start

**Goal:** More relevant information, fewer tokens, fresher data

## Optimization Strategy

### 1. Categorize Context Content

**DYNAMIC (Generate at Session Start):**
- Recent git commits (last 7 days)
- Recent session files (last 30 days)
- Current infrastructure state (K8s nodes, service health)
- Active warnings (storage pressure, certificate renewal)
- Recent failures (last 3 incidents)
- Uncommitted changes count

**STATIC (Keep in Context Files):**
- Top known-good fixes (by success rate)
- Key lessons with usage counters
- Known anti-patterns
- Quick reference (IPs, URLs, commands)
- Critical rules (NEVER/ALWAYS)

**REFERENCE (Move to Separate Docs):**
- Detailed API examples (extract to docs/DNS-API-REFERENCE.md)
- Step-by-step procedures (extract to docs/TEAM-MANAGEMENT-WORKFLOW.md)
- Heavy technical details (extract to docs/)
- Historical context (keep summary only)

### 2. Template System with Placeholders

Create templates with placeholders for dynamic content:

**Example MEMORY-TEMPLATE.md:**
```markdown
# Learning Index

**Last Updated**: {{TIMESTAMP}}
**Session**: {{SESSION_ID}}

## 🔥 Recent Activity (Last 7 Days)

{{RECENT_COMMITS}}

**Recent Sessions**:
{{RECENT_SESSIONS}}

**Uncommitted Changes**:
{{UNCOMMITTED_CHANGES}}

## ⚡ Infrastructure Health

**Cluster Status**: {{CLUSTER_HEALTH}}
**Critical Services**: {{SERVICE_STATUS}}
**Active Warnings**: {{ACTIVE_WARNINGS}}

## 🎯 Top Known-Good Fixes (By Success Rate)

| Command | Success | MTTR | Service | Use When |
|---------|---------|------|---------|----------|
| `~/bin/mouse.sh` | 100% | 30s | Hardware | Mouse unresponsive |
[... static content ...]
```

### 3. Session Start Generator Script

Create script that runs before every session:

**Location:** `~/bin/generate-session-context.py` (or .sh)

**What it does:**
1. Extract recent git commits (last 7 days)
2. Find recent session files (last 30 days)
3. Check uncommitted changes
4. Query live infrastructure state
5. Run health checks and summarize
6. Check warning pattern thresholds
7. Extract recent failures from YAML
8. Render template with fresh data

**Execution time:** 2-5 seconds (acceptable startup cost)

### 4. Configure SessionStart Hook

**File:** `~/.claude/settings.json`

```json
{
  "hooks": {
    "SessionStart": [
      {
        "type": "command",
        "command": "~/bin/generate-session-context.py",
        "statusMessage": "Generating session context",
        "timeout": 15
      }
    ]
  }
}
```

**Behavior:**
- Runs automatically before every session
- User sees "Generating session context" spinner
- Fresh context loaded by time they start typing

## Implementation Workflow

### Phase 1: Analysis (1-2 hours)

**Analyze current context:**
```bash
# Token count per file
wc -w ~/CLAUDE.md ~/.claude/projects/-home-psimmons/memory/MEMORY.md

# Identify static vs dynamic content
# Identify reference material vs quick reference
# Find duplicate information
```

**Document findings:**
- Current token usage
- Static content candidates for templates
- Reference material to extract
- Dynamic data sources available

### Phase 2: Extract Reference Docs (1-2 hours)

**Move heavy reference to docs/:**
```bash
# Example: DNS API reference
# Before: 500 words in CLAUDE.md
# After: Link to docs/DNS-API-REFERENCE.md

# Keep in CLAUDE.md (50 words):
"Use APIs - never ask user to do it manually
See docs/DNS-API-REFERENCE.md for full examples"

# Extract to docs/ (500 words):
Full API examples, code snippets, all credentials paths
```

**Common extractions:**
- API documentation → `docs/API-NAME-REFERENCE.md`
- Workflows → `docs/WORKFLOW-NAME.md`
- Integration guides → `docs/INTEGRATION-NAME.md`

### Phase 3: Create Template (30 min)

```bash
# Create template file
cp ~/.claude/projects/-home-psimmons/memory/MEMORY.md \
   ~/.claude/projects/-home-psimmons/memory/MEMORY-TEMPLATE.md

# Replace dynamic sections with placeholders
# Example:
# Old: "- 2026-02-12: feat: optimize CLAUDE.md"
# New: "{{RECENT_COMMITS}}"
```

**Standard placeholders:**
- `{{TIMESTAMP}}` - Current timestamp
- `{{SESSION_ID}}` - Session identifier
- `{{RECENT_COMMITS}}` - Last N git commits
- `{{RECENT_SESSIONS}}` - Last N session files
- `{{UNCOMMITTED_CHANGES}}` - Git status summary
- `{{CLUSTER_HEALTH}}` - K8s node status
- `{{SERVICE_STATUS}}` - Health check summary
- `{{ACTIVE_WARNINGS}}` - Warning patterns triggered
- `{{RECENT_FAILURES}}` - Last N incidents

### Phase 4: Build Generator Script (2-3 hours)

**Python example structure:**
```python
#!/usr/bin/env python3
import subprocess
import datetime
from pathlib import Path

def get_recent_commits(days=7):
    cmd = f"git log --oneline --since='{days} days ago'"
    # ... return formatted output

def get_cluster_health():
    cmd = "kubectl get nodes --no-headers"
    # ... parse and summarize

def render_template():
    template = Path("MEMORY-TEMPLATE.md").read_text()

    replacements = {
        "{{TIMESTAMP}}": datetime.datetime.now().isoformat(),
        "{{RECENT_COMMITS}}": get_recent_commits(),
        "{{CLUSTER_HEALTH}}": get_cluster_health(),
        # ... more placeholders
    }

    for placeholder, value in replacements.items():
        template = template.replace(placeholder, value)

    Path("MEMORY.md").write_text(template)

if __name__ == "__main__":
    render_template()
    print("✅ Session context generated")
```

### Phase 5: Test and Validate (30 min)

**Test generator manually:**
```bash
~/bin/generate-session-context.py
# Should complete in <5 seconds
# Check MEMORY.md has fresh data
```

**Test SessionStart hook:**
```bash
# Start new Claude Code session
# Verify "Generating session context" appears
# Verify fresh data in context
```

**Validate token reduction:**
```bash
# Before
wc -w ~/CLAUDE.md ~/.claude/projects/-home-psimmons/memory/MEMORY.md

# After
wc -w ~/CLAUDE.md ~/.claude/projects/-home-psimmons/memory/MEMORY.md

# Calculate savings
```

## Success Metrics

**Before Optimization:**
- CLAUDE.md: 1,700 tokens (example)
- MEMORY.md: 2,900 tokens (example)
- Total: ~4,600 tokens
- **Freshness:** Static, potentially stale

**After Optimization:**
- CLAUDE.md: 1,700 tokens (optimized, reference links)
- MEMORY.md: ~1,500 tokens (dynamic, fresh)
- Total: ~3,200 tokens
- **Freshness:** Dynamic, regenerated every session

**Impact Metrics:**
- **Token reduction:** 30-50% typical
- **Information density:** 2-4x more relevant data
- **Startup cost:** 2-5 seconds (acceptable)
- **Maintenance:** Reduced (less manual updating)

## Real-World Example

**From your recent optimization:**

**Before:**
- Static lessons that got stale
- Manual updates to recent activity
- 2,900 words of historical context

**After:**
- Fresh git commits from last 7 days
- Actual uncommitted changes count
- Live cluster status
- Recent session files automatically indexed
- Warning patterns checked against live metrics
- Only 1,500 words, but 4x more relevant

**Result:** 53% token reduction + fresher context

## Common Pitfalls

**Don't:**
- Make generator script too slow (>10 seconds)
- Generate content you don't actually use
- Replace all static content (keep core lessons)
- Break generator without fallback

**Do:**
- Handle failures gracefully (skip section if unavailable)
- Cache expensive operations (if needed)
- Keep core context simple
- Test generator regularly

## Troubleshooting

**Generator times out:**
```json
// Increase timeout in settings.json
{
  "hooks": {
    "SessionStart": [{
      "timeout": 30  // Increase from 15
    }]
  }
}
```

**Generator fails:**
```bash
# Test manually
~/bin/generate-session-context.py

# Check error output
~/bin/generate-session-context.py 2>&1 | tee /tmp/generator-debug.log
```

**Wrong data in context:**
```bash
# Check template has correct placeholders
cat MEMORY-TEMPLATE.md | grep "{{.*}}"

# Verify generator replaces all placeholders
~/bin/generate-session-context.py
grep "{{.*}}" MEMORY.md  # Should be empty
```

## Maintenance

**Monthly:**
- Review what context is actually used in sessions
- Update template if workflow changes
- Check generator performance (should stay <5s)
- Verify token counts haven't crept up

**When to re-optimize:**
- Context files exceed 3,000 tokens again
- Session startup exceeds 10 seconds
- New infrastructure added (update generator)

## Related

- **homelab-session-startup** (if exists): May run health checks generator uses
- Session context hooks: `~/.claude/settings.json`
