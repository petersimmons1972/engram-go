---
name: LongMemEval v8 Benchmark - Full 500-Item Scoring
description: Tracking progress and issues for completing the 500-item memory recall benchmark
type: project
originSessionId: f3ae2e31-3715-47ec-a406-f213ad3ab159
---
# LongMemEval v8 — 500-Item Benchmark Status

**Date**: 2026-05-02 (continued from previous session)
**Goal**: Score all 500 items in the LongMemEval benchmark to understand which question types and retrieval patterns work best with Engram memory system.

## Previous Attempts & Issues

### Attempt 1: c3d9f1-v2-full (with _v2 suffix strategy)
- **Approach**: Added "_v2" suffix to 415 unscored items to bypass Engram's scoring cache
- **Result**: Only 85 items scored (cache limit enforcement)
  - 68 correct (80%)
  - 3 partially correct (3.5%)
  - 14 incorrect (16.5%)
- **Failure Reason**: Remaining 415 "_v2" items failed hypothesis generation
  - Error: `dial tcp: lookup engram-ollama: no such host`
  - Root cause: Infrastructure using hardcoded `engram-ollama:11434` endpoint (doesn't exist)

### Attempt 2: c3d9f1-precision (with correct endpoint)
- **Approach**: Re-ran with correct Ollama endpoint (`precision.petersimmons.com:11434`)
- **Data Issue**: Accidentally used 10.7M-line dataset instead of the 500-item benchmark
- **Result**: Only 85 unique items in final hypotheses (with duplicates)
- **Failure Reason**: Checkpoint file handling issues, run/score stages duplicated entries

## Current Approach: c3d9f1-full-v1

**Start**: 2026-05-02 ~01:40 UTC
**Status**: Running (fresh start, no checkpoint corruption)
**Configuration**:
- Data: `/home/psimmons/projects/engram-go/testdata/longmemeval/longmemeval_m_cleaned.json`
- Max count: 500 (enforce single run)
- Ollama: `http://precision.petersimmons.com:11434`
- Workers: 4
- Output: `/home/psimmons/projects/engram-go/results/longmemeval-full-v1/`

**Process**:
- Using fresh output directory to avoid checkpoint duplication
- Using correct Ollama endpoint from latest infrastructure fixes
- Will capture full 500-item results for analysis

## Key Learnings

1. **Caching Strategy**: The "_v2" suffix approach was logically sound but exposed underlying infrastructure fragility
2. **Endpoint Configuration**: Infrastructure hardcoded `engram-ollama:11434` → must use `precision.petersimmons.com:11434`
3. **Checkpoint Format**: Multi-stage pipeline (ingest → run → score) can corrupt files if stages overlap
4. **Data Integrity**: Must validate that correct 500-item dataset is being processed, not larger subsets

## Next Steps (after c3d9f1-full-v1 completes)

1. Verify all 500 items scored
2. Analyze correctness by question type:
   - Which categories have highest/lowest accuracy
   - Pattern analysis: "I don't know" responses and their causes
3. Understand Engram retrieval effectiveness at scale
4. Document findings for memory system improvement
