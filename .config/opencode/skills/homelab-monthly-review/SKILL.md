---
name: homelab-monthly-review
description: Use on first Monday of month or when user requests "analyze trends" or "monthly review" - analyzes failure patterns, fix effectiveness, and suggests skill improvements.
---

# Monthly Review Analysis

## Overview

Monthly analysis skill that transforms raw learning data into actionable insights. Analyzes failure history, fix effectiveness, assistant mistakes, and warning pattern accuracy to generate comprehensive reports with automation recommendations.

**Purpose**: Convert accumulated learning data into strategic improvements - identify patterns, measure effectiveness, and prioritize automation.

## When to Use

**Triggers**:
- First Monday of month (user initiates)
- User says "analyze trends" / "monthly review" / "run monthly analysis"
- User asks "what patterns have emerged?" / "what should we automate?"
- Quarterly planning sessions
- After major incident to assess systemic issues

**When NOT to use**:
- Session startup (not proactive)
- During active incident (use `homelab:incident-response` instead)
- For single incident analysis (use `homelab:log-incident` instead)
- Without accumulated data (need at least 1 month of data)

## Data Sources

**Required files** (read-only analysis):
```
/home/psimmons/.homelab/knowledge/failure-history.yaml     # All incident records
/home/psimmons/.homelab/knowledge/fix-effectiveness.yaml   # Command/procedure stats
/home/psimmons/.homelab/knowledge/assistant-learning.yaml  # Workflow mistakes
/home/psimmons/.homelab/knowledge/warning-patterns.yaml    # Prediction patterns
```

**Output location**:
```
/home/psimmons/.homelab/analytics/monthly-reviews/YYYY-MM-review.md
```

## Analysis Process

### Step 1: Data Collection

**Read all four data files**:
```yaml
# Gather from failure-history.yaml
failures_this_month = failures.filter(timestamp >= month_start)
total_incidents = count(failures_this_month)
total_downtime = sum(failures_this_month.resolution.actual_mttr_seconds)

# Gather from fix-effectiveness.yaml
commands = all_commands
procedures = all_procedures

# Gather from assistant-learning.yaml
mistakes = workflow_mistakes
corrections = user_corrections
patterns = recurring_patterns

# Gather from warning-patterns.yaml
predictions = all_patterns
```

### Step 2: Failure Analysis

**Calculate metrics**:

1. **Top failures by frequency**:
   ```yaml
   group_by: [service, diagnosis.root_cause_category]
   sort_by: count DESC
   limit: 10
   ```

2. **Top failures by downtime**:
   ```yaml
   group_by: [service, diagnosis.root_cause_category]
   sort_by: sum(actual_mttr_seconds) DESC
   limit: 10
   ```

3. **MTTR comparison**:
   ```yaml
   for each failure:
     expected_mttr = lookup(FAILURE-MODES-CATALOG.md, service, issue)
     actual_mttr = resolution.actual_mttr_seconds
     variance = actual_mttr - expected_mttr
     flag_if: variance > expected_mttr * 0.5  # 50% over expected
   ```

4. **Services with most issues**:
   ```yaml
   group_by: service
   sort_by: count DESC
   calculate: total_downtime_per_service
   ```

5. **Recurring patterns**:
   ```yaml
   find: same (service, root_cause_category) combinations appearing 3+ times
   flag_as: automation_candidate
   ```

### Step 3: Fix Effectiveness Analysis

**Analyze commands**:

1. **High performers** (>90% success rate):
   ```yaml
   filter: success_rate >= 0.90
   sort_by: attempts DESC  # Most used reliable commands
   output: command, success_rate, attempts
   action: Document as "go-to" fixes
   ```

2. **Deprecation candidates** (<50% success rate):
   ```yaml
   filter: success_rate < 0.50
   filter: attempts >= 5  # Only if used enough to be meaningful
   output: command, success_rate, attempts, notes
   action: Flag for replacement or removal
   ```

3. **Procedure effectiveness**:
   ```yaml
   for each procedure:
     calculate: success_rate, avg_resolution_time
     compare: to single-command alternatives
     recommend: if single command beats procedure, simplify
   ```

### Step 4: Assistant Learning Analysis

**Analyze workflow mistakes**:

1. **Mistake categories**:
   ```yaml
   group_by: mistake.category
   count: occurrences
   identify: most common category
   ```

2. **Severity distribution**:
   ```yaml
   group_by: mistake.severity
   calculate: percentage of high/medium/low
   flag_if: high_severity > 20%  # Too many critical mistakes
   ```

3. **Recurring patterns**:
   ```yaml
   from: recurring_patterns
   identify: patterns with frequency > 1
   prioritize: by severity and frequency
   ```

4. **Skills needing updates**:
   ```yaml
   from: corrective_action_taken.long_term
   filter: status != "completed"
   group_by: skill
   output: skills with pending improvements
   ```

5. **User correction patterns**:
   ```yaml
   from: user_corrections
   group_by: interpretation.severity
   identify: common themes in user_statement
   extract: generalization rules not yet implemented
   ```

### Step 5: Prediction Accuracy Analysis

**Analyze warning patterns**:

1. **Pattern accuracy**:
   ```yaml
   for each pattern:
     accuracy = evidence.prediction_accuracy
     occurrences = evidence.occurrences
     confidence_justified = (accuracy >= confidence * 0.9)

   flag_if: accuracy < 0.5  # Pattern predicts wrong >50%
   flag_if: occurrences > 5 AND accuracy < confidence
   ```

2. **Patterns that worked**:
   ```yaml
   filter: prediction_accuracy >= 0.80
   output: pattern_name, accuracy, occurrences
   action: Consider adding to session-startup checks
   ```

3. **Patterns needing adjustment**:
   ```yaml
   filter: prediction_accuracy < 0.70
   filter: occurrences >= 3  # Enough data to judge
   output: pattern_name, current_threshold, suggested_adjustment
   ```

4. **New patterns discovered**:
   ```yaml
   from: failure_history_this_month
   find: warning_signs_identified not in warning-patterns.yaml
   output: candidate new patterns
   ```

### Step 6: Automation Candidate Identification

**Criteria for automation priority**:

```yaml
HIGH priority:
  - failure_count >= 3 in 90 days
  - automation_candidate: true in failure-history
  - fix has 100% success rate (can be scripted safely)

MEDIUM priority:
  - failure_count >= 2 in 90 days
  - fix has >80% success rate
  - resolution time < 5 minutes (quick wins)

LOW priority:
  - failure_count >= 1 in 90 days
  - no existing automation
  - documented procedure exists
```

**Calculate time saved**:
```yaml
for each automation_candidate:
  occurrences_per_month = count / months_of_data
  manual_time = avg_resolution_time_seconds
  automated_time = estimated_30_seconds  # Script execution
  time_saved_per_month = occurrences_per_month * (manual_time - automated_time)
```

### Step 7: Generate Report

**Create report file**:
```
/home/psimmons/.homelab/analytics/monthly-reviews/YYYY-MM-review.md
```

**Report structure**:

```markdown
# Monthly Review: [Month] [Year]

Generated: [timestamp]
Data period: [start_date] to [end_date]
Reviewed by: homelab:monthly-review skill

---

## Executive Summary

| Metric                    | Value        | Trend      |
|---------------------------|--------------|------------|
| Total incidents           | X            | +/-N vs last month |
| Total downtime            | Y hours      | +/-N vs last month |
| Average MTTR              | Z minutes    | +/-N vs last month |
| Prediction accuracy       | W%           | +/-N vs last month |
| Assistant mistakes        | N            | +/-N vs last month |
| Automation candidates     | M            | new this month |

**Health Grade**: [A/B/C/D/F] based on:
- A: <2 incidents, MTTR under expected, >90% prediction accuracy
- B: <5 incidents, MTTR within 20% of expected, >80% prediction accuracy
- C: <10 incidents, MTTR within 50% of expected, >70% prediction accuracy
- D: <15 incidents, MTTR over expected, <70% prediction accuracy
- F: >15 incidents or major outages

---

## Top Issues This Month

### By Frequency
| Rank | Service     | Issue Type       | Occurrences | Total Downtime |
|------|-------------|------------------|-------------|----------------|
| 1    | [service]   | [root_cause_cat] | N           | X min          |
| 2    | ...         | ...              | ...         | ...            |

### By Impact (Downtime)
| Rank | Service     | Issue Type       | Total Downtime | Occurrences |
|------|-------------|------------------|----------------|-------------|
| 1    | [service]   | [root_cause_cat] | X min          | N           |
| 2    | ...         | ...              | ...            | ...         |

### MTTR Analysis
| Service     | Expected MTTR | Actual MTTR | Variance  | Notes          |
|-------------|---------------|-------------|-----------|----------------|
| [service]   | X min         | Y min       | +Z min    | [explanation]  |

---

## Fix Effectiveness Analysis

### High Performers (>90% success rate)
These commands reliably resolve issues - document and promote:

| Command                              | Success Rate | Uses | Avg Time |
|--------------------------------------|--------------|------|----------|
| `~/bin/mouse.sh`                     | 100%         | 15   | 30s      |
| `kubectl apply -f networkpolicy.yaml`| 100%         | 5    | 15s      |

### Deprecation Candidates (<50% success rate)
Consider replacing or removing these approaches:

| Command                           | Success Rate | Uses | Why Failing              |
|-----------------------------------|--------------|------|--------------------------|
| `kubectl delete pod homepage-xxx` | 20%          | 15   | Treats symptom not cause |

**Recommendation**: Replace with [alternative command]

### Procedure Review
| Procedure                    | Success | Notes                            |
|------------------------------|---------|----------------------------------|
| Network policy validation    | 100%    | Promote to standard first step   |
| Traefik troubleshooting      | 80%     | Review failure cases             |

---

## Automation Candidates

| Service  | Issue               | Occurrences | Est. Time Saved/mo | Priority |
|----------|---------------------|-------------|---------------------|----------|
| homepage | network-policy-fix  | 3           | 30 min              | HIGH     |
| mouse    | driver-reset        | 15          | 45 min              | HIGH     |
| nextcloud| apache-restart      | 2           | 10 min              | MEDIUM   |

### Recommended Automation Projects
1. **[Project name]**: [Brief description of automation]
   - Frequency: N times/month
   - Current manual time: X minutes
   - Estimated automation effort: Y hours
   - ROI payback: Z months

---

## Warning Pattern Analysis

### High-Accuracy Predictions (keep and enhance)
| Pattern                    | Accuracy | Occurrences | Action                |
|----------------------------|----------|-------------|-----------------------|
| storage-pressure-vm-freeze | 100%     | 3           | Add to session-startup|
| node-memory-pressure       | 100%     | 2           | Already active        |

### Patterns Needing Adjustment
| Pattern              | Current Accuracy | Issue               | Suggested Fix        |
|----------------------|------------------|---------------------|----------------------|
| [pattern-name]       | 60%              | Too many false +ve  | Raise threshold      |

### New Patterns Discovered
From this month's incidents, potential new patterns:
- [Pattern]: [Warning sign] -> [Predicted failure]

---

## Assistant Learning

### Mistakes Captured: N
| Category             | Count | Severity | Status              |
|----------------------|-------|----------|---------------------|
| missing-prerequisite | 1     | high     | Systemic fix pending|

### Skill Evolution Needed
| Skill                      | Change Needed                        | Priority |
|----------------------------|--------------------------------------|----------|
| superpowers:brainstorming  | Add backup step before file changes  | HIGH     |
| superpowers:writing-plans  | Add backup verification to checklist | HIGH     |

### User Corrections Incorporated: M
| Correction Theme     | Times | Generalized Rule                     |
|----------------------|-------|--------------------------------------|
| "Always backup"      | 1     | Backup critical files before changes |

### Recurring Anti-Patterns
| Pattern                                  | Frequency | Systemic Fix Status |
|------------------------------------------|-----------|---------------------|
| assistant-gives-advice-but-doesnt-follow | 1         | In design phase     |

---

## Recommendations

### Immediate Actions (This Week)
1. **[Action]**: [Why and how]
2. **[Action]**: [Why and how]
3. **[Action]**: [Why and how]

### Short-Term Projects (This Month)
1. **[Project]**: [Expected outcome]
2. **[Project]**: [Expected outcome]

### Long-Term Improvements (This Quarter)
1. **[Improvement]**: [Strategic benefit]
2. **[Improvement]**: [Strategic benefit]

---

## Appendix: Raw Data Summary

### Incidents by Service
[List all incidents with ID, service, timestamp, MTTR]

### Commands by Success Rate
[Full sorted list]

### Assistant Learning Timeline
[Chronological list of mistakes and corrections]

---

*Report generated by `homelab:monthly-review` skill*
*Data sources: failure-history.yaml, fix-effectiveness.yaml, assistant-learning.yaml, warning-patterns.yaml*
```

### Step 8: User Summary

**Output to chat** (brief summary, not full report):

```
Monthly Review Complete

Period: [Month] [Year]
Full report: /home/psimmons/.homelab/analytics/monthly-reviews/YYYY-MM-review.md

TOP 3 ISSUES:
1. [Service] - [Issue type] (X occurrences, Y min total)
2. [Service] - [Issue type] (X occurrences, Y min total)
3. [Service] - [Issue type] (X occurrences, Y min total)

TOP 3 AUTOMATION CANDIDATES:
1. [Service/Issue] - Est. save X min/month (Priority: HIGH)
2. [Service/Issue] - Est. save X min/month (Priority: HIGH)
3. [Service/Issue] - Est. save X min/month (Priority: MEDIUM)

SKILLS PENDING EVOLUTION:
1. [skill-name] - [change needed]
2. [skill-name] - [change needed]

HEALTH GRADE: [A/B/C/D/F]

Would you like me to:
- [ ] Detail any specific section?
- [ ] Start automating a candidate?
- [ ] Update a skill that needs evolution?
```

## Edge Cases

### Insufficient Data
If data is insufficient for meaningful analysis:

```
Monthly review requires at least:
- 1 incident in failure-history.yaml
- 1 command tracked in fix-effectiveness.yaml

Current state:
- Incidents: 0
- Commands: 0

Cannot generate meaningful report. Try again after accumulating more data,
or use /homelab:log-incident to capture recent incidents.
```

### First Review (No Prior Month)
If this is the first review:
- Skip trend comparisons ("vs last month")
- Note: "First monthly review - baselines established"
- Focus on absolute numbers not trends

### Missing Files
If any data file is missing:
```
Warning: [filename] not found
- Creating empty structure
- Analysis for [category] skipped

Consider running /homelab:session-startup to initialize knowledge base.
```

## Integration with Other Skills

**Data sources from**:
- `homelab:log-incident` - populates failure-history.yaml
- `homelab:log-fix-result` - populates fix-effectiveness.yaml
- `homelab:capture-learning` - populates assistant-learning.yaml
- `homelab:predict-failure` - uses and updates warning-patterns.yaml

**Triggers after review**:
- High-priority automation candidate -> suggest `homelab:create-runbook` for procedure
- Skill needing evolution -> suggest running that skill's evolution process
- Pattern discovered -> suggest adding to warning-patterns.yaml

**Used by**:
- Quarterly planning sessions
- Infrastructure improvement prioritization
- ROI calculations for automation projects

## Validation Checklist

Before generating report, verify:

- [ ] All four data files exist and are readable
- [ ] At least 1 incident exists for analysis
- [ ] Output directory exists: `/home/psimmons/.homelab/analytics/monthly-reviews/`
- [ ] No existing report for this month (or confirm overwrite)
- [ ] Calculations are mathematically correct
- [ ] All services mentioned exist in infrastructure

## Example Session

```
User: "Run monthly review"

Assistant: I'll run a comprehensive monthly review of your homelab learning data.

[Reads all four data files]
[Calculates metrics]
[Generates report]

Monthly Review Complete

Period: January 2026
Full report: /home/psimmons/.homelab/analytics/monthly-reviews/2026-01-review.md

TOP 3 ISSUES:
1. homepage - configuration (1 occurrence, 10 min total)
2. (no other incidents this month)
3. (no other incidents this month)

TOP 3 AUTOMATION CANDIDATES:
1. mouse/driver-reset - Est. save 45 min/month (Priority: HIGH)
2. homepage/network-policy-fix - Est. save 30 min/month (Priority: HIGH)
3. nextcloud/apache-restart - Est. save 10 min/month (Priority: MEDIUM)

SKILLS PENDING EVOLUTION:
1. superpowers:brainstorming - Add backup step before file changes
2. superpowers:writing-plans - Add backup verification to checklist

HEALTH GRADE: B (low incident count, first month - establishing baselines)

Would you like me to:
- [ ] Detail any specific section?
- [ ] Start automating a candidate?
- [ ] Update a skill that needs evolution?
```
