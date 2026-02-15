# Model Selection Policy for Agent Teams

**Authority**: Marshal Zhukov (Team Lead)
**Purpose**: Optimize cost and performance by matching model capability to task complexity
**Effective**: 2026-02-09

---

## Selection Matrix

### Haiku (Fast, Cost-Effective)
**Use for straightforward, procedural tasks:**
- Web fetching and text extraction
- File operations (read, write, copy)
- Validation against checklists/rubrics
- Simple scoring (pass/fail, numeric rubrics)
- Routine quality gates with clear criteria

**Examples:**
- Groves: Corpus collection (web scraping, text extraction)
- Gordon: Validation scoring (100-point rubric, mechanical scoring)
- Ogilvy: Brand checklist validation (clear criteria matching)

**Characteristics:**
- Clear success criteria
- Repeatable process
- Minimal creative judgment
- Straightforward execution

---

### Sonnet (Balanced, Standard)
**Use for standard complexity work:**
- Content creation with guidelines
- Coordination and project management
- Moderate analytical work
- Synthesis of multiple sources
- Standard writing tasks

**Examples:**
- Murrow: Content creation following voice guidelines
- Zhukov: Team coordination and progress tracking
- Standard documentation tasks

**Characteristics:**
- Requires judgment but not deep analysis
- Creative work with clear constraints
- Multi-step reasoning
- Synthesis without intensive analysis

---

### Opus (Maximum Capability)
**Use for complex analytical or creative work:**
- Deep pattern analysis (measuring 100+ samples)
- Complex guideline synthesis (converting patterns to actionable rules)
- Original research requiring insight
- High-stakes creative work requiring authenticity
- Multi-layer systematic analysis

**Examples:**
- Rickover: 4-layer voice pattern extraction (1,500+ lines of analysis)
- Bedell Smith: Guidelines creation with traceability matrices
- Complex analytical tasks requiring systematic rigor

**Characteristics:**
- Deep analytical thinking required
- Original insights needed
- Multi-layer complexity
- High precision requirements
- Foundation work others depend on

---

## Decision Process

**When spawning agents, Team Lead assesses:**

1. **Task Complexity**
   - How many steps/layers?
   - Is creativity/insight required?
   - How much judgment is needed?

2. **Downstream Impact**
   - Do others depend on this output?
   - Is this foundation work or execution?
   - What's the cost of errors?

3. **Model Assignment**
   - Simple + low-impact → Haiku
   - Standard + medium-impact → Sonnet
   - Complex + high-impact → Opus

---

## Cost vs. Quality Tradeoffs

**Haiku**:
- Pros: Fast, cheap, sufficient for procedural work
- Cons: May miss nuance in complex tasks
- Use when: Speed and cost matter, task is straightforward

**Sonnet**:
- Pros: Balanced capability, handles most work well
- Cons: More expensive than Haiku, less capable than Opus
- Use when: Default choice for standard work

**Opus**:
- Pros: Maximum capability, best for complex analysis
- Cons: Expensive, slower
- Use when: Foundation work, high-stakes decisions, complex analysis

---

## Examples from Current Team

### Correct Assignments:
✅ **Rickover (Pattern Extraction) → Opus**
- 4-layer analysis of 45K words
- 150+ sentences measured manually
- Foundation work for entire voice system
- High-stakes: errors propagate to guidelines

✅ **Bedell Smith (Guidelines) → Opus**
- Convert 1,500 lines of patterns to actionable guidelines
- Create traceability matrices
- Synthesis requiring deep understanding
- Foundation work: others execute these guidelines

✅ **Zhukov (Team Lead) → Sonnet**
- Coordinate work, track progress
- Standard project management
- Judgment required but not deep analysis

### Optimization Opportunities:
⚠️ **Groves (Corpus Collection) → Could use Haiku**
- Web fetching and text extraction
- Straightforward: fetch URL, extract text, save file
- Low creativity, high procedural

⚠️ **Gordon (Validation Scoring) → Could use Haiku**
- Score content against 100-point rubric
- Mechanical: check presence of patterns, assign points
- Clear criteria, minimal judgment

⚠️ **Ogilvy (Brand Validation) → Could use Haiku**
- Validate against brand checklist
- Clear criteria: voice consistency, positioning, messaging
- Pass/fail or score-based

### Keep Current Model:
✅ **Murrow (Content Creation) → Sonnet**
- Creative work requiring authentic voice
- Must follow complex guidelines
- Quality matters but not foundation work

---

## Implementation

**For future spawns:**
1. Team Lead assesses task using matrix above
2. Specifies model in Task tool: `model: "haiku"`, `model: "sonnet"`, or `model: "opus"`
3. Documents reasoning in spawn prompt

**For current team:**
- Leave existing agents as-is (mid-work)
- Apply policy to new spawns
- Re-evaluate when tasks change significantly

---

## Policy Review

**Review frequency**: After each major project
**Success metrics**:
- Task completion quality maintained
- Cost reduction vs. all-Sonnet baseline
- No quality degradation from model mismatches

**Adjustment triggers**:
- Haiku agents failing complex tasks → Upgrade to Sonnet
- Opus agents doing straightforward work → Downgrade to Sonnet/Haiku
- Cost exceeds value delivered → Re-evaluate assignments

---

**Policy Owner**: Marshal Zhukov (Team Lead)
**Last Updated**: 2026-02-09
**Next Review**: After Orwell voice operational
