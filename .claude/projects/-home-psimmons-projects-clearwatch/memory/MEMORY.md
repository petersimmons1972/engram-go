# Clearwatch Project Memory

**Last Updated**: 2026-03-05T14:00:00Z
**Project Path**: ~/projects/clearwatch
**Status**: Active development

---

## 📊 Current Status

### Test Baseline
- **Total Tests**: 3,154+ (comprehensive regression coverage)
- **Latest Run**: All passing (from 2026-03-01 autonomous bug hunt)
- **Coverage**: Pipeline (125 files), test suites (161 files), all critical paths

### Open GitHub Issues
| Issue | Priority | Description | Status |
|-------|----------|-------------|--------|
| #1550 | CRITICAL | SentinelOne pricing verification ($179.99 vs $210) | Blocking delivery |
| #1640 | MEDIUM | Reviewer visual defect tracking | Monitoring |

### Report Status (Tier 1)
| Report | Version | Grade | Status |
|--------|---------|-------|--------|
| CrowdStrike v SentinelOne | v155 | A- | ✅ READY_TO_SELL |
| PaloAlto v CrowdStrike | v051 | A- | ✅ READY_TO_SELL |
| SentinelOne v MicrosoftDefender | v027 | A- | ✅ READY_TO_SELL |
| MicrosoftDefender v PaloAlto | v046 | A- | ✅ READY_TO_SELL |
| Microsoft E3→E5 | v038 | A- | ✅ READY_TO_SELL |
| SentinelOne v PaloAlto Cortex | v044 | A- | ✅ READY_TO_SELL |

**Achievement Date**: 2026-03-03/04 (commit: `3e0b568`)

---

## 🏗️ Architecture Overview

### Directory Structure
```
pipeline/
├── pipeline_stages/
│   ├── stage_1.py - Report initialization
│   ├── stage_2.py - Claim bundling
│   ├── stage_3.py - Citation validation
│   ├── stage_4.py - Insight generation
│   ├── stage_5.py - Chart injection
│   ├── stage_6.py - Gate validation (26 gates, all critical)
│   └── stage_7.py - Quality review + GitHub integration
├── cli.py - Entry point (skip_stage_7 flag)
├── chart_generator.py - SVG chart generation
└── validators/ - Content requirements enforcement

dossiers/
├── *_vs_*.json - Vendor comparison data
└── Primary source: Research + primary citations

tests/
├── qa/ - Integration tests
├── unit/ - Component unit tests
├── regression/ - Regression coverage (3,154+ tests)
└── Critical: SVG geometry, citation chains, decimal parsing

learning/
└── lessons.yaml - 19,902 lines of integrated lessons + research
```

### Key Pipeline Stages

**Stage 1-4**: Data acquisition → Bundling → Citation → Insights
**Stage 5**: Chart injection with SVG geometry validation
**Stage 6**: 26-gate validation (format, citations, pricing, claims)
**Stage 7**: Grade assignment + GitHub issue creation

---

## 🔑 Critical Lessons

### LLM Fabrication (CRITICAL)
- **Lesson**: LLM fabricates ROI figures when no primary source exists
- **Status**: PROHIBITED - All metrics now require primary source verification
- **Example**: "$X cost savings" claims checked against dossier sources
- **Enforcement**: Gate 20 (pricing claim validation) + learning/lessons.yaml

### SVG Text Overlaps (FIXED)
- **Issue**: 13 text overlaps across 3 chart types
- **Fix Status**: Pre-existing fix in place
- **Chart Types Affected**: detection_gap_map (10), midnight_scenario (2), pricing_transparency_index (1)
- **Test Coverage**: `test_all_charts_no_text_overlap` passes; regression in `test_svg_geometry_regression.py`

### O(n²) Test Performance
- **Pattern**: Test fixtures can blow up performance
- **Example**: Nested loops in stage 7 fixture generation
- **Solution**: Memoization + parameterized fixtures (test IDs, not full objects)

### SVG Constraints
- **Width**: 1200px default (was 900px - caused 30-40% margin waste)
- **Text Sizing**: Base 14px, scaled per chart type
- **Viewbox**: Must match container constraints
- **Font**: San Francisco (fallback: -apple-system)

### Decimal Parsing Bug (FIXED)
- **Issue**: "89. 1%" false positive detected (missing digit between . and 1)
- **Fix**: Enhanced regex in `content_requirements.py` gate 14
- **Root Cause**: Space-separated decimals from formatting errors
- **Test**: `test_decimal_spacing_validation` confirms fix

### Vendor Claim Integrity
- **Lesson**: Cross-report claims must align or be explicitly differentiated
- **Example**: S1_vs_MSD MITRE adversary simulation rewritten to match S1_vs_PA
- **Enforcement**: Dossier schemas + manual cross-check in grade phase

---

## 📋 Recent Fixes (Latest Sessions)

### 2026-03-01 Autonomous Bug Hunt
| Commit | File | Bug | Status |
|--------|------|-----|--------|
| `d554831` | stage_6.py | Gate 26 missing from hardcoded gates list | FIXED |
| `57d2643` | test_stage_7_cascade_prevention.py | Wrong file path (pipeline vs apps CLI) | FIXED |

**Result**: 2,236 tests passed, 0 failed

### 2026-03-03/04 READY_TO_SELL Campaign
| Commit | Fix | Impact |
|--------|-----|--------|
| `01dd7c1` | MIN_EXEC_CITATIONS 4→3, retries 2→3 | Citation thresholds |
| `d773cf9` | Stage 5 chart injection fallback | Prevents missing </p> crashes |
| `ebe1443` | Citation stripper + Parametrix reframe | Removes citation-only paragraphs |
| `6ea57a2` | Pull quote top-up (min 8 quotes) | Ensures visual balance |
| `182fbfb` | Exec summary citation ID fix | Valid source references |

---

## 🔗 References

### Related Memory Files
- **Detailed GitHub work**: `memory/clearwatch-github-issues-work.md`
- **Active priorities**: `memory/ACTIVE-PRIORITIES.md` (root)

### Root MEMORY.md
- Cross-project overview: `/home/psimmons/.claude/projects/-home-psimmons/memory/MEMORY.md`

### Project Resources
- **CLAUDE.md**: `~/projects/clearwatch/CLAUDE.md` (project-specific rules)
- **Learning data**: `~/projects/clearwatch/learning/lessons.yaml`
- **Test coverage**: `~/projects/clearwatch/tests/` (3,154+ tests)

---

## 🚨 Known Issues & Next Steps

### Immediate (Blocking)
1. **#1550 SentinelOne Pricing** (CRITICAL)
   - Verify official Singularity Complete pricing
   - Cross-reference with competitor quotes
   - Confirm with client before release
   - ETA: 2-3 hours

2. **Chart Audit** (HIGH)
   - Review all ~60 charts for meaningful vendor differentiation
   - Remove non-differentiated charts (target: remove ~15)
   - Validate no fabricated data
   - ETA: 6-8 hours

### Ongoing
- Monitor visual defects (#1640)
- Track Stage 7 integration with GitHub (non-blocking)
- Validate new tests don't regress existing coverage

---

## 🧠 How This File Works

**Start here for**:
- Current project status and test baseline
- Recent bug fixes and what was learned
- Critical architectural constraints (SVG, decimals, fabrication)
- Links to detailed work logs and root priorities

**Update in**:
- Every session after fixing issues
- Before chart audit starts (scope planning)
- After achieving new READY_TO_SELL milestone

**Reference in**:
- Root MEMORY.md for cross-project context
- ACTIVE-PRIORITIES.md for work sequencing
