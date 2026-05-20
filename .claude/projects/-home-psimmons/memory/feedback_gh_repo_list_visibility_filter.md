---
name: feedback-gh-repo-list-visibility-filter
description: gh repo list defaults hide non-source repos; always use --visibility all --limit large when checking whether a specific repo exists
metadata:
  type: feedback
originSessionId: home-dir-untangle-2026-05-20
---
**Rule:** `gh repo list <owner>` defaults to source repos and a smallish limit, hiding private/archived/fork repos. When verifying existence of a specific repo, ALWAYS use:
```
gh repo list <owner> --visibility all --limit 200 | grep -i <name>
```
Or skip the list and go direct: `gh repo view <owner>/<repo>`.

**Why:** During home-dir untangle 2026-05-20, first-pass `gh repo list petersimmons1972 | grep homelab` returned nothing, leading to incorrect "repo was deleted" conclusion. Second-pass `gh repo view petersimmons1972/homelab-config` confirmed the repo exists, private, last-updated yesterday. The default list filter hid it.

**How to apply:**
- Existence check for a known repo name: `gh repo view` is faster than `gh repo list`
- Survey for repos matching a pattern: always include `--visibility all --limit 200` (or larger)
- Don't make destructive decisions ("repo is dead, ignore the remote") based on default-filtered list output
