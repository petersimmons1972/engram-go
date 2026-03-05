---
name: integrate-feedback
description: Processes captured feedback from ~/Pictures/claude/ and integrates lessons into security-intelligence-business report generation. Use when user says "process feedback", "integrate feedback", or starts a new report cycle. Detects empty feedback queue, no pending feedback, and feedback processing complete states. Auto-triggered on cycle start.
aliases: [if]
---

# Integrate Feedback

**Purpose:** Process accumulated feedback (text + screenshots) and extract lessons to improve next report generation cycle.

**Invocation:**
- Explicit: `/if` or "process feedback"
- Auto-trigger: When user starts new report cycle (detected by context)

## Context

Part of security-intelligence-business feedback loop system. User accumulates feedback asynchronously:
- Text feedback via `/cf` → `{project}/output/feedback/feedback.txt` (or `~/.claude/feedback/`)
- Screenshots → same directory

**Feedback directory detection (NEVER use ~/Pictures/):**
```bash
GIT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null)
if [[ -n "$GIT_ROOT" && -d "$GIT_ROOT/output/feedback" ]]; then
  FEEDBACK_BASE="$GIT_ROOT/output/feedback"
else
  FEEDBACK_BASE="$HOME/.claude/feedback"
fi
```

This skill processes ALL pending feedback, extracts lessons, archives the batch, and returns actionable insights for the next report generation cycle.

## Feedback Focus (80/20 Rule)

**80% - Charts & Graphs:**
- Layout issues
- Color choices
- Legend placement
- Data visualization clarity
- Chart type selection

**20% - Writing Quality:**
- Citation format
- Evidence strength
- Section clarity
- Structure issues

## Workflow

```
0. Detect feedback directory
   ├─ GIT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null)
   ├─ If $GIT_ROOT/output/feedback/ exists → use it
   └─ Else → use ~/.claude/feedback/

1. Detect pending feedback
   ├─ Read $FEEDBACK_BASE/feedback.txt
   └─ List $FEEDBACK_BASE/Screenshot_*.png

2. Process feedback
   ├─ Analyze text feedback entries
   ├─ OCR/analyze screenshots (if needed)
   └─ Extract positive and negative lessons

3. Categorize lessons
   ├─ Charts/graphs (layout, colors, clarity)
   ├─ Writing quality (structure, citations)
   ├─ Positive patterns (what's working)
   └─ Negative patterns (what to fix)

4. Archive batch
   ├─ Create $FEEDBACK_BASE/.archive/batch-{timestamp}/
   ├─ Move screenshots to batch/screenshots/
   ├─ Move feedback.txt to batch/
   └─ Clear $FEEDBACK_BASE/ (except .archive/)

5. Retention management
   ├─ List archived batches by date
   ├─ Keep 5 most recent
   └─ Delete older batches

6. Return lesson summary
   └─ Actionable insights for next cycle
```

## Implementation

### 1. Detection

```bash
# Determine feedback directory — NEVER use ~/Pictures/
GIT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null)
if [[ -n "$GIT_ROOT" && -d "$GIT_ROOT/output/feedback" ]]; then
  FEEDBACK_BASE="$GIT_ROOT/output/feedback"
else
  FEEDBACK_BASE="$HOME/.claude/feedback"
fi

TEXT_FEEDBACK="$FEEDBACK_BASE/feedback.txt"
SCREENSHOTS="$FEEDBACK_BASE/Screenshot_*.png"

if [[ -f "$TEXT_FEEDBACK" ]] || ls $SCREENSHOTS 2>/dev/null; then
  # Feedback exists - process it
fi
```

### 2. Processing

```bash
# Read text feedback
cat "$FEEDBACK_BASE/feedback.txt"

# List screenshots
ls -1 "$FEEDBACK_BASE"/Screenshot_*.png
```

**Analysis approach:**
- Group feedback by category: VISUAL, DATA, BUSINESS-LOGIC
- Identify patterns (recurring issues)
- Extract specific examples
- Note positive feedback (what's working)
- **Extract BUSINESS-LOGIC insights for master prompt updates**
- Do NOT require report linking - feedback may or may not reference specific reports

**CRITICAL - Domain Knowledge Extraction:**

When feedback contains `BUSINESS-LOGIC:` prefix:
1. **Extract the insight:** What's fundamentally wrong + why it matters
2. **Extract the story:** The $250K/year narrative (baseball games, 3AM calls, free puppy)
3. **Update master prompt:** Add to `$GIT_ROOT/domain-knowledge/REPORT-GENERATION-PROMPT.md` (clearwatch project)
4. **Version bump:** Increment version number
5. **Document change:** Note what was added in integration summary

Example:
```bash
# Found in feedback.txt:
[2026-02-14 13:53:28] BUSINESS-LOGIC: TCO chart missing labor costs - inverts analysis...

# Extract and update:
1. Insight: TCO must include labor (admin, incident, on-call, training, opportunity cost)
2. Story: "Nobody wants 3AM calls" + baseball games + free puppy analogy
3. Update REPORT-GENERATION-PROMPT.md section: "CRITICAL: TCO Methodology"
4. Version: 2.0 → 2.1
5. Document: "Added labor cost requirements to TCO methodology"
```

### 3. Archiving

```bash
# Create batch archive
BATCH_DIR="$FEEDBACK_BASE/.archive/batch-$(date '+%Y%m%d-%H%M%S')"
mkdir -p "$BATCH_DIR/screenshots"

# Move files
mv "$FEEDBACK_BASE"/Screenshot_*.png "$BATCH_DIR/screenshots/" 2>/dev/null
mv "$FEEDBACK_BASE/feedback.txt" "$BATCH_DIR/" 2>/dev/null

# Create batch metadata
cat > "$BATCH_DIR/metadata.txt" <<EOF
Batch: $(date '+%Y-%m-%d %H:%M:%S')
Screenshots: $(ls -1 "$BATCH_DIR/screenshots/" | wc -l)
Feedback entries: $(wc -l < "$BATCH_DIR/feedback.txt")
EOF
```

### 4. Retention

```bash
# Keep last 5 batches
cd "$FEEDBACK_BASE/.archive"
ls -1td batch-* | tail -n +6 | xargs rm -rf
```

## Output Format

Return actionable lesson summary:

```markdown
# Feedback Integration - [Timestamp]

## Summary
- **Screenshots processed:** 12
- **Text feedback entries:** 8
- **Primary focus:** Charts/graphs (75%)

## Key Lessons for Next Cycle

### Charts & Graphs (PRIORITY)
1. **Legend placement:** Move legends outside chart area (3 instances)
2. **Color contrast:** Increase saturation for series differentiation (2 instances)
3. **Chart types:** Use bar charts for categorical comparisons, not line charts

### Writing Quality
1. **Citations:** Good - maintain current format
2. **Section structure:** Executive summary clarity improved

### Positive Patterns (Keep Doing)
- Evidence strength sections consistently well-received
- Risk assessment methodology clear and actionable

### Domain Knowledge Updates (CRITICAL)

**BUSINESS-LOGIC feedback updates the master prompt:**
- TCO methodology updated: Labor costs now mandatory
- MDR positioning updated: "Nobody wants 3AM calls" now primary narrative
- Updated: `domain-knowledge/REPORT-GENERATION-PROMPT.md` v2.0 → v2.1

## Integration Status
- ✅ Archived to: `batch-20260213-143045`
- ✅ Pending feedback cleared
- ✅ **Master prompt updated with domain knowledge**
- ✅ Ready for next cycle

## Next Steps
When generating next batch of reports via ARMY-ORDERS:
1. **Include updated REPORT-GENERATION-PROMPT.md in general's orders**
2. Chart legend positioning (highest impact)
3. Color palette adjustment
4. TCO will automatically include labor (master prompt enforces)
5. MDR will lead narrative (master prompt updated)
```

## Auto-Trigger Detection

Trigger automatically when user says:
- "process feedback"
- "integrate feedback"
- "start new cycle"
- "generate reports" (if pending feedback exists)
- "new batch"

**Check before auto-triggering:**
```bash
GIT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null)
if [[ -n "$GIT_ROOT" && -d "$GIT_ROOT/output/feedback" ]]; then
  FEEDBACK_BASE="$GIT_ROOT/output/feedback"
else
  FEEDBACK_BASE="$HOME/.claude/feedback"
fi
if [[ -f "$FEEDBACK_BASE/feedback.txt" ]] || ls "$FEEDBACK_BASE"/Screenshot_*.png 2>/dev/null; then
  echo "Pending feedback detected. Processing before starting new cycle..."
  # Run integration
fi
```

## Edge Cases

| Situation | Behavior |
|-----------|----------|
| Empty feedback queue (no pending) | "No pending feedback to integrate. Feedback processing complete." |
| Only screenshots (no text) | Process screenshots, note no text feedback |
| Only text (no screenshots) | Process text, note no screenshots |
| Feedback references specific reports | Accept as-is - don't require structured linking |
| Feedback mentions "sections" | Infer context from content |
| Very large batch (>50 items) | Process normally, note high volume in summary |
| Feedback exhausted (all processed) | Output: "Feedback processing complete. Ready for next cycle." |
| Directory doesn't exist | Create feedback directory structure automatically |

## Retention Policy

- **Keep:** Last 5 archived batches
- **Delete:** Batches 6+ (oldest first)
- **Rationale:** ~5 batches = ~5 report cycles = sufficient history

Example retention:
```
.archive/
├── batch-20260213-143045/  ← Current (just archived)
├── batch-20260213-101500/
├── batch-20260212-162300/
├── batch-20260212-094500/
└── batch-20260211-183000/  ← Oldest kept (batch #5)
# batch-20260211-120000/    ← Deleted (too old)
```

## Directory Structure

**In a project with `output/feedback/` (e.g., clearwatch):**
```
{project}/output/feedback/
├── Screenshot_20260213-143045.png  ← Pending (will be archived)
├── feedback.txt                    ← Pending (will be archived)
└── .archive/
    ├── batch-20260213-143045/
    │   ├── screenshots/
    │   ├── feedback.txt
    │   └── metadata.txt
    └── ...
```

**Outside a project (fallback):**
```
~/.claude/feedback/
├── feedback.txt
└── .archive/
```

**Never uses `~/Pictures/claude/`.**

## Anti-Patterns

**DON'T:**
- Require structured report linking (accept freeform feedback)
- Process feedback multiple times (check if already archived)
- Delete feedback without archiving
- Overwhelm user with verbose analysis
- Wait for user to manually move files

**DO:**
- Accept freeform, unstructured feedback
- Infer context from content
- Auto-archive and clean up
- Focus on actionable insights
- Prioritize charts/graphs (80% of feedback)

## Related Skills

- `/cf` - capture-feedback (captures text feedback)
