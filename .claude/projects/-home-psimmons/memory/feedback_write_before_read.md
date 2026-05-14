---
name: Read before write — always
description: Never write to a file path without reading it first, even when intent is to create new content
type: feedback
originSessionId: 6c6c81cb-73f5-4f0a-b6dd-dd1527971204
---
Always read a file before writing to it, even when you believe you are creating new content.

**Why:** On 2026-04-29, wrote over `yourai/docs/engram-architecture.html` which already contained the user's SVG architecture diagram. Replaced it with a plain-text ASCII diagram, destroying the user's visual work. Recovery required `git show <prior-commit>:path`.

**How to apply:** Before any `Write` or `cat >` to a path, check if the file exists and read it. If it exists and has content the user created, preserve that content — edit, don't replace. The `Write` tool itself warns "Read it first before writing to it" — follow that literally every time.
