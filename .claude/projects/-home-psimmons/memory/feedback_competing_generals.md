---
name: Competing Generals Design Pattern
description: Deploying two generals with different perspectives on the same problem produces better plans than a single general
type: feedback
Category: feedback
originSessionId: 98b53916-59ae-42ac-b0a2-e5b0d51c3537
---
Deploy two generals with competing perspectives on architectural decisions. Synthesize the best of both.

**Why:** Yamamoto (strategic) and Grace Hopper (ship-fast) produced fundamentally different designs for the same Engram feature. Yamamoto's semantic chunking was clearly better than Hopper's "keep sentence-window" for document content. Hopper's "just remove the 200-char truncation" was clearly the right Phase 1 move. Neither alone produced the optimal plan.

**How to apply:** For architectural decisions with multiple valid approaches, deploy 2 generals in parallel with the same problem brief. One strategic (Yamamoto, Arnold, Tukhachevsky), one pragmatic (Grace Hopper, Halsey). Synthesize: take the pragmatic Phase 1, the strategic architecture for Phase 2+. Run them in background to avoid blocking.
