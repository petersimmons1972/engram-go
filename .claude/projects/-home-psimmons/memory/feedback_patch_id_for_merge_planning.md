---
name: feedback-patch-id-for-merge-planning
description: Use git patch-id --stable to detect "same content, different SHA" duplicate commits before merging branches that span multiple machines
metadata:
  type: feedback
originSessionId: home-dir-untangle-2026-05-20
---
**Rule:** When planning a merge across branches that may have received the same change applied independently on multiple machines (cherry-pick, manual re-apply), compute patch-ids on both sides:
```
git log --format=%H A..B | xargs -I{} git show {} | git patch-id --stable
git log --format=%H B..A | xargs -I{} git show {} | git patch-id --stable
```
Patch-id is stable per content regardless of commit SHA, parent, author, or timestamp. Matching patch-ids → semantic duplicates.

**Why:** During home-dir untangle 2026-05-20, mid-merge a fetch surfaced 3 new github commits, one of which (`79d073f` "docs: add container-hardening procedure") had the same subject and date as local `e26142a`. Patch-id comparison returned `f374eb4c...` for both — patch-identical, applied on two machines as cherry-picks or independent commits. This made `-X ours` the right merge strategy (auto-resolve conflict to either side; they're the same).

**How to apply:**
- Multi-machine personal repos: run patch-id sweep before any merge that could intersect work from another host
- If duplicates found: `git merge --no-ff -X ours` (or `-X theirs`) auto-resolves the conflict deterministically
- Tolerating one duplicate-SHA log entry is almost always cheaper than rebasing past it, especially if either SHA is externally referenced
- The duplicate pair becomes a useful breadcrumb in history ("here's where the two machines synced")
